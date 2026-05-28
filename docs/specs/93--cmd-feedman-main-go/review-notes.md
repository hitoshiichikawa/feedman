# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-93-impl--cmd-feedman-main-go
- HEAD commit: f92cf1c8a1022aca8188b15665dcb7cd9b2d0752
- Compared to: develop..HEAD

差分構成（`git diff --stat develop..HEAD`）:
`cmd/feedman/main.go`（新規 37 行）/ `cmd/feedman/main_test.go`（新規 136 行）/
`.github/workflows/ci.yml`（+6）/ `.gitignore`（feedman → /feedman）/ spec ファイル 2 件。
`internal/app` / `Dockerfile` / docker compose は無変更。tasks.md / design.md は不在
（design-less impl のため `_Boundary:_` 制約は定義されておらず、boundary 逸脱の判定対象なし）。
Feature Flag Protocol は CLAUDE.md で `**採否**: opt-out`（flag 観点の確認は行わない）。

## Verified Requirements

- 1.1 — `cmd/feedman/main.go` に `package main` / `func main()` を実装。`.gitignore` の
  `feedman` → `/feedman` 修正でソースがコミット可能化（`git check-ignore cmd/feedman/main.go`
  が not-ignored、`feedman` バイナリは引き続き ignore を確認）
- 1.2 — `CGO_ENABLED=0 go build -o /tmp/feedman-rev ./cmd/feedman` をレビュアーが再実行し成功、
  ELF 実行可能バイナリ生成を確認
- 1.3 — `run` が `app.Run` に args をそのまま委譲（独自解釈ロジックなし）。
  main_test.go「受け取った args を改変せず runner にそのまま委譲する」「stdout を runner に委譲する」で担保
- 2.1 — `serve` 起動。解釈は `app.ParseCommand`/`app.Run` 担当。既存 `internal/app/cmd_test.go`
  `TestParseCommand_Serve` でカバー（impl-notes で紐付け明記）
- 2.2 — `worker` 起動。`TestParseCommand_Worker` でカバー
- 2.3 — 引数なし→既定 serve。`TestParseCommand_DefaultsToServe` でカバー。main 側は
  main_test.go「空 args をそのまま runner に委譲する」で委譲を担保
- 2.4 — 未知サブコマンド→serve フォールバック。`TestParseCommand_UnknownDefaultsToServe`
  でカバー（requirements.md Open Question 1 の決定どおり後方互換維持。main 側で独自検証なし）
- 3.1 — `run` で `fmt.Fprintln(stderr, err)`。main_test.go「runner が error を返すとき
  stderr にエラーメッセージが出力される」で担保
- 3.2 — error 時 `return 1`。main_test.go「runner が error を返すとき終了コード 1 を返す」で担保
- 3.3 — 正常時 `return 0`。main_test.go「runner が nil を返すとき終了コード 0」「stderr が空である」で担保
- 4.1 — `Dockerfile` の `go build -o /feedman ./cmd/feedman` が解決可能化。Dockerfile は無変更。
  `CGO_ENABLED=0` 静的ビルド成功（`statically linked` を file 出力で確認）
- 4.2 — `web/Dockerfile` は無変更（本 Issue のビルド不能要因外、現状維持）
- 4.3 — docker compose の api/worker は `cmd/feedman` 解決可能化により build 対象が成立。compose 無変更
- 5.1 — CI backend ジョブに `CGO_ENABLED=0 go build -o /tmp/feedman ./cmd/feedman` ステップ追加
  （エントリポイント不在を機械的に失敗化）
- 5.2 — root Dockerfile 同条件（`CGO_ENABLED=0` / `./cmd/feedman`）の go build で近似検証。
  実 docker build は採用せず（requirements.md Open Question 2 / impl-notes でオーケストレーター決定
  に従う旨を明記。Out of Scope で「検出の具体手段は実装に委ねる」と定義済み）
- 5.3 — エントリポイント存在時は上記 CI ステップが成功（レビュアー再実行で確認）
- NFR 1.1〜1.4 — `internal/app`（`app.Run`/`ParseCommand` 無変更）/ `Dockerfile` の
  `ENTRYPOINT`/`CMD` / compose `command` / healthcheck はいずれも diff に出現せず無変更
- NFR 1.5 — `go test ./cmd/...` / `go test ./internal/app/...` をレビュアー再実行し全 PASS
- NFR 2.1 — `CGO_ENABLED=0` 静的バイナリ生成を確認（`statically linked`）
- NFR 2.2 — 静的リンクのため動的ライブラリ依存なし。distroless ランタイム構成は無変更で維持

## Findings

なし

## Summary

全 numeric AC（Req 1〜5 / NFR 1〜2）が実装または既存テストでカバーされ、Req 2.x の既存テスト
依存も impl-notes に紐付けが明記されている。`internal/app`・Dockerfile・compose は無変更で
後方互換が保たれ、ビルド・テストの再実行も green。tasks.md 不在のため boundary 制約は定義
されておらず、変更範囲はエントリポイント追加スコープに閉じている。

RESULT: approve
