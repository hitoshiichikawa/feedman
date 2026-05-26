# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-37-impl-hsts-csp
- HEAD commit: ea7189a22ee54736df10487dae07263d0af01d47
- Compared to: develop..HEAD

備考: 本 Issue は design-less impl（`design.md` / `tasks.md` 不在）。`_Boundary:_` アノテーションが
存在しないため境界判定は要件スコープ（API サーバーのセキュリティヘッダーミドルウェアとその wiring）と
変更ファイルの突き合わせで行った。CLAUDE.md の `## Feature Flag Protocol` の `**採否**:` は `opt-out`
のため、flag 観点の細目判定は適用せず通常の 3 カテゴリ判定のみを実施した。

## Verified Requirements

- 1.1 — `security_headers.go` で `h.Set("Content-Security-Policy", contentSecurityPolicyValue)` を無条件付与 / `TestSecurityHeaders_CSP`
- 1.2 — CSP 値 `default-src 'none'; frame-ancestors 'none'` に `default-src 'none'` を含む / `TestSecurityHeaders_CSP`（値の完全一致）
- 1.3 — 同値に `frame-ancestors 'none'` を含む / `TestSecurityHeaders_CSP`（値の完全一致）
- 1.4 — CSP はフラグ・配信種別非依存で同一値 / `TestSecurityHeaders_CSP/HTTPリクエストのとき…`（HTTP でも同一 `wantCSP`）
- 2.1 — `if hstsEnabled && isHTTPS(r)` で HTTPS 判定時に HSTS 付与 / `TestSecurityHeaders_HSTS/HSTS有効かつXForwardedProtoがhttps…`、`TestSecurityHeaders_HSTS_DirectTLS`
- 2.2 — HTTP 判定時は HSTS 非付与 / `TestSecurityHeaders_HSTS`（http / 欠落サブケースで present=false）
- 2.3 — `isHTTPS` が `X-Forwarded-Proto == "https"` で HTTPS 判定 / `TestSecurityHeaders_HSTS/…https…`
- 2.4 — `X-Forwarded-Proto` が https 以外（http / 欠落）は HTTP 判定 / `TestSecurityHeaders_HSTS`（http / 欠落サブケース）
- 3.1 — フラグ無効時は HTTPS 判定でも非付与（`hstsEnabled` の AND 条件）/ `TestSecurityHeaders_HSTS/HSTS無効かつHTTPS判定でも…`
- 3.2 — フラグ有効 + HTTPS 判定時に付与 / `TestSecurityHeaders_HSTS/HSTS有効かつXForwardedProtoがhttps…`
- 3.3 — 未指定・不正値時は既定値 false 採用で起動継続（`getEnvBool` が Warn + フォールバック、err 返さず）/ `TestLoad_HSTSEnabled`（未設定 / 不正値）、`TestGetEnvBool`（不正値 Warn）
- 4.1 — `X-Content-Type-Options: nosniff` 維持 / `TestSecurityHeaders_ExistingHeaders`
- 4.2 — `X-Frame-Options: DENY` 維持 / `TestSecurityHeaders_ExistingHeaders`
- 4.3 — `Referrer-Policy: strict-origin-when-cross-origin` 維持 / `TestSecurityHeaders_ExistingHeaders`
- 4.4 — `Permissions-Policy: camera=(), microphone=(), geolocation=()` 維持 / `TestSecurityHeaders_ExistingHeaders`
- 4.5 — 全ルート適用 / `router.go` の `r.Use(NewSecurityHeadersMiddleware(deps.HSTSEnabled))` 配置（Recovery→SecurityHeaders→CORS）を不変に維持。既存 `internal/handler` 統合テスト群が green（適用順序・適用範囲の退行なし）
- NFR 1.1 — 既存 4 ヘッダー値不変 / `TestSecurityHeaders_ExistingHeaders`
- NFR 1.2 — フラグ未設定時 HSTS 非出力（既定 false 経路）/ `TestLoad_HSTSEnabled/…未設定…`、`TestLoad_DefaultValues`（HSTSEnabled=false）
- NFR 2.1 — CSP を HTTP / HTTPS 双方で常時付与 / `TestSecurityHeaders_CSP`（両サブテスト）
- NFR 2.2 — 各ヘッダー 1 値のみ（全て `Set` 使用）/ `TestSecurityHeaders_NoDuplicateHeaders`

## Findings

なし

## Summary

CSP 無条件付与（Req 1）、HSTS のフラグ + HTTPS 判定による条件付き付与（Req 2 / 3）、既存 4 ヘッダーの
後方互換維持（Req 4 / NFR 1）、CSP 常時付与・重複排除（NFR 2）の全 numeric ID に対応する実装とテストを
確認した。`go test ./internal/middleware/... ./internal/config/... ./internal/handler/...` は green。
boundary 逸脱・AC 未カバー・missing test のいずれも検出されなかった。

RESULT: approve
