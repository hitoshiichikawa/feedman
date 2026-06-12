# 実装ノート: Issue #171 native auth エンドポイントの IP rate limit

## 実装サマリ

Issue #171 の「native auth 3 エンドポイント（`POST /api/auth/token` / `POST /api/auth/refresh` /
`POST /api/auth/revoke`）への未認証 IP 単位レート制限の適用」を、design.md / tasks.md の
指針に厳密に従って実装した。

変更は `router.go` の native auth ルート登録 3 行に既存 `unauthIPMW` を重ねるのみ:

- `unauthIPMW` は route 単位チェーンの最外（MaxBodyBytes より外側 / Logging の内側）に配置。
  閾値超過時はボディ上限 wrap・JSON decode・永続化層参照のいずれにも到達せずに既存共通の
  429（Retry-After 付き）で遮断される（Req 1.1〜1.3 の「処理を試行せずに」/ NFR 2.2 の
  既存順序規約と一致）
- **制限値・インスタンスは既存と共用**（design.md「制限値の設計判断」どおり）:
  env `RATE_LIMIT_UNAUTH_IP`（既定 30 req/min/IP）の単一 `IPRateLimiter` を共用し、
  native auth 専用の制限値・設定ノブ・インスタンスは新設しない
- `IPRateLimiter` 本体・config・app.go の wiring・既存ルート・`RouterDeps` はすべて不変
- `NativeAuthHandler` nil（署名鍵未設定）環境では 3 ルート自体が未登録のため本変更は no-op
  （Req 2.4）。`UnauthIPRateLimiter` nil なら素通し（既存未認証ルートと同一の縮退規約）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | router: 3 ルートへ unauthIPMW 適用 + レート制限テスト | c74b0a2 `feat(handler)` | 57b791a `docs(tasks): mark 1` |
| 2 | 縮退・後方互換の regression テスト | 84810cc `test(handler)` | 3bb1b43 `docs(tasks): mark 2` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約（task 2 → 1）は自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: native auth エンドポイントへの IP 単位レート制限

| AC ID | 担保テスト |
|---|---|
| 1.1 (token 超過で 429 / 処理未試行) | `handler.TestNewRouter_NativeAuthIPRateLimit_429OnExcess`（token サブケース: 429 + service 未呼び出し） |
| 1.2 (refresh 超過で 429 / 処理未試行) | 同上（refresh サブケース） |
| 1.3 (revoke 超過で 429 / 処理未試行) | 同上（revoke サブケース） |
| 1.4 (閾値以内は通常どおり通過) | 同上（各サブケースの 1 回目が 200 / 204 で service 到達）/ `_Degradations`（limiter nil の 5 連続通過） |
| 1.5 (Retry-After ヘッダーで待機時間通知) | `_429OnExcess`（Retry-After 非空検証）/ `_SameShapeAsExistingRoutes` |
| 1.6 (IP ごとの独立カウント) | `handler.TestNewRouter_NativeAuthIPRateLimit_IndependentPerIP`（IP A 枯渇後も IP B は通過） |

### Requirement 2: 既存挙動の後方互換

| AC ID | 担保テスト |
|---|---|
| 2.1 (既存未認証 3 ルートの適用範囲・閾値不変) | 既存 `router_unauth_ratelimit_test.go` が無変更で green（実装も既存 3 ルートの登録・単一インスタンス構成を変更していない） |
| 2.2 (認証済み userID 単位制限の不変) | 認証必須グループに diff なし。既存テスト無変更 green |
| 2.3 (429 応答が既存と同一形式) | `handler.TestNewRouter_NativeAuthIPRateLimit_SameShapeAsExistingRoutes`（/health の 429 と status / Content-Type / Retry-After / JSON ボディを直接比較） |
| 2.4 (native auth 無効環境で導入前と同一挙動) | `handler.TestNewRouter_NativeAuthIPRateLimit_Degradations`（NativeAuthHandler nil + limiter ありで 3 ルート 404 のまま） |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (拒否事象の運用ログ) | 既存 `IPRateLimiter` の拒否ログ（`slog.Warn("rate limit exceeded", limit_type=unauth_ip)`）を変更なしで共用 |
| NFR 1.2 (拒否ログに token・PII を含めない) | 同上（#38 設計のまま。ログは limit 種別 + IP のみ） |
| NFR 2.1 (設定未指定でも既定値で有効化) | config / wiring 不変のため、既定 30 req/min/IP がそのまま native auth ルートにも効く |
| NFR 2.2 (既存ルートの middleware 種類・順序不変) | 既存ルートへの diff ゼロ（native auth ルートへの route 単位 `With` 追加のみ）。既存テスト無変更 green |
| NFR 3.1 (within/over を外部依存なしで検証可能) | 追加テスト 4 本すべて httptest + モック service + 小 burst の `IPRateLimiter` 注入で in-process 完結 |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ pass
- `TEST_DATABASE_URL` を設定した実 PostgreSQL 16 でも `go test -p 1 ./...` 全 pass（CI と同条件）
- `gofmt -l <変更ファイル群>`: 出力なし

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- Components and Interfaces の router 変更（3 行への `unauthIPMW` 追加。
  `r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(...))` の形）
- 制限値の共用判断（新 env / 新インスタンス / config 変更なし）
- File Structure Plan（変更は router.go / router_test.go の 2 ファイルのみ）
- Testing Strategy 1〜6 の全ケース（7〜8 は既存テストの無変更 green 確認で充足）

## 実装上の判断

無し（設計が 1 行 × 3 の機械的適用として確定しており、判断余地のある箇所はなかった）。
テストは既存 `router_unauth_ratelimit_test.go` の `doRouterReq` / burst=1 注入パターンを
踏襲し、3 ルートの table-driven 化と /health 429 との直接比較で形式同一性を担保した。

## 追加した依存

無し（go.mod / go.sum 変更なし。テストの `golang.org/x/time/rate` import は既存依存）。

## 後続 Issue への引き継ぎ事項

- **Issue #172 (contract tests)**: native auth 3 ルートの 429 契約（Retry-After + 共通 JSON）は
  本 spec のテストでカバー済み。
- **将来の閾値分離**: native auth に独立した閾値が必要になった場合は、config に値を追加して
  別 `IPRateLimiter` インスタンスを配線するだけで本設計を変えずに分離できる
  （design.md「制限値の設計判断」の拡張点記録どおり。本 spec では実装していない）。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- 同一 IP が native auth へ大量リクエストすると、同じバケットを共有する `/auth/google/*`・
  `/health` も 429 になる（逆も同様）。これは design.md「共用に伴う既知の性質」として
  設計段階で確定済みの安全側挙動（既存 3 ルート間で既に成立している挙動と同種）。

STATUS: complete
