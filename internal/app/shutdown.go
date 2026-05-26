package app

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/hitoshi/feedman/internal/middleware"
)

// shutdownCoordinator は API サーバーのグレースフルシャットダウン手続きを束ねる。
// HTTP リクエスト処理の終了待機（server.Shutdown）と、RateLimiter のクリーンアップ
// goroutine 停止（RateLimiter.Stop）を、整合する順序で 1 回だけ実行する責務を持つ。
//
// RateLimiter.Stop() は内部で stopCh を close するため、複数回呼び出すと panic する。
// 本 coordinator は sync.Once で Stop() を高々 1 回だけ呼ぶことを保証し、シャットダウン
// 経路が重複して起動され得る状況でも panic を発生させない（RateLimiter 本体の公開
// シグネチャ・挙動は変更しない方針）。
type shutdownCoordinator struct {
	server      *http.Server
	rateLimiter *middleware.RateLimiter
	stopOnce    sync.Once
}

// newShutdownCoordinator はシャットダウン手続きを束ねる coordinator を生成する。
// rateLimiter が nil の場合は RateLimiter の停止処理を行わない。
func newShutdownCoordinator(server *http.Server, rateLimiter *middleware.RateLimiter) *shutdownCoordinator {
	return &shutdownCoordinator{
		server:      server,
		rateLimiter: rateLimiter,
	}
}

// shutdown はグレースフルシャットダウンを実行する。
// 稼働中の HTTP リクエスト処理の終了を待機（server.Shutdown）した後に、
// RateLimiter のクリーンアップ goroutine を停止する。この順序により、
// リクエスト処理を不当に阻害せずにバックグラウンド goroutine だけを終了する。
//
// RateLimiter の停止は sync.Once で保護されており、shutdown が複数回呼び出されても
// 高々 1 回だけ実行されるため、二重 close による panic は発生しない。
func (sc *shutdownCoordinator) shutdown(ctx context.Context) error {
	// 1. 稼働中の HTTP リクエスト処理の終了を待機する。
	shutdownErr := sc.server.Shutdown(ctx)

	// 2. リクエスト drain 完了後にクリーンアップ goroutine を停止する（1 回だけ）。
	sc.stopRateLimiter()

	if shutdownErr != nil {
		return fmt.Errorf("server shutdown failed: %w", shutdownErr)
	}
	return nil
}

// stopRateLimiter は RateLimiter の停止処理を高々 1 回だけ実行する。
func (sc *shutdownCoordinator) stopRateLimiter() {
	if sc.rateLimiter == nil {
		return
	}
	sc.stopOnce.Do(func() {
		sc.rateLimiter.Stop()
	})
}
