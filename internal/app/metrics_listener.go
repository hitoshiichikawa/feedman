package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/hitoshi/feedman/internal/metrics"
	"github.com/hitoshi/feedman/internal/middleware"
)

// startWorkerMetricsListener は worker 用の metrics HTTP listener を新規 goroutine で起動する。
//
// gatherer（worker 専用 registry）を SetupMetricsRoute で公開し、NewTrustedCIDRMiddleware(cidrs)
// で前段に信頼 CIDR 制限を重ねた http.Server を addr で待ち受ける。worker は HTTP ルーターを
// 持たないため、メトリクス公開専用の軽量 listener を独立して起動する（Requirement 3.1）。
//
// ctx がキャンセルされると server.Shutdown による graceful stop を行い goroutine リークを防ぐ。
// listener の起動失敗（ポート競合等）は worker 本体を落とさずエラーログにとどめる
// （メトリクス公開の失敗でフェッチ機能全体を止めない）。
func startWorkerMetricsListener(ctx context.Context, addr string, gatherer prometheus.Gatherer, cidrs []string) {
	cidrMiddleware := middleware.NewTrustedCIDRMiddleware(cidrs)
	handler := cidrMiddleware(metrics.SetupMetricsRoute(gatherer))

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ctx キャンセルで graceful shutdown する watcher goroutine。
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("metrics listener のシャットダウンに失敗しました",
				slog.String("addr", addr),
				slog.String("error", err.Error()),
			)
		}
	}()

	// listener 本体を別 goroutine で起動する（worker 本体をブロックしない）。
	go func() {
		slog.Info("worker metrics listener starting",
			slog.String("addr", addr),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics listener の起動に失敗しました",
				slog.String("addr", addr),
				slog.String("error", err.Error()),
			)
		}
	}()
}
