// Package itemsearch は記事の横断 / フィード内検索を担うドメインサービスを提供する。
//
// SearchService はキーワード入力の正規化（前後空白 trim、空入力判定、LIKE メタ文字
// エスケープ）、feed_id 指定時の購読確認、リポジトリ呼び出しによる検索実行、
// limit+1 取得→HasMore 判定と NextCursor 生成までを担う。HTTP 境界とレスポンス形式の
// 整形は handler 層 (`internal/handler.ItemSearchHandler`) と service_adapter
// (`ItemSearchServiceAdapter`) の責務とし、本パッケージは API スキーマに依存しない
// 純粋な検索結果（ItemSearchSummary）を返すことに集中する。
package itemsearch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// 検索結果ページングの既定値と上限値。
//
// design.md「API Contract」節に従い、handler 層が `limit` クエリパラメータ未指定時に
// defaultSearchLimit を使用し、上限を超える場合は maxSearchLimit にクランプする。
// 本サービス層でも、handler を経由しない直接呼び出しや handler 側のバグに対する
// 防御として同じクランプを行う。
const (
	defaultSearchLimit = 50
	maxSearchLimit     = 200
)

// SearchService は記事検索のドメインサービス。
//
// 主な責務:
//   - キーワードの前後空白 trim と空入力判定（Req 1.5）
//   - LIKE メタ文字（%, _, \）のエスケープ（Req 2.4）
//   - feed_id 指定時の購読確認と未購読時の 403 への変換（Req 3.5）
//   - cursor 形式 `<RFC3339Nano>|<uuid>` のパースと形式不正時の 400 への変換
//   - リポジトリへの limit+1 取得依頼、HasMore 判定、NextCursor 生成
//
// 認証チェックは行わない（handler 層の責務）。トランザクションは持たない
// （SELECT 専用）。
type SearchService struct {
	itemRepo repository.ItemSearchRepository
	subRepo  repository.SubscriptionRepository
}

// NewSearchService は SearchService の新しいインスタンスを生成する。
//
// itemRepo は検索 SQL を実行するリポジトリ、subRepo は feed_id 指定時の購読確認に
// 利用するリポジトリ。subRepo.FindByUserAndFeed の戻り値が nil であれば未購読と判定する
// （新規メソッド追加を避けるため既存メソッドを再利用する判断）。
func NewSearchService(
	itemRepo repository.ItemSearchRepository,
	subRepo repository.SubscriptionRepository,
) *SearchService {
	return &SearchService{
		itemRepo: itemRepo,
		subRepo:  subRepo,
	}
}

// SearchResult は SearchService.Search の戻り値。
//
// Items は published_at 降順、同 published_at では id 降順で整列済み。
// HasMore は次ページの存在を示し、NextCursor は次ページ取得用のカーソル文字列
// （`<RFC3339Nano>|<uuid>` 形式）。HasMore が false の場合や末尾項目の PublishedAt が
// ゼロ値の場合、NextCursor は空文字となる。
type SearchResult struct {
	Items      []ItemSearchSummary
	NextCursor string
	HasMore    bool
}

// ItemSearchSummary は検索結果 1 件のサービス層表現。
//
// design.md 行 295-309 で定義された ItemSearchSummary に、Adapter 層で data URL を
// 組み立てるために必要な favicon の生バイト（FaviconData / FaviconMime）を追加した
// 形を採用する。data URL への整形は ItemSearchServiceAdapter（Task 5.3）の責務であり、
// 本サービス層が返す FaviconURL は常に nil。
//
// 背景: design.md の field 一覧は `FaviconURL *string` のみだが、impl-notes Task 2 で
// 「favicon の data URL 化は Task 5.3 の Adapter が担う」と既に責務分離が確定している。
// Service 層が favicon を data URL 化すると Adapter 層と二重責務になるため、生データを
// pass-through する設計を採用する。
type ItemSearchSummary struct {
	ID              string
	FeedID          string
	FeedTitle       string
	FaviconURL      *string // 常に nil（data URL 化は Adapter 層で行う）
	FaviconData     []byte  // Adapter 層が data URL に整形するための生バイト
	FaviconMime     string  // Adapter 層が data URL に整形するための MIME タイプ
	Title           string
	Link            string
	Summary         string
	PublishedAt     time.Time
	IsDateEstimated bool
	IsRead          bool
	IsStarred       bool
	HatebuCount     int
}

// Search は当該ユーザーが購読中のフィードに属する記事から、キーワードに部分一致する
// ものを published_at 降順で返す。
//
// feedID が非 nil の場合、検索範囲を当該フィードに限定する（フィード内検索モード）。
// 正規化後の query が空（rawQuery が空または全て空白）の場合は、Req 1.5 の規定により
// リポジトリを呼ばずに空結果を返す。feedID が非 nil でユーザーが当該フィードを購読
// していない場合は、Req 3.5 の規定により NewFeedNotSubscribedError を返す。
// cursorStr が空でなく形式不正の場合は NewInvalidSearchQueryError を返す。
//
// limit は実取得件数。0 以下が渡された場合は defaultSearchLimit (50) に、
// maxSearchLimit (200) を超える場合は maxSearchLimit にクランプする。
// HasMore 判定のためリポジトリには limit+1 件を要求し、limit を超える件数が返ったら
// 末尾 1 件を切り詰めて HasMore=true、NextCursor を組み立てる。
func (s *SearchService) Search(
	ctx context.Context,
	userID, rawQuery string,
	feedID *string,
	cursorStr string,
	limit int,
) (*SearchResult, error) {
	// クエリ正規化: 前後空白 trim + 空クエリ判定 (Req 1.5)
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return &SearchResult{Items: nil, NextCursor: "", HasMore: false}, nil
	}

	// feed_id 指定時の購読確認 (Req 3.5)
	// 既存 SubscriptionRepository.FindByUserAndFeed を再利用し、nil を未購読として
	// 扱う。新規 Exists メソッドの追加を避け、既存インターフェースに侵襲しない判断。
	if feedID != nil {
		sub, err := s.subRepo.FindByUserAndFeed(ctx, userID, *feedID)
		if err != nil {
			return nil, fmt.Errorf("check subscription: %w", err)
		}
		if sub == nil {
			return nil, model.NewFeedNotSubscribedError(*feedID)
		}
	}

	// cursor のパース (形式不正は 400)
	cursorPublishedAt, cursorID, err := parseCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	// limit のクランプ（防御的）
	effectiveLimit := clampLimit(limit)

	// LIKE メタ文字エスケープ + pattern 組み立て (Req 2.4)
	pattern := "%" + escapeLikePattern(query) + "%"

	// limit+1 取得 → HasMore 判定
	hits, err := s.itemRepo.SearchByUserAndKeyword(
		ctx,
		userID,
		pattern,
		feedID,
		cursorID,
		cursorPublishedAt,
		effectiveLimit+1,
	)
	if err != nil {
		return nil, err
	}

	hasMore := len(hits) > effectiveLimit
	if hasMore {
		hits = hits[:effectiveLimit]
	}

	// hits → ItemSearchSummary 変換
	summaries := make([]ItemSearchSummary, len(hits))
	for i, h := range hits {
		summaries[i] = ItemSearchSummary{
			ID:              h.ID,
			FeedID:          h.FeedID,
			FeedTitle:       h.FeedTitle,
			FaviconURL:      nil, // data URL 化は Adapter 層の責務
			FaviconData:     h.FaviconData,
			FaviconMime:     h.FaviconMime,
			Title:           h.Title,
			Link:            h.Link,
			Summary:         h.Summary,
			PublishedAt:     h.PublishedAt,
			IsDateEstimated: h.IsDateEstimated,
			IsRead:          h.IsRead,
			IsStarred:       h.IsStarred,
			HatebuCount:     h.HatebuCount,
		}
	}

	// NextCursor の組み立て:
	// HasMore=true かつ末尾項目の PublishedAt が非ゼロ値の場合のみ生成する。
	// PublishedAt がゼロ値（NULL マッピング）の場合は (published_at, id) の安定順序が
	// 保てないため、cursor を発行せず HasMore のみで「次ページあり」を示す。
	var nextCursor string
	if hasMore && len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		if !last.PublishedAt.IsZero() {
			nextCursor = last.PublishedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
		}
	}

	return &SearchResult{
		Items:      summaries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// parseCursor は cursorStr を (publishedAt, id) のタプルに分解する。
//
// cursorStr が空文字の場合はゼロ値 + 空 ID を返す（リポジトリは zero value を
// 「先頭ページ」として扱う）。空でない場合は `<RFC3339Nano>|<uuid>` 形式を期待し、
// 区切り `|` が無い・複数ある・RFC3339Nano パース失敗・ID 部が空のいずれでも
// NewInvalidSearchQueryError を返す。
//
// UUID 部の厳密パースは行わない（リポジトリ層が SQL の `$5::uuid` cast で検証する）。
// 本サービス層は「空でない」「タイムスタンプがパース可能」程度の sanity check に留め、
// 過剰な依存追加を避ける。
func parseCursor(cursorStr string) (time.Time, string, error) {
	if cursorStr == "" {
		return time.Time{}, "", nil
	}
	parts := strings.SplitN(cursorStr, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", model.NewInvalidSearchQueryError("cursor の形式が不正です")
	}
	if strings.Contains(parts[1], "|") {
		// `a|b|c` のように区切りが複数ある場合は不正
		return time.Time{}, "", model.NewInvalidSearchQueryError("cursor の形式が不正です")
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", model.NewInvalidSearchQueryError("cursor のタイムスタンプが不正です")
	}
	if strings.TrimSpace(parts[1]) == "" {
		return time.Time{}, "", model.NewInvalidSearchQueryError("cursor の id が空です")
	}
	return publishedAt, parts[1], nil
}

// escapeLikePattern は ILIKE パターンの中身として安全に埋め込めるよう、
// LIKE メタ文字（%, _, \）をエスケープする。
//
// PostgreSQL の標準 LIKE/ILIKE は escape 文字として `\` を使用する。したがって
// `\` 自体を先にエスケープしてから `%` と `_` をエスケープする順序を守る必要がある
// （順序を逆にすると `%` のエスケープに使った `\` 自体が再度エスケープされてしまう）。
//
// 例:
//   "50%off" → "50\%off"
//   "a_b"    → "a\_b"
//   "c\\d"   → "c\\\\d"（Go 文字列リテラル上は `c\\d` → `c\\\\d`）
func escapeLikePattern(s string) string {
	// 順序重要: `\` を先にエスケープ → `%` → `_`
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// clampLimit は limit を [defaultSearchLimit, maxSearchLimit] の範囲に矯正する。
//
// limit ≤ 0 → defaultSearchLimit
// limit > maxSearchLimit → maxSearchLimit
// それ以外 → そのまま
//
// handler 層が同じクランプを行う想定だが、handler を経由しない直接呼び出しや
// handler 側のバグに対する防御として本サービス層でも適用する。
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}
