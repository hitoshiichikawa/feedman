package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/config"
	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/feed"
	"github.com/hitoshi/feedman/internal/handler"
	"github.com/hitoshi/feedman/internal/hatebu"
	"github.com/hitoshi/feedman/internal/item"
	"github.com/hitoshi/feedman/internal/logger"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/security"
	"github.com/hitoshi/feedman/internal/subscription"
	"github.com/hitoshi/feedman/internal/user"
	"github.com/hitoshi/feedman/internal/worker/cleanup"
	fetchpkg "github.com/hitoshi/feedman/internal/worker/fetch"
)

// Init はアプリケーションの初期化を行う。
// 環境変数からConfigを読み込み、JSON構造化ログをセットアップする。
// writerが指定された場合はログ出力先としてそのwriterを使用する。
func Init(w io.Writer) (*config.Config, error) {
	// 1. ログの初期化（設定読み込み前にログを使えるようにする）
	logger.SetupDefault(w)

	// 2. 環境変数から設定を読み込む
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

// Run はアプリケーションのメインエントリーポイント。
// コマンドライン引数からサブコマンドを解析し、対応するモードで起動する。
// argsにはos.Args[1:]を渡す。
func Run(w io.Writer, args []string) error {
	cmd := ParseCommand(args)

	// healthcheck は軽量サブコマンドのため、フル初期化をスキップする
	if cmd == CommandHealthcheck {
		port := os.Getenv("SERVER_PORT")
		if port == "" {
			port = "8080"
		}
		return runHealthcheck(port)
	}

	cfg, err := Init(w)
	if err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	slog.Info("starting application",
		slog.String("command", string(cmd)),
		slog.String("port", cfg.ServerPort),
		slog.String("base_url", cfg.BaseURL),
	)

	switch cmd {
	case CommandServe:
		return runServe(cfg)
	case CommandWorker:
		return runWorker(cfg)
	case CommandMigrate:
		return runMigrate(cfg)
	default:
		return runServe(cfg)
	}
}

// runServe はAPIサーバーモードで起動する。
// DB接続を開き、全依存関係をワイヤリングし、HTTPサーバーを起動する。
// SIGINTまたはSIGTERMシグナルを受信するとグレースフルシャットダウンを行う。
func runServe(cfg *config.Config) error {
	// 1. DB接続
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Info("database connection established")

	// 2. リポジトリの初期化
	userRepo := repository.NewPostgresUserRepo(db)
	identRepo := repository.NewPostgresIdentityRepo(db)
	sessionRepo := repository.NewPostgresSessionRepo(db)
	feedRepo := repository.NewPostgresFeedRepo(db)
	subRepo := repository.NewPostgresSubscriptionRepo(db)
	itemRepo := repository.NewPostgresItemRepo(db)
	itemStateRepo := repository.NewPostgresItemStateRepo(db)

	// 3. セキュリティサービスの初期化
	ssrfGuard := security.NewSSRFGuard()

	// 4. ドメインサービスの初期化
	oauthProvider := auth.NewGoogleOAuthProvider(auth.GoogleOAuthConfig{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
	})
	authService := auth.NewService(
		oauthProvider, userRepo, identRepo, sessionRepo,
		auth.ServiceConfig{SessionMaxAge: cfg.SessionMaxAge},
	)

	feedDetector := feed.NewFeedDetector(ssrfGuard)
	faviconFetcher := feed.NewFaviconFetcher(ssrfGuard)
	feedService := feed.NewFeedService(feedRepo, subRepo, feedDetector, faviconFetcher)

	itemService := item.NewItemService(itemRepo, itemStateRepo)

	subService := subscription.NewService(subRepo, itemStateRepo, feedRepo)
	userService := user.NewService(userRepo, sessionRepo, subRepo, itemStateRepo)

	// 5. ハンドラーアダプタの構築
	subServiceAdapter := handler.NewSubscriptionServiceAdapter(subService)
	userServiceAdapter := handler.NewUserServiceAdapter(userService)
	itemServiceAdapter := handler.NewItemServiceAdapter(itemService)
	itemStateServiceAdapter := handler.NewItemStateServiceAdapter(itemStateRepo)

	// 6. SubscriptionDeleterアダプタの構築
	subDeleterAdapter := handler.NewSubscriptionDeleterAdapter(subRepo, itemStateRepo)

	// 7. ルーターの構築
	rateLimiterCfg := middleware.DefaultRateLimiterConfig()
	// デフォルト値から変更する場合のみ上書き（req/min -> req/sec に変換）
	// configのRateLimitGeneralはreq/min単位なのでreq/secに変換する

	deps := &handler.RouterDeps{
		HealthChecker:     db,
		SessionFinder:     sessionRepo,
		CORSAllowedOrigin: cfg.CORSAllowedOrigin,
		RateLimiter:       middleware.NewRateLimiter(rateLimiterCfg),

		AuthService: authService,
		AuthConfig: handler.AuthHandlerConfig{
			BaseURL:       cfg.BaseURL,
			CookieDomain:  cfg.CookieDomain,
			CookieSecure:  cfg.CookieSecure,
			SessionMaxAge: cfg.SessionMaxAge,
		},

		FeedService:         feedService,
		SubscriptionDeleter: subDeleterAdapter,

		ItemService:      itemServiceAdapter,
		ItemStateService: itemStateServiceAdapter,

		SubscriptionService: subServiceAdapter,
		UserService:         userServiceAdapter,
	}

	router := handler.NewRouter(deps)

	// 8. HTTPサーバーの起動
	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// グレースフルシャットダウンのためのシグナルハンドリング
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("API server starting",
			slog.String("addr", server.Addr),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server listen error", slog.String("error", err.Error()))
		}
	}()

	<-stop
	slog.Info("shutting down API server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	slog.Info("API server stopped gracefully")
	return nil
}

// runWorker はワーカーモードで起動する。
// DB接続を開き、フェッチスケジューラを起動する。
// SIGINTまたはSIGTERMシグナルを受信するとシャットダウンする。
func runWorker(cfg *config.Config) error {
	// 1. DB接続
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Info("database connection established (worker)")

	// 2. リポジトリの初期化
	feedRepo := repository.NewPostgresFeedRepo(db)
	subRepo := repository.NewPostgresSubscriptionRepo(db)
	itemRepo := repository.NewPostgresItemRepo(db)

	// 3. セキュリティサービスの初期化
	ssrfGuard := security.NewSSRFGuard()
	sanitizer := security.NewContentSanitizer()

	// 4. フェッチャーの初期化
	upsertSvc := item.NewItemUpsertService(itemRepo, sanitizer)
	fetcher := fetchpkg.NewFetcher(
		feedRepo, subRepo, upsertSvc, ssrfGuard,
		slog.Default(), cfg.FetchTimeout, cfg.FetchMaxSize,
	)

	// 5. スケジューラの起動
	scheduler := fetchpkg.NewScheduler(
		feedRepo, fetcher, slog.Default(), cfg.FetchMaxConcurrent,
	)

	// 6. クリーンアップジョブの初期化
	cleanupJob := cleanup.NewCleanupJob(db, slog.Default())

	// 7. はてなブックマークバッチジョブの初期化
	hatebuClient := hatebu.NewClient(
		&http.Client{Timeout: 10 * time.Second},
		slog.Default(),
	)
	hatebuBatch := hatebu.NewBatchJob(itemRepo, hatebuClient, slog.Default(), hatebu.BatchConfig{
		BatchInterval:    cfg.HatebuBatchInterval,
		APIInterval:      cfg.HatebuAPIInterval,
		MaxCallsPerCycle: cfg.HatebuMaxCallsPerCycle,
		HatebuTTL:        cfg.HatebuTTL,
	})

	// グレースフルシャットダウンのためのシグナルハンドリング
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		slog.Info("shutting down worker...")
		cancel()
	}()

	slog.Info("worker starting",
		slog.Duration("fetch_interval", cfg.FetchInterval),
		slog.Int("max_concurrent", cfg.FetchMaxConcurrent),
	)

	// はてなブックマークバッチジョブをバックグラウンドで起動
	go hatebuBatch.Start(ctx)

	// クリーンアップジョブを日次でバックグラウンド実行
	go func() {
		// 起動直後に1回実行
		if err := cleanupJob.Run(ctx); err != nil {
			slog.Error("cleanup job failed", slog.String("error", err.Error()))
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cleanupJob.Run(ctx); err != nil {
					slog.Error("cleanup job failed", slog.String("error", err.Error()))
				}
			}
		}
	}()

	// フェッチスケジューラをメインgoroutineで実行（ブロッキング）
	scheduler.Start(ctx, cfg.FetchInterval)

	slog.Info("worker stopped gracefully")
	return nil
}

// runMigrate はデータベースマイグレーションを実行する。
// すべての未適用マイグレーションを順番に適用する。
func runMigrate(cfg *config.Config) error {
	slog.Info("running database migrations",
		slog.String("database_url", maskDatabaseURL(cfg.DatabaseURL)),
	)

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	slog.Info("database migrations completed successfully")
	return nil
}

// runHealthcheck はヘルスチェックを実行する。
// distroless環境でのDockerヘルスチェック用サブコマンド。
// /health エンドポイントにHTTPリクエストを送り、結果を返す。
func runHealthcheck(port string) error {
	url := fmt.Sprintf("http://localhost:%s/health", port)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// maskDatabaseURL はデータベースURLの認証情報をマスクする。
func maskDatabaseURL(url string) string {
	if len(url) > 20 {
		return url[:12] + "***@..."
	}
	return "***"
}
