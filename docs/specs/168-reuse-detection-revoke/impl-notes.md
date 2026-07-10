# 実装ノート: Issue #168 再利用検知の family 失効と POST /api/auth/revoke

## 実装サマリ

Issue #168 の (1) rotation 済み refresh token 再利用検知時の **family 全体失効への昇格**と
(2) **`POST /api/auth/revoke`** を、design.md / tasks.md の指針に厳密に従って実装した。

設計の主要ポイントは原文どおり踏襲している:

- `RotateRefreshToken` の拒否分岐 2 箇所（手順 2 の RotatedAt 検出 / 手順 3 の
  `ErrRefreshTokenAlreadyRotated` 競合敗北）を `RevokeFamily` 実行 + `ErrInvalidRefreshToken`
  に昇格（Req 1.1 / 1.2）。検知ログは `slog.Warn("refresh token reuse detected", family_id,
  hash 先頭 8 文字)`
- `RevokeFamily` 失敗時も拒否は維持（`slog.Error` で記録のみ、新 token は発行しない /
  Req 1.5。内部エラーへ昇格させず安全側に倒す）
- 昇格後も返すのは同一 sentinel `ErrInvalidRefreshToken` → handler は #167 と同じ
  401 単一応答（Req 1.4: 再利用検知を区別できない）
- RevokedAt 済み（失効済み）token の提示は再利用ではないため**昇格しない**単純拒否のまま
  （#167 既存挙動の回帰維持）
- `RevokeRefreshToken`: 不明 token は no-op nil（存在オラクルなし / Req 2.2）、既知 token は
  状態（期限切れ・rotation 済み・失効済み）に関わらず family 失効（Req 2.1）、infra エラー
  のみ error（500）
- `Revoke` handler: 常に 204 ボディなし（成功 / 不明を区別しない）、400 INVALID_REQUEST /
  500 INTERNAL_ERROR は #167 と同一ヘルパー共用
- router: 認証不要グループの `NativeAuthHandler != nil` ガード内に `POST /api/auth/revoke`
  を登録（ボディ上限 middleware 付き / fail-closed 同乗 / NFR 2.2）。既存 Web ログアウト
  （`POST /auth/logout`）の経路は無変更（Req 2.6）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | auth: 再利用検知の昇格と RevokeRefreshToken | 1f38351 `feat(auth)` | 2ddf9e5 `docs(tasks): mark 1` |
| 2 | handler: Revoke エンドポイントと router 登録 | 14287ce `feat(handler)` | 734d9dd `docs(tasks): mark 2` |
| 3 | 統合テスト: 再利用 family 全滅と revoke 後拒否 | eda6320 `test(handler)` | 76b289f `docs(tasks): mark 3` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約（task 2 → 1、task 3 → 2）は
自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: Rotation 済み token 再利用の検知と family 失効

| AC ID | 担保テスト |
|---|---|
| 1.1 (再利用提示で family 全体失効 + 拒否) | `auth.TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke`（RevokeFamily が当該 FamilyID + now で 1 回 / 新 token 未作成）/ `handler.TestIntegration_ReuseDetection_FamilyRevoked` |
| 1.2 (並行競合敗北でも同様に失効) | `auth.TestRotateRefreshToken_RaceLoserEscalatesToFamilyRevoke`（MarkRotated の ErrRefreshTokenAlreadyRotated 経由） |
| 1.3 (失効 family の全 token を以後拒否) | `handler.TestIntegration_ReuseDetection_FamilyRevoked`（A 再提示後、現役だった B も 401）+ #167 既存の RevokedAt 検証分岐（`auth.TestRotateRefreshToken_Revoked`） |
| 1.4 (通常拒否と区別できない同一応答) | `auth.TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke`（同一 sentinel）/ 統合テスト（401 INVALID_REFRESH_TOKEN・message に reuse / revoked / family を含まない） |
| 1.5 (失効永続化失敗でも新 token を発行しない) | `auth.TestRotateRefreshToken_ReuseRevokeFamilyFailureStillRejects`（RevokeFamily 失敗でも ErrInvalidRefreshToken / CreateToken 0 回） |

### Requirement 2: Revoke エンドポイント

| AC ID | 担保テスト |
|---|---|
| 2.1 (既知 token で family 失効 + 204) | `auth.TestRevokeRefreshToken_KnownToken`（4 状態の table-driven）/ `handler.TestNativeAuthHandler_Revoke_Success` / `handler.TestIntegration_RevokeFlow_RefreshRejectedAndIdempotent` |
| 2.2 (不明・各状態でも同一の 204 / 冪等) | `auth.TestRevokeRefreshToken_UnknownTokenIsNoop` / `handler.TestNativeAuthHandler_Revoke_UnknownTokenAlso204` / 統合テスト（再 revoke 204 / 不明 token 204） |
| 2.3 (revoke 後の refresh を拒否) | `handler.TestIntegration_RevokeFlow_RefreshRejectedAndIdempotent`（revoke → refresh 401） |
| 2.4 (Cookie / Bearer なしで呼び出し可能) | `handler.TestNewRouter_NativeAuthRevoke_RegisteredWhenHandlerInjected`（Cookie 無しで 204） |
| 2.5 (不正 JSON / 必須欠落を拒否) | `handler.TestNativeAuthHandler_Revoke_InvalidJSON`（4 サブケース）/ `_MissingField`（欠落 / 空文字） |
| 2.6 (既存 Web ログアウトの挙動不変) | `POST /auth/logout` 経路への diff なし。既存 `TestSetupAuthRoutes_LogoutEndpoint` ほか全既存テスト無変更 green |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (平文を永続化・ログ・エラーに残さない) | `auth.TestRevokeRefreshToken_InfraErrorPropagates`（エラーメッセージに平文なし）。ログは hash 先頭 8 文字のみ |
| NFR 1.2 (応答から存在有無・状態を推測不能) | `handler.TestNativeAuthHandler_Revoke_UnknownTokenAlso204`（同一 204 / ボディなし）/ 統合テスト Act 3〜4 |
| NFR 1.3 (エラー応答に入力値・内部詳細を反射しない) | `handler.TestNativeAuthHandler_Revoke_InternalError`（内部エラー文字列が message に含まれない） |
| NFR 2.1 (既存ルートの挙動不変) | 変更は RotateRefreshToken の拒否分岐内部 + 追加ルートのみ。`go test ./...` 全パッケージ pass（実 DB 接続でも確認、後述） |
| NFR 2.2 (署名鍵未設定で revoke も非公開) | `handler.TestNewRouter_NativeAuthRevoke_NotRegisteredWhenHandlerNil`（404） |
| NFR 3.1 (外部ネットワーク依存なし) | service 7 ケース / handler 5 ケース / router 2 ケース / 統合 2 ケースすべて mock 駆動 |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ pass
- `TEST_DATABASE_URL` を設定した実 PostgreSQL 16 でも `go test -p 1 ./...` 全 pass
  （CI と同条件。#170 で顕在化した「ローカル skip / CI fail」の再発防止として実施）
- `gofmt -l <変更ファイル群>`: 出力なし

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- 再利用検知の昇格フロー（手順 3 / 4 の拒否分岐拡張。RevokeFamily 失敗時の安全側処理を含む）
- Revoke フロー手順 1〜4（decode → FindByHash → 不明なら no-op / 既知なら RevokeFamily → 204）
- Error Handling 表（400 / 204 / 500 / 401 のマッピング）
- File Structure Plan に列挙された 6 ファイル（新規ファイルなし・migration なし・wiring 変更なし）
- `RefreshTokenStore` への `RevokeFamily` 追加（`repository.RefreshTokenRepository` が
  引き続き構造的に充足。app.go の wiring が具象 repo を直接渡すため compile-time で検証される）
- Testing Strategy 1〜10 の全ケース

## 実装上の判断

### 再利用昇格の共通ヘルパー `revokeFamilyOnReuse`

昇格分岐は 2 箇所（RotatedAt 事前検出 / MarkRotated 競合敗北）で同一処理（slog.Warn →
RevokeFamily → 失敗時 slog.Error）のため、private helper に切り出して重複を避けた
（CLAUDE.md「コピペ重複を増やさない」）。

### 統合テスト mock の family 失効追従と #167 通しテストの順序再構成

`internal/handler/integration_test.go` の stateful mock（`mockNativeTokenExchangeService`）を
family 失効の意味論に追従させた（rotated 集合が family を保持し、再利用検知で
`revokedFamilies` へ昇格）。これに伴い #167 の通しテスト
`TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected` は
「新世代 token の有効性検証」を旧 token 再提示より**前**に行う順序へ再構成した。
旧順序のままだと再利用検知後の family 全滅（本 spec の仕様）により新世代 token の 200 を
検証できないため。検証観点自体（refresh 成功 / 新世代有効 / 旧 token 拒否 / uniform 401）は
すべて維持しており、再利用検知後の family 全滅は新設の
`TestIntegration_ReuseDetection_FamilyRevoked` が担う。

### Revoke のリクエスト型・エラーヘルパー共用

`Revoke` のリクエストボディは refresh と同一形式（`{"refresh_token"}`）のため
`refreshRequest` / `invalidRefreshRequestError` を共用し、新型・新ヘルパーを増やしていない。
`DisallowUnknownFields` も #166 / #167 と同一判断で適用（`handler.TestNativeAuthHandler_
Revoke_InvalidJSON` の未知フィールドサブケースで担保）。

## 追加した依存

無し（go.mod / go.sum 変更なし）。

## 後続 Issue への引き継ぎ事項

- **Issue #169 (Bearer middleware)**: 本 spec の変更とは独立。`TokenExchangeService` IF は
  Exchange / Rotate / Revoke の 3 メソッドに拡張済み。
- **Issue #171 (IP レート制限)**: `POST /api/auth/revoke` も token / refresh と同じく
  IP 単位レート制限を経由しない。router.go の登録部位 3 ルートすべてに `unauthIPMW` を
  重ねる変更が想定される（コメント明記済み）。
- **Issue #172 (contract tests)**: revoke の契約（204 / 400 / 500、冪等性）は本 spec の
  handler / 統合テストでカバー済み。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- #167 の通しテストの検証順序を再構成した（上記「実装上の判断」参照。検証観点は不変、
  spec 主導の挙動変更への追従であり assert の弱体化ではない）。
- `RevokeFamily` 失敗時に 500 ではなく 401 を返す（Req 1.5 / design.md の明示指示どおり。
  通常の DB 障害（FindByHash 失敗等）は従来どおり 500）。

STATUS: complete
