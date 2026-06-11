# Implementation Plan

- [x] 1. PKCE S256 検証ユーティリティを追加
  - `internal/auth/pkce.go` を新規作成し、`ValidatePKCES256(challenge, method string) error`
    を実装（method は "S256" 厳密一致、challenge は `^[A-Za-z0-9_-]{43}$` の事前 compile
    regex。純粋関数・I/O なし）
  - `internal/auth/pkce_test.go` を新規作成し、design.md Testing Strategy 1 の table-driven
    ケース（正常 / method 欠落 / plain / 小文字 s256 / challenge 欠落 / 42・44 文字 /
    不正文字）を実装
  - _Requirements: 1.2, 1.3, 1.4, NFR 1.2_

- [ ] 2. auth.Service に HandleNativeCallback を追加
  - `internal/auth/service.go` の `HandleCallback` から OAuth 交換〜ユーザー解決（手順 1〜3）を
    `resolveUserFromOAuth(ctx, code) (string, error)` に抽出（挙動・ログ出力は不変）
  - `internal/auth/native.go` を新規作成: `AuthCodeCreator` 最小 interface /
    `NativeAuthCodeTTL = 60 * time.Second` / `HashNativeSecret`（SHA-256 hex lowercase）/
    `generateAuthCode`（crypto/rand 32 byte → base64.RawURLEncoding）/
    `HandleNativeCallback`（design.md Components 参照。セッションは作成しない）
  - `NewService` に `authCodeRepo AuthCodeCreator` 引数を追加し、auth パッケージ内の
    既存テスト呼び出しを追従（既存ケースの検証内容は変えない）
  - `internal/auth/native_test.go` を新規作成し、design.md Testing Strategy 2 のケース
    （成功時の CodeHash/PKCEChallenge/ExpiresAt/Used 検証、既存・新規ユーザー両パス、
    repo 失敗、OAuth 失敗、エラーメッセージに平文を含まない）を実装
  - _Requirements: 2.2, 2.4, 2.5, 3.4, NFR 1.1, NFR 3.1_
  - _Depends: 1_

- [ ] 3. AuthHandler.Login の flow=native 分岐を追加
  - `internal/handler/auth_handler.go` に `oauthNativeChallengeCookie` /
    `nativeAuthCallbackURL` 定数を追加
  - `Login`: `flow=native` のとき `ValidatePKCES256` で検証し、不合格なら 400
    `invalid pkce parameters`（state cookie 未発行・redirect なし）、合格なら
    `oauth_native_challenge` cookie（MaxAge 600 / HttpOnly / Lax / Secure は config 従属）を
    設定して既存の state 発行 → redirect に合流
  - `flow=native` なしのとき、リクエストに残存 `oauth_native_challenge` cookie がある場合のみ
    削除 Set-Cookie を発行（無い場合は応答ヘッダ完全不変、NFR 2.1）
  - `internal/handler/auth_handler_test.go` に design.md Testing Strategy 4〜6 のケースを追加
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, NFR 1.3, NFR 2.1_
  - _Depends: 1_

- [ ] 4. AuthHandler.Callback の native 分岐を追加
  - `AuthServiceInterface` に `HandleNativeCallback(ctx, code, pkceChallenge string) (string, error)`
    を追加し、handler テスト用モックを追従
  - `Callback`: state 検証（既存・位置不変）通過後に native cookie を読み、present なら
    cookie 削除 → challenge 再検証（不正 400）→ `HandleNativeCallback` 呼び出し →
    成功時 303 `feedman://auth/callback?auth_code=<url.QueryEscape(code)>`・失敗時 500
    `authentication failed`。セッション作成・session_id cookie・旧セッション rotation は
    行わない。absent なら既存 Web flow に fallthrough（コード変更なし）
  - `internal/handler/auth_handler_test.go` に design.md Testing Strategy 7〜9 のケースを追加
    （303 Location 検証 / session_id Set-Cookie 不在 / native cookie 削除 / サービスエラー 500 /
    state 不正 400）
  - _Requirements: 2.1, 2.3, 3.1, 3.2, 3.3, 3.4, 4.1, NFR 1.3_
  - _Depends: 2, 3_

- [ ] 5. Wiring と統合テスト
  - `internal/app/app.go`: `repository.NewPostgresAuthCodeRepo(db)` を生成し
    `auth.NewService` へ注入
  - `internal/handler/integration_test.go` に native flow の通しケース（login(native) →
    callback → auth_code 付きリダイレクト・session cookie 不在）を 1 本追加
  - 既存 Web flow の統合テスト（`TestIntegration_AuthFlow_LoginCallbackMeLogout`）が無変更で
    green であることを確認（Req 4.1 / NFR 2.1 の回帰検証）
  - _Requirements: 2.1, 2.3, 4.1, 4.2, NFR 2.1, NFR 3.1_
  - _Depends: 4_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。DB 結合テストは `TEST_DATABASE_URL` 未接続時に `t.Skip` され、
unit テスト + 静的解析のみで green 判定される（CI でも同条件）。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
