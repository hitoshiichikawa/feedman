package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/middleware"
)

// SetupAuthRoutes は認証関連のルーティングを設定したchi.Routerを返す。
func SetupAuthRoutes(service AuthServiceInterface, config AuthHandlerConfig) http.Handler {
	r := chi.NewRouter()
	h := NewAuthHandler(service, config)

	r.Route("/auth", func(r chi.Router) {
		// OAuthフロー
		r.Get("/google/login", h.Login)
		r.Get("/google/callback", h.Callback)

		// セッション管理
		r.Post("/logout", h.Logout)
		r.Get("/me", h.Me)
	})

	return r
}

// HealthChecker はヘルスチェック用のDB疎通確認インターフェース。
type HealthChecker interface {
	PingContext(ctx context.Context) error
}

// RouterDeps はNewRouterに必要な依存関係をまとめた構造体。
type RouterDeps struct {
	// ヘルスチェック
	HealthChecker HealthChecker

	// ミドルウェア依存
	SessionFinder     middleware.SessionFinder
	CORSAllowedOrigin string
	RateLimiter       *middleware.RateLimiter

	// UnauthIPRateLimiter は未認証エンドポイント（/auth/google/login・
	// /auth/google/callback・/health）に適用する IP 単位レート制限。
	// nil の場合は IP レート制限を適用せず、既存ルーティングを完全に不変に保つ（後方互換）。
	UnauthIPRateLimiter *middleware.IPRateLimiter

	// HSTSEnabled は HSTS（Strict-Transport-Security）ヘッダーの出力可否。
	// false（既定）の場合は HTTPS 配信でも HSTS を付与しない。
	HSTSEnabled bool

	// アクセスログ出力に使用する構造化ロガー。
	// nil の場合は slog.Default() にフォールバックする（後方互換）。
	Logger *slog.Logger

	// メトリクス（任意）
	// MetricsHandler が非 nil のときのみ認証不要グループに /metrics を登録する。
	// nil の場合は登録せず、既存ルーティングを完全に不変に保つ（後方互換）。
	MetricsHandler http.Handler
	// MetricsMiddleware は /metrics の前段に重ねるミドルウェア（信頼 CIDR 制限など）。
	// MetricsHandler が非 nil でも MetricsMiddleware が nil の場合は、無防備な公開を
	// 避けるため /metrics を登録しない（fail-closed）。MetricsHandler が nil のときは参照しない。
	MetricsMiddleware func(http.Handler) http.Handler

	// 認証
	AuthService AuthServiceInterface
	AuthConfig  AuthHandlerConfig

	// Native Auth トークン交換（Issue #166）
	// NativeAuthHandler が非 nil のときのみ認証不要グループに POST /api/auth/token を
	// 登録する。nil の場合は登録せず 404 で応答する（NATIVE_AUTH_JWT_SECRET 未設定環境の
	// fail-closed パターン。/metrics と同じ後方互換指針）。
	NativeAuthHandler *NativeAuthHandler

	// JWTVerifier は Bearer access token の検証器（任意 / Issue #169）。
	// nil の場合、認証必須グループは従来どおり Cookie セッション認証のみで動作する
	// （NATIVE_AUTH_JWT_SECRET 未設定環境の後方互換。NewBearerOrSessionMiddleware が
	// nil 縮退で既存 SessionMiddleware をそのまま返す）。
	JWTVerifier middleware.JWTVerifier

	// フィード
	FeedService         FeedServiceInterface
	SubscriptionDeleter SubscriptionDeleter

	// 記事
	ItemService      ItemServiceInterface
	ItemStateService ItemStateServiceInterface

	// 記事検索
	ItemSearchService ItemSearchServiceInterface

	// 購読
	SubscriptionService SubscriptionServiceInterface

	// ユーザー
	UserService UserServiceInterface

	// 横断新着一覧（Issue #121）
	CrossFeedService CrossFeedServiceInterface
}

// NewRouter は全APIエンドポイントのルーティングとミドルウェアチェーンを構成したchi.Routerを返す。
//
// ミドルウェアスタックの実行順序:
//   - 全ルート共通（最上位）: Recovery → SecurityHeaders → CORS
//   - 認証不要ルート（/health, /auth/*）: 上記共通 → Logging
//   - うち /health・/auth/google/login・/auth/google/callback の 3 ルートのみ
//     Logging の内側に IP 単位レート制限（UnauthIPRateLimiter）を重ねる。
//     /auth/logout・/auth/me には適用しない（セッションを持つため）。
//   - 認証必須ルート（/api/*）: 上記共通 → Session → RateLimit(General) → Logging
//
// Logging を Session の内側（後ろ）に置くことで、認証済みリクエストの user_id を
// アクセスログに含められる。/health・/auth/* は Session を通らないため user_id は付与されない。
// いずれのリクエストもアクセスログは 1 件のみ出力される（二重ログにならない）。
//
// deps.Logger が nil の場合は slog.Default() を使用する（後方互換）。
func NewRouter(deps *RouterDeps) http.Handler {
	r := chi.NewRouter()

	// panic recovery を最上位に適用
	r.Use(middleware.NewRecoveryMiddleware())

	// セキュリティヘッダーを適用（全ルートに効く）
	r.Use(middleware.NewSecurityHeadersMiddleware(deps.HSTSEnabled))

	// CORS ミドルウェアを適用（全ルートに効く）
	r.Use(middleware.NewCORSMiddleware(deps.CORSAllowedOrigin))

	// アクセスログ用ロガー。未指定時はアプリ標準ロガー（slog.Default）にフォールバック。
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logging := middleware.NewLoggingMiddleware(logger)

	authHandler := NewAuthHandler(deps.AuthService, deps.AuthConfig)
	feedHandler := NewFeedHandler(deps.FeedService, deps.SubscriptionDeleter)
	itemHandler := NewItemHandler(deps.ItemService, deps.ItemStateService)
	itemSearchHandler := NewItemSearchHandler(deps.ItemSearchService)
	subHandler := NewSubscriptionHandler(deps.SubscriptionService)
	userHandler := NewUserHandler(deps.UserService)
	// CrossFeedService が nil の場合は CrossFeedHandler を生成しない（後方互換のため、
	// 既存テスト・既存ルート構成への影響を回避）。Issue #121 の本実装では app.go の
	// runServe が必ず CrossFeedService を配線するため production 経路では常に非 nil。
	var crossFeedHandler *CrossFeedHandler
	if deps.CrossFeedService != nil {
		crossFeedHandler = NewCrossFeedHandler(deps.CrossFeedService)
	}

	// 未認証エンドポイント向け IP 単位レート制限ミドルウェア。
	// UnauthIPRateLimiter が nil の場合は素通し（制限なし）として扱い、既存ルーティングを
	// 完全に不変に保つ（後方互換）。login・callback・health の 3 ルートにのみ適用し、
	// セッションを持つ logout・me には適用しない（Requirement 1, 4）。
	unauthIPMW := func(next http.Handler) http.Handler { return next }
	if deps.UnauthIPRateLimiter != nil {
		unauthIPMW = deps.UnauthIPRateLimiter.Middleware()
	}

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		httpStatus := http.StatusOK

		if deps.HealthChecker != nil {
			if err := deps.HealthChecker.PingContext(r.Context()); err != nil {
				status = "unhealthy"
				httpStatus = http.StatusServiceUnavailable
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	}

	// --- 認証不要のルート ---
	// アクセスログのみ適用（Session を通らないため user_id は付与されない）。
	r.Group(func(r chi.Router) {
		r.Use(logging)

		// ヘルスチェック（IP 単位レート制限を適用）
		r.With(unauthIPMW).Get("/health", healthHandler)

		// 認証ルート（OAuthフロー）
		r.Route("/auth", func(r chi.Router) {
			// OAuth フローの入口は IP 単位レート制限を適用する（OAuth フラッディング対策）。
			r.With(unauthIPMW).Get("/google/login", authHandler.Login)
			r.With(unauthIPMW).Get("/google/callback", authHandler.Callback)
			// logout・me はセッションを持つ実質認証エンドポイントのため IP 制限の対象外。
			r.Post("/logout", authHandler.Logout)
			r.Get("/me", authHandler.Me)
		})

		// メトリクス公開エンドポイント（任意）。
		// MetricsHandler が非 nil かつ MetricsMiddleware も非 nil のときのみ登録し、前段に
		// MetricsMiddleware（信頼 CIDR 制限）を重ねる。MetricsMiddleware が nil の場合は
		// 無防備な公開を避けるため登録しない（fail-closed）。MetricsHandler が nil の場合も
		// 登録せず既存ルーティングを完全に不変に保つ（後方互換）。
		if deps.MetricsHandler != nil {
			if deps.MetricsMiddleware != nil {
				r.With(deps.MetricsMiddleware).Handle("/metrics", deps.MetricsHandler)
			} else {
				logger.Warn("MetricsHandler is set but MetricsMiddleware is nil; /metrics is not registered to avoid unprotected exposure")
			}
		}

		// Native Auth トークン交換エンドポイント（Issue #166）・refresh ローテーション
		// エンドポイント（Issue #167）・revoke エンドポイント（Issue #168）。
		// NATIVE_AUTH_JWT_SECRET 未設定のデプロイは NativeAuthHandler が nil となり、全ルートを
		// 登録しない（404 / fail-closed / NFR 2.2）。Session / Bearer middleware は通らない
		// （Req 1.5 / #168 Req 2.4: revoke は token 所持自体を失効権限とみなす）。
		// 巨大ボディ DoS 防止のためボディ上限ミドルウェア（DefaultMaxBodyBytes）を重ねる。
		// 未認証で公開される token 系入口のため、既存未認証 3 ルートと同じ IP 単位
		// レート制限（unauthIPMW）を route チェーン最外に重ねる（Issue #171 / SERVER.md §1.7。
		// 閾値超過時はボディ上限 wrap・JSON decode・永続化層参照に到達せず 429 で遮断）。
		if deps.NativeAuthHandler != nil {
			r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
				Post("/api/auth/token", deps.NativeAuthHandler.Token)
			r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
				Post("/api/auth/refresh", deps.NativeAuthHandler.Refresh)
			r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
				Post("/api/auth/revoke", deps.NativeAuthHandler.Revoke)
		}
	})

	// --- 認証が必要なルート ---
	// ミドルウェアスタック: BearerOrSession → RateLimit(General) → Logging
	// Logging を認証 middleware の後ろに置くことで user_id をログに含める。
	// BearerOrSession（Issue #169）は Authorization: Bearer があれば JWT 検証で認証し、
	// 無ければ既存 SessionMiddleware に委譲する。deps.JWTVerifier が nil
	// （NATIVE_AUTH_JWT_SECRET 未設定）の場合は SessionMiddleware そのものが返るため、
	// 構成は本機能導入前と完全に同一（NFR 2.2）。RateLimit / MaxBodyBytes / Logging の
	// 順序・位置・適用範囲は不変（NFR 2.1）。
	r.Group(func(r chi.Router) {
		r.Use(middleware.NewBearerOrSessionMiddleware(deps.JWTVerifier, deps.SessionFinder))
		r.Use(deps.RateLimiter.GeneralMiddleware())
		// リクエストボディ上限を適用し、巨大ボディによるメモリ枯渇 DoS を防ぐ。
		// 上限超過時は各ハンドラの json.Decode がエラーを返し 400 応答となる。
		r.Use(middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes))
		r.Use(logging)

		// フィード管理
		r.Route("/api/feeds", func(r chi.Router) {
			// POST /api/feeds - フィード登録（登録専用レート制限を追加）
			r.With(deps.RateLimiter.FeedRegistrationMiddleware()).Post("/", feedHandler.RegisterFeed)

			// GET /api/feeds/starred/items - 全フィード横断スター記事一覧（Issue #117）
			// chi v5 のトライ木は静的セグメント `starred` を動的パラメータ `{id}` より優先するため、
			// 登録順を問わず `/api/feeds/{id}/items` と衝突しない。可読性のため `/{id}` ブロックの
			// 直前に置く。
			r.Get("/starred/items", itemHandler.ListStarredItems)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", feedHandler.GetFeed)
				r.Patch("/", feedHandler.UpdateFeedURL)
				r.Delete("/", feedHandler.DeleteFeed)

				// GET /api/feeds/{id}/items - フィードごとの記事一覧
				r.Get("/items", itemHandler.ListItems)
			})
		})

		// 記事検索（/api/items/{id} よりも前に登録する必要がある。
		// chi は static segment `/search` を `{id}` よりも優先するが、明示的に
		// 先に登録することで `search` が `{id}` の捕捉に吸われる可能性を確実に排除する）。
		r.Get("/api/items/search", itemSearchHandler.Search)

		// 横断新着一覧（Issue #121 / Req 1.2, 2.1, 4.3, 4.7）。
		// /api/items/{id} よりも前に登録し、`cross-feed` セグメントが `{id}` の動的
		// パラメータに吸われないようにする（既存 starred 同様の保護）。
		// CrossFeedService が未配線の deps では登録しない（後方互換）。
		if crossFeedHandler != nil {
			r.Get("/api/items/cross-feed", crossFeedHandler.ListItems)
		}

		// 記事管理
		r.Route("/api/items/{id}", func(r chi.Router) {
			r.Get("/", itemHandler.GetItem)
			r.Put("/state", itemHandler.UpdateItemState)
		})

		// 購読管理
		r.Route("/api/subscriptions", func(r chi.Router) {
			r.Get("/", subHandler.ListSubscriptions)

			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", subHandler.Unsubscribe)
				r.Put("/settings", subHandler.UpdateSettings)
				r.Post("/resume", subHandler.ResumeFetch)
				// Issue #115: 手動フェッチ API（同期）。
				// 認証ミドルウェア + General レート制限はグループ単位で適用済み（NFR 2.1, 2.2）。
				r.Post("/fetch", subHandler.ManualFetch)
			})
		})

		// ユーザー管理
		r.Route("/api/users", func(r chi.Router) {
			r.Delete("/me", userHandler.Withdraw)
			// PUT /api/users/me/cross-feed-last-seen - 横断一覧の最終閲覧時刻更新（Issue #121）
			// CrossFeedService が未配線の deps では登録しない（後方互換）。
			if crossFeedHandler != nil {
				r.Put("/me/cross-feed-last-seen", crossFeedHandler.TouchLastSeen)
			}
		})
	})

	return r
}
