# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T08:55:00Z -->

## Reviewed Scope

- Branch: claude/issue-168-impl-reuse-detection-revoke
- HEAD commit: 4d01336
- Compared to: develop..HEAD

## Verified Requirements

### Requirement 1: Rotation 済み token 再利用の検知と family 失効

- **1.1** (再利用提示で family 全体失効 + 拒否) — `internal/auth/token_service.go` の
  `RotateRefreshToken` 手順 2 で `stored.RotatedAt != nil` 分岐に `revokeFamilyOnReuse`
  ヘルパーを差し込み、`RevokeFamily` 実行後に `ErrInvalidRefreshToken` を返す。
  `auth.TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke`（RevokeFamily が当該 FamilyID
  + now で 1 回呼ばれ、新 token 未作成）と `handler.TestIntegration_ReuseDetection_FamilyRevoked`
  Act 2（401 + 拒否応答が reuse/revoked/family を含まない）で担保。
- **1.2** (並行 rotation 競合敗北でも同様に失効) — 手順 3 の `MarkRotated` race 分岐
  （`ErrRefreshTokenAlreadyRotated`）でも同 helper を呼ぶ。
  `auth.TestRotateRefreshToken_RaceLoserEscalatesToFamilyRevoke` で担保。
- **1.3** (失効 family の全 token を以後拒否) — `RevokeFamily` 経由で family と配下 token の
  `RevokedAt` が一括 set される（#164 既存実装に委譲）。
  `handler.TestIntegration_ReuseDetection_FamilyRevoked` Act 3 で「A 再提示後、現役だった
  B も 401」を確認。
- **1.4** (通常拒否と区別できない同一応答) — 昇格時も返すのは同一 sentinel
  `ErrInvalidRefreshToken`。`auth.TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke` /
  `handler.TestIntegration_ReuseDetection_FamilyRevoked`（message に reuse/revoked/family
  を含まないことを検証）で担保。
- **1.5** (失効永続化失敗でも新 token を発行しない) — `revokeFamilyOnReuse` 内で
  `RevokeFamily` の error は `slog.Error` 記録のみで内部エラーへ昇格させず、呼び出し側の
  `ErrInvalidRefreshToken` 拒否を維持する。`auth.TestRotateRefreshToken_ReuseRevokeFamilyFailureStillRejects`
  で `CreateToken` 0 回 / `ErrInvalidRefreshToken` を検証。

### Requirement 2: Revoke エンドポイント

- **2.1** (既知 token で family 失効 + 204) — `TokenService.RevokeRefreshToken` が
  `FindByHash` → `RevokeFamily` を実行し、`NativeAuthHandler.Revoke` が 204 を返す。
  `auth.TestRevokeRefreshToken_KnownToken`（4 状態の table-driven）/
  `handler.TestNativeAuthHandler_Revoke_Success` /
  `handler.TestIntegration_RevokeFlow_RefreshRejectedAndIdempotent` Act 1 で担保。
- **2.2** (不明・各状態でも同一の 204 / 冪等) — `FindByHash` nil で no-op nil（service）/
  handler は同一 204 ボディなし。`auth.TestRevokeRefreshToken_UnknownTokenIsNoop` /
  `handler.TestNativeAuthHandler_Revoke_UnknownTokenAlso204` / 統合 Act 3 (再 revoke) /
  Act 4 (不明 token) で担保。
- **2.3** (revoke 後の refresh 拒否) — `handler.TestIntegration_RevokeFlow_RefreshRejectedAndIdempotent`
  Act 2 で revoke → 同 token refresh → 401 を確認。
- **2.4** (Cookie / Bearer なしで呼び出し可能) — `router.go` の認証不要グループ
  （Session middleware を経由しない）に `POST /api/auth/revoke` を登録。
  `handler.TestNewRouter_NativeAuthRevoke_RegisteredWhenHandlerInjected` が Cookie 無しで
  204 を確認。
- **2.5** (不正 JSON / 必須欠落を拒否) — handler が `json.Decoder.DisallowUnknownFields` +
  `req.RefreshToken == ""` で 400 INVALID_REQUEST を返す。
  `handler.TestNativeAuthHandler_Revoke_InvalidJSON`（4 サブケース）/
  `handler.TestNativeAuthHandler_Revoke_MissingField`（2 サブケース）で担保。
- **2.6** (既存 Web ログアウトの挙動不変) — `internal/handler/router.go` diff は認証不要
  グループ内の `POST /api/auth/revoke` 追加のみで、既存 `POST /auth/logout` 経路に変更なし。
  既存テストが green のまま（NFR 2.1 と整合）。

### Non-Functional Requirements

- **NFR 1.1** (平文を永続化・ログ・エラーに残さない) — service は `HashNativeSecret` 後に
  平文 token を破棄。slog は `hash[:8]` のみ。`auth.TestRevokeRefreshToken_InfraErrorPropagates`
  でエラーメッセージに平文 token が含まれないことを検証。
- **NFR 1.2** (応答から存在有無・状態を推測不能) — 不明 token も既知 token も同一の 204
  ボディなし。`handler.TestNativeAuthHandler_Revoke_UnknownTokenAlso204` / 統合テスト Act 3〜4
  で担保。
- **NFR 1.3** (エラー応答に入力値・内部詳細を反射しない) — 400/500 ともに固定 APIError
  メッセージ。`handler.TestNativeAuthHandler_Revoke_InternalError` が「db connection refused」
  が message に含まれないことを検証。
- **NFR 2.1** (既存ルートの挙動不変) — `RotateRefreshToken` の変更は拒否分岐内部のみ
  （拒否応答 sentinel は変えていない）。`POST /auth/logout` / 既存 API 経路への diff なし。
  `go test ./internal/auth/ ./internal/handler/` 全 pass で確認済み。
- **NFR 2.2** (署名鍵未設定で revoke も非公開) — `NativeAuthHandler != nil` ガード内に
  登録しているため nil 時は revoke も未登録。
  `handler.TestNewRouter_NativeAuthRevoke_NotRegisteredWhenHandlerNil` で 404 を確認。
- **NFR 3.1** (外部ネットワーク依存なし) — auth/handler とも service・store・issuer の
  最小 mock 駆動。全テストが mock のみで構成され、外部ネットワーク・実 DB 接続なしで pass。

## Findings

なし（全 AC カバレッジ、対応テスト追加、boundary 準拠を確認）。

### 補足: #167 既存統合テストの順序再構成に関する独立評価

`TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected` は本実装で
3 つの観点（rotation 成功 / 新世代 token B の有効性 / 旧 token A 再提示の 401 拒否）の
**実行順序を再構成**している（B 有効性検証を A 再提示より前へ移動）。これを assertion の
弱体化に当たるかどうか独立に検証した結果、**弱体化ではない**と判断する。理由:

- 元の 3 観点はすべて維持されている（削除・置換なし。Act ラベルと assert メッセージが
  更新されたのみ）
- 順序入れ替えは #168 Req 1.3（再利用検知後の family 全滅）の必然的帰結。旧順序のままだと
  A 再提示後に family が全滅し、新世代 B も拒否されてしまうため、`refresh with new token
  status = 200` の assert が成立しなくなる
- 「再利用検知後に新世代 token も拒否される」挙動は本 spec の新規 AC であり、独立した
  新テスト `TestIntegration_ReuseDetection_FamilyRevoked` Act 3 で明示的に検証されている
- impl-notes.md「実装上の判断」節でも spec 主導の挙動変更への追従である旨が明文化されている

CLAUDE.md 禁止事項「テストを通すために実装ではなくテスト側を書き換えて弱めること」には
該当しない（assert の削除・緩和ではなく、新 spec に整合させた配置変更）。

## Summary

Issue #168 の AC（Req 1.1〜1.5 / 2.1〜2.6 / NFR 1.1〜1.3 / 2.1〜2.2 / 3.1）すべてに対応する
実装とテストが存在し、design.md の File Structure Plan で許可された 6 ファイル
（+ impl-notes.md / tasks.md）に閉じている。boundary 逸脱なし。新規追加テストおよび既存
テスト全パッケージ pass を確認した（`go test ./internal/auth/ ./internal/handler/`）。
#167 既存統合テストの順序再構成は spec 主導の挙動変更への追従であり、assertion の弱体化に
該当しない。

RESULT: approve
