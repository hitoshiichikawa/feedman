# 実装ノート: 非認証エンドポイントへの IP ベースレート制限（Issue #38）

## (a) 実装方針の要約

未認証エンドポイント（`/auth/google/login`・`/auth/google/callback`・`/health`）に対し、
接続元 IP 単位のレート制限を追加した。既存の userID ベース `RateLimiter` には一切手を加えず、
専用の `IPRateLimiter` 型を新設することで後方互換を確保した（Req 4）。

- **`internal/middleware/clientip.go`（新規）**: `r.RemoteAddr` からクライアント IP を抽出する
  共通ヘルパ `clientIPFromRemoteAddr` を切り出した。`net.SplitHostPort` → `net.ParseIP` を経て
  正規化済み IP 文字列を返し、IP として解釈できない場合は空文字を返す。X-Forwarded-For は
  参照しない（Req 3.1, 3.2）。
- **`internal/middleware/trusted_cidr.go`（変更）**: 重複していた IP 抽出ロジックを
  `clientIPFromRemoteAddr` に委譲するようリファクタした（挙動は不変。既存テスト全通過）。
- **`internal/middleware/ip_ratelimit.go`（新規）**: `IPRateLimiter` 型を新設。`map[string]*userLimiter`
  を IP キーで管理し、既存 `userLimiter` 型と `writeRateLimitResponse` を再利用。`Middleware()` /
  `Stop()` / `LimiterCount()` / cleanup goroutine を持つ。429 応答には Retry-After を付与し、
  拒否ログは `limit_type=unauth_ip` のみでセッション情報・トークンを含めない（Req 1.6, NFR 1.1, 1.2）。
- **`internal/config/config.go`（変更）**: `RateLimitUnauthIP`（env `RATE_LIMIT_UNAUTH_IP`、既定 30、
  不正値は既存 `getEnvInt` により既定フォールバック）を追加（Req 2）。
- **`internal/handler/router.go`（変更）**: `RouterDeps.UnauthIPRateLimiter` を追加し、
  `/health`・`/auth/google/login`・`/auth/google/callback` の 3 ルートにのみ `r.With(...)` で
  個別適用。`/auth/logout`・`/auth/me`・`/metrics` には適用しない。nil の場合は素通し（後方互換）。
- **`internal/app/app.go` / `internal/app/shutdown.go`（変更）**: `IPRateLimiter` を wiring し、
  `shutdownCoordinator` で userID 側と同一の `sync.Once` 保護下で `Stop()` を呼ぶ（goroutine
  リーク防止 / NFR 3.1）。

## (b) 受入基準 → 実装/テスト対応表

| AC | 実装箇所 | 担保テスト |
|---|---|---|
| 1.1 login 超過で 429 | `ip_ratelimit.go` `Middleware` + `router.go` | `TestIPRateLimiter_Returns429WhenLimitExceeded_PerEndpoint/auth_google_login`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/login` |
| 1.2 callback 超過で 429 | 同上 | `..._PerEndpoint/auth_google_callback`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/callback` |
| 1.3 health 超過で 429 | 同上 | `..._PerEndpoint/health`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/health` |
| 1.4 閾値以内は通過 | `Middleware` `limiter.Allow()` | `TestIPRateLimiter_AllowsRequestsWithinLimit` |
| 1.5 IP 独立カウント | `getOrCreateLimiter`（IP キー） | `TestIPRateLimiter_IsolatesRateLimitsPerIP`、`TestNewRouter_UnauthIPRateLimit_IsolatesPerIP` |
| 1.6 Retry-After 通知 | `writeRateLimitResponse` 再利用 | `TestIPRateLimiter_Returns429WithRetryAfterHeader` |
| 2.1 既定 30 req/min/IP | `config.go` `getEnvInt("RATE_LIMIT_UNAUTH_IP", 30)` | `TestLoad_Defaults`（RateLimitUnauthIP=30）、`TestDefaultIPRateLimiterConfig` |
| 2.2 設定値で上書き | 同上 | `TestLoad_CustomValues`（RATE_LIMIT_UNAUTH_IP=15） |
| 2.3 不正値は既定フォールバック | 既存 `getEnvInt` の Warn+fallback | `TestLoad_InvalidRateLimitUnauthIP_FallsBackToDefault` |
| 3.1 接続元アドレスから判定 | `clientIPFromRemoteAddr` | `TestIPRateLimiter_IgnoresXForwardedFor`（RemoteAddr ベース動作） |
| 3.2 X-Forwarded-For 非信頼 | `clientIPFromRemoteAddr`（XFF 参照なし） | `TestIPRateLimiter_IgnoresXForwardedFor` |
| 3.3 判定不能時は無制限通過を許さない | `unknownIPKey` 固定キーで制限 | `TestIPRateLimiter_IndeterminateIP_NotUnlimited` |
| 4.1/4.2/4.3 認証済みは userID 単位を維持 | 既存 `RateLimiter` 無改変 + 3 ルート限定適用 | 既存 `ratelimit_test.go` 全通過、`TestNewRouter_UnauthIPRateLimit_NotAppliedToLogoutAndMe`、`TestNewRouter_UnauthIPRateLimit_NilLimiter_NoRestriction` |
| NFR 1.1 拒否を運用ログに記録 | `slog.Warn("rate limit exceeded", limit_type=unauth_ip)` | `TestIPRateLimiter_*`（ログ出力経路を実行） |
| NFR 1.2 ログにセッション/トークンを含めない | ログ属性は `limit_type` のみ | コードレビュー＋ログ出力経路を通る各テスト |
| NFR 2.1 未設定で既定起動 | 既定 30 で wiring | `TestLoad_Defaults` |
| NFR 2.2 既存ルーティング/順序不変 | `r.With(...)` 個別適用・nil 素通し | `TestNewRouter_UnauthIPRateLimit_NilLimiter_NoRestriction`、既存 router_*_test 全通過 |
| NFR 3.1 一定期間アクセスなし IP を解放 | `cleanupLoop`/`cleanup`（TTL=2×interval） | `TestIPRateLimiter_CleanupRemovesExpiredEntries`、`TestShutdownCoordinator_StopsIPRateLimiterCleanupGoroutine` |

## (c) 決定事項

- **IP 判定不能時の扱い（Req 3.3）**: `r.RemoteAddr` から IP を抽出できない場合、固定キー
  `__unknown_ip__` を割り当て、その単一バケットでまとめてレート制限する。これにより無制限
  通過を許さず（安全側）、かつ判定不能リクエストが他の正常 IP のカウントを汚染しない。
  「拒否（即 403/429）」ではなく「制限付き通過」を選んだ理由は、要件が「無制限通過を許さない」
  に留まり完全拒否を求めていないこと、および判定不能ケース（テスト環境・特殊プロキシ等）で
  正常クライアントを巻き添えに完全遮断するリスクを避けるため。
- **閾値 env var 名**: `RATE_LIMIT_UNAUTH_IP`（既存 `RATE_LIMIT_GENERAL` / `RATE_LIMIT_FEED_REG`
  の命名慣習に準拠）。単位は req/min/IP、既定 30。
- **burst の扱い**: 既存 General/FeedReg と同じく `rate.Limit(reqPerMin/60)` を rate、
  `reqPerMin` を burst に設定（`DefaultIPRateLimiterConfig`）。これにより 1 分あたり最大
  `reqPerMin` 件のバーストを許容しつつ定常レートで補充される。`reqPerMin < 1`（0/負値）は
  最低 1 にフォールバックし、rate=0 による恒常拒否を防ぐ（config 側で不正値は既に既定 30 に
  なっているため通常は発生しないが、二重の安全策）。
- **XFF 非信頼方針**: `trusted_cidr.go` の前例に揃え、クライアント IP 判定は `r.RemoteAddr`
  のみを用いる。X-Forwarded-For は一切参照しない。両ミドルウェアで IP 抽出ロジックを
  `clientIPFromRemoteAddr` に共通化した。
- **適用ルートの限定**: login・callback・health の 3 ルートに `r.With(unauthIPMW)` で個別適用。
  グループ全体 `r.Use` を使わなかったのは、同一グループ内の logout・me・metrics を巻き込まない
  ため。これにより既存ミドルウェア順序（NFR 2.2）も不変に保たれる。
- **goroutine リーク防止**: `IPRateLimiter` も cleanup goroutine を持つため、`shutdownCoordinator`
  に第 3 引数として渡し、userID 側と同じ `sync.Once` 保護下で `Stop()` を呼ぶ。既存
  `shutdown_test.go` の 3 呼び出しは新シグネチャに合わせ第 3 引数 `nil` を渡すよう機械的に
  更新した（テストの検証意図は不変）。

## (d) 確認事項

- なし（requirements.md は確定済みで矛盾は検出されなかった）。

### 補足（スコープ外の既存事情）

- `gofmt -l ./internal/` は本実装と無関係な複数の既存ファイル（`internal/middleware/ratelimit_test.go`
  ほか）を未整形として報告するが、これらはいずれも本 Issue 着手前から存在する baseline の
  整形差分であり、本 Issue のスコープ外のため変更していない。本実装で新規追加・変更した
  ファイルはすべて `gofmt` 整形済みで、`gofmt -l` 報告に含まれない。

STATUS: complete
