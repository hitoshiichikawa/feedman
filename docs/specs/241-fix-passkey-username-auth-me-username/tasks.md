# Implementation Plan

- [ ] 1. パスキー新規登録 finish で users.name を username と同値で初期化する
  - `internal/passkey/registration_service.go` の `FinishRegistrationNew` 内 `newUser := &model.User{...}`
    構築箇所に **`Name: normalized`** を追加（既存 tx オーケストレーション / Issue #230 の
    `BeginTx → CreateUserOnlyExec → CreateExec → Commit` 境界内で完結。追加の tx 制御は書かない）
  - `internal/passkey/registration_service_test.go` の既存成功サブテスト
    `"成功: user 行と credential 行を単一 tx で作成し Commit / userID を返す ..."` に
    `users.lastCreated.Name == pendingUsername` の assert を追加（Req 1.1 の unit test）
  - 既存の失敗系サブテスト（credential 重複 / infra 障害 / session factory 失敗）が
    `commitCalled == 0` かつ `rollbackCalled == 1` を既に検証していることを確認し、これが
    「Name を含む users 行が永続化されない」の担保として機能することを test コメントで明示
    （Req 1.3 の Rollback 経路 regression）
  - `internal/repository/postgres_passkey_registration_tx_db_test.go` の
    `TestPasskeyRegistrationTx_HappyPathCommitsBoth` に `found.Name == normalized` の assert を
    1 行追加（Req 1.1 の DB-backed regression。stub test に対する real DB 二重化）
  - 既存 `internal/passkey/username.go` の `ValidateAndNormalize` が非空を保証する契約に
    依拠し、Name が空文字になり得ない前提を成立させる（追加検証は不要）
  - Google OAuth 経路（`internal/auth/service.go::HandleCallback` →
    `PostgresUserRepo.CreateWithIdentity`）のコードは一切変更しないことを確認（Req 4.3 / NFR 3.1）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 4.3, NFR 2.2, NFR 3.1_
  - _Boundary: passkey.RegistrationService, PostgresPasskeyRegistrationTx_

- [ ] 2. `GET /auth/me` レスポンスに username フィールドを追加する
  - `internal/handler/auth_handler.go` の `Me()` 応答生成を **`map[string]interface{}` から
    専用 struct `meResponse` へ切り替え** し、`Username *string` フィールドを `json:"username"`
    （`omitempty` なし）で追加（design.md「Response Struct（Go）」節参照）
  - handler 内で `user.Username != ""` を判定し、非空なら `*string`、空文字なら `nil` を
    assign（Req 2.2 / Req 2.3）。DB 層（`PostgresUserRepo.FindByID`）と model 層は無変更
  - Content-Type ヘッダ `application/json` の設定行、401 経路（Cookie 不在 / 検証失敗）、
    既存 `slog.Error(...)` ログは **一切変更しない**（Req 2.5 / Req 2.6 / NFR 1.2）
  - `internal/handler/auth_handler_test.go` の既存
    `TestAuthHandler_Me_CookiePathUnchanged/Cookie_Present_ReturnsExistingShape` を更新:
    `allowed` set を `{id, email, name} → {id, email, name, username}` に拡張。既存の
    `forbidden` set（`avatar_url` / `session_id` / `refresh_token` / `password` /
    `password_hash` / `access_token`）は無変更で維持（NFR 2.1 regression net）
  - 同ファイル `TestAuthHandler_Me_CookiePathUnchanged` に新規サブテスト追加:
    - `Cookie_Present_UsernameSet_ReturnsUsernameString`: `user.Username = "alice"` のとき
      `body["username"] == "alice"`（string 型）を検証（Req 2.1 / Req 2.2 / Req 2.4）
    - `Cookie_Present_UsernameUnset_ReturnsNull`: `user.Username = ""`（Google 由来ユーザー
      相当）のとき **`body` にキー `"username"` が存在し** かつ **値が nil**（JSON 上
      `"username": null`）を検証（Req 2.3 / Req 4.1 / NFR 2.1）
  - 既存 `NoCookie_ReturnsUnauthorized` サブテストは無変更（Req 2.6 / Req 4.1 の 401 経路
    非regression）
  - `internal/auth/service.go::GetCurrentUser` は無変更（service 層は `*model.User` を返す
    既存契約を維持し、handler 層でのみ struct 化する）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 4.1, NFR 1.1, NFR 1.2, NFR 2.1_
  - _Boundary: AuthHandler_

- [ ] 3. Web の `User` 型と `useCurrentUser` フックの mock を username 対応にする
  - `web/src/types/auth.ts` の `User` interface に **必須プロパティ**として
    `username: string | null` を追加（`?` optional ではなく union with `null`。API が
    常にキーを返すため / Req 2.1）。既存 `id / email / name / created_at` は無変更
  - `web/src/hooks/use-auth.test.tsx` の既存
    `it("認証済みユーザー情報を取得できること", ...)` の mock 応答 JSON に
    `username: "test-user"` を追加、`toEqual` 期待値も同期して更新（TypeScript 型互換と
    hook decode 経路の一貫性を維持）
  - 同ファイルの `it("未認証時（401）はエラー状態になること", ...)` は無変更（401 経路は
    JSON body を返さないため username 追加の影響を受けない）
  - `useLogout` 系テストは本変更の影響を受けないため無変更
  - `web/src/components/auth-guard.tsx` は `useCurrentUser` の `data` を認証済み判定にのみ
    利用しており、username フィールドを参照していないため無変更で動作することを確認
  - _Requirements: 2.1, 2.2, 2.3, 3.1_
  - _Boundary: types/auth.ts, useCurrentUser hook_

- [ ] 4. アカウント設定ダイアログに username 表示行を追加する
  - `web/src/components/account-settings-dialog.tsx` の `AccountInfoSection` に username
    表示ブロックを追加。判定式 `user.username != null && user.username !== ""` を満たす
    ときのみ「ユーザー名」ラベル + `<span data-testid="account-info-username">` を描画。
    null / 空文字なら `{condition && <div>...}` パターンで **要素そのものを DOM に出さない**
    （Req 3.3 / 既存 email 未設定プレースホルダのような代替ラベルは出さない）
  - 表示フォーマットは装飾なしで username 文字列そのものを表示（PM Open Question に対する
    design.md 「表示フォーマットの設計判断」節の結論を採用。`@` プレフィックスは付けない）
  - ラベル文字列は `"ユーザー名"`（既存の `"表示名"` / `"メールアドレス"` と同じ日本語ラベル
    統一）。className は既存行の `text-xs font-medium text-muted-foreground` を流用
  - 配置位置は表示名行の直後 / email 行の直前を推奨（Req 3 の Objective「ログイン主体
    識別」に沿い、表示名と username を隣接させる）
  - 既存の `account-info-name` / `account-info-email` / `account-info-email-unset` /
    `account-settings-loading` / `account-settings-error` / `account-withdraw-section` /
    `withdraw-trigger` の testID・描画ロジック・className・条件式は **一切変更しない**
    （Req 3.4 / Req 4.2 の非破壊性）
  - `web/src/components/account-settings-dialog.test.tsx` の `setupMockFetch` の
    `"ok"` 分岐 mock 応答に `username: "alice-id"` を、`"ok-empty-email"` 分岐 mock 応答に
    `username: null` を追加（Google 由来ユーザー相当）
  - 同ファイルに新規テスト追加:
    - `it("username が非 null / 非空のとき username が「ユーザー名」ラベル付きで表示される
      こと (Req 3.2)")` — `getByTestId("account-info-username")` が `"alice-id"` を含み、
      ラベル `"ユーザー名"` も表示される
    - `it("username が null のとき username 表示要素が描画されず、既存の表示名 / email 表示に
      影響しないこと (Req 3.3 / Req 4.2)")` — `queryByTestId("account-info-username")` が
      null、`account-info-name` / `account-info-email-unset` が既存通り表示
    - `it("username が空文字のとき username 表示要素が描画されないこと (Req 3.3)")` — mock
      応答に `username: ""` を注入し `queryByTestId("account-info-username")` が null
  - 既存の「表示名と email が表示されること」「email 未設定プレースホルダ」「退会フロー」
    テストが mock fixture の username 追加後も pass することを確認（Req 3.4 の既存動線
    非破壊性）
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 4.2_
  - _Boundary: AccountSettingsDialog_
  - _Depends: 3_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを構造化
ブロックで宣言する。Go 側は `go vet` + `go test`、Web 側は Vitest + ESLint を対象とする
（本修正は Go / Web 双方に触れるため両者を verify する必要がある）。

<!-- stage-a-verify -->
```sh
go vet ./... && go test ./... && cd web && npm run lint && npm test
```
