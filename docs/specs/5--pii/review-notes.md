# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-5-impl--pii
- HEAD commit: 985cd822fdd23254037bcb70c772c18200ef6628
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `internal/auth/service.go:126` で「new user created」ログの email を `maskEmail(userInfo.Email)` に変更。平文 `userInfo.Email` は出力されない（auth パッケージ内の `slog.String("email", ...)` は当該 1 箇所のみで grep 確認済み）
- 1.2 — 同上。マスク値を出力。`mask_test.go` `TestMaskEmail` 正常系（`hitoshi@example.com` → `h***@example.com`）
- 2.1 — `mask.go` ローカル部 2 文字以上で先頭 1 文字のみ残し残余を固定伏字 `***` に置換。`TestMaskEmail`「通常のメールアドレス〜」
- 2.2 — `mask.go` ドメイン部（最初の `@` 以降）をそのまま保持。`TestMaskEmail`「別ドメイン〜」（`alice@feedman.test` → `a***@feedman.test`）
- 2.3 — 復元不能形式。ローカル部 1 文字以下は先頭も伏せる（`h@example.com` → `***@example.com`）。`TestMaskEmailDoesNotLeakLocalPart`（2 文字目以降 `ecretuser` 非漏洩）で検証
- 3.1 — `service.go:125` の `slog.String("user_id", userID)` を保持（diff 上変更なし）
- 3.2 — `service.go:127` の `slog.String("provider", userInfo.Provider)` を保持（diff 上変更なし）
- 3.3 — diff は email 値の行のみ変更。`user_id` / `provider` のキー名不変
- 4.1 — 空文字でパニックせず `***` を返す。`TestMaskEmail`「空文字のとき〜」
- 4.2 — `@` を含まない不正形式（`notanemail`）でパニックせず `***` を返す。`TestMaskEmail`「@を含まない不正形式〜」
- 4.3 — 空文字・不正形式・ローカル部 1 文字いずれも復元可能な平文を出力しない。`TestMaskEmail` 該当ケース群
- NFR 1.1 — 平文（`@` を含む完全な値）を出力しない。1.1 と同根拠
- NFR 2.1 — `user_id` / `provider` のキー名・出力有無不変。3.1/3.2/3.3 と同根拠

`go test ./internal/auth/...` を実行し ok（全 pass）を確認した。

## Findings

なし

## Summary

design-less impl のため `tasks.md` / `design.md` は不在で `_Boundary:_` 制約は存在せず、変更は要件スコープと一致する `internal/auth/` 配下に閉じている。Feature Flag Protocol は CLAUDE.md で opt-out のため flag 観点は適用しない。全 numeric AC（1.1〜4.3 + NFR 1.1 / 2.1）に観測可能な実装と対応テストが揃い、auth パッケージのテストも green。

RESULT: approve
