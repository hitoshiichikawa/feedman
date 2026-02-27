package handler

import (
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

// RouterDeps はNewRouterに必要な依存関係をまとめた構造体。
type RouterDeps struct {
	// ミドルウェア依存
	SessionFinder     middleware.SessionFinder
	CORSAllowedOrigin string
	RateLimiter       *middleware.RateLimiter

	// 認証
	AuthService AuthServiceInterface
	AuthConfig  AuthHandlerConfig

	// フィード
	FeedService         FeedServiceInterface
	SubscriptionDeleter SubscriptionDeleter

	// 記事
	ItemService      ItemServiceInterface
	ItemStateService ItemStateServiceInterface

	// 購読
	SubscriptionService SubscriptionServiceInterface

	// ユーザー
	UserService UserServiceInterface
}

// NewRouter は全APIエンドポイントのルーティングとミドルウェアチェーンを構成したchi.Routerを返す。
//
// ミドルウェアスタックの実行順序:
//
//	CORSMiddleware → SessionMiddleware → RateLimitMiddleware(GeneralMiddleware)
//
// 認証ルート（/auth/*）はミドルウェアチェーンの外に配置する。
func NewRouter(deps *RouterDeps) http.Handler {
	r := chi.NewRouter()

	// CORS ミドルウェアを最上位に適用（全ルートに効く）
	r.Use(middleware.NewCORSMiddleware(deps.CORSAllowedOrigin))

	authHandler := NewAuthHandler(deps.AuthService, deps.AuthConfig)
	feedHandler := NewFeedHandler(deps.FeedService, deps.SubscriptionDeleter)
	itemHandler := NewItemHandler(deps.ItemService, deps.ItemStateService)
	subHandler := NewSubscriptionHandler(deps.SubscriptionService)
	userHandler := NewUserHandler(deps.UserService)

	// --- 認証不要のルート ---

	// 認証ルート（OAuthフロー）
	r.Route("/auth", func(r chi.Router) {
		r.Get("/google/login", authHandler.Login)
		r.Get("/google/callback", authHandler.Callback)
		r.Post("/logout", authHandler.Logout)
		r.Get("/me", authHandler.Me)
	})

	// --- 認証が必要なルート ---
	// ミドルウェアスタック: Session → RateLimit(General)
	r.Group(func(r chi.Router) {
		r.Use(middleware.NewSessionMiddleware(deps.SessionFinder))
		r.Use(deps.RateLimiter.GeneralMiddleware())

		// フィード管理
		r.Route("/api/feeds", func(r chi.Router) {
			// POST /api/feeds - フィード登録（登録専用レート制限を追加）
			r.With(deps.RateLimiter.FeedRegistrationMiddleware()).Post("/", feedHandler.RegisterFeed)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", feedHandler.GetFeed)
				r.Patch("/", feedHandler.UpdateFeedURL)
				r.Delete("/", feedHandler.DeleteFeed)

				// GET /api/feeds/{id}/items - フィードごとの記事一覧
				r.Get("/items", itemHandler.ListItems)
			})
		})

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
			})
		})

		// ユーザー管理
		r.Route("/api/users", func(r chi.Router) {
			r.Delete("/me", userHandler.Withdraw)
		})
	})

	return r
}
