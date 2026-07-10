# 実装ノート: Issue #167 POST /api/auth/refresh（rotation 付き再発行）

## 実装サマリ

Issue #167 の `POST /api/auth/refresh` を、design.md / tasks.md の指針に厳密に従って実装した。
有効な refresh token を rotation（旧 token の使用不可化 + 同一 family での新 token 発行）し、
新しい access token（HS256 JWT / 15 分）と refresh token（256bit / hash 保存 / スライディング
30 日）を #166 と同形の JSON で返すエンドポイントを、認証不要グループに追加した。

設計の主要ポイントは原文どおり踏襲している:

- 拒否は単一 sentinel `auth.ErrInvalidRefreshToken` に正規化し、handler は単一の
  401 `INVALID_REFRESH_TOKEN` を返す（Req 2.6 の uniform 拒否 / SERVER.md §1.3）
- rotation の正本ゲートは `MarkRotated` の atomic UPDATE（`WHERE rotated_at IS NULL`、
  #164 設計）。事前の状態検証（RevokedAt / ExpiresAt / RotatedAt）は応答高速化のための
  事前判定で、並行 rotation の高々 1 件成功は手順 3 が保証する（Req 3.1）
- 新 token は同一 `FamilyID`・`ExpiresAt = now + RefreshTokenTTL`（30 日スライディング、
  Req 1.3 / 1.4）。乱数生成・hash は #166 の `generateRefreshToken` / `HashNativeSecret` を共用
- `NATIVE_AUTH_JWT_SECRET` 未設定環境では `NativeAuthHandler` が nil となり token / refresh
  両ルートとも未登録（404 / fail-closed、NFR 2.2。#166 の縮退に同乗、追加の縮退制御なし）
- 平文 refresh token は応答 JSON のみ。ログは hash 先頭 8 文字（NFR 1.2 / 1.3）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | auth: TokenService.RotateRefreshToken を追加 | f67aeee `feat(auth)` | b8cd562 `docs(tasks): mark 1` |
| 2 | handler: Refresh エンドポイントと router 登録 | 77d3342 `feat(handler)` | 9d5df17 `docs(tasks): mark 2` |
| 3 | 統合テスト: token 交換 → refresh → 旧 token 拒否 | 4d908b2 `test(handler)` | 977af9e `docs(tasks): mark 3` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約（task 2 → 1、task 3 → 2）は
自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: Rotation 付き再発行の成功パス

| AC ID | 担保テスト |
|---|---|
| 1.1 (新 4 値 JSON 返却) | `handler.TestNativeAuthHandler_Refresh_Success` / `handler.TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected` |
| 1.2 (旧 token rotation 確定・以後使用不可) | `auth.TestRotateRefreshToken_Success`（MarkRotated が旧 ID で呼ばれる）/ 統合テスト（旧 token 再 refresh → 401） |
| 1.3 (hash 保存 / 同一 rotation family) | `auth.TestRotateRefreshToken_Success`（TokenHash = HashNativeSecret(新平文) / FamilyID・UserID 同一） |
| 1.4 (スライディング 30 日) | `auth.TestRotateRefreshToken_Success`（ExpiresAt ≈ now + 30d） |
| 1.5 (Cookie / Bearer なしで呼び出し可能) | `handler.TestNewRouter_NativeAuthRefresh_DoesNotRequireSession` |
| 1.6 (token 交換と同一フィールド名) | `handler.TestNativeAuthHandler_Refresh_Success`（access_token / refresh_token / token_type / expires_in 検証） |

### Requirement 2: 再発行の拒否パス

| AC ID | 担保テスト |
|---|---|
| 2.1 (token 不明を拒否) | `auth.TestRotateRefreshToken_NotFound` / `handler.TestIntegration_RefreshFlow_UnknownRefreshTokenReturns401` |
| 2.2 (期限切れを拒否) | `auth.TestRotateRefreshToken_Expired` |
| 2.3 (失効済みを拒否) | `auth.TestRotateRefreshToken_Revoked` |
| 2.4 (rotation 済みを拒否) | `auth.TestRotateRefreshToken_AlreadyRotated` / 統合テスト（旧 token 再 refresh → 401） |
| 2.5 (不正 JSON / 必須欠落を拒否) | `handler.TestNativeAuthHandler_Refresh_InvalidJSON` / `_MissingField` |
| 2.6 (拒否応答 uniform 化) | `handler.TestNativeAuthHandler_Refresh_InvalidRefreshToken`（sentinel と wrap の両方で同一 401 / 詳細反射なし）+ service 全拒否パスの sentinel 正規化検証 |
| 2.7 (拒否時に新 token 永続化・状態変更なし) | `auth.TestRotateRefreshToken_NotFound` / `_Expired` / `_Revoked` / `_AlreadyRotated` / `_MarkRotatedRaceLoser`（いずれも CreateToken 0 回） |

### Requirement 3: 並行 rotation の安全性

| AC ID | 担保テスト |
|---|---|
| 3.1 (高々 1 件のみ成功) | `auth.TestRotateRefreshToken_MarkRotatedRaceLoser`（MarkRotated が ErrRefreshTokenAlreadyRotated を返す race 敗者 → ErrInvalidRefreshToken / CreateToken 0 回） |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (256bit エントロピー) | #166 の `generateRefreshToken`（crypto/rand 32 byte → base64url）を共用。`auth.TestRotateRefreshToken_Success` で TokenHash 長 64 文字を検証 |
| NFR 1.2 (平文を永続化・ログに残さない) | `auth.TestRotateRefreshToken_Success`（保存 TokenHash != 平文）/ `_DoesNotLeakPlainSecretsInError`。slog は hash 先頭 8 文字のみ |
| NFR 1.3 (エラー応答に入力値・内部詳細を反射しない) | `handler.TestNativeAuthHandler_Refresh_InternalError`（内部エラー文字列がメッセージに含まれない）/ `auth.TestRotateRefreshToken_DoesNotLeakPlainSecretsInError` |
| NFR 2.1 (既存ルートの挙動不変) | 全既存テスト green（`go test ./...` 全パッケージ pass）。既存 `createIntegrationRouter` / 既存ルート登録は不変 |
| NFR 2.2 (署名鍵未設定で refresh も非公開) | `handler.TestNewRouter_NativeAuthRefresh_NotRegisteredWhenHandlerNil`（404） |
| NFR 3.1 (外部ネットワーク依存なし) | service 9 ケース / handler 6 ケース / router 3 ケース / 統合 2 ケースすべて mock 駆動で DB / net 不要 |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ pass（2026-06-12 引き継ぎセッションで再確認済み）
- `gofmt -l <変更ファイル群>`: 出力なし（フォーマット差分なし）

DB 結合テスト（`internal/repository/*_db_test.go`）は `TEST_DATABASE_URL` 未設定時に skip
する既存設計（CI では postgres サービス上で実行される）。本 spec は repository 層に変更を
加えていない。

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- Rotation フロー手順 1〜7（JSON decode → FindByHash → 状態検証 → MarkRotated →
  新 token 発行 → JWT 発行 → 応答）
- Error Handling 表（400 INVALID_REQUEST / 401 INVALID_REFRESH_TOKEN / 500 INTERNAL_ERROR）
- File Structure Plan に列挙された全ファイル（追加 / 変更ともに同一パス・同一責務。
  新規ファイルなし・migration なし・wiring 変更なし）
- `RefreshTokenStore` の `FindByHash` / `MarkRotated` 拡張（`repository.RefreshTokenRepository`
  が引き続き構造的に充足することを既存の compile-time check で確認）
- Testing Strategy 1〜10 の全ケース

## 実装上の判断

### `DisallowUnknownFields` の採用（#166 と同一判断）

`Refresh` handler も #166 の `Token` handler と同じく `json.Decoder.DisallowUnknownFields()`
で未知フィールドを拒否する。device_label 等の任意メタデータが Out of Scope（requirements.md）
に明記されているため、フォーマット契約を厳密に保つ。`handler.TestNativeAuthHandler_Refresh_
UnknownFieldRejected` で担保。エンドポイント間で JSON 受理規則が揃う。

### FindByHash のインフラエラーは 401 に正規化しない

`FindByHash` がインフラ起因のエラーを返した場合は `ErrInvalidRefreshToken` に正規化せず
`%w` で wrap して 500 に振り分ける（token の有効性を判定できていないため、401 で「無効」と
断定しない）。`auth.TestRotateRefreshToken_LookupErrorPropagates` で担保。design.md の
Error Handling 表「上記以外 → 500」に従った分類。

### 部分失敗（rotation 確定後の発行失敗）

`MarkRotated` 成功後に新 token 生成・永続化・JWT 発行が失敗した場合、旧 token は消費済みの
まま 500 を返す（クライアントは再ログイン）。requirements.md Open Questions の「安全側に
倒して燃やす」方針どおり rollback は導入していない。`auth.TestRotateRefreshToken_
CreateTokenFailure` で「ErrInvalidRefreshToken ではないこと」を担保。

## 追加した依存

無し（#166 までの依存で完結。go.mod / go.sum 変更なし）。

## 後続 Issue への引き継ぎ事項

- **Issue #168 (再利用検知 + revoke)**: 本 spec の拒否分岐のうち以下の 2 箇所が #168 で
  「family 全体失効（RevokeFamily）+ 同一 401」へ昇格する拡張点（design.md Security
  Considerations で局所化済み）:
  1. `RotateRefreshToken` 手順 2 の `stored.RotatedAt != nil` 分岐（rotation 済み token の再利用検知）
  2. 手順 3 の `ErrRefreshTokenAlreadyRotated` 分岐（並行 race 敗者 = 同時再利用）
  また `RefreshTokenStore` IF は `FindByHash` / `MarkRotated` まで公開済み。#168 で
  `RevokeFamily` の公開拡張が必要。
- **Issue #169 (Bearer middleware)**: 本 spec で発行される access token の検証側。変更なし。
- **Issue #171 (IP レート制限)**: `POST /api/auth/refresh` も `/api/auth/token` と同じく
  IP 単位レート制限を経由しない。`router.go` の登録部位に `unauthIPMW` を重ねる追加変更が
  想定される（router.go のコメントに #171 の領分である旨を明記済み）。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- `Refresh` handler は #166 と同じく `DisallowUnknownFields` を採用（design.md /
  requirements.md には明示されていない判断だが、#166 実装判断との整合を優先）。
- `FindByHash` インフラエラーの 500 分類（401 に正規化しない）は design.md Error Handling
  表の解釈として実装した判断。

STATUS: complete
