# Implementation Plan

- [x] 1. router: native auth 3 ルートへ unauthIPMW を適用しレート制限テストを追加
  - `internal/handler/router.go`: `NativeAuthHandler != nil` ガード内の
    `POST /api/auth/token` / `POST /api/auth/refresh` / `POST /api/auth/revoke` の
    route 単位チェーン最外（MaxBodyBytes より外側）に既存 `unauthIPMW` を追加する
    （design.md「Components and Interfaces > router」のとおり。`IPRateLimiter` 本体・
    `RouterDeps`・app.go・config は変更しない）
  - `internal/handler/router_test.go` に design.md Testing Strategy 1〜5 を追加
    （超過 429 + Retry-After + モック service 未呼び出し × 3 ルート / 閾値以内の通常応答 /
    IP 独立カウント / 429 応答が既存未認証ルートと同一形式）
  - 拒否ログ（NFR 1.1, 1.2）は既存 `IPRateLimiter` の共用で充足（実装変更なし）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.3, NFR 1.1, NFR 1.2, NFR 3.1_

- [x] 2. 縮退・後方互換の regression テスト
  - `internal/handler/router_test.go` に design.md Testing Strategy 6 を追加
    （`NativeAuthHandler` nil で 3 ルートが 404 のまま = 本変更 no-op /
    `UnauthIPRateLimiter` nil で 3 ルートが制限なしで到達する縮退規約）
  - 既存テスト（既存未認証 3 ルートの IP 制限・認証必須グループ・middleware）が無変更で
    green であることを確認（design.md Testing Strategy 7〜8。既存テストの書き換えは行わない）
  - _Requirements: 2.1, 2.2, 2.4, NFR 2.1, NFR 2.2_
  - _Depends: 1_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
