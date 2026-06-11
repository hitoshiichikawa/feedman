# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T00:00:00Z -->

## Reviewed Scope

- Branch: `claude/issue-167-impl-auth-refresh-rotation`
- HEAD commit: `5ff825c417a30ec92eea48d3f6c3011af6c1fcb8`
- Compared to: `merge-base(develop, HEAD)..HEAD`
- 変更ファイル数: 9（spec 2 / 実装 2 / テスト 4 / router 1）
- 変更行数: +1429 / -17

### CLAUDE.md / Feature Flag Protocol 確認

- `feedman/CLAUDE.md` の `## Feature Flag Protocol` の `**採否**: opt-out`。
  opt-in 細目（旧パス削除 / `if (flag)` 分岐 / flag-off 不変 / 命名規約）は
  **適用しない**。通常の 3 カテゴリ判定のみで評価した。

## Verified Requirements

### Requirement 1（rotation 付き再発行の成功パス）

- **1.1** — `internal/handler/native_auth_handler.go:Refresh` が `tokenResponse`
  で `access_token` / `refresh_token` / `token_type: "Bearer"` / `expires_in` (900) を
  返却。`internal/handler/native_auth_handler_test.go:TestNativeAuthHandler_Refresh_Success`
  および統合テスト `TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected`
  で 4 フィールド検証。
- **1.2** — `internal/auth/token_service.go:RotateRefreshToken` 手順 3 で
  `MarkRotated(stored.ID, now)`。
  `TestRotateRefreshToken_Success`（`lastMarkID == stored.ID`）/ 統合テスト（旧 token
  再 refresh で 401）で担保。
- **1.3** — 新 token の `FamilyID: stored.FamilyID` / `TokenHash: refreshHash`、
  `CreateFamily` 未呼び出し。`TestRotateRefreshToken_Success` が同一 family / hash 一致 /
  平文非保存を検証。
- **1.4** — `ExpiresAt: now.Add(RefreshTokenTTL)`（30 日）。
  `TestRotateRefreshToken_Success` が `wantExpiresAt == now+30d` で検証。
- **1.5** — `internal/handler/router.go` で認証不要グループ（Session/Bearer middleware
  非適用）に登録。`TestNewRouter_NativeAuthRefresh_DoesNotRequireSession` が
  Cookie なしで 200 を確認。
- **1.6** — `tokenResponse` 構造体を `Token` handler と共用。
  `TestNativeAuthHandler_Refresh_Success` が snake_case 4 フィールドを検証。

### Requirement 2（再発行の拒否パス）

- **2.1** — `FindByHash` nil 戻り → `ErrInvalidRefreshToken`。
  `TestRotateRefreshToken_NotFound` / 統合テスト
  `TestIntegration_RefreshFlow_UnknownRefreshTokenReturns401` で担保。
- **2.2** — `!stored.ExpiresAt.After(now)`（境界値 `==` も無効として扱う）。
  `TestRotateRefreshToken_Expired` の 2 サブテスト（`<` と `==`）で検証。
- **2.3** — `stored.RevokedAt != nil` → `ErrInvalidRefreshToken`。
  `TestRotateRefreshToken_Revoked` で担保。
- **2.4** — `stored.RotatedAt != nil` → `ErrInvalidRefreshToken`（事前判定 + atomic）。
  `TestRotateRefreshToken_AlreadyRotated` および統合テストの 2 回目 refresh で担保。
- **2.5** — `Refresh` handler で `json.Decoder` decode 失敗 / `req.RefreshToken == ""`
  → 400 INVALID_REQUEST。`TestNativeAuthHandler_Refresh_InvalidJSON` /
  `_MissingField`（空文字含む 2 ケース）で担保。
- **2.6** — 全拒否を `auth.ErrInvalidRefreshToken` sentinel に正規化、handler は
  `errors.Is` で 401 INVALID_REFRESH_TOKEN の固定 message を返却。
  `TestNativeAuthHandler_Refresh_InvalidRefreshToken` が sentinel + `%w` wrap 両方で
  同一応答を検証し、`message` に "expired" / "revoked" / "rotated" / "unknown" が
  含まれないことを assert。
- **2.7** — 全拒否ケースのテストで `createTokenCalls == 0` を検証。race 敗者
  ケース（`_MarkRotatedRaceLoser`）でも新 token は永続化されない。

### Requirement 3（並行 rotation の安全性）

- **3.1** — `MarkRotated` の atomic UPDATE（`WHERE rotated_at IS NULL` / #164 設計）を
  正本ゲートとして利用し、`ErrRefreshTokenAlreadyRotated` を `ErrInvalidRefreshToken` に
  正規化。`TestRotateRefreshToken_MarkRotatedRaceLoser` が
  `markRotatedCalls == 1` + `createTokenCalls == 0` で「race 敗者 = 高々 1 件のみ成功」を担保。

### Non-Functional Requirements

- **NFR 1.1** — `generateRefreshToken`（#166 既存、`crypto/rand` 32 byte = 256 bit）を
  共用。`TestRotateRefreshToken_Success` が TokenHash 64 文字（SHA-256 hex）であることを
  間接的に検証。
- **NFR 1.2** — 永続化は `HashNativeSecret(plain)` のみ、平文はレスポンス JSON のみ。
  ログは hash 先頭 8 文字（`slog.Info("refresh token rotation succeeded", ...,
  slog.String("old_refresh_token_hash", tokenHash[:8]), ...)`）。
  `TestRotateRefreshToken_Success`（`TokenHash != plain`）/
  `_DoesNotLeakPlainSecretsInError` で担保。
- **NFR 1.3** — エラー応答は固定 `APIError` メッセージで、内部エラー文字列を反射しない。
  `TestNativeAuthHandler_Refresh_InternalError` が `"db connection refused"` が response
  body に含まれないことを assert。
- **NFR 2.1** — handler / service / router の追加のみ、既存ルートとロジック不変。
  `go test ./internal/auth/... ./internal/handler/...` を実行し全パッケージ pass を
  確認した（cached 含む）。
- **NFR 2.2** — `router.go` の `if deps.NativeAuthHandler != nil` ガード内に
  `POST /api/auth/refresh` を登録。`TestNewRouter_NativeAuthRefresh_NotRegisteredWhenHandlerNil`
  が nil 時 404 を担保。
- **NFR 3.1** — service / handler / router / 統合の全テストが mock 駆動で外部
  ネットワーク・DB を必要としない（`testing` パッケージのみ）。

### Boundary 確認

design.md File Structure Plan で列挙された 6 ファイル
（`internal/auth/token_service.go` / `_test.go`、`internal/handler/native_auth_handler.go` /
`_test.go`、`internal/handler/router.go` / `_test.go`、`internal/handler/integration_test.go`）
+ spec 内 2 ファイル（`tasks.md` / `impl-notes.md`）のみが変更されており、
File Structure Plan の境界に完全に合致する。

- handler 層に SQL / 認可ロジックは含まれない（service 層に集約）。CLAUDE.md
  「アーキテクチャと機能追加ガイド §1 レイヤリング」遵守。
- repository 層は変更なし（#164 / #166 の既存 IF を構造的に拡張するのみ）。
- `internal/repository/` / `internal/middleware/` / `internal/model/` /
  `internal/security/` / migration 等への波及なし。
- 既存ファイル `integration_test.go` への変更は #167 範囲（rotation 通し）に
  限定されており、既存 `mockNativeTokenExchangeService` の `ExchangeAuthCode` 動作は
  refresh token 平文の活性化を追加するのみで、token 交換側の AC（Issue #166）に
  影響を与えない（既存テスト全 pass）。

## Findings

なし（approve）。

## Summary

Issue #167 の AC（Req 1.1〜1.6, 2.1〜2.7, 3.1, NFR 1.1〜1.3 / 2.1〜2.2 / 3.1）すべてに
対応する実装と単体・統合テストが揃っており、design.md File Structure Plan の境界内に
収まっている。`go test ./internal/auth/... ./internal/handler/...` も pass。CLAUDE.md
「Feature Flag Protocol」は opt-out のため flag 細目は適用しない。3 カテゴリ
（AC 未カバー / missing test / boundary 逸脱）いずれにも該当する findings なし。

RESULT: approve
