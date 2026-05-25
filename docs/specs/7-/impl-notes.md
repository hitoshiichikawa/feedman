# 実装メモ: リクエストログミドルウェアをルーターに登録（Issue #7）

## 概要

リクエストログ用ミドルウェア `NewLoggingMiddleware`（`internal/middleware/logging.go`）が
`NewRouter`（`internal/handler/router.go`）のミドルウェアチェーンに未登録だった不具合を修正した。
本変更により全登録済みエンドポイント（`/health` / `/auth/*` / `/api/*`）でアクセスログが
1 件ずつ出力されるようになる。`NewLoggingMiddleware` のロジック自体は変更していない（スコープ外）。

## 変更ファイル

- `internal/handler/router.go` — `RouterDeps` に `Logger *slog.Logger` を追加し、`NewRouter` の
  2 箇所にロギングミドルウェアを登録。doc コメントの実行順序を更新。
- `internal/app/app.go` — `RouterDeps` 構築時に `Logger: slog.Default()` を設定（NFR 3.1）。
- `internal/handler/router_logging_test.go`（新規）— AC 1.x / 2.x / NFR 3.1 のテスト。

## 実装上の判断

### ロギングミドルウェアの 2 箇所登録

`NewLoggingMiddleware` は `next.ServeHTTP(rec, r)` の後に **元の `r.Context()`** から
`user_id` を読む。`SessionMiddleware` は `r.WithContext` で新しい `*http.Request` を生成して
下流へ渡すため、ロギングをチェーン最上位に置くと `user_id` を捕捉できない。よって以下に分割:

- **認証不要グループ**（`/health` / `/auth/*`）: `r.Use(logging)` のみ適用（Session/RateLimit なし）。
  `user_id` は付与されない。
- **認証必須グループ**（`/api/*`）: 既存の Session → RateLimit の **後ろ** に `r.Use(logging)` を追加。
  認証済みリクエストの `user_id` がログに含まれる。

両グループはルートが排他的（`/health` `/auth/*` と `/api/*`）なので、いずれのリクエストも
アクセスログは 1 件のみ（二重ログにならない / AC 1.2）。

最上位の `Recovery → SecurityHeaders → CORS` の相対順序は不変（AC 3.1）。`/api/*` の最終的な
実行順序は `Recovery → SecurityHeaders → CORS → Session → RateLimit → Logging`。

### ロガー供給経路（DI）

`RouterDeps` に `Logger *slog.Logger` を追加。`NewRouter` 内で `deps.Logger == nil` のとき
`slog.Default()` にフォールバックする（後方互換 / 既存テストはゼロ値 nil でも壊れない）。
`app.go` では `slog.Default()` を注入し、アプリ標準ログ出力と同一経路に書き出す（NFR 3.1）。

## 確認事項（人間レビュー向け）

本実装は、PM が requirements.md の Open Questions に挙げた 2 点について、オーケストレーター経由で
**人間判断を仰いだ結果として適用されたデフォルト**である。後から見直せるよう明記する:

1. **ロギングミドルウェアの配置 = 「両立（2 箇所登録）」を採用**。
   `/health`・`/auth/*` はログ対象だが `user_id` 無し、`/api/*` は `user_id` 付き。AC 1.2〜1.5 と
   AC 2.2 の両立要求を満たす配置。技術的制約（Session が新 Request を生成し元 Context から
   `user_id` を読む仕様）により、`user_id` を含めるには Logging を Session の内側に置く必要がある。
2. **ロガー供給 = `RouterDeps.Logger` を追加し nil 時 `slog.Default()` フォールバック**。
   テストで `*slog.Logger`（`slog.NewJSONHandler` を `bytes.Buffer` に向ける）を注入してログを
   キャプチャ検証できるようにするための DI 経路。

いずれも observable 挙動に影響する判断であり、別解（片方優先で緩和する等）を採る場合は
requirements.md の AC 自体の見直しが必要になるため、PM / 人間の再確認に委ねる。

## 受入基準カバレッジ

| AC | 内容 | 担保テスト |
|---|---|---|
| 1.1 | ロギングをチェーンに登録 | `TestNewRouter_Logging_EmitsSingleAccessLogPerEndpoint`（ログが出ること全般） |
| 1.2 | 任意エンドポイントでログ 1 件 | `TestNewRouter_Logging_EmitsSingleAccessLogPerEndpoint`（count == 1 を assert） |
| 1.3 | `/health` でログ 1 件 | 同上（health endpoint サブテスト） |
| 1.4 | `/auth/*` でログ 1 件 | 同上（auth route サブテスト） |
| 1.5 | `/api/*` でログ 1 件 | 同上（api route サブテスト） |
| 2.1 | method/path/status/duration_ms を含む | `TestNewRouter_Logging_IncludesRequestFields` |
| 2.2 | 認証済みで `user_id` を含む | `TestNewRouter_Logging_AuthenticatedRequest_IncludesUserID` |
| 2.3 | 未認証で `user_id` 空/非出力 | `TestNewRouter_Logging_UnauthenticatedRequest_OmitsUserID`（正常系）/ `TestNewRouter_Logging_ProtectedRoute_NoSession_NoUserIDLog`（401 境界） |
| 2.4 | 5xx 時にログ status が実 status と一致 | `TestNewRouter_Logging_5xxStatusMatchesResponse`（503 / 異常系） |
| 3.1 | 既存ミドルウェア相対順序の保持 | doc コメント明記 + 既存テスト群（CORS/Session 挙動）が green |
| 3.2 | セッション無しの認証拒否（401）保持 | `TestNewRouter_ProtectedRoute_NoSession_Returns401`（既存）/ `TestNewRouter_Logging_ProtectedRoute_NoSession_NoUserIDLog` |
| 3.3 | セッション有りの成功応答保持 | `TestNewRouter_ProtectedRoute_WithSession_GET_Succeeds`（既存） |
| 3.4 | レート制限挙動の維持 | 既存 RateLimiter テスト群が green（`internal/middleware`） |
| 3.5 | 全エンドポイントのルーティング保持 | `TestNewRouter_*Routes_AllEndpoints`（既存全件）が green |
| NFR 1.1 | 同期ログ 1 件・追加 I/O なし | `NewLoggingMiddleware` のロジック不変（スコープ外）/ ログ count == 1 で担保 |
| NFR 2.1 | 変更前に成功したリクエストの status/body 不変 | 既存 `router_full_test.go` / `integration_test.go` が green |
| NFR 3.1 | アプリ標準ログ出力と同一経路 | `app.go` で `slog.Default()` 注入 / `TestNewRouter_Logging_NilLogger_FallsBackToDefault`（フォールバック検証） |

## 検証結果

- `go test ./...` — 全パッケージ green
- `gofmt -l`（対象ファイル）— 差分なし
- `go vet ./internal/handler/... ./internal/app/...` — 警告なし

## Feature Flag Protocol

本リポジトリ `CLAUDE.md` の `## Feature Flag Protocol` は `**採否**: opt-out` のため、
本タスクは通常フロー（単一実装パス）で実装した。flag 追加なし。

STATUS: complete
