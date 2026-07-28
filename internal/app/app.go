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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/config"
	"github.com/hitoshi/feedman/internal/crossfeed"
	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/feed"
	"github.com/hitoshi/feedman/internal/handler"
	"github.com/hitoshi/feedman/internal/hatebu"
	"github.com/hitoshi/feedman/internal/item"
	"github.com/hitoshi/feedman/internal/itemsearch"
	"github.com/hitoshi/feedman/internal/logger"
	"github.com/hitoshi/feedman/internal/metrics"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/passkey"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/security"
	"github.com/hitoshi/feedman/internal/subscription"
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
	// native auth（#165）: flow=native callback の auth_code 保存に使用する。
	authCodeRepo := repository.NewPostgresAuthCodeRepo(db)
	// native auth（#166）: POST /api/auth/token の refresh family / token 永続化に使用する。
	refreshTokenRepo := repository.NewPostgresRefreshTokenRepo(db)
	// passkey credential（#216）: 退会 tx cleanup と passkey handler の両方で共有する。
	//   - 退会 tx cleanup（Req 7.1, 7.2）は env 未設定でも動く必要があるため、
	//     passkey wiring ブロックの外側で常時生成する（NFR 2.2）。
	//   - passkey handler 用途と退会 cleanup 用途で同一インスタンスを共有する。
	//   - passkey handler が nil（env 未設定）でも newTxUserService に非 nil の
	//     passkeyCredentialRepo を渡すことで、退会時の cleanup 段が有効になる。
	passkeyCredentialRepo := repository.NewPostgresPasskeyCredentialRepo(db)
	feedRepo := repository.NewPostgresFeedRepo(db)
	subRepo := repository.NewPostgresSubscriptionRepo(db)
	itemRepo := repository.NewPostgresItemRepo(db)
	itemStateRepo := repository.NewPostgresItemStateRepo(db)
	userCrossFeedViewRepo := repository.NewPostgresUserCrossFeedViewRepo(db)

	// 3. セキュリティサービスの初期化
	ssrfGuard := security.NewSSRFGuard()
	sanitizer := security.NewContentSanitizer()

	// 4. ドメインサービスの初期化
	oauthProvider := auth.NewGoogleOAuthProvider(auth.GoogleOAuthConfig{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
	})
	authService := auth.NewService(
		oauthProvider, userRepo, identRepo, sessionRepo, authCodeRepo,
		auth.ServiceConfig{SessionMaxAge: cfg.SessionMaxAge},
	)

	feedDetector := feed.NewFeedDetector(ssrfGuard)
	faviconFetcher := feed.NewFaviconFetcher(ssrfGuard)
	feedService := feed.NewFeedService(feedRepo, subRepo, feedDetector, faviconFetcher)

	// itemService / itemStateService は subRepo を SubscriptionChecker として注入し、
	// 記事詳細取得・状態更新時に購読外フィードへの越境アクセスを拒否する（#175）。
	// itemService には追加で feedRepo を FeedMetaProvider として注入し、記事詳細応答に
	// 所属フィードの表示メタデータ（タイトル / favicon）を付与する（Issue #207 / Req 3.1, 3.2）。
	itemService := item.NewItemService(itemRepo, itemStateRepo, subRepo, feedRepo)
	itemStateService := item.NewItemStateService(itemRepo, itemStateRepo, subRepo)

	// 横断新着一覧サービス（Issue #121）。itemRepo の ListNewAcrossFeeds と
	// userCrossFeedViewRepo の Get / Upsert を利用する。
	crossFeedService := crossfeed.NewService(itemRepo, userCrossFeedViewRepo)

	// serve 専用の Prometheus registry と Collector を生成する。
	// Collector は手動フェッチ系のカウンタ（feedman_manual_fetch_total）も保持しており、
	// subscription.Service.ManualFetch から記録される（Issue #115 Req 8.x）。
	// /metrics エンドポイントは信頼 CIDR 制限付きで公開される（Requirement 1.1, 5.1）。
	serveRegistry := prometheus.NewRegistry()
	serveCollector := metrics.NewCollector(serveRegistry)

	// 手動フェッチ用の Fetcher を組み立てる（Issue #115 task 6.1）。
	// worker 側 (runWorker) と同じ依存配線パターンで、SSRFGuard / Sanitizer / UpsertSvc /
	// Fetcher を構築する。タイムアウト・最大サイズも自動経路と同一の cfg 値を使う（NFR 1.1）。
	upsertSvc := item.NewItemUpsertService(itemRepo, sanitizer, item.WithMetrics(serveCollector))
	fetcher := fetchpkg.NewFetcher(
		feedRepo, subRepo, upsertSvc, ssrfGuard,
		slog.Default(), cfg.FetchTimeout, cfg.FetchMaxSize,
		fetchpkg.WithMetrics(serveCollector),
	)

	// 記事検索ドメインサービス。itemRepo を ItemSearchRepository として、subRepo を
	// SubscriptionRepository（feed_id 指定時の購読確認用）として注入する。
	itemSearchService := itemsearch.NewSearchService(itemRepo, subRepo)

	// 退会処理と手動フェッチで共有する DB トランザクション基盤。
	// 退会処理は単一トランザクションで原子化する（途中失敗時は全ロールバック）。
	txBeginner := repository.NewSQLTxBeginner(db)
	// 手動フェッチ用 tx beginner は repository.TxBeginner とは別 interface（Commit / Rollback を
	// 含めた tx ハンドルライフサイクル抽象化）を必要とするため別途構築する（Issue #115）。
	manualFetchTxBeginner := subscription.NewSQLManualFetchTxBeginner(db)
	subService := subscription.NewService(
		subRepo, itemStateRepo, feedRepo,
		fetcher, manualFetchTxBeginner, serveCollector,
	)
	// Issue #170: 退会トランザクションへ native auth 認証状態（auth_codes /
	// refresh_token_families）の明示削除を統合するため、authCodeRepo /
	// refreshTokenRepo を newTxUserService に渡す。これらの repo は #165 / #166 で
	// 上記の native auth 配線にも共用される。
	// Issue #216: passkey_credentials の cleanup も同 tx に統合するため
	// passkeyCredentialRepo を末尾に追加で渡す（Req 7.1〜7.4）。当該 repo は
	// passkey handler wiring と共用され、env 未設定でも常時作成されるため env
	// 未設定環境でも退会時の cleanup 段が動作する（NFR 2.2）。
	userService := newTxUserService(
		txBeginner, userRepo, sessionRepo, subRepo, itemStateRepo,
		authCodeRepo, refreshTokenRepo, passkeyCredentialRepo,
	)

	// 5. ハンドラーアダプタの構築
	subServiceAdapter := handler.NewSubscriptionServiceAdapter(subService)
	userServiceAdapter := handler.NewUserServiceAdapter(userService)
	itemServiceAdapter := handler.NewItemServiceAdapter(itemService)
	itemStateServiceAdapter := handler.NewItemStateServiceAdapter(itemStateService)
	itemSearchServiceAdapter := handler.NewItemSearchServiceAdapter(itemSearchService)
	crossFeedServiceAdapter := handler.NewCrossFeedServiceAdapter(crossFeedService)

	// 6. SubscriptionDeleterアダプタの構築
	subDeleterAdapter := handler.NewSubscriptionDeleterAdapter(subRepo, itemStateRepo)

	// 7. ルーターの構築
	rateLimiterCfg := middleware.DefaultRateLimiterConfig()
	// デフォルト値から変更する場合のみ上書き（req/min -> req/sec に変換）
	// configのRateLimitGeneralはreq/min単位なのでreq/secに変換する

	// RateLimiter はバックグラウンドでクリーンアップ goroutine を起動するため、
	// シャットダウン時に Stop() を呼べるよう変数参照を保持する（goroutine リーク防止）。
	rateLimiter := middleware.NewRateLimiter(rateLimiterCfg)

	// 未認証エンドポイント（/auth/google/login・/auth/google/callback・/health）向けの
	// IP 単位レート制限。閾値は cfg.RateLimitUnauthIP（既定 30 req/min/IP、不正値は config 側で
	// 既定フォールバック済み）から構築する。これもクリーンアップ goroutine を持つため
	// シャットダウン時に Stop() を呼べるよう参照を保持する（goroutine リーク防止）。
	unauthIPRateLimiter := middleware.NewIPRateLimiter(
		middleware.DefaultIPRateLimiterConfig(cfg.RateLimitUnauthIP),
	)

	// Native Auth トークン交換（Issue #166）と Bearer 認証の検証器（Issue #169）:
	// NATIVE_AUTH_JWT_SECRET が設定されているときのみ issuer / service / handler /
	// verifier を組み立てて RouterDeps に注入する。未設定なら nil のまま、router 側で
	// POST /api/auth/token は登録されず 404（fail-closed / Req 3.2）、Bearer 認証は
	// 無効化され認証必須 API は従来どおり Cookie セッション認証のみで動作する
	// （#169 Req 4.2 / 4.4。起動は成功する）。起動時に運用者向け Warn を 1 回記録する。
	//
	// jwtVerifier は interface 型のため secret 設定時のみ非 nil の具象
	// （*auth.JWTVerifier）を代入する（typed-nil を作らない）。
	// Web パスキーログインと直接登録 session は、同一の SessionFactory を共有する。
	// registration は NATIVE_AUTH_JWT_SECRET に依存しないため、本 factory は native auth
	// の条件分岐より外で一度だけ構築する（Issue #231 Delta 1）。
	sessionTTL := time.Duration(cfg.SessionMaxAge) * time.Second
	sessionFactory := auth.NewSessionFactory(sessionTTL)
	var nativeAuthHandler *handler.NativeAuthHandler
	var jwtVerifier middleware.JWTVerifier
	if cfg.NativeAuthJWTSecret != "" {
		jwtIssuer := auth.NewJWTIssuer([]byte(cfg.NativeAuthJWTSecret), cfg.NativeAuthJWTKid)
		nativeTokenService := auth.NewTokenService(authCodeRepo, refreshTokenRepo, jwtIssuer)
		// Web パスキー導線用 Session 交換サービス（Issue #223 / task 2）: 既存 authCodeRepo /
		// sessionRepo を再利用する（interface segregation の SessionCreator は SessionRepo が
		// 構造的に充足する）。session の ID / 時刻 / TTL は、直接登録経路と同じ
		// SessionFactory instance に委譲する（Issue #231 Delta 1）。
		sessionExchangeService := auth.NewSessionExchangeService(authCodeRepo, sessionRepo, sessionFactory)
		// Cookie 属性は既存 Google OAuth Callback（handler.AuthHandlerConfig）と厳密一致させる
		// ため、CookieDomain / CookieSecure / SessionMaxAge を同じ cfg 値から注入する。
		nativeAuthHandler = handler.NewNativeAuthHandler(
			nativeTokenService,
			handler.WithSessionExchange(
				sessionExchangeService,
				cfg.CookieDomain,
				cfg.CookieSecure,
				cfg.SessionMaxAge,
			),
			// Web パスキー専用の strict exact Origin。未設定・不正値は空になり、
			// /api/auth/session が全リクエストを 403 に倒す（Issue #231 Delta 4/6）。
			handler.WithSessionAllowedOrigin(cfg.WebPasskeyAllowedOrigin),
		)
		// 検証は発行と同一の env 値（署名鍵）を共用する（#169 Req 4.1）。
		jwtVerifier = auth.NewJWTVerifier([]byte(cfg.NativeAuthJWTSecret))
		slog.Info("native token exchange enabled",
			slog.String("kid", cfg.NativeAuthJWTKid),
		)
	} else {
		// NATIVE_AUTH_JWT_SECRET 未設定時は NativeAuthHandler が nil となり、/api/auth/token /
		// refresh / revoke / session と Bearer 認証がすべて無効化される（fail-closed）。
		// /api/auth/session が無効になると Web パスキーフローの session 合流が成立しないため、
		// GET /api/passkey/capability も 404 に倒れ、Web はパスキー導線を非表示にして
		// Google 単体構成へ縮退する（Issue #223 review #3: capability と session の有効化条件を統一）。
		slog.Warn("NATIVE_AUTH_JWT_SECRET is not set; POST /api/auth/token, /api/auth/session, /api/passkey/capability, and Bearer auth are disabled (Web passkey degrades to Google-only)")
	}

	// Passkey / AASA wiring（Issue #216）: fail-closed 縮退。
	//   - WEBAUTHN_RP_ID と WEBAUTHN_ORIGINS の両方が設定されている場合のみ passkey handler を組む
	//     （どちらか欠けたら Warn を 1 回出し passkeyHandler は nil のまま / NFR 2.2）。
	//   - AASA は WEBAUTHN_IOS_APP_ID が設定されている場合のみ組む
	//     （passkey handler の nil 判定とは独立 / Req 5.1〜5.4）。
	//   - env が未設定なら本機能は完全に無効化され、既存挙動と等価（Req 8.1〜8.5 / NFR 2.1 / NFR 2.2）。
	//
	// NewGoWebAuthnAdapter は空 origins 等の library 側検証で error を返し得るため、初期化失敗時は
	// serve 起動を中断して運用者に構成不備を早期通知する（fail-closed の一環）。
	//
	// PasskeyCredentialRepo は退会 cleanup（Req 7.1, 7.2）でも必要となるため、passkey handler 用途と
	// 退会 cleanup 用途で **同一インスタンスを共有** する形で本ブロック外（上記 repository 初期化
	// セクション）で常時生成している（Issue #216 task 8）。本ブロックでは共有インスタンスをそのまま
	// 参照する（passkey handler 未生成でも退会 cleanup 側に注入されることで env 未設定時も cleanup が
	// 動作する / NFR 2.2）。
	var passkeyHandler *handler.PasskeyHandler
	var aasaHandler *handler.AASAHandler
	if cfg.WebAuthnRPID != "" && len(cfg.WebAuthnOrigins) > 0 {
		passkeyChallengeRepo := repository.NewPostgresPasskeyChallengeRepo(db)
		webAuthnAdapter, err := passkey.NewGoWebAuthnAdapter(
			cfg.WebAuthnRPID, cfg.WebAuthnRPDisplayName, cfg.WebAuthnOrigins,
		)
		if err != nil {
			// NFR 1.2: RPID / origins 生値をメッセージに含めない（wrap のみ）。
			return fmt.Errorf("failed to construct WebAuthn adapter: %w", err)
		}
		challengeStore := passkey.NewChallengeStore(passkeyChallengeRepo, cfg.PasskeyChallengeTTL)
		// now=nil で time.Now を既定採用（Task 5 impl-notes 参照）。
		// Issue #230 / Req 1.1〜1.6: 新規パスキー登録 finish の users / passkey_credentials
		// を 1 tx で INSERT するため、退会 tx と同じ *repository.SQLTxBeginner を
		// passkeyRegistrationTxBeginnerAdapter 経由で注入する（DB 接続は共有）。
		passkeyRegTxBeginner := newPasskeyRegistrationTxBeginner(txBeginner)
		registrationSvc := passkey.NewRegistrationService(
			webAuthnAdapter, challengeStore, userRepo, passkeyCredentialRepo,
			passkeyRegTxBeginner, sessionRepo, sessionFactory, nil,
		)
		authenticationSvc := passkey.NewAuthenticationService(
			webAuthnAdapter, challengeStore, passkeyCredentialRepo, userRepo, authCodeRepo, nil,
		)
		passkeyHandler = handler.NewPasskeyHandler(
			registrationSvc,
			authenticationSvc,
			handler.WithWebRegistrationSession(
				cfg.WebPasskeyAllowedOrigin,
				cfg.CookieDomain,
				cfg.CookieSecure,
				cfg.SessionMaxAge,
			),
		)
		slog.Info("passkey handlers enabled",
			slog.String("rp_id", cfg.WebAuthnRPID),
			slog.Int("origins", len(cfg.WebAuthnOrigins)),
			slog.Duration("challenge_ttl", cfg.PasskeyChallengeTTL),
		)
		// Web パスキーフロー（GET /api/passkey/capability + POST /api/auth/session）は
		// session 合流に NativeAuthHandler を必要とする。passkey は有効でも
		// NATIVE_AUTH_JWT_SECRET 未設定なら capability / session が 404 となり、Web は
		// パスキー導線を出さずに Google 単体構成へ縮退する（iOS 側 passkey API は有効なまま）。
		// 運用者が Web パスキーを意図している場合の取りこぼしを防ぐため Warn を 1 回出す
		// （Issue #223 review #3 / #4）。
		if cfg.NativeAuthJWTSecret == "" {
			slog.Warn("passkey is enabled but NATIVE_AUTH_JWT_SECRET is not set; Web passkey (GET /api/passkey/capability, POST /api/auth/session) stays disabled and the login screen shows Google only")
		}
	} else {
		slog.Warn("passkey is disabled (WEBAUTHN_RP_ID or WEBAUTHN_ORIGINS not set)")
	}

	if cfg.WebAuthnIOSAppID != "" {
		// NewAASAHandler は空文字なら nil を返す契約だが、fail-closed の意図を明示するため
		// wiring 側でも空判定を先に行う（既存 NativeAuthHandler と同じ縮退パターン）。
		aasaHandler = handler.NewAASAHandler(cfg.WebAuthnIOSAppID)
	} else {
		slog.Warn("AASA is disabled (WEBAUTHN_IOS_APP_ID not set)")
	}

	deps := &handler.RouterDeps{
		HealthChecker:       db,
		SessionFinder:       sessionRepo,
		CORSAllowedOrigin:   cfg.CORSAllowedOrigin,
		RateLimiter:         rateLimiter,
		UnauthIPRateLimiter: unauthIPRateLimiter,
		HSTSEnabled:         cfg.HSTSEnabled,
		Logger:              slog.Default(),

		MetricsHandler:    metrics.SetupMetricsRoute(serveRegistry),
		MetricsMiddleware: middleware.NewTrustedCIDRMiddleware(cfg.TrustedCIDRs),

		AuthService: authService,
		AuthConfig: handler.AuthHandlerConfig{
			BaseURL:       cfg.BaseURL,
			CookieDomain:  cfg.CookieDomain,
			CookieSecure:  cfg.CookieSecure,
			SessionMaxAge: cfg.SessionMaxAge,
		},

		// Native Auth (Issue #166 / #169)
		NativeAuthHandler: nativeAuthHandler,
		JWTVerifier:       jwtVerifier,

		// Passkey / AASA (Issue #216)
		// いずれも env 未設定時は nil のまま。router 側で個別に fail-closed 判定される
		// （PasskeyHandler nil → /api/passkey/* 404 / AASAHandler nil →
		// /.well-known/apple-app-site-association 404 / NFR 2.2）。
		PasskeyHandler:          passkeyHandler,
		WebPasskeyAllowedOrigin: cfg.WebPasskeyAllowedOrigin,
		AASAHandler:             aasaHandler,

		FeedService:         feedService,
		SubscriptionDeleter: subDeleterAdapter,

		ItemService:      itemServiceAdapter,
		ItemStateService: itemStateServiceAdapter,

		ItemSearchService: itemSearchServiceAdapter,

		SubscriptionService: subServiceAdapter,
		UserService:         userServiceAdapter,

		CrossFeedService: crossFeedServiceAdapter,
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

	// グレースフルシャットダウン: 稼働中リクエストの drain 完了後に
	// RateLimiter のクリーンアップ goroutine を停止する（高々 1 回）。
	coordinator := newShutdownCoordinator(server, rateLimiter, unauthIPRateLimiter)
	if err := coordinator.shutdown(ctx); err != nil {
		return err
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

	// 4. worker 専用の registry と Collector を生成し、各レイヤへ注入する。
	// フェッチ／UPSERT は worker プロセスで実行されるため、フェッチ系メトリクスは
	// この registry に蓄積され、後述の metrics listener 経由でスクレイプ可能になる（Requirement 3.1）。
	workerRegistry := prometheus.NewRegistry()
	collector := metrics.NewCollector(workerRegistry)

	// 5. フェッチャーの初期化（WithMetrics で Collector を注入）
	upsertSvc := item.NewItemUpsertService(itemRepo, sanitizer, item.WithMetrics(collector))
	fetcher := fetchpkg.NewFetcher(
		feedRepo, subRepo, upsertSvc, ssrfGuard,
		slog.Default(), cfg.FetchTimeout, cfg.FetchMaxSize,
		fetchpkg.WithMetrics(collector),
	)

	// 6. スケジューラの起動
	scheduler := fetchpkg.NewScheduler(
		feedRepo, fetcher, slog.Default(), cfg.FetchMaxConcurrent,
	)

	// 7. クリーンアップジョブの初期化
	cleanupJob := cleanup.NewCleanupJob(db, slog.Default())

	// 8. はてなブックマークバッチジョブの初期化
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

	// worker 専用の metrics listener を起動する（信頼 CIDR 制限付き）。
	// worker は HTTP ルーターを持たないため独立 listener で /metrics を公開し、
	// ctx キャンセルで graceful stop する（Requirement 1.2, 3.1, 3.2, 3.3）。
	startWorkerMetricsListener(ctx, ":"+cfg.MetricsPort, workerRegistry, cfg.TrustedCIDRs)

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
