# Design Document

## Overview

**Purpose**: 本 spec は #166〜#168 が導入する native auth の 3 エンドポイント
（`POST /api/auth/token` / `POST /api/auth/refresh` / `POST /api/auth/revoke`）に、
#38 導入済みの未認証 IP 単位レート制限（`middleware.IPRateLimiter`、router 上の
`unauthIPMW`）を適用する（SERVER.md §1.7）。

**Users**: サービス運用者（未認証 token 系エンドポイントへのフラッディング・総当たりの
入口を制限したい）。正常な Feedman iOS クライアントの利用頻度（token 交換 ≈ ログイン毎 1 回・
refresh ≈ 15 分毎 1 回・revoke ≈ ログアウト毎 1 回）は閾値に対して十分小さく、影響しない。

**Impact**: 変更は `router.go` の native auth ルート登録 3 行に `unauthIPMW` を重ねるのみ。
`IPRateLimiter` 本体・config・app.go の wiring・既存ルートはすべて不変。
`NativeAuthHandler` nil（署名鍵 `NATIVE_AUTH_JWT_SECRET` 未設定）の環境では 3 ルート自体が
未登録のため、**本 spec の変更は no-op**（#166 の fail-closed に同乗）。

### Goals

- native auth 3 ルートへの IP 単位レート制限の適用（Req 1.1〜1.4, 1.6）
- 閾値超過時は handler / service に到達する前に既存共通の 429 応答（Retry-After 付き）で
  遮断する（Req 1.1〜1.3, 1.5, 2.3）
- 既存未認証 3 ルート・認証済み API・縮退環境（native auth 無効）の完全な後方互換
  （Req 2.1, 2.2, 2.4）

### Non-Goals

- `IPRateLimiter` の実装・IP 判定方針・資源管理・閾値設定機構の変更（#38 設計のまま）
- native auth 専用の制限値・設定ノブの新設（後述「制限値の設計判断」で共用を採用）
- Bearer middleware（#169）/ contract tests（#172）/ 認証済み API の制限変更

## 前提（#166〜#168 design への依存）

本 spec の実装は #166〜#168 の実装 PR が merge 済みであることを前提とする（requirements.md
の `Depends on:` どおり）。本 design が参照する router 上のルート登録は、#166 / #167 / #168
各 design の「router（変更）」節で確定済みの内容（認証不要グループ内・
`NativeAuthHandler != nil` ガード・ボディ上限ミドルウェア付き）である。#166 design は
「IP レート制限（`unauthIPMW`）の適用は Issue #171 の領分」と明記しており、本 spec が
その残置部分を埋める。

## Architecture Pattern & Boundary Map

認証不要グループのミドルウェアチェーンに、既存未認証ルートと同じ位置づけで `unauthIPMW` を
挿入する。

```
（認証不要グループ共通: Recovery → SecurityHeaders → CORS → Logging）
  /health               : unauthIPMW → handler                               （既存・不変）
  /auth/google/login    : unauthIPMW → handler                               （既存・不変）
  /auth/google/callback : unauthIPMW → handler                               （既存・不変）
  /api/auth/token       : unauthIPMW → MaxBodyBytes → NativeAuthHandler.Token   （unauthIPMW を追加）
  /api/auth/refresh     : unauthIPMW → MaxBodyBytes → NativeAuthHandler.Refresh （unauthIPMW を追加）
  /api/auth/revoke      : unauthIPMW → MaxBodyBytes → NativeAuthHandler.Revoke  （unauthIPMW を追加）
```

- `unauthIPMW` は route 単位チェーンの最外（Logging の内側）に置く。閾値超過時はボディ上限の
  wrap・JSON decode・永続化層参照のいずれにも到達せずに遮断され（Req 1.1〜1.3 の
  「処理を試行せずに」）、既存ルートの「Logging の内側に IP 制限」という順序規約とも一致する
  （NFR 2.2）
- 429 応答も既存ルート同様にアクセスログへ記録される（可観測性の一貫性）

## 制限値の設計判断（共用 vs 分離）

**判断: 既存の `IPRateLimiter` インスタンスと閾値（env `RATE_LIMIT_UNAUTH_IP`、
既定 30 req/min/IP）をそのまま共用する。native auth 専用の制限値・インスタンスは新設しない。**

| 観点 | 共用（採用） | 分離（不採用） |
|---|---|---|
| 設定構造 | 既存ノブ（`RateLimitUnauthIP`）1 個のまま | 新 env + config フィールド + 第 2 `IPRateLimiter` インスタンス + cleanup goroutine + shutdown coordinator 配線が増える |
| 正常トラフィック | token 交換 ≈ ログイン毎 / refresh ≈ 15 分毎 / revoke ≈ ログアウト毎。30 req/min/IP は NAT 配下の多数台でも余裕（refresh 定常で約 450 台/IP 相当） | 専用値を必要とするトラフィック特性が現状存在しない |
| 防御の位置づけ | auth_code / refresh token は 256bit 乱数 + hash 永続化（#164〜#168）で列挙は計算量的に不可能。制限は多層防御 + 資源保護であり、OAuth 入口（/auth/google/*）と同強度で釣り合う | 攻撃面の差が薄く、強度を変える根拠が立たない |
| 既存意味論との整合 | 「IP あたりの未認証リクエスト予算」という #38 の意味論（login / callback / health は既に IP ごとの単一バケットを共有）に native auth が加わるだけ | エンドポイント別バケットは #38 意味論からの逸脱になる |

**共用に伴う既知の性質**: 同一 IP が native auth エンドポイントへ大量リクエストすると、
その IP からの `/auth/google/*`・`/health` も同じバケットで 429 になる（逆も同様）。これは
「同一の攻撃元を全未認証入口で一括制限する」安全側の性質であり、既存 3 ルート間で既に
成立している挙動と同種である（既存ルートの閾値・適用範囲・429 形式は不変のため Req 2.1 の
後方互換と矛盾しない）。将来 native auth に独立した閾値が必要になった場合は、config に
値を追加して別インスタンスを配線するだけで本設計を変えずに分離できる（拡張点の記録のみ。
本 spec では実装しない）。

## Technology Stack

新規依存なし。`internal/middleware` の既存 `IPRateLimiter`（`golang.org/x/time/rate`）と
共通 429 応答ヘルパー `writeRateLimitResponse`（Retry-After ヘッダー + 固定 JSON）を
変更なしで使用する。

## File Structure Plan

```
internal/
├── handler/
│   ├── router.go        # 変更: NativeAuthHandler != nil ガード内の 3 ルート登録に
│   │                    #       unauthIPMW を追加（既存 unauthIPMW 変数を共用）
│   └── router_test.go   # 変更: native auth 3 ルートの within/over limit・IP 独立・
│                        #       429 形式・縮退（nil handler / nil limiter）ケースを追加
```

config / app.go / middleware / 既存テストファイルの変更なし。

## Requirements Traceability

| Req | 設計要素 |
|---|---|
| 1.1, 1.2, 1.3 | 3 ルートの route チェーン最外に `unauthIPMW`。超過時は handler 到達前に 429（モック service 未呼び出しで検証） |
| 1.4 | 閾値以内は素通しで後続（MaxBodyBytes → handler）へ。既存 `IPRateLimiter.Middleware` の挙動を共用 |
| 1.5 | 共通 `writeRateLimitResponse` の Retry-After ヘッダー（変更なしで共用） |
| 1.6 | `IPRateLimiter` の IP 別バケット（#38 実装。本 spec は適用のみで変更しない） |
| 2.1 | 既存 3 ルートの登録・`RATE_LIMIT_UNAUTH_IP` 閾値・単一インスタンス構成は無変更（「制限値の設計判断」参照） |
| 2.2 | 認証必須グループ（Session → RateLimit(General) → Logging）に変更なし |
| 2.3 | 既存と同一の `writeRateLimitResponse`（429 + Retry-After + 固定 JSON）を共用 |
| 2.4 | `NativeAuthHandler` nil 時は 3 ルート未登録のため本変更は no-op（#166 fail-closed に同乗） |
| NFR 1.1 | 既存 `IPRateLimiter` の拒否ログ（`slog.Warn("rate limit exceeded", limit_type=unauth_ip)`）を共用 |
| NFR 1.2 | 同上（ログは limit 種別のみで token・セッション情報・IP 以外の個人情報なし。#38 設計） |
| NFR 2.1 | 設定・wiring 不変。既定 30 req/min/IP がそのまま native auth ルートにも効く |
| NFR 2.2 | 既存ルートのミドルウェア種類・順序は無変更（native auth ルートへの route 単位 `With` 追加のみ） |
| NFR 3.1 | `router_test.go` で httptest + モック service による within/over の検証（Testing Strategy 1〜4） |

## Components and Interfaces

### router（変更 / router.go のみ）

```go
// 認証不要グループ内（#166〜#168 で確定済みの登録に unauthIPMW を追加）
if deps.NativeAuthHandler != nil {
	r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
		Post("/api/auth/token", deps.NativeAuthHandler.Token)
	r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
		Post("/api/auth/refresh", deps.NativeAuthHandler.Refresh)
	r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).
		Post("/api/auth/revoke", deps.NativeAuthHandler.Revoke)
}
```

- `unauthIPMW` は既存定義（`deps.UnauthIPRateLimiter` nil なら素通しの no-op closure）を
  そのまま参照する。`UnauthIPRateLimiter` nil + `NativeAuthHandler` 非 nil の組合せでは
  native auth ルートは制限なしで到達可能（既存未認証ルートと同一の縮退規約）
- `RouterDeps` / app.go / config の変更は不要（双方とも #38 / #166 の配線に同乗）

## Data Models

なし（新規モデル・migration・設定フィールドの追加なし）。

## Error Handling

| 状況 | HTTP | 応答 | 備考 |
|---|---|---|---|
| 閾値超過 | 429 | `Retry-After: <秒>` ヘッダー + JSON `{"code": "rate_limit_exceeded", "message": ..., "category": "system", "action": ...}` | 既存 `writeRateLimitResponse` を共用（Req 2.3）。handler 未到達のため `INVALID_GRANT` / `INVALID_REFRESH_TOKEN` 等の native auth エラー契約（#166〜#168）とは独立 |
| 閾値以内 | - | 各 handler の既存契約（#166〜#168）どおり | 本 spec では変更しない |

## Testing Strategy

### ルーティングテスト（router_test.go 追加分）

1. 閾値超過: 小さい burst の `IPRateLimiter` を注入し、burst を使い切った後の
   `POST /api/auth/token` が 429 + Retry-After ヘッダー付きで応答し、モック service が
   呼ばれないこと（Req 1.1, 1.5）
2. 同様に `/api/auth/refresh`（Req 1.2）・`/api/auth/revoke`（Req 1.3）の超過 429 +
   モック service 未呼び出し
3. 閾値以内: 3 ルートとも通常応答（モック service への到達）が維持されること（Req 1.4）
4. IP 独立: 別 IP（RemoteAddr 差し替え）からの要求は超過 IP の影響を受けないこと（Req 1.6）
5. 429 応答 JSON・ヘッダーが既存未認証ルート（例: /health）の 429 と同一形式であること
   （Req 2.3）

### 縮退・後方互換 regression

6. `NativeAuthHandler` nil なら 3 ルートは 404 のまま（= 本変更は no-op。Req 2.4）。
   `UnauthIPRateLimiter` nil なら 3 ルートは制限なしで到達（既存縮退規約の維持）
7. 既存未認証 3 ルート（/health・/auth/google/login・/auth/google/callback）の IP 制限挙動と
   既存テストが無変更で green（Req 2.1, NFR 2.2）
8. 認証必須グループ（/api/*）の既存テストが無変更で green（Req 2.2）

## Security Considerations

- 本制限は多層防御: auth_code / refresh token 自体が 256bit 乱数 + hash 永続化
  （#164〜#168）であり、レート制限は列挙対策の主防御ではなく資源保護・フラッディング抑止
- クライアント IP 判定は接続元アドレスのみ（X-Forwarded-For 非信頼）・判定不能時は固定キーの
  単一バケットで安全側 — いずれも #38 設計を変更なしで継承
- 署名鍵未設定環境では 3 ルート未登録（fail-closed）のため、本変更による新たな露出はゼロ
  （no-op）

## Supporting References

- feedman-ios `design/SERVER.md` §1.7（未認証エンドポイントへの IP rate limit 要求）
- `docs/specs/38--auth-google-health-ip/`（IPRateLimiter の要件・IP 判定方針・既定閾値の正本）
- `docs/specs/166-auth-token-exchange/design.md` / `docs/specs/167-auth-refresh-rotation/design.md` /
  `docs/specs/168-reuse-detection-revoke/design.md`（router 節 = 本 spec が前提とするルート登録。
  #166 design が IP レート制限を #171 の領分として明示）
