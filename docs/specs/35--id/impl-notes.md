# 実装メモ（Issue #35: セッション固定攻撃対策 / セッション ID 旋回）

## 採用した設計判断と根拠

### 旧セッション無効化を Handler 層（`Callback`）で実施

Issue が委ねた「旧セッション破棄を Handler 層で行うか Service 層に集約するか」の判断について、
**Handler 層（`internal/handler/auth_handler.go` の `Callback`）で実施**する方針を採用した。
根拠:

1. `Callback` は既にリクエスト Cookie（`oauth_state`）を読んでおり、旧 `session_id` Cookie の
   読み取りも Handler の責務に自然に収まる。
2. `HandleCallback(ctx, code)` のシグネチャ（`code` のみ）を変えずに済み、既存テスト・既存
   呼び出し側への波及がない。
3. 破棄処理は既存の `AuthService.Logout(ctx, sessionID)` をそのまま再利用でき、新規メソッド・
   新規インターフェースを追加しなくて済む。

### 旧セッション無効化の失敗時の扱い（AC R3.3）

既存 `Logout` ハンドラーの「失敗してもログ出力して継続」パターンに倣い、`Logout` が error を
返してもログイン処理は中断せず、`slog.Error` で運用者が追跡可能な形で記録した上で新セッション
Cookie の発行へ進む。これにより、旧セッション無効化の失敗がログインの可用性を損なわないことを
担保する。

### 旧 session_id と新 session_id が同一の場合の無効化スキップ

`createSession` は毎回 32 byte のランダム ID を生成するため、新 ID が旧 ID と一致することは
実用上発生しない。ただし防御的に、旧 Cookie の値が新セッション ID と一致する場合は無効化を
スキップする条件（`oldCookie.Value != session.ID`）を入れ、誤って発行直後の新セッションを
削除しないようにした。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/handler/auth_handler.go` | `Callback` に旧 `session_id` 無効化（旋回）処理を追加。手順コメントを 4→5→6 に再採番 |
| `internal/handler/auth_handler_test.go` | 旋回・無効化・境界系・Cookie 属性後方互換のテストを追加 |
| `internal/auth/service_test.go` | 連続ログインで session_id が毎回異なることを担保するテストを追加 |
| `docs/specs/35--id/impl-notes.md` | 本ファイル（新規） |

## テスト観点と AC の対応

| AC | テスト | 観点 |
|---|---|---|
| R1.1（旧セッション無効化） | `TestAuthHandler_Callback_RotatesOldSession_RevokesPreviousSessionID` | 旧 `session_id` Cookie 保持時に `Logout(旧ID)` が呼ばれること（正常系） |
| R1.2（旧 ID 拒否） | 上記 R1.1（無効化が呼ばれること）+ `TestGetCurrentUser_ExpiredSession_ReturnsError`（既存）で「DB に無いセッションは認証拒否」を担保 | 旧 ID 無効化後は認証拒否されること |
| R1.3 / R2.2（新 ID ≠ 旧 ID） | `TestAuthHandler_Callback_RotatesOldSession_NewCookieDiffersFromOld` / `TestHandleCallback_IssuesDistinctSessionIDPerLogin` | 発行後の Cookie が旧 ID と異なること / 連続ログインで ID が異なること |
| R2.1（新 ID 発行・Cookie 設定） | `TestAuthHandler_Callback_RotatesOldSession_NewCookieDiffersFromOld` / `TestAuthHandler_Callback_Success_SetsCookieAndRedirects`（既存） | 新 `session_id` Cookie が設定されること |
| R2.3（新 ID 受付） | `TestGetCurrentUser_ValidSession_ReturnsUser`（既存） | 有効なセッションでの認証付きリクエストが受け付けられること |
| R3.1（Cookie 不在でも正常完了） | `TestAuthHandler_Callback_NoOldSessionCookie_DoesNotRevokeAndCompletes` | Cookie 不在時に無効化を試みず正常完了すること（正常系） |
| R3.2（旧セッションが DB に無くても完了） | `TestAuthHandler_Callback_RevokeFails_StillCompletesLogin`（`Logout` が error/no-op を返しても完了） | 旧セッションが存在しなくてもログイン継続（境界値） |
| R3.3（無効化失敗時も記録して継続） | `TestAuthHandler_Callback_RevokeFails_StillCompletesLogin` | 無効化失敗でもログインがエラーにならず新 Cookie を発行（異常系） |
| NFR 2.1（Cookie 属性後方互換） | `TestAuthHandler_Callback_PreservesCookieAttributes` | Cookie 名・HttpOnly/Secure/SameSite/Domain/MaxAge を維持 |
| NFR 2.2（リダイレクト後方互換） | `TestAuthHandler_Callback_PreservesCookieAttributes`（Location 検証）+ `TestAuthHandler_Callback_Success_SetsCookieAndRedirects`（既存） | リダイレクト先・挙動を維持 |
| NFR 1.1（旋回） | R1.1 + R2.1 + R1.3 の組み合わせで担保 | 認証境界での識別子旋回 |

補足: R1.2 / R2.3 はセッションの認証可否を判定する `GetCurrentUser`（認証付きリクエストの
基盤）の既存テストで担保される。旧 ID の無効化（R1.1）が呼ばれること + 「DB に無い／削除済み
セッションは `GetCurrentUser` が error を返す」（`TestGetCurrentUser_ExpiredSession_ReturnsError`）
の組み合わせで、旧 ID が無効化後に認証拒否される（R1.2）／新 ID は受け付けられる（R2.3）挙動を
担保する。Handler の `Logout` 経由で `SessionRepository.DeleteByID` が呼ばれること自体は
`internal/auth/service_test.go` の `TestLogout_DeletesSession`（既存）が担保している。

## 検証結果

- `gofmt -l`（変更ファイル `internal/handler/auth_handler.go` / `internal/handler/auth_handler_test.go` /
  `internal/auth/service_test.go`）: 差分なし
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ green

## 確認事項（レビュワー判断ポイント）

- R1.2「ログイン完了後に旧 `session_id` で認証付きリクエストが拒否される」は、Handler 単体では
  リクエスト/レスポンスを跨ぐ E2E 的観測が難しいため、「旧 ID の無効化が呼ばれること（R1.1）」
  と「無効化済みセッションが `GetCurrentUser` で認証拒否されること（既存テスト）」の合成で担保
  している。フルスタックの E2E（実 DB を介した login → 旧 ID でアクセス → 拒否）が必要であれば
  別 Issue として切り出す候補。
- 本実装は `requirements.md` の Out of Scope（無効化処理をどの層で行うかは実装フェーズの領分）に
  従い、Handler 層に配置した。Service 層集約への変更が望ましいとの判断があれば差し戻されたい。

STATUS: complete
