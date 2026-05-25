// Package item は記事の管理機能を提供する。
package item

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/security"
)

// ItemUpsertService は記事の同一性判定とUPSERT処理を提供する。
// 3段階の同一性判定ロジックにより、重複登録を防ぎつつ既存記事の上書き更新を行う。
type ItemUpsertService struct {
	itemRepo  repository.ItemRepository
	sanitizer security.ContentSanitizerService
}

// NewItemUpsertService はItemUpsertServiceの新しいインスタンスを生成する。
func NewItemUpsertService(
	itemRepo repository.ItemRepository,
	sanitizer security.ContentSanitizerService,
) *ItemUpsertService {
	return &ItemUpsertService{
		itemRepo:  itemRepo,
		sanitizer: sanitizer,
	}
}

// preparedItem はサニタイズと content_hash 計算を終えた永続化前の中間表現。
type preparedItem struct {
	parsed           model.ParsedItem
	sanitizedContent string
	sanitizedSummary string
	contentHash      string
}

// UpsertItems はフィードから取得した記事をUPSERTする。
// 3段階の同一性判定ロジック:
//  1. (feed_id, guid_or_id) - 最優先
//  2. (feed_id, link) - 第2優先
//  3. hash(title + published + summary) - 第3優先
//
// 記事件数に比例した DB 往復を避けるため、既存記事の一括取得 → Go 側での同一性判定 →
// 新規一括 INSERT・既存一括 UPDATE を単一トランザクションで実行する。
// 永続化中にエラーが発生した場合はバッチ全件をロールバックし、(0, 0, err) を返す。
//
// 戻り値は挿入数、更新数、エラー。
func (s *ItemUpsertService) UpsertItems(
	ctx context.Context,
	feedID string,
	items []model.ParsedItem,
) (inserted int, updated int, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	now := time.Now()

	// サニタイズと content_hash 計算を全件先行実行する（アルゴリズムは現状不変）。
	prepared := s.prepareItems(items)

	// バッチ内重複は最終要素を優先（後勝ち）して dedup する。
	deduped := dedupByIdentity(prepared)

	// 同一性判定に必要な候補キー群を収集し、既存記事を一括取得する（DB 往復は定数オーダー）。
	guids, links, hashes := collectIdentityKeys(deduped)
	existing, findErr := s.itemRepo.FindExistingForUpsert(ctx, feedID, guids, links, hashes)
	if findErr != nil {
		slog.Error("既存記事の一括取得でエラー",
			"feed_id", feedID,
			"error", findErr,
		)
		return 0, 0, fmt.Errorf("既存記事の一括取得に失敗: %w", findErr)
	}

	// Go 側で 3 段階優先順位判定を行い、新規/更新を仕分けする。
	var toCreate []*model.Item
	var toUpdate []*model.Item
	for _, p := range deduped {
		match := matchExisting(existing, p)
		if match != nil {
			toUpdate = append(toUpdate, buildUpdatedItem(match, p, now))
		} else {
			toCreate = append(toCreate, buildNewItem(feedID, p, now))
		}
	}

	// 新規一括 INSERT と既存一括 UPDATE を単一トランザクションで永続化する。
	if upErr := s.itemRepo.BulkUpsert(ctx, toCreate, toUpdate); upErr != nil {
		slog.Error("記事のバルク UPSERT でエラー",
			"feed_id", feedID,
			"to_create", len(toCreate),
			"to_update", len(toUpdate),
			"error", upErr,
		)
		return 0, 0, fmt.Errorf("記事のバルク UPSERT に失敗: %w", upErr)
	}

	inserted = len(toCreate)
	updated = len(toUpdate)

	slog.Info("記事UPSERT完了",
		"feed_id", feedID,
		"inserted", inserted,
		"updated", updated,
	)

	return inserted, updated, nil
}

// prepareItems は各記事のコンテンツ・サマリーをサニタイズし content_hash を計算する。
func (s *ItemUpsertService) prepareItems(items []model.ParsedItem) []preparedItem {
	prepared := make([]preparedItem, 0, len(items))
	for _, parsed := range items {
		sanitizedContent := s.sanitizer.Sanitize(parsed.Content)
		sanitizedSummary := s.sanitizer.Sanitize(parsed.Summary)
		// content_hashはサニタイズ後のサマリーを使用する（現状アルゴリズム不変）。
		contentHash := computeContentHash(parsed.Title, parsed.PublishedAt, sanitizedSummary)
		prepared = append(prepared, preparedItem{
			parsed:           parsed,
			sanitizedContent: sanitizedContent,
			sanitizedSummary: sanitizedSummary,
			contentHash:      contentHash,
		})
	}
	return prepared
}

// identityKey は 3 段階の優先順位に沿って同一性判定の代表キーを返す。
// guid_or_id > link > content_hash の順で最初に非空のキーを採用する。
// いずれも空の場合は空キー（kind="" / value=""）を返し、dedup 対象外として扱う。
func identityKey(p preparedItem) (kind, value string) {
	if p.parsed.GuidOrID != "" {
		return "guid", p.parsed.GuidOrID
	}
	if p.parsed.Link != "" {
		return "link", p.parsed.Link
	}
	if p.contentHash != "" {
		return "hash", p.contentHash
	}
	return "", ""
}

// dedupByIdentity はバッチ内で同一性判定上同一とみなされる記事を最終要素優先（後勝ち）で
// 1 件に集約する。代表キーが空の記事は dedup せずそのまま保持する。
// 元の出現順は最終出現位置を基準に保たれる。
func dedupByIdentity(items []preparedItem) []preparedItem {
	// 各キーの最終出現 index を記録する。
	lastIndex := make(map[string]int)
	for i, p := range items {
		kind, value := identityKey(p)
		if kind == "" {
			continue
		}
		lastIndex[kind+"|"+value] = i
	}

	result := make([]preparedItem, 0, len(items))
	for i, p := range items {
		kind, value := identityKey(p)
		if kind == "" {
			// 代表キーを持たない記事は重複判定の対象外。
			result = append(result, p)
			continue
		}
		// 最終出現位置のみを採用する（後勝ち）。
		if lastIndex[kind+"|"+value] == i {
			result = append(result, p)
		}
	}
	return result
}

// collectIdentityKeys は既存記事の一括取得に必要な guid_or_id / link / content_hash 群を収集する。
func collectIdentityKeys(items []preparedItem) (guids, links, hashes []string) {
	for _, p := range items {
		if p.parsed.GuidOrID != "" {
			guids = append(guids, p.parsed.GuidOrID)
		}
		if p.parsed.Link != "" {
			links = append(links, p.parsed.Link)
		}
		if p.contentHash != "" {
			hashes = append(hashes, p.contentHash)
		}
	}
	return guids, links, hashes
}

// matchExisting は 3 段階の優先順位で既存記事を引き当てる。
// 優先順位: (feed_id, guid_or_id) > (feed_id, link) > content_hash。
// 一致しない場合は nil を返す（新規記事とみなす）。
func matchExisting(existing *repository.ExistingItems, p preparedItem) *model.Item {
	if p.parsed.GuidOrID != "" {
		if item, ok := existing.ByGUID[p.parsed.GuidOrID]; ok {
			return item
		}
	}
	if p.parsed.Link != "" {
		if item, ok := existing.ByLink[p.parsed.Link]; ok {
			return item
		}
	}
	if p.contentHash != "" {
		if item, ok := existing.ByContentHash[p.contentHash]; ok {
			return item
		}
	}
	return nil
}

// buildUpdatedItem は既存記事に新しい内容を反映した更新後の記事を構築する。
// 既存の id を保持し、新規採番は行わない。履歴は保持しない。
func buildUpdatedItem(existing *model.Item, p preparedItem, now time.Time) *model.Item {
	updated := *existing
	updated.GuidOrID = p.parsed.GuidOrID
	updated.Title = p.parsed.Title
	updated.Link = p.parsed.Link
	updated.Content = p.sanitizedContent
	updated.Summary = p.sanitizedSummary
	updated.Author = p.parsed.Author
	updated.ContentHash = p.contentHash
	updated.UpdatedAt = now

	// published_atの設定。parsed.PublishedAtがnilの場合は既存の値を維持する。
	if p.parsed.PublishedAt != nil {
		updated.PublishedAt = p.parsed.PublishedAt
		updated.IsDateEstimated = false
	}

	return &updated
}

// buildNewItem は新規記事を構築する。
// published_at未設定の場合はfetched_atを代用し、推定フラグを付与する。
func buildNewItem(feedID string, p preparedItem, now time.Time) *model.Item {
	item := &model.Item{
		ID:          uuid.New().String(),
		FeedID:      feedID,
		GuidOrID:    p.parsed.GuidOrID,
		Title:       p.parsed.Title,
		Link:        p.parsed.Link,
		Content:     p.sanitizedContent,
		Summary:     p.sanitizedSummary,
		Author:      p.parsed.Author,
		ContentHash: p.contentHash,
		FetchedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// published_atの設定: 未設定の場合はfetched_atを代用し推定フラグを付与する。
	if p.parsed.PublishedAt != nil {
		item.PublishedAt = p.parsed.PublishedAt
		item.IsDateEstimated = false
	} else {
		item.PublishedAt = &now
		item.IsDateEstimated = true
	}

	return item
}

// computeContentHash はtitle + published + summaryのSHA-256ハッシュを計算する。
// 同一性判定の第3優先手段として使用される。
func computeContentHash(title string, publishedAt *time.Time, summary string) string {
	pubStr := ""
	if publishedAt != nil {
		pubStr = publishedAt.UTC().Format(time.RFC3339)
	}
	data := fmt.Sprintf("%s|%s|%s", title, pubStr, summary)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
