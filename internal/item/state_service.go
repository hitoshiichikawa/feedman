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
}

// NewItemStateService はItemStateServiceの新しいインスタンスを生成する。
func NewItemStateService(
	itemRepo repository.ItemRepository,
	itemStateRepo repository.ItemStateRepository,
) *ItemStateService {
	return &ItemStateService{
		itemRepo:      itemRepo,
		itemStateRepo: itemStateRepo,
	}
}

// UpdateState は記事の既読・スター状態を冪等に更新する。
// nilフィールドは変更せず、既存の値を維持する部分更新を行う。
// 記事が存在しない場合はITEM_NOT_FOUNDエラーを返す。
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

	// 記事状態をUPSERT（user_idを常に条件に含める）
	state, err := s.itemStateRepo.Upsert(ctx, userID, itemID, isRead, isStarred)
	if err != nil {
		return nil, err
	}

	return state, nil
}
