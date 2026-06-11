# Implementation Plan

- [ ] 1. auth: TokenService.RotateRefreshToken を追加
  - `internal/auth/token_service.go` に `ErrInvalidRefreshToken` sentinel を追加し、
    `RefreshTokenStore` interface を `FindByHash` / `MarkRotated` まで拡張
    （`repository.RefreshTokenRepository` が引き続き構造的に充足することを compile-time
    check で確認）
  - `RotateRefreshToken(ctx, refreshToken string) (*TokenPair, error)` を design.md の
    rotation フロー手順 2〜6 どおり実装（拒否は全て `ErrInvalidRefreshToken` に正規化。
    新 token は同一 FamilyID・`now + RefreshTokenTTL` のスライディング 30 日。
    乱数生成・hash は #166 の内部ヘルパーを共用）
  - `internal/auth/token_service_test.go` に design.md Testing Strategy 1〜7 のケースを追加
    （既存 #166 ケースは変更しない。モックの interface 拡張追従は可）
  - _Requirements: 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 2.6, 2.7, 3.1, NFR 1.1, NFR 1.2, NFR 3.1_

- [ ] 2. handler: Refresh エンドポイントと router 登録
  - `internal/handler/native_auth_handler.go`: `TokenExchangeService` interface に
    `RotateRefreshToken` を追加し、`Refresh` handler を実装（200 / 400 INVALID_REQUEST /
    401 INVALID_REFRESH_TOKEN / 500 を `middleware.WriteErrorResponse` で応答。
    成功応答は Token と同一の tokenResponse 形式を共用）
  - `internal/handler/router.go`: 認証不要グループの `NativeAuthHandler != nil` ガード内に
    `POST /api/auth/refresh` を登録（ボディ上限 middleware 付き）
  - `internal/handler/native_auth_handler_test.go` に design.md Testing Strategy 8 を、
    `internal/handler/router_test.go` に Testing Strategy 9 を追加
  - _Requirements: 1.1, 1.5, 1.6, 2.5, 2.6, NFR 1.3, NFR 2.1, NFR 2.2_
  - _Depends: 1_

- [ ] 3. 統合テスト: token 交換 → refresh → 旧 token 拒否
  - `internal/handler/integration_test.go` に「token 交換で pair 取得 → refresh 成功で
    新 pair 取得 → 旧 refresh token による再 refresh が 401」の通しケースを追加
    （issue AC「old token after rotation」に対応）
  - 既存の統合テスト・既存ルートが無変更で green であることを確認
  - _Requirements: 1.1, 1.2, 2.4, NFR 2.1, NFR 3.1_
  - _Depends: 2_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
