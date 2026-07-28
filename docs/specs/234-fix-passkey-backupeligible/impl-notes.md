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

## 確認事項

なし（task 1 は design.md §Schema Delta / §Physical Data Model と 1:1 で追加のみのため自明）。

## 検証結果

- `go build ./...`: pass
- `go vet ./...`: pass
- `test -z "$(gofmt -l internal/)"`: pass（差分ゼロ）
- `go test ./internal/repository/... ./internal/model/...`: pass（DB backed テストは
  `TEST_DATABASE_URL` 未設定 / DB 未起動環境のため `t.Skip` でスキップされ、compile-time
  interface check と非 DB 系テストは pass。`TEST_DATABASE_URL` が使える環境では Case 8 の
  `want` map が 12 列 = 実 information_schema 列数 と一致することで green を維持する想定）。
