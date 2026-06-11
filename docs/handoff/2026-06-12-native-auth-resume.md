# 引き継ぎドキュメント: native auth（umbrella #163）の開発再開

> **対象読者**: 本作業を引き継ぐ Claude セッション（および人間運用者）
> **作成**: 2026-06-12 / 前任セッション（ローカル Claude Code）
> **ゴール**: umbrella Issue #163 の残子 Issue（#167 #168 #169 #170 #171 #172）を
> 設計 PR → 実装 PR の idd-claude フローで完了させ、#163 にサマリをコメントする

## 0. 最初に読むもの

1. `CLAUDE.md`（プロジェクト憲章。言語方針・規約・エージェント連携ルール）
2. 本ドキュメント
3. 各 Issue の spec: `docs/specs/<番号>-<slug>/`（**#167〜#171 の設計はすべて確定・develop マージ済み**）

## 1. 運用ルール（CLAUDE.md に無いセッション固有の取り決め）

- **PR の base は `develop`**（main ではない。develop→main の multi-branch 運用）
- develop へ直接 push しない（禁止事項）。必ず PR 経由。マージ方式は **merge commit**（`gh pr merge <N> --merge`）
- **2 PR 制**: 設計 PR（`claude/issue-<N>-design-<slug>`、spec 3 点のみ）→ merge 後に実装 PR（`claude/issue-<N>-impl-<slug>`）。今回の残作業は設計が全て merge 済みのため**実装 PR のみ**（#172 を除く）
- 実装完了後・PR 作成前に **Reviewer を独立 context（サブエージェント）で起動**し、`docs/specs/<番号>-<slug>/review-notes.md` を作成させる（判定 3 カテゴリ: AC 未カバー / missing test / boundary 逸脱。最終行は `RESULT: approve` または `RESULT: reject` のみの行）。approve 後に review-notes をコミットして PR を作成する
- develop にマージしたら、対象 Issue に **`staged-for-release` ラベルを付与**し（`blocked` が付いていれば外す）、マージ内容を日本語でコメントする。**Issue は close しない**（main 到達時に close する運用）
- 実装 PR 本文には「確認事項（レビュワー判断ポイント）」セクションと Reviewer 判定の言及を含め、末尾に `🤖 Generated with [Claude Code](https://claude.com/claude-code)` を付ける
- tasks.md の checkbox（`- [ ]` → `- [x]`）は実装 PR 内で更新してよい（`docs(tasks): mark <N> as done` の専用コミット）。**spec の本文（requirements/design/tasks の内容）は実装 PR で書き換えない**
- コミット末尾に `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`（引き継ぎ後は自モデル名で可）
- **CI が flaky に見えても安易に再実行で流さない**こと。ただし既知の一時障害（Docker Hub への接続タイムアウトによる Trivy 失敗）は再実行で回復した実績がある

## 2. 現在の状態（2026-06-12 時点）

| Issue | 状態 | 次のアクション |
|---|---|---|
| #164 永続化レイヤー | ✅ develop マージ済み | なし |
| #165 flow=native callback | ✅ develop マージ済み（設計 #188 / 実装 #189） | なし |
| #166 POST /api/auth/token | ✅ develop マージ済み（設計 #190 / 実装 #196） | なし |
| #167 POST /api/auth/refresh | 🔶 **実装コミット完了・未レビュー**。branch `claude/issue-167-impl-auth-refresh-rotation`（origin に push 済み、全 3 task 実装・checkbox 更新済み） | impl-notes.md 作成 → Reviewer → PR → merge |
| #168 再利用検知 + revoke | 設計のみ（`docs/specs/168-reuse-detection-revoke/`） | #167 マージ後に実装 |
| #169 Bearer-or-Session | 設計のみ（`docs/specs/169-bearer-or-session/`） | #168 マージ後に実装 |
| #170 退会 cleanup | 🔶 **実装コミット完了・未レビュー**。branch `claude/issue-170-impl-withdraw-native-auth-cleanup`（origin に push 済み、全 3 task 実装・checkbox 更新済み） | impl-notes.md 作成 → Reviewer → PR → merge（#167 と独立、いつでも可） |
| #171 IP rate limit | 設計のみ（`docs/specs/171-native-auth-ip-ratelimit/`） | #169 マージ後に実装 |
| #172 contract tests | ❌ 未着手（spec も無し） | 他の全実装マージ後に spec 作成（設計 PR）→ 実装 |
| #163 umbrella | open | 全子 Issue 完了後にサマリをコメント |

## 3. 実行順序と理由

```
(並行可) #167 仕上げ ──→ #168 実装 ──→ #169 実装 ──→ #171 実装 ──→ #172 spec+実装 → #163 サマリ
(並行可) #170 仕上げ ──────────────────────────────────────────────↗
```

- **#168 / #169 / #171 は `internal/handler/router.go`・`internal/auth/token_service.go`・
  `internal/handler/native_auth_handler.go` を順に触るため、必ず逐次**（並行させるとコンフリクトする）
- **#170 は `internal/user/` / `internal/repository/` / `internal/app/withdraw_wiring.go` が主戦場**で
  上記と独立。任意のタイミングでマージ可
- 各実装ブランチは **直前のマージ後の develop から切る**（#168 のブランチは #167 マージ後の develop から、等）

## 4. Issue ごとの具体的な残作業

### #167（branch `claude/issue-167-impl-auth-refresh-rotation` に実装済み）

1. branch を checkout し `go build ./... && go vet ./... && go test ./...` で green を確認
2. `docs/specs/167-auth-refresh-rotation/impl-notes.md` を作成（実装サマリ / task↔commit 対応 / design 逸脱の有無 / 検証結果 / #168 への引き継ぎ。書式は `docs/specs/166-auth-token-exchange/impl-notes.md` を踏襲）してコミット
3. Reviewer サブエージェントで独立レビュー → `review-notes.md` 作成 → approve ならコミット
4. 実装 PR 作成（タイトル `feat(#167): POST /api/auth/refresh で refresh token をローテーションする`、本文に Closes #167 / 設計 PR #191 への参照）→ CI green → merge → Issue #167 に `staged-for-release` 付与 + `blocked` 除去 + コメント

### #170（branch `claude/issue-170-impl-withdraw-native-auth-cleanup` に実装済み）

- #167 と同一手順（spec dir は `docs/specs/170-withdraw-native-auth-cleanup/`、設計 PR は #194、
  PR タイトル `feat(#170): 退会時に native auth state を cleanup する`）
- 注意: 最後のコミット（app wiring / task 3）は前任セッションが developer エージェント停止後に
  検証（build/vet/test green 確認）してコミットしたもの。Reviewer は通常どおり全 diff を見れば良い

### #168 / #169 / #171（設計済み・実装未着手）

各 Issue とも同じ進め方:

1. 最新 develop から `claude/issue-<N>-impl-<slug>` を作成
   - #168: `claude/issue-168-impl-reuse-detection-revoke`
   - #169: `claude/issue-169-impl-bearer-or-session`
   - #171: `claude/issue-171-impl-native-auth-ip-ratelimit`
2. `docs/specs/<番号>-<slug>/` の design.md / tasks.md に**厳密に従って**実装
   （Developer は仕様を追加・解釈しない。矛盾があれば impl-notes / PR 確認事項に記録）
3. task ごとにコミット + checkbox 更新コミット → impl-notes.md → Reviewer → PR → merge → Issue 更新
4. 設計上の要点（詳細は各 design.md が正本）:
   - #168: rotation 拒否分岐 2 箇所を `RevokeFamily` + 同一 401 へ昇格（strict 方針）。
     `POST /api/auth/revoke` は未認証 + 常に 204（冪等・列挙オラクルなし）
   - #169: `auth.JWTVerifier` 新設 + `middleware.NewBearerOrSessionMiddleware`（nil verifier なら
     既存 SessionMiddleware をそのまま返す縮退）。既存 SessionMiddleware は削除しない
   - #171: 既存 `unauthIPMW`（`RATE_LIMIT_UNAUTH_IP`、既定 30 req/min/IP）を
     `/api/auth/token|refresh|revoke` の 3 ルートに重ねるだけ（新 env・新インスタンス不要）

### #172（spec から）

1. 全実装マージ後、`gh issue view 172` で要件確認 → `docs/specs/172-<slug>/` に
   requirements.md（EARS）/ design.md / tasks.md を作成（`.claude/rules/` のゲート類に従い
   Mechanical Checks を自己実行）→ 設計 PR（`claude/issue-172-design-<slug>`）→ merge
2. 実装 PR → Reviewer → merge（手順は他と同じ）

### 仕上げ

- 全子 Issue の develop マージ完了後、umbrella #163 に全体サマリ（各 Issue の PR 番号一覧・
  iOS 側が使えるようになった API の一覧・既存 Web への影響なしの確認結果）をコメント
- #163 の本文チェックリストは Issue close 時に自動連動しないため、コメントで状況を明示する

## 5. 未解決の人間判断ポイント（引き継ぎ後も人間待ち）

- **Issue #178 項目 3**: GitHub Actions（`issue-to-pr.yml`）の prompt injection 対策。
  vendored テンプレートのため上流（idd-claude）対応が本筋 / ローカル暫定対応 / 受容、の
  方針判断を Issue #178 のコメントで人間に確認中。**勝手に着手しないこと**

## 6. 検証コマンド（全実装 PR 共通）

```sh
go build ./... && go vet ./... && go test ./...
```

- DB 結合テストは `TEST_DATABASE_URL` 未設定なら自動 skip（CI では postgres サービス上で実行される）
- web は今回の作業範囲では原則触らない（触った場合のみ `cd web && npm test`）
- `gofmt -l` は**自分が触ったファイルのみ**差分ゼロを確認（develop には既存の未フォーマット
  ファイルが複数あり、それらの一括整形は本作業のスコープ外）
