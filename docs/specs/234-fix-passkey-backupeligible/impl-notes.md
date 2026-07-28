# Implementation Notes

## Implementation Notes

### Task 1

- **採用方針**: `passkey_credentials` に BOOLEAN 2 列（`backup_eligible` / `backup_state`）を
  `NOT NULL DEFAULT false` で追加する golang-migrate 準拠のペア（`20260728120000_add_passkey_credential_backup_flags.{up,down}.sql`）を新設し、Case 8（NFR 1.1 回帰）の期待列 map に 2 列を追加した。
- **重要な判断**:
  - `NOT NULL DEFAULT false` により、本修正前に登録された既存 credential 行にも自動的に安全側
    （BE=0 / BS=0）の値が入るため、NFR 1.3（旧行読み出しでアプリケーションを異常終了させない）を
    DB 側で構造的に担保する。アプリケーションコード側の nullable 対応は不要（`*bool` ではなく
    `bool` scan で扱える）。
  - `ADD COLUMN ... NOT NULL DEFAULT false` は PostgreSQL 11+ で全行 rewrite せず system catalog
    レベルで default を保持するため、long-running lock を発生させずに即時完了する（design.md
    §Migration Strategy の記述と整合）。
  - Case 8 の `want` map を 10 列 → 12 列に更新した。BE/BS も「WebAuthn credential 検証情報」の
    範疇であり NFR 1.1（保存対象を検証情報のみに限定）の許容範囲内であることをコメントで明記した。
  - BE/BS は WHERE 句で使わないため追加インデックスは張らない（design.md §Persistence Layer
    Responsibilities & Constraints の指針に従う）。
- **残存課題**: なし。以降のタスクは以下で対応する:
  - task 2: model・repository interface・postgres 実装・repo DB test（Case 1 / Case 5 / NFR 1.3
    round-trip）に BE/BS を通す。
  - task 3〜6: adapter・service・E2E 経路への BE/BS 伝搬。

### Task 2

- **採用方針**: `model.PasskeyCredential` に `BackupEligible bool` / `BackupState bool` を追加し、
  `PasskeyCredentialRepository` interface から `UpdateSignCount` を除去して後継の
  `UpdateAuthenticationState(ctx, id, signCount, backupState, lastUsedAt) error` を追加した。
  postgres 実装では INSERT / SELECT 列並びに `backup_eligible, backup_state` を追加し、
  `UpdateAuthenticationState` は `sign_count / backup_state / last_used_at` の 3 列のみを UPDATE
  する（`backup_eligible` を SET 句に含めず Req 4.3 を SQL レベルで保証）。DB backed テストは
  Case 1 に BE=true / BS=true の round-trip、Case 5 を `UpdateAuthenticationState` 系に更新（BE 不変
  assert 付き）、Case 5b に NFR 1.3 regression（旧行 = BE/BS 列 DEFAULT）を追加した。
- **重要な判断**:
  - BE/BS は BOOLEAN NOT NULL DEFAULT false（migration Task 1 で担保）なので Scan は `bool`
    直値で受けた（`*bool` / `sql.NullBool` は不要 = 旧 credential 行も DEFAULT false で安全に読める）。
  - Case 5b の旧行 regression は `db.ExecContext` で `backup_eligible` / `backup_state` 列を
    INSERT 対象から省いた素の SQL を発行することで、migration 適用前に永続化された行が
    DB DEFAULT false で読み出せることを再現的に検証している（NFR 1.3）。
  - `*PostgresPasskeyCredentialRepo.UpdateSignCount` **メソッド本体は経過措置として残置**した。
    理由は下記「確認事項」に記載（task 2 の boundary 内では消せないため、task 5 で
    consumer 差し替えと同時に除去する申し送り）。interface からは既に除去済みなので、他の
    repository（テスト mock / 別実装）は影響を受けない。
  - `UpdateAuthenticationState` の 0 rows 挙動は既存 `UpdateSignCount` 流儀に合わせてエラー
    化しない（呼び出し側が事前に `FindByCredentialID` で存在確認する前提）。エラーメッセージ
    には id / credential_id 等の機密値を含めない（NFR 1.2）。
- **残存課題 / 次 task 申し送り**:
  - task 3: adapter の `ParsedCredential` 拡張 / `FinishLogin` 戻り値追加。
  - task 4: registration service で `parsed.BE/BS` を `PasskeyCredential` に反映して保存。
  - task 5: authentication service の lookup closure で stored BE/BS を `Flags` 反映し、
    `credentials.UpdateSignCount(...)` を `credentials.UpdateAuthenticationState(...)` に切替
    と同時に、残置した `*PostgresPasskeyCredentialRepo.UpdateSignCount` メソッドを除去する。
  - task 6: E2E で BE=1 / BE=0 双方の regression と DB sanity。

## 確認事項

- **task 2 と task 5 の boundary スコープ不整合（Issue #234 task 2 実装時に検出）**:
  - **事象**: task 2 の文面は「postgres 実装から `UpdateSignCount` を除去」と要求するが、
    consumer 側の差し替えは task 5 スコープであり、`UpdateSignCount` は以下 3 経路で
    具体型 `*PostgresPasskeyCredentialRepo` に依存している:
    - `internal/passkey/authentication_service.go:37`（narrow interface `PasskeyCredentialReader.UpdateSignCount`）
    - `internal/passkey/authentication_service.go:313`（呼び出し `s.credentials.UpdateSignCount(...)`）
    - `internal/app/app.go:318` の wiring（具体型 `*PostgresPasskeyCredentialRepo` を
      `PasskeyCredentialReader` として渡す）

    postgres 実装から `UpdateSignCount` メソッド本体を今除去すると、`internal/passkey` と
    `internal/app` のコンパイルが壊れ、tasks.md 冒頭の「各タスクはコンパイル・既存テストを
    壊さない」不変条件および stage-a-verify（`go vet ./...`）に反する。
  - **対応**: task 2 の `_Boundary:_`（PasskeyCredentialRepository, PasskeyCredentialModel）を
    厳守しつつビルドを green に保つため、以下を採用した:
    - `repository.PasskeyCredentialRepository` **interface** からは `UpdateSignCount` を除去
      （文面どおり）
    - `*PostgresPasskeyCredentialRepo` の **`UpdateSignCount` メソッド本体は当面残置**
      （narrow interface `PasskeyCredentialReader` と app.go wiring を満たし続けるため）
    - `UpdateAuthenticationState` は新規に追加し、両メソッドが共存する過渡状態にした
    - `internal/passkey/*` / `internal/app/*` / `internal/handler/*`（e2e test 含む）は **一切変更しない**
      （task 5 / task 6 の boundary）
    - 残す `UpdateSignCount` メソッドの doc comment に「task 5 で authentication_service の呼び出しが
      UpdateAuthenticationState へ切替された後に除去する経過措置である」旨を明記
  - **task 5 への申し送り**: authentication_service.go の `s.credentials.UpdateSignCount(...)`
    呼び出しを `s.credentials.UpdateAuthenticationState(...)` へ切替える際に、残置した postgres
    実装の `UpdateSignCount` メソッドを併せて除去すること（そうしないと orphan メソッドが残る）。
    合わせて `PasskeyCredentialReader` narrow interface からも `UpdateSignCount` を除去し
    `UpdateAuthenticationState` を追加する必要がある。

## 検証結果

- `go build ./...`: pass
- `go vet ./...`: pass
- `test -z "$(gofmt -l internal/)"`: pass（差分ゼロ）
- `go test ./internal/repository/... ./internal/model/...`: pass（DB backed テストは
  `TEST_DATABASE_URL` 未設定 / DB 未起動環境のため `t.Skip` でスキップされ、compile-time
  interface check と非 DB 系テストは pass。`TEST_DATABASE_URL` が使える環境では Case 8 の
  `want` map が 12 列 = 実 information_schema 列数 と一致することで green を維持する想定）。
