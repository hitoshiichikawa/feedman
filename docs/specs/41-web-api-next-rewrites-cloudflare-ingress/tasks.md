# Implementation Plan

- [x] 1. API クライアントの NEXT_PUBLIC_API_URL 依存撤去
- [x] 1.1 `web/src/lib/api.ts` を相対パス固定にする
  - `API_BASE_URL` を常に `""` にし、`process.env.NEXT_PUBLIC_API_URL` 参照とフォールバック記述を撤去する
  - エンドポイントパス・`credentials: "include"`・`ApiError`・`createApiClient` のシグネチャは不変
  - doc comment を単一オリジン相対パス方針に更新する
  - _Requirements: 1.2, 1.4, 2.3, 5.2, 5.3, 6.3, NFR 1.1_
- [x] 1.2 `web/src/lib/api.test.ts` を更新する
  - 既存の相対パス期待ケース（`/api/feeds` 等）を維持する（Req 6.1）
  - `NEXT_PUBLIC_API_URL` が設定されても相対パスで `fetch` されることを確認するケースを追加する
  - _Requirements: 1.4, 6.1_
  - _Depends: 1.1_

- [x] 2. rewrites 生成ロジックと API_INTERNAL_URL 検証の純粋モジュール追加
- [x] 2.1 `web/src/lib/rewrites.ts` を新規作成する (P)
  - `API_INTERNAL_URL_ENV` 定数、`resolveApiInternalUrl(env)`（未設定/空で throw・末尾スラッシュ正規化）、
    `buildRewrites(base)`（`/api/:path*`・`/auth/:path*` をプレフィックス保持で転送する 2 ルール生成）を実装する
  - _Requirements: 2.1, 2.2, 4.1, 4.2, 4.3_
  - _Boundary: Rewrites Proxy, Startup Validation_
- [x] 2.2 `web/src/lib/rewrites.test.ts` を新規作成する (P)
  - `buildRewrites` の 2 ルール内容・末尾スラッシュ正規化を検証する
  - `resolveApiInternalUrl` の未設定/空 throw（メッセージに変数名を含む）・有効値の正規化を検証する
  - _Requirements: 2.1, 2.2, 4.1, 4.2, 4.3_
  - _Boundary: Rewrites Proxy, Startup Validation_
  - _Depends: 2.1_

- [x] 3. next.config.ts に rewrites() を追加する
  - `web/next.config.ts` に `async rewrites()` を追加し、`buildRewrites(resolveApiInternalUrl(process.env))` を返す
  - `output: "standalone"` と既存 `headers()` は維持する
  - _Requirements: 1.3, 2.1, 2.2, 2.4, 2.5, 3.1, 3.2, 5.1, 5.2, NFR 3.3_
  - _Depends: 2.1_

- [ ] 4. 起動時 fail-fast entrypoint の追加
- [x] 4.1 `web/server-entrypoint.mjs` を新規作成する
  - `resolveApiInternalUrl(process.env)` を呼び、未設定/空なら stderr にエラーメッセージ（変数名を含む）を
    出力して非ゼロ終了する。通過時に standalone `server.js` を起動する
  - _Requirements: 4.1, 4.2, 4.3, NFR 2.1_
  - _Depends: 2.1_
- [ ] 4.2 `web/Dockerfile` を更新する
  - builder stage の `ARG/ENV NEXT_PUBLIC_API_URL` を削除する
  - runner stage で `server-entrypoint.mjs` をコピーし `CMD ["node", "server-entrypoint.mjs"]` に変更する
  - _Requirements: 1.1, 4.1, 4.3, 5.3, NFR 1.2_
  - _Depends: 4.1_

- [ ] 5. 構成・環境変数・ドキュメントの単一オリジン化
- [ ] 5.1 `docker-compose.yml` を更新する
  - `web.build.args.NEXT_PUBLIC_API_URL` を削除し、`web.environment` に
    `API_INTERNAL_URL=${API_INTERNAL_URL:-http://api:8080}` を追加する
  - `api` の `GOOGLE_REDIRECT_URL` / `BASE_URL` を単一オリジン前提のコメントで補足する
  - _Requirements: 1.1, 1.3, 5.1, 5.3, NFR 1.2_
- [ ] 5.2 `.env.sample` と `README.md` を更新する
  - `.env.sample`: `NEXT_PUBLIC_API_URL` 削除、`API_INTERNAL_URL` 追加、`GOOGLE_REDIRECT_URL` /
    `BASE_URL` を単一オリジン（`https://<host>` / `https://<host>/auth/google/callback`）の説明に更新
  - `README.md`: アーキテクチャ（2 オリジン→単一オリジン）、環境変数表、本番デプロイ注意事項、
    Google Cloud Console redirect URI 登録手順、セキュリティ節（CSRF/Cookie）を更新
  - _Requirements: 3.3, 5.3, NFR 1.2, NFR 3.1, NFR 3.2_

- [ ] 6. バックエンド非改修の回帰確認（Cookie 属性・OAuth フロー不変）
  - `internal/handler/auth_handler.go` の Cookie 属性（`SameSite=Lax` / `HttpOnly` / `Secure` 自動判定）と
    OAuth state 検証失敗時の挙動を変更しないことを確認する（first-party 化のためコード改修は不要）
  - `go test ./...` を実行し既存バックエンドテストスイートが全件成功することを確認する
  - _Requirements: 3.4, 3.5, 6.2_

- [ ]* 7. 結合テスト / PoC でのプロキシ透過検証の補完
  - web standalone 経由で `/api/*`・`/auth/*` が `API_INTERNAL_URL` 先へ転送され、status・本文・`Set-Cookie` が
    透過されることを結合テストで確認する
  - OAuth ログイン手動 PoC（first-party Cookie / リダイレクト / CORS プリフライト非発生）を実施する
  - _Requirements: 2.4, 2.5, 3.1, 3.3_
