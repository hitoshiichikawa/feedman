package handler

import (
	"context"
	"time"

	"github.com/hitoshi/feedman/internal/item"
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

// --- compile-time interface checks ---

var _ SubscriptionServiceInterface = (*SubscriptionServiceAdapter)(nil)
var _ UserServiceInterface = (*UserServiceAdapter)(nil)
var _ ItemServiceInterface = (*ItemServiceAdapterFromDomain)(nil)
var _ ItemStateServiceInterface = (*ItemStateServiceAdapterFromRepo)(nil)
var _ SubscriptionDeleter = (*SubscriptionDeleterAdapter)(nil)

// zeroTime はゼロ値のtime.Time。
var zeroTime time.Time
