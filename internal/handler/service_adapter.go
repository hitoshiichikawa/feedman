package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/crossfeed"
	"github.com/hitoshi/feedman/internal/item"
	"github.com/hitoshi/feedman/internal/itemsearch"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/subscription"
	"github.com/hitoshi/feedman/internal/user"
)

// SubscriptionServiceAdapter は subscription.Service を SubscriptionServiceInterface に適合させるアダプタ。
type SubscriptionServiceAdapter struct {
	svc *subscription.Service
}

// NewSubscriptionServiceAdapter はSubscriptionServiceAdapterを生成する。
func NewSubscriptionServiceAdapter(svc *subscription.Service) *SubscriptionServiceAdapter {
	return &SubscriptionServiceAdapter{svc: svc}
}

// ListSubscriptions はユーザーの購読一覧をhandlerレスポンス型で返す。
func (a *SubscriptionServiceAdapter) ListSubscriptions(ctx context.Context, userID string) ([]subscriptionResponse, error) {
	infos, err := a.svc.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	results := make([]subscriptionResponse, len(infos))
	for i, info := range infos {
		results[i] = toSubscriptionResponse(info)
	}
	return results, nil
}

// UpdateSettings は購読のフェッチ間隔を更新しhandlerレスポンス型で返す。
func (a *SubscriptionServiceAdapter) UpdateSettings(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
	info, err := a.svc.UpdateSettings(ctx, userID, subscriptionID, minutes)
	if err != nil {
		return nil, err
	}
	resp := toSubscriptionResponse(*info)
	return &resp, nil
}

// Unsubscribe は購読を解除する。
func (a *SubscriptionServiceAdapter) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	return a.svc.Unsubscribe(ctx, userID, subscriptionID)
}

// ResumeFetch は停止中フィードのフェッチを再開しhandlerレスポンス型で返す。
func (a *SubscriptionServiceAdapter) ResumeFetch(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
	info, err := a.svc.ResumeFetch(ctx, userID, subscriptionID)
	if err != nil {
		return nil, err
	}
	resp := toSubscriptionResponse(*info)
	return &resp, nil
}

// ManualFetch は手動フェッチを実行し、更新後の購読情報を handler レスポンス型で返す（Issue #115）。
// クールダウン中 / 行ロック競合時はサービス層が APIError を返し、本アダプタはそのまま透過する。
func (a *SubscriptionServiceAdapter) ManualFetch(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
	info, err := a.svc.ManualFetch(ctx, userID, subscriptionID)
	if err != nil {
		return nil, err
	}
	resp := toSubscriptionResponse(*info)
	return &resp, nil
}

// toSubscriptionResponse はドメインのSubscriptionInfoをhandlerのレスポンス型に変換する。
func toSubscriptionResponse(info subscription.SubscriptionInfo) subscriptionResponse {
	return subscriptionResponse{
		ID:                   info.ID,
		UserID:               info.UserID,
		FeedID:               info.FeedID,
		FeedTitle:            info.FeedTitle,
		FeedURL:              info.FeedURL,
		FaviconURL:           info.FaviconURL,
		FetchIntervalMinutes: info.FetchIntervalMinutes,
		FeedStatus:           info.FeedStatus,
		ErrorMessage:         info.ErrorMessage,
		UnreadCount:          info.UnreadCount,
		CreatedAt:            info.CreatedAt,
	}
}

// UserServiceAdapter は user.Service を UserServiceInterface に適合させるアダプタ。
type UserServiceAdapter struct {
	svc *user.Service
}

// NewUserServiceAdapter はUserServiceAdapterを生成する。
func NewUserServiceAdapter(svc *user.Service) *UserServiceAdapter {
	return &UserServiceAdapter{svc: svc}
}

// Withdraw はユーザーの退会処理を実行する。
func (a *UserServiceAdapter) Withdraw(ctx context.Context, userID string) error {
	return a.svc.Withdraw(ctx, userID)
}

// ItemServiceAdapterFromDomain は item.ItemService を ItemServiceInterface に適合させるアダプタ。
type ItemServiceAdapterFromDomain struct {
	svc *item.ItemService
}

// NewItemServiceAdapter は item.ItemService から ItemServiceInterface を生成する。
func NewItemServiceAdapter(svc *item.ItemService) ItemServiceInterface {
	return &ItemServiceAdapterFromDomain{svc: svc}
}

// ListItems はフィードの記事一覧を返す。
func (a *ItemServiceAdapterFromDomain) ListItems(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
	result, err := a.svc.ListItems(ctx, userID, feedID, filter, cursor, limit)
	if err != nil {
		return nil, err
	}

	items := make([]itemSummaryResponse, len(result.Items))
	for i, it := range result.Items {
		items[i] = itemSummaryResponse{
			ID:              it.ID,
			FeedID:          it.FeedID,
			Title:           it.Title,
			Link:            it.Link,
			Summary:         it.Summary,
			PublishedAt:     it.PublishedAt,
			IsDateEstimated: it.IsDateEstimated,
			IsRead:          it.IsRead,
			IsStarred:       it.IsStarred,
			HatebuCount:     it.HatebuCount,
		}
	}

	return &itemListResult{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// ListStarredItems は全フィード横断スター記事一覧を handler のレスポンス型で返す。
// ドメイン層 *item.StarredItemListResult を handler 層 *starredItemListResult に変換する。
// 各記事行に feed_title を併記する（Requirement 2.4 / 4.10）。
func (a *ItemServiceAdapterFromDomain) ListStarredItems(ctx context.Context, userID, cursorStr string, limit int) (*starredItemListResult, error) {
	result, err := a.svc.ListStarredItems(ctx, userID, cursorStr, limit)
	if err != nil {
		return nil, err
	}

	items := make([]starredItemSummaryResponse, len(result.Items))
	for i, it := range result.Items {
		items[i] = starredItemSummaryResponse{
			itemSummaryResponse: itemSummaryResponse{
				ID:              it.ID,
				FeedID:          it.FeedID,
				Title:           it.Title,
				Link:            it.Link,
				Summary:         it.Summary,
				PublishedAt:     it.PublishedAt,
				IsDateEstimated: it.IsDateEstimated,
				IsRead:          it.IsRead,
				IsStarred:       it.IsStarred,
				HatebuCount:     it.HatebuCount,
			},
			FeedTitle: it.FeedTitle,
		}
	}

	return &starredItemListResult{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// GetItem は記事詳細を返す。
func (a *ItemServiceAdapterFromDomain) GetItem(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
	detail, err := a.svc.GetItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, nil
	}

	return &itemDetailResponse{
		itemSummaryResponse: itemSummaryResponse{
			ID:              detail.ID,
			FeedID:          detail.FeedID,
			Title:           detail.Title,
			Link:            detail.Link,
			PublishedAt:     detail.PublishedAt,
			IsDateEstimated: detail.IsDateEstimated,
			IsRead:          detail.IsRead,
			IsStarred:       detail.IsStarred,
			HatebuCount:     detail.HatebuCount,
		},
		Content: detail.Content,
		Summary: detail.Summary,
		Author:  detail.Author,
	}, nil
}

// ItemStateServiceAdapterFromRepo は repository.ItemStateRepository を ItemStateServiceInterface に適合させるアダプタ。
type ItemStateServiceAdapterFromRepo struct {
	repo repository.ItemStateRepository
}

// NewItemStateServiceAdapter は repository.ItemStateRepository から ItemStateServiceInterface を生成する。
func NewItemStateServiceAdapter(repo repository.ItemStateRepository) ItemStateServiceInterface {
	return &ItemStateServiceAdapterFromRepo{repo: repo}
}

// UpdateState は記事の既読・スター状態を冪等に更新する。
func (a *ItemStateServiceAdapterFromRepo) UpdateState(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
	return a.repo.Upsert(ctx, userID, itemID, isRead, isStarred)
}

// SubscriptionDeleterAdapter はリポジトリ層を SubscriptionDeleter に適合させるアダプタ。
type SubscriptionDeleterAdapter struct {
	subRepo       repository.SubscriptionRepository
	itemStateRepo repository.ItemStateRepository
}

// NewSubscriptionDeleterAdapter はSubscriptionDeleterAdapterを生成する。
func NewSubscriptionDeleterAdapter(subRepo repository.SubscriptionRepository, itemStateRepo repository.ItemStateRepository) SubscriptionDeleter {
	return &SubscriptionDeleterAdapter{subRepo: subRepo, itemStateRepo: itemStateRepo}
}

// DeleteByUserAndFeed はユーザーIDとフィードIDで購読と関連item_statesを削除する。
func (a *SubscriptionDeleterAdapter) DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error {
	// 関連item_statesを削除
	if err := a.itemStateRepo.DeleteByUserAndFeed(ctx, userID, feedID); err != nil {
		return err
	}

	// 購読を検索して削除
	sub, err := a.subRepo.FindByUserAndFeed(ctx, userID, feedID)
	if err != nil {
		return err
	}
	if sub == nil {
		return nil
	}
	return a.subRepo.Delete(ctx, sub.ID)
}

// ItemSearchServiceAdapter は itemsearch.SearchService を ItemSearchServiceInterface に
// 適合させるアダプタ。
//
// 主な責務は、サービス層が返す `itemsearch.SearchResult`（ドメイン型）を
// `itemSearchResponse`（HTTP レスポンス型）に変換すること。favicon は Service 層が
// `FaviconData []byte` / `FaviconMime string` の生バイト + MIME を pass-through するため、
// 本アダプタが `data:<mime>;base64,...` 形式の data URL に整形して `FaviconURL` を populate
// する（subscription.Service.ListSubscriptions と同じ data URL 化パターンを再利用）。
type ItemSearchServiceAdapter struct {
	svc *itemsearch.SearchService
}

// NewItemSearchServiceAdapter は ItemSearchServiceAdapter を生成する。
func NewItemSearchServiceAdapter(svc *itemsearch.SearchService) *ItemSearchServiceAdapter {
	return &ItemSearchServiceAdapter{svc: svc}
}

// Search はサービス層を呼び出し、結果を handler 用レスポンス型に変換して返す。
//
// favicon の data URL 化は本メソッドの責務（サービス層は生バイトのまま pass-through する）。
// 生バイトと MIME のいずれかが空の場合は data URL を生成せず nil を保持し、JSON では
// `omitempty` でフィールドごと省略される。
func (a *ItemSearchServiceAdapter) Search(
	ctx context.Context,
	userID, rawQuery string,
	feedID *string,
	cursorStr string,
	limit int,
) (*itemSearchResponse, error) {
	result, err := a.svc.Search(ctx, userID, rawQuery, feedID, cursorStr, limit)
	if err != nil {
		return nil, err
	}

	hits := make([]itemSearchHitResponse, len(result.Items))
	for i, it := range result.Items {
		hit := itemSearchHitResponse{
			ID:              it.ID,
			FeedID:          it.FeedID,
			FeedTitle:       it.FeedTitle,
			Title:           it.Title,
			Link:            it.Link,
			Summary:         it.Summary,
			PublishedAt:     it.PublishedAt,
			IsDateEstimated: it.IsDateEstimated,
			IsRead:          it.IsRead,
			IsStarred:       it.IsStarred,
			HatebuCount:     it.HatebuCount,
			HatebuFetchedAt: it.HatebuFetchedAt,
		}
		// favicon の生バイト + MIME が揃っている場合のみ data URL を組み立てる。
		// 既存 subscription.Service.ListSubscriptions と同じ流儀
		// （`data:<mime>;base64,<base64>`）で整形し、欠落時は nil を保持する。
		if len(it.FaviconData) > 0 && it.FaviconMime != "" {
			dataURL := fmt.Sprintf("data:%s;base64,%s", it.FaviconMime, base64.StdEncoding.EncodeToString(it.FaviconData))
			hit.FaviconURL = &dataURL
		}
		hits[i] = hit
	}

	return &itemSearchResponse{
		Items:      hits,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// CrossFeedServiceAdapter は crossfeed.Service を CrossFeedServiceInterface に適合させる
// アダプタ。domain 層の crossfeed.NewItemsResult を handler 層 *crossFeedListResult に変換する。
//
// favicon_url は service 層が既に `data:<mime>;base64,...` 形式の data URL 文字列まで
// 構築済（CrossFeedItemSummary.FeedFaviconURL *string）なので、本アダプタは pass-through する。
type CrossFeedServiceAdapter struct {
	svc *crossfeed.Service
}

// NewCrossFeedServiceAdapter は CrossFeedServiceAdapter を生成する。
func NewCrossFeedServiceAdapter(svc *crossfeed.Service) *CrossFeedServiceAdapter {
	return &CrossFeedServiceAdapter{svc: svc}
}

// ListNewItems は service 層を呼び出し、結果を handler 用レスポンス型に変換して返す。
// overrideSince は service 層へそのまま転送する（Req 4.7）。
func (a *CrossFeedServiceAdapter) ListNewItems(
	ctx context.Context,
	userID string,
	cursorStr string,
	limit int,
	overrideSince *time.Time,
) (*crossFeedListResult, error) {
	result, err := a.svc.ListNewItems(ctx, userID, cursorStr, limit, overrideSince)
	if err != nil {
		return nil, err
	}

	items := make([]crossFeedItemResponse, len(result.Items))
	for i, it := range result.Items {
		items[i] = crossFeedItemResponse{
			ID:              it.ID,
			FeedID:          it.FeedID,
			FeedTitle:       it.FeedTitle,
			FeedFaviconURL:  it.FeedFaviconURL,
			Title:           it.Title,
			Link:            it.Link,
			Summary:         it.Summary,
			PublishedAt:     it.PublishedAt,
			IsDateEstimated: it.IsDateEstimated,
			IsRead:          it.IsRead,
			IsStarred:       it.IsStarred,
			HatebuCount:     it.HatebuCount,
		}
	}

	return &crossFeedListResult{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		SinceTime:  result.SinceTime,
	}, nil
}

// TouchLastSeen は service 層へそのまま転送する（Req 4.3）。
func (a *CrossFeedServiceAdapter) TouchLastSeen(ctx context.Context, userID string) error {
	return a.svc.TouchLastSeen(ctx, userID)
}

// --- compile-time interface checks ---

var _ SubscriptionServiceInterface = (*SubscriptionServiceAdapter)(nil)
var _ UserServiceInterface = (*UserServiceAdapter)(nil)
var _ ItemServiceInterface = (*ItemServiceAdapterFromDomain)(nil)
var _ ItemStateServiceInterface = (*ItemStateServiceAdapterFromRepo)(nil)
var _ ItemSearchServiceInterface = (*ItemSearchServiceAdapter)(nil)
var _ SubscriptionDeleter = (*SubscriptionDeleterAdapter)(nil)
var _ CrossFeedServiceInterface = (*CrossFeedServiceAdapter)(nil)

// zeroTime はゼロ値のtime.Time。
var zeroTime time.Time
