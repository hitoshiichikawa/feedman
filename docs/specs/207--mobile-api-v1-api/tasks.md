# Implementation Plan

- [x] 1. Mobile API Contract Document を新規作成する (P)
  - `docs/specs/207--mobile-api-v1-api/mobile-api-contract.md` を新規作成
  - 構成は design.md「Mobile API Contract Document の構成」節に従う:
    概要 / 共通方針（認証ヘッダ・エラー応答形式・JSON 命名規約）/ Native Auth サマリ /
    モバイル統一ユーザー情報取得 / v1 共通動線エンドポイント（記事一覧・記事詳細・検索・
    購読操作・クロスフィード）/ v1 スコープ外（次フェーズ）/ 後方互換ポリシー
  - 各エンドポイントについて URL / 認証方式 / 主要クエリ・パスパラメータ / 成功応答 JSON
    フィールド / エラー応答を記載し、サーバ実装ソースを参照せずに実装可能な粒度にする
  - native auth 系（`POST /api/auth/token` / `/refresh` / `/revoke` / `feedman://auth/callback`）は
    既存 spec（#163 / #165〜#171 / #172）へのリンクを含めたサマリ記載に留める
  - v1 スコープ外として `/api/devices` / `/api/keywords` を明示し、次フェーズ予定として記録
  - 記事詳細の応答フィールドには本 spec で追加する `feed_title` / `feed_favicon_url` を
    必ず含める（Task 7 と整合）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.3, 2.4, 3.1, 3.2, 3.4, NFR 2.1, NFR 2.2_
  - _Boundary: mobile-api-contract.md_

- [ ] 2. README の API エンドポイント一覧を更新する (P)
  - `README.md` の `## API エンドポイント` 配下の表を以下の通り更新:
    - 「ユーザー管理（認証必須）」に `GET /api/users/me`（モバイル / Web 共通の current
      user 取得）を新設項として追加
    - Native Auth 系（`POST /api/auth/token` / `POST /api/auth/refresh` /
      `POST /api/auth/revoke`）を追記（認証不要グループとして区別）
    - 既存表に未掲載の項目を追加: `GET /api/feeds/starred/items` / `GET /api/items/search` /
      `GET /api/items/cross-feed` / `POST /api/subscriptions/{id}/fetch` /
      `PUT /api/users/me/cross-feed-last-seen`
    - 表の直後に Mobile API Contract Document
      （`docs/specs/207--mobile-api-v1-api/mobile-api-contract.md`）への参照リンクを追加
  - 既存表の他項目（フィード管理 / 記事管理 / 監視 / ミドルウェアスタック / データベース
    スキーマ等）の記述は変更しない
  - _Requirements: 1.6_
  - _Boundary: README.md_

- [ ] 3. user.Service に GetByID を追加し単体テストを通す
  - `internal/user/service.go` に `func (s *Service) GetByID(ctx context.Context, userID string) (*model.User, error)`
    を追加。実装は `s.userRepo.FindByID` を呼ぶ（レガシーパス）、または
    `s.txUserDeleter.FindByID` を呼ぶ（txBeginner パス）薄い wrapper
  - 見つからない / nil の場合は `model.NewUserNotFoundError()` を返す（既存 `Withdraw` と
    同パターン）
  - 認可は行わない（caller userID = lookup userID であり middleware で済んでいる旨を
    doc comment に記述）
  - `internal/user/service_test.go` に以下の単体テストを追加:
    - `TestService_GetByID_Success`（mock UserRepository が user を返し、それがそのまま返る）
    - `TestService_GetByID_NotFound_ReturnsUserNotFoundError`（mock が nil を返したとき
      `model.NewUserNotFoundError` が返る）
    - `TestService_GetByID_RepoError_PropagatesError`（mock がエラーを返したとき wrap されて
      返ることを確認）
    - レガシーパスと txBeginner パスの両方で挙動が同一であることを確認（既存 service_test
      の構築パターンを踏襲）
  - _Requirements: 2.1, 2.2, 2.3_
  - _Boundary: user.Service_

- [ ] 4. GET /api/users/me ハンドラ・アダプタ・ルーティング配線を追加し契約テストを通す
  - `internal/handler/user_handler.go` に以下を追加:
    - `UserServiceInterface` に `GetCurrent(ctx context.Context, userID string) (*currentUserResponse, error)`
      メソッド追加
    - `currentUserResponse` 構造体（`id` / `email` / `name` / `avatar_url *string omitempty`）追加
    - `UserHandler.GetCurrent(w, r)` 実装（`UserIDFromContext` 失敗 → 401 UNAUTHORIZED /
      service 呼出 / `handleServiceError` で USER_NOT_FOUND → 404 / 成功 → JSON 200）
    - `SetupUserRoutes` 内の `r.Route("/api/users", ...)` ブロックに `r.Get("/me", h.GetCurrent)` を追加
  - `internal/handler/service_adapter.go` に `UserServiceAdapter.GetCurrent` を実装:
    `a.svc.GetByID(ctx, userID)` を呼び `currentUserResponse` に変換。`avatar_url` は
    当面 `nil` 固定（design.md「設計判断」節）
  - `internal/handler/router.go` の認証必須 group 内 `r.Route("/api/users", ...)` ブロックに
    `r.Get("/me", userHandler.GetCurrent)` を追加（既存 `r.Delete("/me", ...)` の前後どちらでも可）
  - `internal/handler/user_handler_test.go` に以下のテストを追加:
    - `mockUserService` に `getCurrentFn` フィールドと `GetCurrent` メソッド追加
    - `TestUserHandler_GetCurrent_Success_ReturnsCurrentUser`（withUserID で注入 → 200 +
      `{id, email, name}` を含む JSON / Req 4.1, 4.2: BearerOrSession 通過後の handler は
      経路非依存で同一動作するため両経路を 1 つのテストで担保）
    - `TestUserHandler_GetCurrent_NoUserID_ReturnsUnauthorized`（withUserID 未注入 → 401）
    - `TestUserHandler_GetCurrent_UserNotFound_Returns404`（service が NewUserNotFoundError → 404）
    - `TestUserHandler_GetCurrent_OmitsSecrets`（応答 JSON のキー集合が
      `{id, email, name}` ⊆ X ⊆ `{id, email, name, avatar_url}` であり session_id /
      refresh_token 等を含まないことを assert）
    - `TestUserHandler_GetCurrent_OmitsAvatarWhenNil`（avatar_url が nil なら JSON から省略
      されること / Req 2.4）
  - `internal/handler/router_full_test.go` に
    `TestRouter_GetUsersMe_RequiresAuth`（`GET /api/users/me` が認証必須グループに乗り、
    Authorization / Cookie のいずれも無しで 401 を返すことを確認 / Req 2.5）を追加
  - `internal/app/app.go` の wiring は `UserServiceAdapter` 既存生成箇所のままで GetCurrent
    が追加で見えるようになる（adapter 拡張のみで配線変更不要）
  - _Requirements: 1.3, 2.1, 2.2, 2.3, 2.4, 2.5, 4.1, 4.2_
  - _Boundary: UserHandler, UserServiceAdapter, router.go_
  - _Depends: 3_

- [ ] 5. /auth/me の Cookie 経路 non-regression テストを追加する
  - `internal/handler/auth_handler_test.go` に
    `TestAuthHandler_Me_CookiePathUnchanged` を追加:
    - 既存 `mockAuthService` パターンを踏襲し、Cookie `session_id` を持つリクエストで
      `AuthHandler.Me` を呼ぶ
    - 応答 200 / `Content-Type: application/json` / JSON が **既存形** `{id, email, name}`
      を含み、本 spec 導入前と同一であることを assert
    - `avatar_url` のような新フィールドが `/auth/me` から漏れて返らないことも合わせて assert
    - Cookie 不在時の 401 挙動の non-regression も同 test で確認
  - `internal/handler/auth_handler.go` および `SetupAuthRoutes` は **変更しない**（このタスク
    はテスト追加のみで既存挙動の保護を目的とする）
  - _Requirements: 2.6, 4.3, NFR 1.1_
  - _Boundary: AuthHandler_

- [ ] 6. item.ItemService.GetItem を拡張し FeedMetaProvider 依存を導入する
  - `internal/item/service.go` に以下を追加:
    - `FeedMetaProvider` interface（`FindByID(ctx context.Context, id string) (*model.Feed, error)`
      の 1 メソッドのみ。既存 `SubscriptionChecker` と同じ interface segregation パターン）
    - `ItemService` 構造体に `feedRepo FeedMetaProvider` フィールドを追加
    - `NewItemService` シグネチャに `feedRepo FeedMetaProvider` を追加（呼出側全箇所を更新）
    - `ItemDetail` 構造体に `FeedTitle string` / `FeedFaviconURL *string` を追加
    - `GetItem` 内の既存認可フロー（FindByID → SubscriptionChecker → FindByUserAndItem）の
      **後**に `feedRepo.FindByID(ctx, item.FeedID)` を呼び、`Feed.Title` を `FeedTitle`、
      `model.FaviconDataURL(feed.FaviconData, feed.FaviconMime)` を `FeedFaviconURL` に populate
    - feedRepo が nil を返したらエラーとして上位に伝播する（fail-fast / FK 制約により実運用
      では発生しない想定）
  - 既存認可フロー（ITEM_NOT_FOUND 応答条件 / SubscriptionChecker の挙動）を **絶対に変えない**
    （Req 3.5）
  - `internal/item/service_test.go` に以下の単体テストを追加:
    - `TestItemService_GetItem_PopulatesFeedMetadata`（mock FeedMetaProvider が feed を返す →
      ItemDetail.FeedTitle / FeedFaviconURL が populate される）
    - `TestItemService_GetItem_NullFaviconWhenFeedHasNone`（mock が FaviconData/Mime 空の feed
      を返す → FeedFaviconURL が nil / Req 3.4）
    - `TestItemService_GetItem_FeedRepoError_PropagatesError`（mock feedRepo がエラーを返す →
      error が上位に伝播し ItemDetail nil）
    - 既存 GetItem のテスト（未購読 → ITEM_NOT_FOUND 等）が **変更なしで通過する**ことを確認
      （Req 3.5 / NFR 1.3）
  - `internal/app/app.go` の `item.NewItemService(...)` 呼出しに既存 `feedRepo` 変数を追加引数
    として渡す（変数は同一スコープに既に存在 / 配線追加 1 行）
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.4_
  - _Boundary: item.ItemService, app.go wiring_

- [ ] 7. itemDetailResponse を拡張し記事詳細契約テストを通す
  - `internal/handler/item_handler.go` の `itemDetailResponse` に以下を追加:
    - `FeedTitle string \`json:"feed_title"\``
    - `FeedFaviconURL *string \`json:"feed_favicon_url,omitempty"\``
  - 既存フィールド（`itemSummaryResponse` 経由の 9 フィールド + `Content` / `Summary` /
    `Author`）の **フィールド名・型・タグを一切変更しない**（NFR 1.2 / Req 3.3）
  - `internal/handler/service_adapter.go` の `ItemServiceAdapterFromDomain.GetItem` に
    `FeedTitle: detail.FeedTitle` / `FeedFaviconURL: detail.FeedFaviconURL` の 2 行を追加
  - `internal/handler/item_handler_test.go` に以下を追加（既存 `mockItemService.getItemFn` を
    流用）:
    - `TestItemHandler_GetItem_ReturnsFeedMetadata`（mock が feed_title / favicon URL 入りの
      itemDetailResponse を返す → JSON に `feed_title` と `feed_favicon_url` が出現 / Req 4.4）
    - `TestItemHandler_GetItem_PreservesExistingFields`（既存 11 フィールド全てが出現し、
      フィールド名・型が不変であることを assert / Req 3.3 / 4.5 / NFR 1.2）
    - `TestItemHandler_GetItem_OmitsFaviconWhenNil`（FeedFaviconURL nil → JSON から省略 /
      Req 3.4）
    - `TestItemHandler_GetItem_NotFound_PreservesExistingBehavior`（既存 ITEM_NOT_FOUND 応答
      が変わらないことを assert / Req 3.5）
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.4, 4.5, NFR 1.2_
  - _Boundary: ItemHandler, ItemServiceAdapter_
  - _Depends: 6_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを
構造化ブロックで宣言する。Go バックエンドのみが本 spec の変更対象であるため、Go の
test + vet を実行する（lint は CI 既定の `go vet` で代替し、本 spec 専用追加なし）。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./...
```
