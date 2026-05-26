# 実装ノート: Issue #37 HSTS / CSP セキュリティヘッダー

本 Issue は Architect を経由しない design-less impl（`design.md` / `tasks.md` 不在）。
要件定義（`requirements.md`）に基づき Developer が実装方針を決定した。

## 実装方針の決定

### CSP（Content-Security-Policy） — Requirement 1

- 全レスポンスに常時 `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'` を付与。
- HTTP / HTTPS いずれの配信でも同一値（フラグ非依存・無条件付与）。値は定数
  `contentSecurityPolicyValue` に集約（マジックストリングの定数化）。

### HSTS（Strict-Transport-Security） — Requirement 2, 3

- HSTS 値: `Strict-Transport-Security: max-age=31536000; includeSubDomains`（定数
  `strictTransportSecurityValue`）。**`preload` は付与しない**（確認事項 1 の方針）。
- HSTS 有効化フラグ: 環境変数 **`HSTS_ENABLED`**（bool、**既定値 `false`**）を `config.Config.HSTSEnabled`
  に追加。未設定時 false = 本機能導入前と等価（NFR 1.2）。不正値時は既定値 false を採用し
  `slog.Warn` を出力して起動継続（Requirement 3.3）。`config.getEnvBool` ヘルパーを新設
  （既存 `getEnvInt` / `getEnvDuration` の Warn パターンに準拠、`strconv.ParseBool` を使用）。
- HTTPS 配信判定（`middleware.isHTTPS`）:
  1. `X-Forwarded-Proto` ヘッダー値が `https` のとき HTTPS と判定（リバースプロキシ配下の主経路、
     Requirement 2.3）。
  2. それ以外で `r.TLS != nil` のとき HTTPS と判定（直結 TLS 終端のフォールバック）。
  3. 上記いずれでもなければ HTTP 扱い（`X-Forwarded-Proto` が `http` または欠落、Requirement 2.4）。
- 付与条件: `HSTSEnabled == true` **かつ** HTTPS 判定時のみ HSTS を付与（Requirement 2.1 / 3.2）。
  フラグ無効時は HTTPS 判定でも非付与（Requirement 3.1）、HTTP 配信時はフラグに関わらず非付与
  （Requirement 2.2）。

### ミドルウェアのシグネチャ変更

- `middleware.NewSecurityHeadersMiddleware()` → `NewSecurityHeadersMiddleware(hstsEnabled bool)` に変更。
- ヘッダー設定は全て `http.Header.Set`（`Add` ではない）を使用し、各ヘッダー名につき 1 値のみ
  設定（重複ヘッダーを生成しない、NFR 2.2）。

### wiring 変更ファイル一覧

- `internal/config/config.go`: `Config.HSTSEnabled` フィールド追加 / `HSTS_ENABLED` 読み込み /
  `getEnvBool` ヘルパー新設。
- `internal/handler/router.go`: `RouterDeps.HSTSEnabled` フィールド追加 /
  `NewSecurityHeadersMiddleware(deps.HSTSEnabled)` 呼び出しに変更（適用位置は従来どおり
  Recovery の次・CORS の前で全ルートに効く）。
- `internal/app/app.go`: `RouterDeps` 構築時に `HSTSEnabled: cfg.HSTSEnabled` を渡す
  （`CORSAllowedOrigin` の流し方に倣う）。
- `internal/middleware/security_headers.go`: ミドルウェア本体実装。
- `.env.sample`: `HSTS_ENABLED`（既定 false）を解説付きでコメントアウト追記。

### 後方互換

- 既存 4 ヘッダー（`X-Content-Type-Options: nosniff` / `X-Frame-Options: DENY` /
  `Referrer-Policy: strict-origin-when-cross-origin` /
  `Permissions-Policy: camera=(), microphone=(), geolocation=()`）の値・全ルート適用を
  一字一句維持（Requirement 4 / NFR 1.1）。CSP は新規常時付与（ヘッダー追加のみで既存挙動非破壊）。
  HSTS は既定 false で非出力のため、未設定環境では導入前と完全に等価。

## AC トレーサビリティ（どのテストで担保したか）

| AC | 担保テスト |
|---|---|
| Req 1.1 CSP を全レスポンスに付与 | `TestSecurityHeaders_CSP`（HTTP / HTTPS 両サブテスト） |
| Req 1.2 `default-src 'none'` を含む | `TestSecurityHeaders_CSP`（値の完全一致で `default-src 'none'` を含む） |
| Req 1.3 `frame-ancestors 'none'` を含む | `TestSecurityHeaders_CSP`（値の完全一致で `frame-ancestors 'none'` を含む） |
| Req 1.4 HTTP 配信時も HTTPS と同一 CSP | `TestSecurityHeaders_CSP/HTTPリクエストのときCSPが付与される`（HTTP でも同一 `wantCSP`） |
| Req 2.1 HTTPS 判定時に HSTS 付与 | `TestSecurityHeaders_HSTS/HSTS有効かつXForwardedProtoがhttpsのとき…`、`TestSecurityHeaders_HSTS_DirectTLS`（r.TLS フォールバック） |
| Req 2.2 HTTP 判定時は HSTS 非付与 | `TestSecurityHeaders_HSTS/…XForwardedProtoがhttpのとき…`、`…欠落のとき…` |
| Req 2.3 `X-Forwarded-Proto: https` → HTTPS | `TestSecurityHeaders_HSTS/…XForwardedProtoがhttpsのとき…` |
| Req 2.4 `X-Forwarded-Proto` が https 以外 → HTTP | `TestSecurityHeaders_HSTS/…httpのとき…`（http）、`…欠落のとき…`（欠落） |
| Req 3.1 フラグ無効時は HTTPS でも非付与 | `TestSecurityHeaders_HSTS/HSTS無効かつHTTPS判定でもHSTSが付与されない` |
| Req 3.2 フラグ有効 + HTTPS で付与 | `TestSecurityHeaders_HSTS/HSTS有効かつXForwardedProtoがhttpsのとき…` |
| Req 3.3 未指定・不正値時は既定値で起動継続 | `TestLoad_HSTSEnabled`（未設定 false / 不正値 false かつ err なし）、`TestGetEnvBool`（不正値 Warn + フォールバック） |
| Req 4.1-4.4 既存 4 ヘッダーの値維持 | `TestSecurityHeaders_ExistingHeaders` |
| Req 4.5 全ルートに付与 | 既存 `router_integration_test.go` のミドルウェア適用順序検証 + `router.go` 適用位置（Recovery→SecurityHeaders→CORS、全ルート最上位）を不変 |
| NFR 1.1 既存 4 ヘッダー値不変 | `TestSecurityHeaders_ExistingHeaders` |
| NFR 1.2 フラグ未設定時 HSTS 非出力 | `TestLoad_HSTSEnabled/…未設定のとき…false`、`TestSecurityHeaders_HSTS/HSTS無効…`（既定 false 経路） |
| NFR 2.1 CSP を常時欠落させない | `TestSecurityHeaders_CSP`（HTTP / HTTPS 双方で付与） |
| NFR 2.2 重複ヘッダーを生成しない | `TestSecurityHeaders_NoDuplicateHeaders`（各ヘッダー出現回数 1） |
| 確認事項 1（preload 非付与） | `TestSecurityHeaders_HSTS_NoPreload` |

> Req 4.5 について: ミドルウェアの全ルート適用は `router.go` の `r.Use(...)` 配置（chi の
> 全ルート共通ミドルウェア）で構造的に担保される。本変更では適用位置・適用範囲を変更して
> いないため、既存の統合テストが全ルート適用の退行を検出する。

## 確認事項（requirements.md Open Questions の引き継ぎ・人間判断が必要）

- **確認事項 1（preload 非付与の方針確定）**: 本実装では `Strict-Transport-Security` に
  `preload` を **付与しない**方針を採用した（HSTS 値 = `max-age=31536000; includeSubDomains`）。
  `TestSecurityHeaders_HSTS_NoPreload` で preload 不在を検証済み。preload list 登録は一度登録
  すると当該ドメイン・サブドメインが恒久的に HTTPS 強制となり取り消しに時間を要するため、
  運用判断（preload list への登録是非）が確定するまでは付与しない。将来 preload 登録を行う
  場合は別途要件化が必要。**この方針で確定してよいか人間の確認を求める。**

- **確認事項 2（X-Forwarded-Proto の信頼境界）**: 本実装は `X-Forwarded-Proto: https` を
  無条件に信頼して HTTPS 配信と判定する（リバースプロキシ配下前提）。Go サーバーへの到達経路が
  信頼できるプロキシ（Cloudflare / リバースプロキシ）経由に限定されていない構成では、信頼できない
  ネットワークから直接到達できる場合にヘッダー偽装で判定を欺かれ得る。ただし HSTS の偽装付与は
  クライアント側に HTTPS 強制を課すのみで、攻撃者にとって直接の利得は乏しい（むしろ可用性低下方向）。
  **実運用で Go サーバーの到達経路が信頼プロキシ経由に限定されている前提でよいか、デプロイ構成に
  依存する判断のため人間の確認を求める。** 信頼プロキシ経由でない構成を許容する必要が出た場合は、
  信頼するプロキシ IP / ヘッダーの allowlist 機構を別途要件化する。

## 追加した環境変数

| 環境変数 | 型 | 既定値 | 用途 |
|---|---|---|---|
| `HSTS_ENABLED` | bool | `false` | HSTS ヘッダーの出力可否。`true` かつ HTTPS 配信判定時のみ HSTS を付与。`false`（既定）の場合は HTTPS でも非付与（後方互換）。`.env.sample` に解説付きで追記済み。 |

## 検証結果

- `go test ./...`: 全パッケージ green（`internal/middleware` / `internal/config` の新規テスト含む）。
- `go vet ./...`: クリーン（exit 0）。
- `go build ./...`: 成功（exit 0）。
- `gofmt -l`: 変更ファイル全て整形済み（差分なし）。
- Red 観測: HSTS 判定を一時的に無効化して HSTS 系テストが落ちることを確認後、復元して green を再確認。

> 環境補足: ローカルの Go は 1.22.2 だが go.mod は `go 1.25` を要求するため、テスト/ビルドは
> `GOTOOLCHAIN=go1.25.0`（完全バージョン指定）で実行した。`GOTOOLCHAIN=auto` のままだと
> `go1.25`（パッチ無し文字列）の toolchain 解決に失敗するため、CI 等で同様の事象が出る場合は
> 完全バージョン指定が必要。これは本機能の実装内容とは独立した環境上の注意点。

STATUS: complete
