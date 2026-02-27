package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
)

// PostgresItemStateRepo はPostgreSQLを使用した記事状態リポジトリ。
type PostgresItemStateRepo struct {
	db *sql.DB
}

// NewPostgresItemStateRepo はPostgresItemStateRepoを生成する。
func NewPostgresItemStateRepo(db *sql.DB) *PostgresItemStateRepo {
	return &PostgresItemStateRepo{db: db}
}

// FindByUserAndItem はユーザーIDと記事IDで記事状態を取得する。見つからない場合はnilを返す。
func (r *PostgresItemStateRepo) FindByUserAndItem(ctx context.Context, userID, itemID string) (*model.ItemState, error) {
	state := &model.ItemState{}
	var readAt, starredAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, item_id, is_read, is_starred, read_at, starred_at, created_at, updated_at
		 FROM item_states WHERE user_id = $1 AND item_id = $2`,
		userID, itemID,
	).Scan(
		&state.ID, &state.UserID, &state.ItemID,
		&state.IsRead, &state.IsStarred,
		&readAt, &starredAt,
		&state.CreatedAt, &state.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("記事状態の取得に失敗しました: %w", err)
	}

	if readAt.Valid {
		state.ReadAt = &readAt.Time
	}
	if starredAt.Valid {
		state.StarredAt = &starredAt.Time
	}

	return state, nil
}

// Upsert は記事状態を冪等にUPSERTする。
// nilフィールドは変更せず、既存の値を維持する部分更新を行う。
// UNIQUE(user_id, item_id)制約を利用したINSERT ON CONFLICTで実装する。
func (r *PostgresItemStateRepo) Upsert(
	ctx context.Context,
	userID, itemID string,
	isRead *bool,
	isStarred *bool,
) (*model.ItemState, error) {
	now := time.Now().UTC()

	// 既存レコードを確認
	existing, err := r.FindByUserAndItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		// 新規作成
		state := &model.ItemState{
			ID:        uuid.New().String(),
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    false,
			IsStarred: false,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if isRead != nil {
			state.IsRead = *isRead
			if *isRead {
				state.ReadAt = &now
			}
		}
		if isStarred != nil {
			state.IsStarred = *isStarred
			if *isStarred {
				state.StarredAt = &now
			}
		}

		_, err := r.db.ExecContext(ctx,
			`INSERT INTO item_states (id, user_id, item_id, is_read, is_starred, read_at, starred_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (user_id, item_id) DO UPDATE SET
			     is_read = EXCLUDED.is_read,
			     is_starred = EXCLUDED.is_starred,
			     read_at = EXCLUDED.read_at,
			     starred_at = EXCLUDED.starred_at,
			     updated_at = EXCLUDED.updated_at`,
			state.ID, state.UserID, state.ItemID,
			state.IsRead, state.IsStarred,
			state.ReadAt, state.StarredAt,
			state.CreatedAt, state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("記事状態の作成に失敗しました: %w", err)
		}

		return state, nil
	}

	// 既存レコードの部分更新
	existing.UpdatedAt = now
	if isRead != nil {
		existing.IsRead = *isRead
		if *isRead && existing.ReadAt == nil {
			existing.ReadAt = &now
		} else if !*isRead {
			existing.ReadAt = nil
		}
	}
	if isStarred != nil {
		existing.IsStarred = *isStarred
		if *isStarred && existing.StarredAt == nil {
			existing.StarredAt = &now
		} else if !*isStarred {
			existing.StarredAt = nil
		}
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE item_states SET
		    is_read = $3, is_starred = $4, read_at = $5, starred_at = $6, updated_at = $7
		 WHERE user_id = $1 AND item_id = $2`,
		existing.UserID, existing.ItemID,
		existing.IsRead, existing.IsStarred,
		existing.ReadAt, existing.StarredAt,
		existing.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("記事状態の更新に失敗しました: %w", err)
	}

	return existing, nil
}

// DeleteByUserAndFeed はユーザーIDとフィードIDに関連する記事状態を全て削除する。
// item_statesテーブルのitem_idをitemsテーブルのfeed_idと結合して削除対象を特定する。
func (r *PostgresItemStateRepo) DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM item_states
		 WHERE user_id = $1 AND item_id IN (
		     SELECT id FROM items WHERE feed_id = $2
		 )`,
		userID, feedID,
	)
	if err != nil {
		return fmt.Errorf("ユーザーとフィードに関連する記事状態の削除に失敗しました: %w", err)
	}
	return nil
}

// DeleteByUserID はユーザーIDに関連する全ての記事状態を削除する。
func (r *PostgresItemStateRepo) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM item_states WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("ユーザーの記事状態の削除に失敗しました: %w", err)
	}
	return nil
}

// compile-time interface check
var _ ItemStateRepository = (*PostgresItemStateRepo)(nil)
