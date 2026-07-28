# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-28T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-230-impl-fix-passkey-credential
- HEAD commit: dc797f144003418e47d990bc6a96ce1e43f1ad0d
- Compared to: develop..HEAD

## Verified Requirements

### Requirement 1（原子性）

- 1.1 — `FinishRegistrationNew` が `CreateUserOnlyExec` → `CreateExec` → `tx.Commit` を単一 tx で実行（`registration_service.go`）。Unit「成功: user 行と credential 行を単一 tx で作成し Commit」/ DB `TestPasskeyRegistrationTx_HappyPathCommitsBoth`
- 1.2 — Commit で users/passkey_credentials の合成永続化を保証。DB `TestPasskeyRegistrationTx_HappyPathCommitsBoth` が credential_id 逆引きで対応 user 解決を確認
- 1.3 — `CreateExec` が `ErrCredentialAlreadyRegistered` → defer rollback + `ErrRegistrationFailed`。Unit「credential 重複 ... tx rollback」/ DB `TestPasskeyRegistrationTx_CredentialDuplicateRollsBackUser`（Rollback 後 user 未永続化を実 DB で検証）
- 1.4 — `CreateExec` が任意 error → wrap + defer rollback。Unit「credential インフラ障害 ... tx rollback + wrap」（commit=0 / rollback=1 を検証、uniform sentinel に再マップしない）
- 1.5 — `CreateUserOnlyExec` が `ErrUsernameTaken` を返した時点で credential INSERT を発行せず defer rollback。Unit「CreateUserOnlyExec の UNIQUE 衝突 ... credential は永続化されない」/ DB `TestPasskeyRegistrationTx_UserRaceRollsBackCredential`
- 1.6 — 全失敗経路で defer rollback + uniform 拒否。早期拒否（challenge 期限切れ / attestation 失敗 / malformed challenge・session）では `BeginTx` 自体が呼ばれないことを Unit で検証

### Requirement 2（#216 API 契約維持）

- 2.1 — 外部 HTTP 契約は無変更（`NewRegistrationService` の内部シグネチャのみ変更）。E2E `TestE2E_PasskeyFullFlow_DBBacked` が登録→認証→保護 API 到達を green で通す
- 2.2 — credential 重複 / username race / attestation 失敗は `ErrRegistrationFailed` に uniform 化（handler 応答マッピングは無変更）。Unit 群で継続検証
- 2.3 — 内部詳細（credential 重複 / infra / race の区別）を応答に反射しない。infra error は wrap（500）、拒否は uniform sentinel。Unit「infra error must not be re-mapped to ErrRegistrationFailed」等で検証
- 2.4 — request body / response JSON / status 形式は無変更。E2E フルフローで担保

### Non-Functional

- NFR 1.1 / 1.2 — tx 境界により部分 commit を作らない（早期拒否は無 INSERT、tx 開始後失敗は defer rollback、成功は Commit）
- NFR 2.1 / 2.2 — 既存 `logRejection` を維持。追加 wrap メッセージ（`failed to begin/commit registration transaction: %w`）に username / credential_id 生値を含めない
- NFR 3.1 / 3.2 — マイグレーション追加なし・既存テーブル構造変更なし。既存 `Create` / `CreateUserOnly` は `*Exec` への非破壊委譲で後方互換維持。既存テストスイート無変更で green
- NFR 4.1〜4.4 — Unit（stub tx で Commit/Rollback/querier 同一性を検証、外部ネットワーク非依存）+ DB 統合テスト（`TEST_DATABASE_URL` 未設定時は `t.Skip`）で検証可能

## Findings

なし

## Summary

新規パスキー登録 finish の users / passkey_credentials INSERT を単一トランザクション化する
実装バグ修正。Req 1（1.1〜1.6）・Req 2（2.1〜2.4）・NFR 1〜4 のすべてに実装と対応テスト
（unit + DB 統合 + E2E）の紐付けを確認。design-less impl（tasks.md 不在）だが変更は Passkey
Registration Service とその repository/wiring/test に閉じ、Out of Scope（追加登録・認証
サービス）は未変更。既存 `Create`/`CreateUserOnly` は非破壊委譲。build / vet / go test（passkey
/ repository / app / handler）green を確認。

RESULT: approve
