// Package crossfeed はフィード横断新着一覧のドメインロジックを提供する。
//
// 「最後に横断一覧を開いた時刻」の取得・更新と、当該時刻以降に公開された
// 記事の横断集約を担う。Issue #121 / Req 2.1, 2.2, 2.3, 4.1〜4.5, 4.7。
package crossfeed

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/hitoshi/feedman/internal/item"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// defaultFallbackWindow は初回横断一覧表示時に採用する基準時刻の遡及窓（Req 4.4）。
// 「lastSeen 記録なし + overrideSince 未指定」のユーザーに対しては
// `now - defaultFallbackWindow` を新着判定基準時刻として採用する。
const defaultFallbackWindow = 24 * time.Hour

// Service は横断新着 timeline のサービス層。
type Service struct {
	itemRepo              repository.ItemRepository
	userCrossFeedViewRepo repository.UserCrossFeedViewRepository
	// nowFn はテスト容易性のため time.Now を差し替え可能にする内部 hook。
	// 通常運用では nil（time.Now が使われる）。
	nowFn func() time.Time
}

// NewService は Service の新しいインスタンスを生成する。
func NewService(
	itemRepo repository.ItemRepository,
	userCrossFeedViewRepo repository.UserCrossFeedViewRepository,
) *Service {
	return &Service{
		itemRepo:              itemRepo,
		userCrossFeedViewRepo: userCrossFeedViewRepo,
	}
}

// now は現在時刻を返す。nowFn が設定されている場合はそちらを優先する（テスト用 hook）。
func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

// NewItemsResult は ListNewItems の戻り値。
type NewItemsResult struct {
	// Items は published_at DESC, id DESC で並んだ新着記事のサマリ集合。
	Items []CrossFeedItemSummary
	// NextCursor は次ページ取得用カーソル。空文字列の場合は更なるページなし。
	// 形式は <published_at(RFC3339Nano)>:<item_id(UUID)>。
	NextCursor string
	// HasMore は次ページの有無。
	HasMore bool
	// SinceTime は実際に新着判定に採用した基準時刻。
	// クライアントが session-level baseline として保持し、以降のリクエストで
	// `since` query parameter に送り戻すことで baseline 固定を成立させる（Req 4.7）。
	SinceTime time.Time
}

// CrossFeedItemSummary は横断一覧で返す記事サマリ。
// 既存 item.ItemSummary（FeedID を含む）に発信元フィードのタイトルと favicon を併記する。
type CrossFeedItemSummary struct {
	item.ItemSummary
	// FeedTitle は当該記事が所属するフィードのタイトル（feeds.title）。
	FeedTitle string
	// FeedFaviconURL は favicon を data URL 形式（`data:<mime>;base64,<encoded>`）にしたもの。
	// favicon 未設定（FaviconData が空または FaviconMime が空文字列）の場合は nil。
	// 形式は subscription.SubscriptionInfo.FaviconURL と整合させる。
	FeedFaviconURL *string
}

// ListNewItems はユーザーの全購読フィードから sinceTime 以降の新着を published_at 降順で取得する。
//
// sinceTime の決定順序:
//
//	(1) overrideSince が非 nil なら *overrideSince を採用（クライアント主導の session-level
//	    baseline。userCrossFeedViewRepo は参照しない / Req 4.7）。
//	(2) overrideSince が nil の場合: userCrossFeedViewRepo.Get で lastSeen を取得。
//	    lastSeen 非 nil なら lastSeen.LastSeenAt を採用。
//	(3) lastSeen も nil（初回ユーザー）なら now - 24h を fallback として採用（Req 4.4）。
//
// cursorStr は `<published_at(RFC3339Nano)>:<item_id(UUID)>` 形式の複合カーソル。
// 末尾の `:` から分割するため（RFC3339Nano にも `:` が含まれる）、内部では
// strings.LastIndex(s, ":") で複合分解する。空文字列は先頭ページ取得を意味する。
// 不正形式は model.NewInvalidFilterError を返す。
//
// 戻り値の Items は published_at DESC, id DESC の決定論順序で並ぶ。
func (s *Service) ListNewItems(
	ctx context.Context,
	userID string,
	cursorStr string,
	limit int,
	overrideSince *time.Time,
) (*NewItemsResult, error) {
	// (1) sinceTime の決定
	sinceTime, err := s.resolveSinceTime(ctx, userID, overrideSince)
	if err != nil {
		return nil, err
	}

	// (2) cursor の分解
	cursorPublishedAt, cursorItemID, err := parseCrossFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	// (3) limit+1 件取得で HasMore 判定
	fetchLimit := limit + 1
	rows, err := s.itemRepo.ListNewAcrossFeeds(ctx, userID, sinceTime, cursorPublishedAt, cursorItemID, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("横断新着記事の取得に失敗しました: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	// (4) row → CrossFeedItemSummary 変換
	summaries := make([]CrossFeedItemSummary, len(rows))
	for i, row := range rows {
		summaries[i] = toCrossFeedItemSummary(row)
	}

	// (5) NextCursor 組み立て: <published_at(RFC3339Nano)>:<item_id>
	var nextCursor string
	if hasMore && len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		nextCursor = formatCrossFeedCursor(last.PublishedAt, last.ID)
	}

	return &NewItemsResult{
		Items:      summaries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		SinceTime:  sinceTime,
	}, nil
}

// TouchLastSeen は「最後に横断一覧を開いた時刻」を now() で UPSERT する。
// 単独で呼び出され、ListNewItems からは呼ばない（リトライ・冪等性のため分離 / Req 4.3）。
func (s *Service) TouchLastSeen(ctx context.Context, userID string) error {
	if err := s.userCrossFeedViewRepo.Upsert(ctx, userID, s.now()); err != nil {
		return fmt.Errorf("最終閲覧時刻の更新に失敗しました: %w", err)
	}
	return nil
}

// resolveSinceTime は新着判定基準時刻を 3 段優先順位で決定する。
//   - overrideSince 非 nil → *overrideSince（client baseline / Req 4.7）
//   - overrideSince nil + lastSeen 記録あり → lastSeen.LastSeenAt
//   - overrideSince nil + lastSeen 記録なし → now - 24h fallback（Req 4.4）
func (s *Service) resolveSinceTime(ctx context.Context, userID string, overrideSince *time.Time) (time.Time, error) {
	if overrideSince != nil {
		return *overrideSince, nil
	}
	lastSeen, err := s.userCrossFeedViewRepo.Get(ctx, userID)
	if err != nil {
		return time.Time{}, fmt.Errorf("最終閲覧時刻の取得に失敗しました: %w", err)
	}
	if lastSeen != nil {
		return lastSeen.LastSeenAt, nil
	}
	return s.now().Add(-defaultFallbackWindow), nil
}

// parseCrossFeedCursor は `<RFC3339Nano>:<itemID>` 形式の複合カーソルを分解する。
// 空文字列の場合は (ゼロ値, "", nil) を返し、呼び出し側で「先頭ページ取得」を意味する。
// 不正形式は model.NewInvalidFilterError を返す（既存エラーコード INVALID_FILTER の再利用）。
func parseCrossFeedCursor(cursorStr string) (time.Time, string, error) {
	if cursorStr == "" {
		return time.Time{}, "", nil
	}
	// RFC3339Nano は ":" を含むため、末尾の ":" で分割する
	idx := strings.LastIndex(cursorStr, ":")
	if idx <= 0 || idx == len(cursorStr)-1 {
		return time.Time{}, "", model.NewInvalidFilterError("invalid cursor: " + cursorStr)
	}
	publishedAtStr := cursorStr[:idx]
	itemID := cursorStr[idx+1:]

	publishedAt, err := time.Parse(time.RFC3339Nano, publishedAtStr)
	if err != nil {
		// RFC3339 でも parse を試みる（fallback、横断 API のカーソル規約を緩める）
		publishedAt, err = time.Parse(time.RFC3339, publishedAtStr)
		if err != nil {
			return time.Time{}, "", model.NewInvalidFilterError("invalid cursor: " + cursorStr)
		}
	}
	return publishedAt, itemID, nil
}

// formatCrossFeedCursor は published_at と item_id から `<RFC3339Nano>:<itemID>` 形式の
// 複合カーソルを組み立てる。
func formatCrossFeedCursor(publishedAt time.Time, itemID string) string {
	return publishedAt.Format(time.RFC3339Nano) + ":" + itemID
}

// toCrossFeedItemSummary は repository.CrossFeedItem を CrossFeedItemSummary に変換する。
// favicon の data URL 構築は subscription.Service.ListSubscriptions と同方式で行う
// （`data:<mime>;base64,<base64-encoded>`、未設定時は nil）。
func toCrossFeedItemSummary(row repository.CrossFeedItem) CrossFeedItemSummary {
	pubAt := time.Time{}
	if row.PublishedAt != nil {
		pubAt = *row.PublishedAt
	}
	summary := CrossFeedItemSummary{
		ItemSummary: item.ItemSummary{
			ID:              row.ID,
			FeedID:          row.FeedID,
			Title:           row.Title,
			Link:            row.Link,
			Summary:         row.Summary,
			PublishedAt:     pubAt,
			IsDateEstimated: row.IsDateEstimated,
			IsRead:          row.IsRead,
			IsStarred:       row.IsStarred,
			HatebuCount:     row.HatebuCount,
		},
		FeedTitle: row.FeedTitle,
	}
	if len(row.FaviconData) > 0 && row.FaviconMime != "" {
		dataURL := fmt.Sprintf("data:%s;base64,%s", row.FaviconMime, base64.StdEncoding.EncodeToString(row.FaviconData))
		summary.FeedFaviconURL = &dataURL
	}
	return summary
}
