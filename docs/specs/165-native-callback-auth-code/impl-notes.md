# Implementation Notes

## 実装サマリ

design.md / tasks.md（PR #188 で merge 済み）に従い、5 タスクを 4 実装コミットで完了した。

| Task | コミット | 内容 |
|---|---|---|
| 1 | `feat(auth): PKCE S256 検証ユーティリティを追加` | `internal/auth/pkce.go` + table-driven test |
| 2 | `feat(auth): HandleNativeCallback で auth_code を発行する service 層を追加` | `native.go` / `service.go` の抽出リファクタ / `app.go` wiring |
| 3+4 | `feat(handler): OAuth login/callback に flow=native 分岐を追加` | Login / Callback の native 分岐 + handler tests |
| 5 | `test(handler): flow=native の通し統合テストを追加` | integration test |

## tasks.md からの逸脱（2 点・いずれも軽微）

1. **app.go の wiring（task 5 記載分）を task 2 のコミットに前倒し**: `NewService` の
   シグネチャ変更（`AuthCodeCreator` 追加）と同一コミットにしないと中間コミットがビルド不能に
   なるため。task 5 は統合テスト追加のみとなった
2. **task 3 と task 4 を 1 コミットに統合**: いずれも `auth_handler.go` /
   `auth_handler_test.go` への変更で、hunk 分割によるコミット分離はリスクに見合わないため

## 設計どおりの実装ポイント

- native flow 文脈は `oauth_native_challenge` HttpOnly Cookie（MaxAge 600 / Lax / Secure は
  config 従属）で保持。callback では成否に関わらず単回破棄
- `auth_code` は crypto/rand 32 byte → base64.RawURLEncoding（43 文字）、保存は
  `HashNativeSecret`（SHA-256 hex lowercase）の hash のみ。TTL は `NativeAuthCodeTTL`（60 秒）
- 平文 code はリダイレクト URL のみ。ログは hash 先頭 8 文字（`hashSessionIDForLog` と同方針）
- `HandleCallback` の OAuth 交換〜ユーザー解決は `resolveUserFromOAuth` に抽出（挙動・ログ不変。
  既存 service テストが無修正で green であることを確認）
- Web flow の応答ヘッダ完全互換: native cookie の削除 Set-Cookie は**残存 cookie がある
  リクエストのみ**発行（`TestAuthHandler_Login_Web_NoNativeCookie_HeadersUnchanged` で検証）

## 検証結果

- `go build ./...` / `go vet ./...` / `go test ./...` すべて green（ローカル）
- `gofmt -l` で本 PR の変更ファイルに差分なし（既存の未フォーマット 11 ファイルは
  develop 既存のもので本 PR のスコープ外）
- DB 結合テストは `TEST_DATABASE_URL` 未設定環境では skip（既存慣習どおり）

## 後続 Issue への引き継ぎ

- #166（token 交換）は `auth.HashNativeSecret` で受領 auth_code を hash 化し
  `AuthCodeRepository.FindByHash` → PKCE verifier 検証（S256(verifier) == 保存 challenge）→
  `MarkUsed` の順で消費する想定
- `AuthServiceInterface`（handler）は今回 `HandleNativeCallback` を追加済み。#166 では
  token 交換用の別 interface / handler を新設する想定（design.md Non-Goals 参照）
