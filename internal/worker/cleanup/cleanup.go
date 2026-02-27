// Package cleanup は記事データの自動削除ジョブを提供する。
// 保持期間（デフォルト180日）を超過した記事と関連するitem_statesを
// 日次バッチで削除する。item_statesはCASCADE削除で自動的に処理される。
package cleanup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Executor はSQLのExecContextを抽象化するインターフェース。
// *sql.DB や *sql.Tx を受け付けることができる。
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// CleanupJob は保持期間を超過した記事の自動削除ジョブ。
// 日次実行のバッチジョブとして設計されており、冪等な削除処理を保証する。
type CleanupJob struct {
	db            Executor
	logger        *slog.Logger
	RetentionDays int // 記事の保持日数（デフォルト: 180）
}

// NewCleanupJob は新しいCleanupJobを生成する。
// デフォルトの保持日数は180日。
func NewCleanupJob(db Executor, logger *slog.Logger) *CleanupJob {
	return &CleanupJob{
		db:            db,
		logger:        logger,
		RetentionDays: 180,
	}
}

// Run は保持期間を超過した記事を削除する。
// created_atがRetentionDays日前より古い記事をDELETEする。
// item_statesはCASCADE削除により自動的に削除される。
// 冪等: 削除対象がない場合でもエラーにならない。
func (j *CleanupJob) Run(ctx context.Context) error {
	start := time.Now()

	interval := fmt.Sprintf("%d days", j.RetentionDays)

	query := `DELETE FROM items WHERE created_at < now() - $1::interval`
	result, err := j.db.ExecContext(ctx, query, interval)
	if err != nil {
		j.logger.Error("記事クリーンアップジョブの実行に失敗しました",
			slog.String("error", err.Error()),
			slog.Int("retention_days", j.RetentionDays),
		)
		return fmt.Errorf("記事クリーンアップの実行に失敗: %w", err)
	}

	deletedCount, err := result.RowsAffected()
	if err != nil {
		j.logger.Error("削除件数の取得に失敗しました",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("削除件数の取得に失敗: %w", err)
	}

	duration := time.Since(start)
	j.logger.Info("記事クリーンアップジョブが完了しました",
		slog.Int64("deleted_count", deletedCount),
		slog.Int("retention_days", j.RetentionDays),
		slog.Float64("duration_ms", float64(duration.Milliseconds())),
	)

	return nil
}
