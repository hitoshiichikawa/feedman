# Implementation Plan

- [x] 1. auth: 再利用検知の昇格と RevokeRefreshToken
  - `internal/auth/token_service.go`: `RefreshTokenStore` interface に `RevokeFamily` を追加
    （compile-time check で `repository.RefreshTokenRepository` 充足を確認）
  - `RotateRefreshToken` の拒否分岐 2 箇所（手順 3 の RotatedAt 検出 / 手順 4 の
    `ErrRefreshTokenAlreadyRotated`）を design.md「再利用検知の昇格」どおり
    `RevokeFamily` 実行 + `ErrInvalidRefreshToken` に昇格（`slog.Warn` で family_id と
    hash 先頭 8 文字を記録。RevokeFamily 失敗時も拒否は維持し `slog.Error` 記録）
  - `RevokeRefreshToken(ctx, refreshToken string) error` を design.md「Revoke フロー」どおり
    実装（不明 token は no-op nil / 状態に関わらず family 失効 / infra エラーのみ error）
  - `internal/auth/token_service_test.go` に design.md Testing Strategy 1〜7 を追加
    （#167 既存ケースの検証内容は変えない。モックの interface 追従は可）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, NFR 1.1, NFR 1.3, NFR 3.1_

- [x] 2. handler: Revoke エンドポイントと router 登録
  - `internal/handler/native_auth_handler.go`: `TokenExchangeService` interface に
    `RevokeRefreshToken` を追加し、`Revoke` handler を実装（204 ボディなし /
    400 INVALID_REQUEST / 500 INTERNAL_ERROR）
  - `internal/handler/router.go`: 認証不要グループの `NativeAuthHandler != nil` ガード内に
    `POST /api/auth/revoke` を登録（ボディ上限 middleware 付き）
  - `internal/handler/native_auth_handler_test.go` に design.md Testing Strategy 8 を、
    `internal/handler/router_test.go` に Testing Strategy 9 を追加
  - _Requirements: 2.1, 2.2, 2.4, 2.5, 2.6, NFR 1.2, NFR 2.1, NFR 2.2_
  - _Depends: 1_

- [x] 3. 統合テスト: 再利用 family 全滅と revoke 後拒否
  - `internal/handler/integration_test.go` に design.md Testing Strategy 10 の 2 シナリオ
    （再利用 → family 全滅 / revoke 204 → refresh 401 → 再 revoke 204 の冪等）を追加
  - 既存の統合テスト・既存ルートが無変更で green であることを確認
  - _Requirements: 1.1, 1.3, 2.1, 2.2, 2.3, NFR 2.1, NFR 3.1_
  - _Depends: 2_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
