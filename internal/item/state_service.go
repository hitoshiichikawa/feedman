package item

import (
	"context"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// ItemStateService は記事の既読・スター状態の管理サービス。
// 冪等な明示的更新（トグルではない）で状態を変更する。
type ItemStateService struct {
	itemRepo      repository.ItemRepository
	itemStateRepo repository.ItemStateRepository
	subChecker    SubscriptionChecker
}

// NewItemStateService はItemStateServiceの新しいインスタンスを生成する。
// subChecker は状態更新時に呼び出しユーザーが当該フィードを購読しているかを
// 確認するために使用する（購読外フィードの記事への越境書き込みを防ぐ）。
func NewItemStateService(
	itemRepo repository.ItemRepository,
	itemStateRepo repository.ItemStateRepository,
	subChecker SubscriptionChecker,
) *ItemStateService {
	return &ItemStateService{
		itemRepo:      itemRepo,
		itemStateRepo: itemStateRepo,
		subChecker:    subChecker,
	}
}

// UpdateState は記事の既読・スター状態を冪等に更新する。
// nilフィールドは変更せず、既存の値を維持する部分更新を行う。
// 記事が存在しない、または呼び出しユーザーが当該フィードを未購読の場合は
// ITEM_NOT_FOUND エラーを返す（越境書き込みを防ぐため存在を秘匿する）。
// ユーザーデータ分離（全クエリにuser_id条件付与）をRepository層で強制する。
func (s *ItemStateService) UpdateState(
	ctx context.Context,
	userID, itemID string,
	isRead *bool,
	isStarred *bool,
) (*model.ItemState, error) {
	// 記事の存在確認
	item, err := s.itemRepo.FindByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, model.NewItemNotFoundError(itemID)
	}

	// 認可: 呼び出しユーザーが当該記事のフィードを購読していることを確認する。
	// 未購読フィードの記事に対する状態書き込み・ID 列挙を防ぐ。
	sub, err := s.subChecker.FindByUserAndFeed(ctx, userID, item.FeedID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, model.NewItemNotFoundError(itemID)
	}

	// 記事状態をUPSERT（user_idを常に条件に含める）
	state, err := s.itemStateRepo.Upsert(ctx, userID, itemID, isRead, isStarred)
	if err != nil {
		return nil, err
	}

	return state, nil
}
