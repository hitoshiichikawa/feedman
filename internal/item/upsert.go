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

// UpsertItems はフィードから取得した記事をUPSERTする。
// 3段階の同一性判定ロジック:
//  1. (feed_id, guid_or_id) - 最優先
//  2. (feed_id, link) - 第2優先
//  3. hash(title + published + summary) - 第3優先
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

	for _, parsed := range items {
		// コンテンツとサマリーにサニタイズ処理を適用
		sanitizedContent := s.sanitizer.Sanitize(parsed.Content)
		sanitizedSummary := s.sanitizer.Sanitize(parsed.Summary)

		// content_hashを計算（サニタイズ後のサマリーを使用）
		contentHash := computeContentHash(parsed.Title, parsed.PublishedAt, sanitizedSummary)

		// 3段階の同一性判定で既存記事を検索
		existing, findErr := s.findExistingItem(ctx, feedID, parsed, contentHash)
		if findErr != nil {
			slog.Error("記事の同一性判定でエラー",
				"feed_id", feedID,
				"guid_or_id", parsed.GuidOrID,
				"error", findErr,
			)
			return inserted, updated, fmt.Errorf("記事の同一性判定に失敗: %w", findErr)
		}

		if existing != nil {
			// 既存記事を上書き更新
			updateErr := s.updateExistingItem(ctx, existing, parsed, sanitizedContent, sanitizedSummary, contentHash, now)
			if updateErr != nil {
				slog.Error("記事の更新でエラー",
					"feed_id", feedID,
					"item_id", existing.ID,
					"error", updateErr,
				)
				return inserted, updated, fmt.Errorf("記事の更新に失敗: %w", updateErr)
			}
			updated++
		} else {
			// 新規記事を挿入
			createErr := s.createNewItem(ctx, feedID, parsed, sanitizedContent, sanitizedSummary, contentHash, now)
			if createErr != nil {
				slog.Error("記事の挿入でエラー",
					"feed_id", feedID,
					"guid_or_id", parsed.GuidOrID,
					"error", createErr,
				)
				return inserted, updated, fmt.Errorf("記事の挿入に失敗: %w", createErr)
			}
			inserted++
		}
	}

	slog.Info("記事UPSERT完了",
		"feed_id", feedID,
		"inserted", inserted,
		"updated", updated,
	)

	return inserted, updated, nil
}

// findExistingItem は3段階の同一性判定で既存記事を検索する。
// 優先順位: (feed_id, guid_or_id) > (feed_id, link) > hash(title+published+summary)
func (s *ItemUpsertService) findExistingItem(
	ctx context.Context,
	feedID string,
	parsed model.ParsedItem,
	contentHash string,
) (*model.Item, error) {
	// 第1優先: feed_id + guid_or_id
	if parsed.GuidOrID != "" {
		item, err := s.itemRepo.FindByFeedAndGUID(ctx, feedID, parsed.GuidOrID)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}

	// 第2優先: feed_id + link
	if parsed.Link != "" {
		item, err := s.itemRepo.FindByFeedAndLink(ctx, feedID, parsed.Link)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}

	// 第3優先: content_hash
	if contentHash != "" {
		item, err := s.itemRepo.FindByContentHash(ctx, feedID, contentHash)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}

	return nil, nil
}

// updateExistingItem は既存記事を上書き更新する。履歴は保持しない。
func (s *ItemUpsertService) updateExistingItem(
	ctx context.Context,
	existing *model.Item,
	parsed model.ParsedItem,
	sanitizedContent, sanitizedSummary, contentHash string,
	now time.Time,
) error {
	existing.GuidOrID = parsed.GuidOrID
	existing.Title = parsed.Title
	existing.Link = parsed.Link
	existing.Content = sanitizedContent
	existing.Summary = sanitizedSummary
	existing.Author = parsed.Author
	existing.ContentHash = contentHash
	existing.UpdatedAt = now

	// published_atの設定
	if parsed.PublishedAt != nil {
		existing.PublishedAt = parsed.PublishedAt
		existing.IsDateEstimated = false
	}
	// 既存記事の更新時、parsed.PublishedAtがnilの場合は既存の値を維持

	return s.itemRepo.Update(ctx, existing)
}

// createNewItem は新規記事を作成する。
// published_at未設定の場合はfetched_atを代用し、推定フラグを付与する。
func (s *ItemUpsertService) createNewItem(
	ctx context.Context,
	feedID string,
	parsed model.ParsedItem,
	sanitizedContent, sanitizedSummary, contentHash string,
	now time.Time,
) error {
	item := &model.Item{
		ID:          uuid.New().String(),
		FeedID:      feedID,
		GuidOrID:    parsed.GuidOrID,
		Title:       parsed.Title,
		Link:        parsed.Link,
		Content:     sanitizedContent,
		Summary:     sanitizedSummary,
		Author:      parsed.Author,
		ContentHash: contentHash,
		FetchedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// published_atの設定: 未設定の場合はfetched_atを代用し推定フラグを付与
	if parsed.PublishedAt != nil {
		item.PublishedAt = parsed.PublishedAt
		item.IsDateEstimated = false
	} else {
		item.PublishedAt = &now
		item.IsDateEstimated = true
	}

	return s.itemRepo.Create(ctx, item)
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
