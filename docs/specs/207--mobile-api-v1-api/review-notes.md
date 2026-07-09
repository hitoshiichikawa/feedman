# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-8 timestamp=2026-06-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-207-impl--mobile-api-v1-api
- HEAD commit: 4cb6ebfb959d96de3168fa18396f1c74f0bf8a61
- Compared to: develop..HEAD

## Verified Requirements

### Requirement 1（v1 モバイル API 契約ドキュメント）
- 1.1 — `mobile-api-contract.md` §2.1 認証方式 / §2.2 エラー応答形式 / §2.4 JSON 命名規約 に共通方針を記載
- 1.2 — 同 §3 Native Auth サマリ（token/refresh/revoke/native callback）+ 既存 spec #163/#172 参照
- 1.3 — 同 §4.1 `GET /api/users/me`（URL/認証方式/成功応答 JSON/エラー応答）を記載
- 1.4 — 同 §5.1〜§5.13（記事一覧・記事詳細・検索・クロスフィード・購読操作）の URL/認証/パラメータ/応答/エラーを記載
- 1.5 — 同 §6 v1 スコープ外（`/api/devices` / `/api/keywords`）を次フェーズとして明示
- 1.6 — `README.md` API エンドポイント一覧に `GET /api/users/me`・native auth 系・starred/search/cross-feed/manual fetch を追記し、契約文書参照リンクを追加

### Requirement 2（統一ユーザー情報取得エンドポイント）
- 2.1 — `UserHandler.GetCurrent` + `UserServiceAdapter.GetCurrent` + `user.Service.GetByID` + router.go 認証必須グループ配線。`TestUserHandler_GetCurrent_Success_ReturnsCurrentUser`
- 2.2 — 同一 handler が BearerOrSession 通過後に経路非依存で動作。`TestNewRouter_UserRoutes_GetCurrentEndpoint`（Cookie 経路 200）
- 2.3 — `currentUserResponse{id,email,name,avatar_url}`。`TestUserHandler_GetCurrent_OmitsSecrets`
- 2.4 — `AvatarURL *string` + `omitempty`。`TestUserHandler_GetCurrent_OmitsAvatarWhenNil`
- 2.5 — handler 401 + router.go 認証グループ配下。`TestUserHandler_GetCurrent_NoUserID_ReturnsUnauthorized` / `TestRouter_GetUsersMe_RequiresAuth`
- 2.6 — `auth_handler.go` / `SetupAuthRoutes` 未変更。`TestAuthHandler_Me_CookiePathUnchanged`

### Requirement 3（記事詳細フィードメタデータ）
- 3.1 — `ItemDetail.FeedTitle` / `itemDetailResponse.feed_title`。`TestItemService_GetItem_PopulatesFeedMetadata` / `TestItemHandler_GetItem_ReturnsFeedMetadata`
- 3.2 — `FeedFaviconURL` を `model.FaviconDataURL` で整形。同上テスト
- 3.3 — 既存 11 フィールドのフィールド名・型・JSON タグ不変。`TestItemHandler_GetItem_PreservesExistingFields`
- 3.4 — `*string` + `omitempty`。`TestItemService_GetItem_NullFaviconWhenFeedHasNone` / `TestItemHandler_GetItem_OmitsFaviconWhenNil`
- 3.5 — feed lookup を既存認可フロー（SubscriptionChecker）の後に配置し ITEM_NOT_FOUND 条件不変。`TestItemService_GetItem_Unsubscribed_ReturnsNotFound`（変更なし pass）/ `TestItemHandler_GetItem_NotFound_PreservesExistingBehavior`

### Requirement 4（契約テスト）
- 4.1 — `TestUserHandler_GetCurrent_Success_ReturnsCurrentUser`（認証済み要求で current user を返す）。impl-notes に経路等価の紐付け記載あり
- 4.2 — `TestNewRouter_UserRoutes_GetCurrentEndpoint`（Cookie 経路 200）+ handler success test
- 4.3 — `TestAuthHandler_Me_CookiePathUnchanged`（`/auth/me` Cookie 経路の shape 非回帰）
- 4.4 — `TestItemHandler_GetItem_ReturnsFeedMetadata`（feed_title / feed_favicon_url 両出現）
- 4.5 — `TestItemHandler_GetItem_PreservesExistingFields`（既存フィールド一式の存続）

### Non-Functional Requirements
- NFR 1.1/1.2/1.3 — `/auth/me` 未変更・既存 item フィールド温存・既存 item テスト群は `defaultFeedProvider()` 追加引数のみで全 pass（後述 verify で green 確認）
- NFR 2.1/2.2 — 契約文書 §1〜§7 がエンドポイント詳細と v1 スコープ内外を独立記載
- NFR 3.1 — 追加テストは標準 `go test` 配置・外部ネットワーク/実 OAuth/実フィード非依存

## Verify 実行結果（reviewer 再実行）
- `go build ./...` — OK
- `go test ./internal/user/... ./internal/item/... ./internal/handler/...` — all ok
- `go vet ./internal/user/... ./internal/item/... ./internal/handler/... ./internal/app/...` — clean

## Boundary 確認
- 全変更ファイルが tasks.md の各 `_Boundary:_` に収まる（user.Service / UserHandler / UserServiceAdapter / router.go / AuthHandler[test] / item.ItemService / app.go wiring / ItemHandler / ItemServiceAdapter / mobile-api-contract.md / README.md）
- `tasks.md` は `- [ ]`→`- [x]` の進捗 checkbox 編集のみ（spec 本文・アノテーション不変）
- `docs/openapi/mobile-v1.yaml` の削除表示は本ブランチの変更ではなく develop 側の追加に対する two-dot diff 上の見かけ（merge-base..HEAD の three-dot diff では非該当を確認）。boundary 逸脱に当たらない
- Feature Flag Protocol は CLAUDE.md で `opt-out`。flag 観点の細目は適用せず通常 3 カテゴリ判定

## Findings

なし

## Summary

全 numeric AC（Req 1.x / 2.x / 3.x / 4.x / NFR 1.x / 2.x / 3.x）が実装・テスト・契約文書で観測可能にカバーされ、boundary 逸脱なし。build / test / vet を reviewer 側でも再実行し green を確認した。

RESULT: approve
