# Implementation Plan

本 spec はテストおよび文書追加のみで production code を変更しない。タスクは「契約観点の
追加テスト」「DB-backed E2E テスト」「SERVER.md 同期文書」の 3 系統を独立コミット単位で
完成させる順序で並べる。各タスクは追加後に `go test ./internal/handler/...`（および E2E
タスクでは `TEST_DATABASE_URL` 接続時のみ実行されるサブテスト）が green であることを
確認してから次タスクへ進む。

- [x] 1. JSON 応答契約テスト（token / refresh / revoke）を追加する
  - `internal/handler/integration_test.go` の末尾に以下 3 ケースを追加する
  - `TestContract_TokenResponse_ExactJSONShape`: 既存 `createNativeAuthIntegrationRouter`
    + 既存 `runNativeLoginCallbackAndExchange` を用いて 200 を取得し、レスポンス JSON
    を `map[string]any` にデコードして {access_token, refresh_token, token_type:"Bearer",
    expires_in:900} の 4 フィールド一致と **総キー数 4**（余剰キー混入なし）をアサート
  - `TestContract_RefreshResponse_ExactJSONShape`: 同様に refresh 200 応答で 4 フィールド
    厳密集合を固定。`token_type` / `expires_in` の値も同時にアサート
  - `TestContract_RevokeResponse_204AndEmptyBody`: revoke 成功時の `StatusNoContent` +
    `w.Body.Len() == 0` を直接固定
  - 既存テスト群（`TestIntegration_NativeAuthFlow_*` / `TestIntegration_RefreshFlow_*` /
    `TestIntegration_RevokeFlow_*`）は変更しない（NFR 2.2）
  - `_Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_`

- [x] 2. native callback の Location 契約テストを追加する
  - `internal/handler/integration_test.go` の末尾に
    `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` を追加する
  - native login → callback で得た 303 応答の `Location` ヘッダが
    `feedman://auth/callback?auth_code=` で始まり、`?auth_code=` 値部分が空でないことを
    `url.Parse` で抽出してアサート
  - `Location` 全体一致は既存 `TestIntegration_NativeAuthFlow_LoginCallbackReturnsAuthCode`
    で固定値（"integration-native-auth-code"）を検証済みのため、本タスクは契約形式
    （scheme / host / path / クエリ名）の正規表現的固定に専念する
  - `_Requirements: 2.6_`

- [x] 3. Bearer access token が既存 API に到達し Cookie と同一ユーザーで応答する契約テストを追加する
  - `internal/handler/integration_test.go` の末尾に
    `TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie` を追加する
  - 既存 `auth.NewJWTIssuer` + `auth.NewJWTVerifier` を同一 secret で生成し、
    `IssueAccessToken("contract-user-1")` で発行した JWT を Bearer として
    `/api/subscriptions` に提示
  - 同じ `contract-user-1` を `UserID` に持つ `model.Session` を Cookie 経由でも提示する
    ケースを別途送り、両者で **同じ subscription 一覧 JSON** が返ることをアサート
    （注入する `SubscriptionService` を `userID` 引数で固定 fixture を返すモックにする）
  - 既存 `TestNewRouter_BearerAuth_IssuerVerifierRoundTrip` との重複を避けるため、本テスト
    は「JWT sub と Cookie UserID が同じ場合に **応答内容まで等価**」までを契約として
    固定する（既存 round-trip は 200 status 到達のみ）
  - `_Requirements: 1.5_`

- [x] 4. Bearer 拒否 4 区分の uniform 契約テストを追加する
  - `internal/handler/integration_test.go` の末尾に
    `TestContract_BearerToken_RejectionUniformity_AllRejectionShapes` を追加する
  - 同一 secret の `JWTVerifier` を `RouterDeps.JWTVerifier` に注入した router を構築し、
    以下 4 サブケースを table-driven で `/api/subscriptions` に提示:
    a. 署名不正（別 secret で sign した token）
    b. 期限切れ（`exp` が過去の token、既存 `TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401`
       と同手法）
    c. `token_use` 不一致（`"refresh"` または欠落）
    d. 形式不正（`Bearer not-a-jwt` 等）
  - 各ケースで 401 status と **完全同一の response body**（`BearerOrSessionMiddleware` は
    `http.Error(w, "unauthorized", 401)` 固定）が返ることをアサート。Cookie 併送時にも
    fallback しないことを 1 ケースだけ追加検証する
  - `_Requirements: 3.5, 3.6_`

- [x] 5. E2E DB-backed full-flow テストを追加する
  - 新規ファイル `internal/handler/native_auth_e2e_db_test.go` を作成し、以下を実装する
  - `TEST_DATABASE_URL` 接続セットアップ（既存 `setupRefreshTokenTestDB` / `setupWithdrawTestDB`
    と同型）を関数 `setupNativeAuthE2EDB(t *testing.T) *sql.DB` として配置。DB 未到達時は
    `t.Skipf` で silent skip（NFR 1.1）
  - real `PostgresAuthCodeRepo` + real `PostgresRefreshTokenRepo` + real `JWTIssuer`
    （固定 secret `[]byte("e2e-contract-test-secret-32bytes!")`） + real `JWTVerifier`
    （同 secret）+ real `TokenService` を組み立て、real `Service`（fake `OAuthProvider`
    を注入）と共に `RouterDeps` に wire して `NewRouter` を起動
  - `fakeOAuthProviderForE2E` を本ファイル内で 30 行未満で定義し、`ExchangeCode` が固定
    `OAuthUserInfo{ProviderUserID:"google-sub-e2e", Email:"e2e@example.com", Name:"E2E"}` を
    返すよう実装
  - 関数 `TestE2E_NativeAuthFullFlow_DBBacked` を 1 件追加し、design.md「State transitions
    checked」の step1〜6 を逐次実行。各 step 後に 200/303/204 status と JSON 応答契約
    （token_type:"Bearer", expires_in:900 等）をアサート
  - 拒否サブケース（step3-reject / step4-reject / revoke 不明 token）も同テスト内に組み込み、
    `code:INVALID_GRANT` / `code:INVALID_REFRESH_TOKEN` / 204 を固定。応答 `message` に
    "used" / "expired" / "rotated" / "revoked" 等の原因区別語が含まれないことを既存
    `TestIntegration_*` と同方針で `strings.Contains` 否定アサート
  - step6 では step3 で発行された **real JWT** を Bearer として `/api/subscriptions` に
    提示し、200 を取得する。`SubscriptionService` には real impl ではなく fake を注入し、
    context に注入された userID が fake `OAuthProvider` の `ProviderUserID` から解決された
    user.ID と一致する応答を返すように設定
  - DB 検証クエリ（auth_codes.used / refresh_tokens.rotated_at / refresh_token_families.revoked_at）
    を最小限差し込み、`DB の永続化状態が API 応答と整合する` ことまで検証
  - `_Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 3.1, 3.2, 3.3, 3.4_`

- [x] 6. SERVER.md §1 ↔ 実装の契約同期文書を作成する
  - 新規ファイル `docs/specs/172-native-auth-contract-tests/contract-notes.md` を作成
  - design.md「Contract Notes Document」の `Structure` に従い、以下のセクションを記述:
    a. `## §1.3 Endpoint contracts` — `/token` / `/refresh` / `/revoke` 各エンドポイント
       について Request / Response / Error の対照表
    b. `## §1.4 Token design` — access JWT 15 分 / refresh opaque 30 日 / kid 付与の対照
    c. `## §1.5 DB schema` — auth_codes / refresh_token_families / refresh_tokens のスキーマ
       と SERVER.md 例との差分
    d. `## §1.8 受け入れ基準 ↔ テスト ID` — 各受け入れ基準を本 spec および既存テストの
       関数名にマッピング
    e. `## 承認済み逸脱` — 4.3 revoke 未認証化（#168 / RFC 7009 §2.1 public client 慣行）
       と 4.4 DB スキーマ分離（#164 で `refresh_token_families` 追加）を明文化
    f. `## 未承認差分エスカレーション方針` — 整合確認の過程で未承認差分を発見した場合は
       本 spec で確定させず Issue コメントで人間判断を仰ぐ旨を明記
  - 整合確認の作業として、SERVER.md §1.3 / §1.4 / §1.5 / §1.8 を 1 章ずつ精読し、実装
    （`internal/handler/native_auth_handler.go` / `internal/auth/token_service.go` /
    マイグレーション SQL）との差分を全て抽出。**承認済み逸脱の 2 件以外に未承認差分が
    見つかった場合は本タスクを完了させず**、PR 本文「確認事項」に列挙して人間判断を仰ぐ
    （Req 4.5）
  - `_Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_`

- [ ] 7. 既存テストとの非重複・既存挙動非干渉の最終確認
  - 本タスクは独立コミット単位として 1〜6 完了後にチェックする整理タスク（diff は
    生じない想定だが、必要に応じて重複箇所をリファクタリング）
  - 確認事項:
    a. 既存 `TestIntegration_NativeAuthFlow_*` / `TestIntegration_RefreshFlow_*` /
       `TestIntegration_ReuseDetection_FamilyRevoked` / `TestIntegration_RevokeFlow_*` /
       `TestNewRouter_BearerAuth_*` / `TestNewRouter_NativeAuthIPRateLimit_*` の関数本体
       が変更されていないこと（NFR 2.2）
    b. 既存ヘルパー `createIntegrationRouter` / `createNativeAuthIntegrationRouter` /
       `runNativeLoginCallbackAndExchange` / `mockNativeTokenExchangeService` の関数本体が
       変更されていないこと
    c. `go test ./...` を full pass（CI 同条件の `-p 1`）で実行
    d. `TEST_DATABASE_URL` 未設定環境（DB 不在）で本 spec 追加テスト群が `TestE2E_*`
       のみ skip され、それ以外は green になること
    e. リポジトリの既存 spec の `tasks.md` / `requirements.md` / `design.md` を本 spec から
       書き換えていないこと（impl spec の責務境界）
  - `_Requirements: NFR 2.1, NFR 2.2, NFR 3.1_`

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを構造化
ブロックで宣言する。CI の `backend` ジョブと同条件で go test を回し、加えて handler
パッケージのみを `-run` 絞り込みで明示再実行することで、本 spec の追加テストが新規に
存在することを担保する。

<!-- stage-a-verify -->
```sh
go vet ./... && go test -p 1 ./...
```
