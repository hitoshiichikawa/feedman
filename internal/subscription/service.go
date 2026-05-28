// Package subscription は購読管理のドメインロジックを提供する。
package subscription

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/worker/fetch"
)

// manualFetchCooldown は同一フィードへの手動フェッチを抑制するクールダウン期間（Req 2.1, 2.3）。
const manualFetchCooldown = 10 * time.Minute

// ManualFetchTxBeginner は ManualFetch が必要とする最小限のトランザクション開始抽象化。
// production では *sql.DB をラップする SQLManualFetchTxBeginner を用い、test では mock を使う。
// repository.TxBeginner（*sql.Tx を直接返す）とは別の subinterface として定義し、
// commit/rollback も含めた tx ハンドルライフサイクルを抽象化する。
type ManualFetchTxBeginner interface {
	BeginManualFetchTx(ctx context.Context) (ManualFetchTx, error)
}

// ManualFetchTx は ManualFetch が使う tx ハンドルの最小契約。
// Tx() は repository 層（LockFeedForUpdateNowait）に渡す *sql.Tx を返す。
type ManualFetchTx interface {
	Tx() *sql.Tx
	Commit() error
	Rollback() error
}

// ManualFetchMetricsRecorder は ManualFetch が必要とする 4 種のメトリクス記録口を抽象化する。
// task 5（metrics package 拡張）完了後、metrics.MetricsCollector を実装する型が自然に
// 本 interface を充足する。本 task 単独で compile / test 可能にする目的でローカル subinterface を採用。
type ManualFetchMetricsRecorder interface {
	RecordManualFetchSuccess()
	RecordManualFetchFailure(reason string)
	RecordManualFetchCooldownRejected()
	RecordManualFetchLockConflict()
}

// SQLManualFetchTxBeginner は *sql.DB をラップして ManualFetchTxBeginner を実装する。
// runServe での依存配線（task 6.1）から渡される production 実装。
type SQLManualFetchTxBeginner struct {
	db *sql.DB
}

// NewSQLManualFetchTxBeginner は SQLManualFetchTxBeginner を生成する。
func NewSQLManualFetchTxBeginner(db *sql.DB) *SQLManualFetchTxBeginner {
	return &SQLManualFetchTxBeginner{db: db}
}

// BeginManualFetchTx は新しい tx を開始し、ManualFetchTx として返す。
func (b *SQLManualFetchTxBeginner) BeginManualFetchTx(ctx context.Context) (ManualFetchTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin tx: %w", err)
	}
	return &sqlManualFetchTx{tx: tx}, nil
}

type sqlManualFetchTx struct {
	tx *sql.Tx
}

func (h *sqlManualFetchTx) Tx() *sql.Tx     { return h.tx }
func (h *sqlManualFetchTx) Commit() error   { return h.tx.Commit() }
func (h *sqlManualFetchTx) Rollback() error { return h.tx.Rollback() }

// SubscriptionInfo は購読情報とフィード情報を結合したドメインオブジェクト。
type SubscriptionInfo struct {
	ID                   string
	UserID               string
	FeedID               string
	FeedTitle            string
	FeedURL              string
	FaviconURL           *string
	FetchIntervalMinutes int
	FeedStatus           string
	ErrorMessage         *string
	UnreadCount          int
	CreatedAt            time.Time
}

// Service は購読管理のサービス層。
// 購読一覧取得、設定更新、購読解除、フェッチ再開、手動フェッチのビジネスロジックを提供する。
type Service struct {
	subRepo         repository.SubscriptionRepository
	itemStateRepo   repository.ItemStateRepository
	feedRepo        repository.FeedRepository
	feedFetcher     fetch.FeedFetcherService
	txBeginner      ManualFetchTxBeginner
	metricsRecorder ManualFetchMetricsRecorder
}

// NewService はServiceの新しいインスタンスを生成する。
// feedFetcher / txBeginner / metricsRecorder は ManualFetch でのみ使用され、
// ListSubscriptions / UpdateSettings / Unsubscribe / ResumeFetch の各経路では参照されない。
// app.go の wiring（task 6.1）が完了するまでは nil を渡しても既存パスは正常動作する。
func NewService(
	subRepo repository.SubscriptionRepository,
	itemStateRepo repository.ItemStateRepository,
	feedRepo repository.FeedRepository,
	feedFetcher fetch.FeedFetcherService,
	txBeginner ManualFetchTxBeginner,
	metricsRecorder ManualFetchMetricsRecorder,
) *Service {
	return &Service{
		subRepo:         subRepo,
		itemStateRepo:   itemStateRepo,
		feedRepo:        feedRepo,
		feedFetcher:     feedFetcher,
		txBeginner:      txBeginner,
		metricsRecorder: metricsRecorder,
	}
}

// ListSubscriptions はユーザーの購読一覧をフィード情報付きで返す。
func (s *Service) ListSubscriptions(ctx context.Context, userID string) ([]SubscriptionInfo, error) {
	rows, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読一覧の取得に失敗しました: %w", err)
	}

	results := make([]SubscriptionInfo, len(rows))
	for i, row := range rows {
		info := SubscriptionInfo{
			ID:                   row.ID,
			UserID:               row.UserID,
			FeedID:               row.FeedID,
			FeedTitle:            row.FeedTitle,
			FeedURL:              row.FeedURL,
			FetchIntervalMinutes: row.FetchIntervalMinutes,
			FeedStatus:           string(row.FetchStatus),
			UnreadCount:          row.UnreadCount,
			CreatedAt:            row.CreatedAt,
		}

		// faviconデータがある場合はdata URLに変換
		if len(row.FaviconData) > 0 && row.FaviconMime != "" {
			dataURL := fmt.Sprintf("data:%s;base64,%s", row.FaviconMime, base64.StdEncoding.EncodeToString(row.FaviconData))
			info.FaviconURL = &dataURL
		}

		// エラーメッセージがある場合
		if row.ErrorMessage != "" {
			msg := row.ErrorMessage
			info.ErrorMessage = &msg
		}

		results[i] = info
	}

	return results, nil
}

// fetchIntervalMin はフェッチ間隔の下限（分）。
const fetchIntervalMin = 30

// fetchIntervalMax はフェッチ間隔の上限（分、12時間）。
const fetchIntervalMax = 720

// fetchIntervalStep はフェッチ間隔の刻み幅（分）。
const fetchIntervalStep = 30

// isValidFetchInterval はフェッチ間隔が許容範囲（30〜720分・30分刻み）かを判定する。
func isValidFetchInterval(minutes int) bool {
	return minutes >= fetchIntervalMin && minutes <= fetchIntervalMax && minutes%fetchIntervalStep == 0
}

// UpdateSettings は購読のフェッチ間隔を更新する。
// minutes が許容範囲（30〜720分・30分刻み）外の場合は更新を行わず INVALID_FETCH_INTERVAL を返す。
func (s *Service) UpdateSettings(ctx context.Context, userID, subscriptionID string, minutes int) (*SubscriptionInfo, error) {
	if !isValidFetchInterval(minutes) {
		return nil, model.NewInvalidFetchIntervalError(minutes)
	}

	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	if err := s.subRepo.UpdateFetchInterval(ctx, subscriptionID, minutes); err != nil {
		return nil, fmt.Errorf("フェッチ間隔の更新に失敗しました: %w", err)
	}

	// 更新後の購読情報を取得して返す
	infos, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読情報の再取得に失敗しました: %w", err)
	}

	for _, info := range infos {
		if info.ID == subscriptionID {
			result := &SubscriptionInfo{
				ID:                   info.ID,
				UserID:               info.UserID,
				FeedID:               info.FeedID,
				FeedTitle:            info.FeedTitle,
				FeedURL:              info.FeedURL,
				FetchIntervalMinutes: info.FetchIntervalMinutes,
				FeedStatus:           string(info.FetchStatus),
				UnreadCount:          info.UnreadCount,
				CreatedAt:            info.CreatedAt,
			}
			return result, nil
		}
	}

	return nil, model.NewSubscriptionNotFoundError(subscriptionID)
}

// Unsubscribe は購読を解除する。
// subscription と関連 item_states を削除する。
func (s *Service) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// 関連item_statesを削除
	if s.itemStateRepo != nil {
		if err := s.itemStateRepo.DeleteByUserAndFeed(ctx, userID, sub.FeedID); err != nil {
			return fmt.Errorf("記事状態の削除に失敗しました: %w", err)
		}
	}

	// 購読を削除
	if err := s.subRepo.Delete(ctx, subscriptionID); err != nil {
		return fmt.Errorf("購読の削除に失敗しました: %w", err)
	}

	return nil
}

// ResumeFetch は停止中フィードのフェッチを再開する。
func (s *Service) ResumeFetch(ctx context.Context, userID, subscriptionID string) (*SubscriptionInfo, error) {
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// フィード状態を取得
	feed, err := s.feedRepo.FindByID(ctx, sub.FeedID)
	if err != nil {
		return nil, fmt.Errorf("フィードの取得に失敗しました: %w", err)
	}
	if feed == nil {
		return nil, fmt.Errorf("フィードが見つかりません: %s", sub.FeedID)
	}

	if feed.FetchStatus != model.FetchStatusStopped {
		return nil, model.NewFeedNotStoppedError()
	}

	// フェッチ状態をactiveに戻す
	feed.FetchStatus = model.FetchStatusActive
	feed.ErrorMessage = ""
	feed.ConsecutiveErrors = 0
	feed.NextFetchAt = time.Now()

	if err := s.feedRepo.UpdateFetchState(ctx, feed); err != nil {
		return nil, fmt.Errorf("フィード状態の更新に失敗しました: %w", err)
	}

	// 更新後の購読情報を返す
	infos, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読情報の再取得に失敗しました: %w", err)
	}

	for _, info := range infos {
		if info.ID == subscriptionID {
			result := &SubscriptionInfo{
				ID:                   info.ID,
				UserID:               info.UserID,
				FeedID:               info.FeedID,
				FeedTitle:            info.FeedTitle,
				FeedURL:              info.FeedURL,
				FetchIntervalMinutes: info.FetchIntervalMinutes,
				FeedStatus:           string(info.FetchStatus),
				UnreadCount:          info.UnreadCount,
				CreatedAt:            info.CreatedAt,
			}
			return result, nil
		}
	}

	return nil, model.NewSubscriptionNotFoundError(subscriptionID)
}


// ManualFetch は指定購読のフィードを手動で同期フェッチする。
// クールダウン中は外部 HTTP を発行せず FEED_COOLDOWN を返し、
// 行ロック競合時は FEED_FETCH_IN_PROGRESS を返す。
// 成功時は更新後の SubscriptionInfo を返す。
//
// フロー（Req 1.1, 2.1, 3.1, 3.4）:
//
//	(1) subRepo.FindByID で認可確認（subID 不存在 / UserID 不一致は SUBSCRIPTION_NOT_FOUND）
//	(2) BeginManualFetchTx で tx 開始（defer rollback）
//	(3) LockFeedForUpdateNowait で行ロック取得（NOWAIT、競合時は ErrFeedLocked）
//	(4) クールダウン判定（last_successful_fetch_at + 10min > now なら FEED_COOLDOWN）
//	(5) クールダウン外: COMMIT で行ロックを解放してから fetcher.Fetch を実行
//	    （既存 Fetcher は *sql.DB 経由で UpdateFetchState を呼ぶため、tx ロック保持中に
//	    fetcher を実行すると自己 deadlock するため事前に解放する）
//	(6) Fetch が nil 返却 + FetchStatus=active + ConsecutiveErrors=0 のとき成功と判定し
//	    UpdateLastSuccessfulFetchAt + success メトリクスを記録
//	(7) ListByUserIDWithFeedInfo で最新 SubscriptionInfo を取得して返す
func (s *Service) ManualFetch(ctx context.Context, userID, subscriptionID string) (*SubscriptionInfo, error) {
	// (1) 認可確認
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil || sub.UserID != userID {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// (2) tx 開始
	tx, err := s.txBeginner.BeginManualFetchTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	txClosed := false
	defer func() {
		if !txClosed {
			_ = tx.Rollback()
		}
	}()

	// (3) 行ロック取得（NOWAIT）
	feed, err := s.feedRepo.LockFeedForUpdateNowait(ctx, tx.Tx(), sub.FeedID)
	if err != nil {
		if errors.Is(err, repository.ErrFeedLocked) {
			s.metricsRecorder.RecordManualFetchLockConflict()
			return nil, model.NewFeedFetchInProgressError()
		}
		return nil, fmt.Errorf("フィードのロック取得に失敗しました: %w", err)
	}
	if feed == nil {
		// design 上 subID 存在時は feedID も存在するはずだが防御的に nil チェック
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// (4) クールダウン判定
	now := time.Now()
	if feed.LastSuccessfulFetchAt != nil {
		elapsed := now.Sub(*feed.LastSuccessfulFetchAt)
		if elapsed < manualFetchCooldown {
			remaining := manualFetchCooldown - elapsed
			retryAfterSeconds := int(math.Ceil(remaining.Seconds()))
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
			}
			txClosed = true
			s.metricsRecorder.RecordManualFetchCooldownRejected()
			return nil, model.NewFeedCooldownError(retryAfterSeconds)
		}
	}

	// (5) クールダウン外: fetcher を呼ぶ前に COMMIT で行ロックを解放する
	// 既存 Fetcher は *sql.DB 経由で UpdateFetchState を発行するため、tx ロック保持中に
	// fetcher を実行すると同一行への UPDATE が自己 deadlock する。design.md は
	// 「fetcher 内で tx-aware に動作させる」記述があるが、本機能では fetcher のシグネチャを
	// 変更しない後方互換最優先（design.md「データ更新の責務」節）方針に従い、
	// クールダウン判定までを tx 内に閉じ、fetcher 開始前に lock を解放する。
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
	}
	txClosed = true

	// (6) fetcher.Fetch 実行
	fetchErr := s.feedFetcher.Fetch(ctx, feed)
	if fetchErr != nil {
		reason := classifyFetchError(fetchErr)
		s.metricsRecorder.RecordManualFetchFailure(reason)
		if reason == "ssrf_blocked" {
			// Req 4.2: SSRF 拒否は中立的失敗（攻撃者に SSRF ガードの存在を露呈しない）
			return nil, model.NewFetchFailedError("一時的なエラー")
		}
		return nil, model.NewFetchFailedError(fetchErr.Error())
	}

	// (7) 結果分類: fetcher が nil 返却でもフィード状態で成功/失敗を見極める
	switch {
	case feed.FetchStatus == model.FetchStatusActive && feed.ConsecutiveErrors == 0:
		// 成功パス（200 OK / 304 Not Modified）
		// UpdateLastSuccessfulFetchAt は Fetcher 側で既に呼ばれているが、design.md
		// 「手動経路: Service.ManualFetch が UpdateLastSuccessfulFetchAt を呼ぶ」に従い
		// 手動経路の責務として明示的に「リクエスト完了時刻」（Req 1.2）で再度更新する。
		if updateErr := s.feedRepo.UpdateLastSuccessfulFetchAt(ctx, feed.ID, time.Now()); updateErr != nil {
			// 成功時刻の更新失敗は degradation。task 3 と同じく warning 扱い相当
			// （ここでは戻り値で warning を返せないため、エラー値だけ無視）
			_ = updateErr
		}
		s.metricsRecorder.RecordManualFetchSuccess()

	case strings.Contains(feed.ErrorMessage, "記事UPSERT失敗") ||
		strings.Contains(feed.ErrorMessage, "UPSERT"):
		// UPSERT 失敗（既存 fetcher の ApplyParseFailure 経由）
		s.metricsRecorder.RecordManualFetchFailure("upsert_error")
		return nil, model.NewFetchFailedError("記事の保存に失敗しました")

	case strings.Contains(feed.ErrorMessage, "パース失敗") ||
		feed.FetchStatus == model.FetchStatusError:
		// パース失敗
		s.metricsRecorder.RecordManualFetchFailure("parse_error")
		return nil, model.NewParseFailedError()

	case strings.Contains(feed.ErrorMessage, "SSRF"):
		// SSRF 検証失敗（ApplyStopFeed 経由、Fetcher 内で nil 返却にはならないが防御的に分岐）
		s.metricsRecorder.RecordManualFetchFailure("ssrf_blocked")
		return nil, model.NewFetchFailedError("一時的なエラー")

	default:
		// ApplyBackoff / ApplyStopFeed 経由の失敗（5xx / 429 / 404 / 410 等）
		s.metricsRecorder.RecordManualFetchFailure("fetch_error")
		msg := feed.ErrorMessage
		if msg == "" {
			msg = "フィードの取得に失敗しました"
		}
		return nil, model.NewFetchFailedError(msg)
	}

	// (8) 成功時のみ最新 SubscriptionInfo を返す
	infos, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読情報の再取得に失敗しました: %w", err)
	}
	for _, info := range infos {
		if info.ID == subscriptionID {
			result := &SubscriptionInfo{
				ID:                   info.ID,
				UserID:               info.UserID,
				FeedID:               info.FeedID,
				FeedTitle:            info.FeedTitle,
				FeedURL:              info.FeedURL,
				FetchIntervalMinutes: info.FetchIntervalMinutes,
				FeedStatus:           string(info.FetchStatus),
				UnreadCount:          info.UnreadCount,
				CreatedAt:            info.CreatedAt,
			}
			if len(info.FaviconData) > 0 && info.FaviconMime != "" {
				dataURL := fmt.Sprintf("data:%s;base64,%s", info.FaviconMime, base64.StdEncoding.EncodeToString(info.FaviconData))
				result.FaviconURL = &dataURL
			}
			if info.ErrorMessage != "" {
				msg := info.ErrorMessage
				result.ErrorMessage = &msg
			}
			return result, nil
		}
	}

	return nil, model.NewSubscriptionNotFoundError(subscriptionID)
}

// classifyFetchError は Fetcher が返したエラーから failure metric 用の reason ラベルを推定する。
// 既存 fetcher のエラーラッピング文字列を見て分類する（SSRF / HTTP 系 / その他）。
func classifyFetchError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "SSRF") {
		return "ssrf_blocked"
	}
	if strings.Contains(msg, "パース") || strings.Contains(msg, "parse") {
		return "parse_error"
	}
	if strings.Contains(msg, "UPSERT") || strings.Contains(msg, "upsert") {
		return "upsert_error"
	}
	return "fetch_error"
}
