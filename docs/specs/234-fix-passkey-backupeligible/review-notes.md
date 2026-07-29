# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-8 timestamp=2026-07-29T03:30:31Z -->

## Reviewed Scope

- Branch: claude/issue-234-impl-fix-passkey-backupeligible
- HEAD commit: de4305b3a805adea68cdf8b861d34f62bcb56280
- Compared to: develop..HEAD
- Feature Flag Protocol: CLAUDE.md `**採否**: opt-out` のため flag 観点は非適用（通常の 3 カテゴリ判定のみ）

補足: `git diff develop..HEAD --stat` には `web/src/components/*` および `docs/specs/236-*`
の削除が現れるが、これは develop が #236 マージで先行した一方 impl ブランチが古い develop
（merge-base `0b96c18`）から分岐したことによる差分アーティファクトである。`git log develop..HEAD`
は passkey 系 commit のみ（14 件）を示し、impl commit による実変更ではなく merge 時に消えるため
boundary 逸脱として扱わない。

## Verified Requirements

- 1.1 — `registration_service.go` `FinishRegistrationNew` が `parsed.BackupEligible/BackupState` を永続化 / `registration_service_test.go`「adapter が BE=true...(Req 1.1, 1.3)」 / E2E 登録直後 DB sanity
- 1.2 — `FinishAddCredential` が同様に永続化 / `TestRegistrationService_FinishAddCredential`「adapter が BE=true...(Req 1.2, 1.3)」
- 1.3 — BE=true/BS=true → true 保存（上記各テストで assert）
- 1.4 — BE=false/BS=false → false 保存（新規/追加両経路の独立 subtest で assert）
- 1.5 — 既存 1-tx オーケストレーション無変更・新規失敗経路なし（NOT NULL DEFAULT）。既存 rollback テストで担保（impl-notes に紐付け明記）
- 2.1 — `authentication_service.go` lookup closure で `webauthn.CredentialFlags{BackupEligible, BackupState}` を反映 / service test「lookup が返す...Flags に stored BE/BS が反映される」/ repo DB Case 1 round-trip
- 2.2 — stored BE/BS=true → Flags BE=1/BS=1（service test capturedFlags で assert）
- 2.3 — stored BE/BS=false → Flags BE=0/BS=0（service test「stored BE=false のとき」）
- 2.4 — lookup 未解決 → adapter が `ErrAuthenticationFailed` へ正規化し uniform 拒否（既存契約維持・rejected assertion テスト）
- 3.1 — BE=1/BS=1 synthetic authenticator の Web 全動線 E2E `TestE2E_PasskeyFullFlow_DBBacked_BackupEligible` + adapter roundtrip `..._BackupEligible`
- 3.2 — Web/native は同一 `FinishLogin`→adapter→lookup 経路を共有（route 分岐なし）。共有実装で成立（E2E コメントに根拠明記）
- 3.3 — BE=0 baseline `TestE2E_PasskeyFullFlow_DBBacked` を無変更維持 + adapter roundtrip の BE=0 assert
- 3.4 — adapter 失敗を uniform 拒否へ正規化する実装 + rejected assertion テスト（更新/auth_code 発行なし）。BE 不一致の library 拒否は go-webauthn login.go:371 が構造的に強制
- 3.5 — `logRejection` は `shortID(challengeID)` のみ記録し assertion/challenge 生値を出さない（無変更）
- 4.1 — 成功時 `UpdateAuthenticationState` で sign_count/last_used_at 更新 / service test で引数 assert
- 4.2 — BS を assertion 由来最新値へ更新（adapter 4 値目 `updatedBackupState`）/ service test（true/false 両系）/ repo DB Case 5 / E2E 認証直後 DB sanity
- 4.3 — BE を SET 句に含めず不変保持（interface/SQL 設計）/ repo DB Case 5 で BE 不変 assert / E2E 認証後 BE=true 不変 assert
- 5.1 — BE=1 認証成功テスト（adapter roundtrip + E2E）
- 5.2 — BE=0 認証成功テスト（既存 E2E baseline 維持）
- 5.3 — mismatch → uniform 拒否。service レベルの uniform 拒否契約は既存/新規テストで担保。専用 mismatch E2E は spec が `- [ ]*` deferrable（task 6.1）として明示分離しており未実装だが、当該挙動は library 構造強制 + BE=1 成功テストで保護される（Findings / Summary 参照）
- 5.4 — 既存 synthetic E2E を無変更維持（`go build`/`go vet`/passkey・model unit を独立再実行し green を確認）
- NFR 1.1 — migration は列追加のみ（既存列不変）/ repo DB Case 8 の列数 map を 12 列へ更新
- NFR 1.2 — 追加列 DEFAULT false（安全側）
- NFR 1.3 — 旧行（BE/BS 列 DEFAULT）を異常終了させず読み出す / repo DB Case 5b regression
- NFR 2.1/2.2/2.3 — BE/BS は boolean のみ扱い生値をログ/エラー/レスポンスに出さない・uniform 拒否契約・challenge prefix 記録を無変更で継承

## Findings

なし（approve）。

補足（reject には該当しない informational）:
- Req 5.3 の専用 mismatch regression（`TestE2E_..._BackupEligibleMismatch`）は tasks.md 上で
  `- [ ]*` deferrable（task 6.1）として Architect が明示的に分離した optional test であり未実装。
  reviewer.md / tasks-generation.md の deferrable 規約に照らし spec が容認する繰延であるため
  missing test の reject 対象としない。将来サイクルで 6.1 を消化することを推奨する。
- task 3 が `registration_service_test.go` の stub `FinishLogin` シグネチャを 5 値化（挙動不変の
  compile glue）、task 5 が task 2 申し送りに従い postgres 実装の残置 `UpdateSignCount`（interface
  から既に除去済みの dead code）を除去。いずれも impl-notes で文書化された順序化された過渡措置で
  あり no-dead-code 方針とも整合する。boundary 逸脱として扱わない。

## Summary

Issue #234 の根本原因（credential backup 属性の未永続化と認証時 lookup 未反映）に対し、
migration・model・repository・adapter・registration/authentication service・E2E の全層で BE/BS を
一貫して伝搬する修正が入っており、全 numeric AC（Req 1〜5 / NFR 1〜2）に観測可能な実装または
テストが対応している。`go build ./...` / `go vet` / passkey・model unit test を独立再実行し green を
確認（DB/E2E は既存同様に未接続環境で skip、CI で実行）。唯一の未実装は spec が `- [ ]*` として
容認した deferrable な mismatch E2E（task 6.1）のみで、当該 uniform 拒否挙動は service レベルで
汎用的にテスト済み + go-webauthn library が構造的に強制するため reject 事由に当たらない。
boundary 逸脱・missing test・AC 未カバーはいずれも検出されなかった。

RESULT: approve
