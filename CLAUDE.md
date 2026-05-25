# プロジェクトガイド（Claude Code 全エージェント共通）

このファイルは Claude Code 本体および全サブエージェントが毎回参照するプロジェクト憲章です。
**すべてのエージェントは、作業開始前にこのファイルを読み直してください。**

Feedman は Web ベースの RSS/Atom フィードリーダーです（Google OAuth 認証・2 ペイン UI・
バックグラウンドフェッチ・はてなブックマーク連携）。`api`（Go）／`worker`（Go）／`web`
（Next.js）／PostgreSQL の 4 コンポーネント構成です。詳細は `README.md` を参照してください。

---

## 言語方針（思考言語と出力言語）

本リポジトリで稼働するすべての Claude エージェント（PM / Architect / Developer / Reviewer /
PjM）は、以下の方針で **内部思考言語と出力言語を使い分ける**こと。reasoning トークン消費を
抑制しつつ、運用者・レビュワーの可読性を維持するための規約です。

### 基本原則

- **内部思考（reasoning / chain-of-thought / 内部スクラッチパッド）は英語ベース**で行う
  （英語の方が同等内容を表現するのに必要なトークン数が少ないため）
- **ユーザーが直接読むアウトプットは日本語ベース**で出力する（運用者の可読性優先）
- 言及されていない種別は **既定で日本語ベース**を選択する（fallback ルール）

### 種別ごとの言語選択

| 種別 | 言語 | 補足 |
|---|---|---|
| LLM の内部 reasoning / scratchpad | **英語** | ユーザーに見えない領域。トークン効率優先 |
| GitHub Issue / PR の本文・コメント・レビューコメント | **日本語** | 運用者・レビュワー向け |
| `docs/specs/<番号>-<slug>/` 配下の markdown（`requirements.md` / `design.md` / `tasks.md` / `impl-notes.md` / `review-notes.md`） | **日本語** | 成果物の本文 |
| EARS トリガーキーワード（`When` / `If` / `While` / `Where` / `shall`） | **英語固定** | `.claude/rules/ears-format.md` の規約に従う。可変部のみ日本語可 |
| Conventional Commits プレフィックス（`feat` / `fix` / `docs` / `refactor` / `chore` / `test`） | **英語固定** | prefix と scope は ASCII |
| ブランチ名（`claude/issue-<番号>-<slug>`） | **英語固定** | slug は ASCII（lowercase ハイフン区切り） |
| 識別子・コマンド名・ファイルパス・env var 名・ラベル名 | **英語固定** | コード／運用と整合させる |
| コミットメッセージ本文（prefix 後の説明部分） | **日本語ベース** | 各プロジェクトの既存 git log 慣習に準拠（混在許容、技術用語の英語そのまま記述は可） |
| PR タイトル | **日本語ベース** | prefix（`feat(scope):` 等）は英語固定、説明部分は日本語 |
| アプリケーションコード／スクリプトのログ出力 | **混在許容** | 各コンポーネントの既存実装に準拠。新規追加分は日本語ベースを推奨するが、既存実装の書き換えは本方針の対象外 |
| `.claude/agents/*.md` のエージェント定義本文 | **日本語** | 人間運用者向けの指示書きであり、エージェント自身の出力ではない |

### 既存規約との整合

- EARS の英語固定トリガーキーワードは本方針の例外規定に含まれる（reasoning 中もそのまま英語表記を保持）
- Conventional Commits / ブランチ命名規約 / 識別子は英語固定。日本語化しない
- 本方針と `.claude/rules/*.md` の他ルールに矛盾が生じた場合、エージェントは独自解釈で確定せず
  PM / 人間にエスカレーションする

---

## 技術スタック

Feedman は Go バックエンド（`api` / `worker`）と Next.js フロントエンド（`web`）の
モノレポ構成です。

### Backend（`api` / `worker` — Go）

- 言語: Go 1.25
- HTTP ルーター: [chi/v5](https://github.com/go-chi/chi)
- DB: PostgreSQL 16（`lib/pq`）+ マイグレーション [golang-migrate](https://github.com/golang-migrate/migrate)
- フィード解析: [gofeed](https://github.com/mmcdole/gofeed)（RSS / Atom）
- セキュリティ: [bluemonday](https://github.com/microcosm-cc/bluemonday)（HTML サニタイズ）/
  [safeurl](https://github.com/doyensec/safeurl)（SSRF 対策）
- メトリクス: Prometheus（`prometheus/client_golang`）
- レート制限: `golang.org/x/time/rate`
- テスト: 標準 `testing` パッケージ（`go test ./...`）

### Frontend（`web` — Next.js）

- フレームワーク: Next.js 15（App Router）+ React 19 + TypeScript 5
- データ取得: TanStack React Query
- UI: Tailwind CSS 4 + shadcn/ui（Radix UI ベース）
- テスト: Vitest + Testing Library（jsdom）
- Lint: ESLint（`eslint-config-next`）
- パッケージマネージャ: npm

### 共通基盤

- コンテナ: Docker / docker-compose（`web` は standalone ビルド）
- CI: GitHub Actions（`.github/workflows/ci.yml` が `go test ./...` と `npm test` を実行）

---

## コード規約

> **2 段構成**: 「共通（言語非依存・必ず守る）」と、本リポジトリの 2 言語（Go / TypeScript）
> 向けの具体例。Backend を触る場合は Go 例、`web/` を触る場合は TypeScript 例に従ってください。

### 共通（言語非依存・必ず守る）

- **単一責務**: 関数は 1 つのことだけをする。複数責務が混ざっていたら分割する
- **関数サイズの制限**: 長い関数は切り出しを検討する（目安は下の言語別例を参照）
- **マジックナンバーは定数化**: 意味のある名前を付けて共有
- **エラーを明示的に扱う**: silent fail を作らない。Go は error 値を `%w` で wrap、
  TypeScript は独自 Error クラスで wrap して呼び出し側でログ出力する
- **公開 API にドキュメンテーションコメント**: シグネチャだけで意図が読み取れないなら
  「目的・引数・返り値・副作用」を記述
- **処理は直線的に書く**: 深いネストやコールバックチェーンよりも直線的に読める書き方を優先する
- **テストは対象コードの近傍に配置する**: 離れた場所に散らさない

### Go（`api` / `worker` / `internal/*`）

- 関数は単一責務とし、長くなったら切り出す（明確な行数上限は設けないが肥大化を避ける）
- **エラーは `fmt.Errorf("...: %w", err)` で wrap** し、呼び出し側で `errors.Is` / `errors.As`
  により判別する。`_ = err` のような握り潰しは禁止
- **exported な識別子には doc comment** を付ける（`// FuncName は ...` 形式、識別子名で開始）
- **`context.Context` は第一引数で引き回す**（`ctx context.Context`）。構造体に保持しない
- `gofmt` / `go vet` に通すこと。import の整形も `gofmt` に従う
- パッケージ境界は `internal/<domain>/`（`feed` / `item` / `subscription` / `user` /
  `auth` / `repository` / `handler` / `worker` 等）の責務に沿って配置する

### TypeScript（`web/`）

- 関数は単一責務・**40 行以内**を目安とする
- 公開 API・共有ユーティリティには **TSDoc** を付与する
- エラーは独自 Error クラスで wrap し、呼び出し側でログ出力する
- 非同期処理は `async/await` を優先し、Promise チェーンは避ける
- ESLint（`eslint-config-next`）に通すこと
- テストは対象コードの近傍（同一ディレクトリの `*.test.ts` / `*.test.tsx`）に配置する

---

## テスト規約

> **2 段構成**: ほぼすべての規約は言語非依存で共通。Go / TypeScript 固有の命名記法と
> fixture パスだけ末尾の言語別ブロックに分離しています。

### 共通（言語非依存・必ず守る）

#### 粒度の使い分け

- **単体テスト**: 純粋関数・個別モジュールのロジック。最も数が多くなる層
- **結合テスト**: DB / 外部サービスを介したユースケース。モックより実物（テスト用 PostgreSQL /
  テストサーバ）を優先
- **E2E**: 主要ユーザーストーリーのゴールデンパスに絞る。網羅を狙わない

#### 命名と構造

- テスト名だけで「何を検証しているか」が分かるようにする（`<対象>: <条件>のとき<期待結果>` が読み取れる形式）
- 各テストは **Arrange / Act / Assert** の 3 パートに明示的に分離する
- **1 テスト = 1 検証対象**。複数観点を 1 つのテストにまとめない

#### モック方針

- **モックしてよい**: HTTP / DB / 時刻 / ファイル / 外部 SDK（はてブ API・外部フィード取得など）の外部副作用
- **モックしない**: 自分が書いた純粋ロジック、テスト対象と同一パッケージ内の関数
- 認証・マイグレーション・SSRF/サニタイズなどモックと本番挙動が乖離しやすい領域は、
  実物に近い fixture を優先する

#### カバレッジ・観点

- 目標は **変更箇所の分岐をすべてカバー**。全体カバレッジ率は KPI にしない
- 各 AC に対して、正常系だけでなく **異常系・境界値・空入力を最低 1 ケース**用意する
- AC と 1 対 1 に紐付かないテストは spec に戻って AC を追加するか、テスト自体を削除する

#### 運用

- **flaky テスト**は quarantine せず、原因を特定して修正するか削除する。一時的 skip を入れた場合は即時に Issue 化する
- **テストデータ fixture** は言語・フレームワーク慣習に沿った場所に集約し、テスト間で共有する
- **Red → Green → Refactor**: 新規テストは一度失敗することを確認してから実装で通す（書いた瞬間に pass するテストは観点不備を疑う）

### Go（`api` / `worker` / `internal/*`）

- **命名**: `func TestXxx(t *testing.T)` を基本とし、ケースは `t.Run("<条件>のとき<期待結果>", ...)`
  のサブテストで分ける。複数入力は table-driven（`cases := []struct{...}`）で記述する
- **配置**: 対象と同一パッケージに `*_test.go` を置く（例: `internal/feed/service_test.go`）
- **fixture**: 固定データは `testdata/` ディレクトリに置く（`go test` が無視する規約ディレクトリ）
- **DB 結合テスト**: モックではなくテスト用 PostgreSQL を優先する（`internal/repository/*_test.go` 参照）

### TypeScript（`web/`）

- **命名**: `describe('対象') > it('<条件>のとき<期待結果>')` 形式（Vitest の BDD スタイル）
- **ツール**: Vitest + Testing Library（jsdom 環境）。`npm test`（= `vitest run`）で実行
- **配置**: 対象コンポーネント／フックの近傍に `*.test.ts` / `*.test.tsx` を置く
- **Snapshot**: 差分が出た時は実装変更の意図と一致しているかを必ず確認してから更新する。
  盲目的な `-u` は禁止（本規約は禁止事項にも記載）

---

## ブランチ・コミット規約

- ブランチ名: `claude/issue-<番号>-<slug>` を原則とする
- コミット: [Conventional Commits](https://www.conventionalcommits.org/) に準拠する
  - `feat(scope): ...` / `fix(scope): ...` / `test(scope): ...` / `docs(scope): ...` / `refactor(scope): ...` / `chore(scope): ...`
- 1 PR = 1 Issue を原則とする（スコープが膨らむ場合は PM が Issue を分割提案する）

---

## 禁止事項

- base ブランチ（既定 `main`、`BASE_BRANCH` 設定によっては `develop` 等）への直接 push
- `.env` や実値を含む Secrets のコミット（`.env.sample` 以外の `.env*` は commit しない）
- 外部サービス呼び出し時に API Key / OAuth client secret を埋め込むこと（環境変数化を徹底）
- 公開リポジトリ上の第三者コードを、ライセンス確認なしにコピペすること
- テストをコメントアウトして PR を出すこと（scope 外に分離する場合は Issue を切る）
- テストを通すために実装ではなくテスト側を書き換えて弱めること（mock を過度に強める / assert を緩める / スナップショットを盲目的に更新する等）

---

## エージェント連携ルール

- **Product Manager** は実装方針を書かない。要件と受入基準の明確化に専念する
- **Architect**（条件付き起動）は要件を変更しない。モジュール構成・データモデル・公開 IF・処理フロー・実装分割の設計に専念する
- **Developer** は仕様を追加・解釈しない。不明点があれば PM / Architect に差し戻す
- **Reviewer**（impl 系モードで自動起動）は Developer 完了後の独立レビューのみを担当し、要件・設計・実装・テストの追加や書き換えを行わない。判定は AC 未カバー / missing test / boundary 逸脱 の 3 カテゴリに限定する（スタイル / lint 観点では reject しない）
- **Project Manager** はコードを変更しない。PR 作成と進捗管理に専念する
- Architect は Triage の `needs_architect: true` 判定時のみ PM と Developer の間に挟まれる
- Architect が起動した Issue では **設計 PR ゲート**を経由する（設計 PR を merge してから実装 PR が別途作られる）
- Reviewer は impl / impl-resume の Developer 完了直後に **独立 context** で起動され、reject 時は Developer に最大 1 回だけ自動差し戻し、再 reject では `claude-failed` で人間に委ねる（差し戻しループは Reviewer 最大 2 回 / Developer 最大 2 回で打ち切り）
- **Debugger**（`DEBUGGER_ENABLED=true` の opt-in 環境でのみ起動 / #22 Phase 3）はコード書き換え・判定・ラベル付け替え・commit / PR 作成を **一切行わず**、`docs/specs/<番号>-<slug>/debugger-notes.md` に Fix Plan（根本原因 / 修正手順 / 検証方法 / 関連参考資料）を構造化 markdown で出力するだけの独立サブエージェント。Reviewer Round 2 reject 直前 / Developer の `BLOCKED: <reason>` 宣言時に fresh な Claude セッション + web search 権限で起動され、**1 Issue（Phase 2 per-task loop 有効時は 1 task）あたり最大 1 回**に制限される（sentinel file: `debugger-notes.md` の存在）。Debugger 経由で再起動された Developer は Fix Plan の `修正手順` を inline 注入された prompt から参照して再実装する
- **`impl-resume` の branch policy（#67 / #112 以降デフォルト有効）**:
  - `IMPL_RESUME_PRESERVE_COMMITS=true`（#112 以降の既定）の状態では、`impl-resume` モードは既存 origin branch の commit を温存したまま resume する。Developer は `git reset` / `git rebase` / branch 切替を行わず、未完了タスクの先頭から続行すること
  - 同条件下で `IMPL_RESUME_PROGRESS_TRACKING=true`（既定）が有効なら、Developer は各タスク完了ごとに `tasks.md` の `- [ ]` → `- [x]` 行内編集を行い、`docs(tasks): mark <task-id> as done` で **専用 commit** を積む。タスク本文 / `_Requirements:_` / `_Boundary:_` / `_Depends:_` / 順序は変更しない
  - 詳細規約は `.claude/agents/developer.md` の「impl-resume / tasks.md 進捗追跡規約」節を参照
  - `IMPL_RESUME_PRESERVE_COMMITS=false` を明示した場合のみ、本ルールは適用されず本機能導入前の挙動（`origin/<BASE_BRANCH>` 起点で fresh init + force-push、`<BASE_BRANCH>` 未指定時は `main`）に戻る
- Developer は **実装 PR** で `design.md` / `tasks.md` / `requirements.md` を書き換えない（設計 PR で人間レビュー済みのため）。矛盾は PR 本文「確認事項」で指摘する
- **PR Iteration（`needs-iteration` ラベル）の責務境界**:
  - **設計 PR (`claude/issue-<N>-design-<slug>`)** で `needs-iteration` が付いた場合、watcher が次サイクルで Architect 役割の iteration を起動する。`docs/specs/<N>-<slug>/` 配下（`requirements.md` / `design.md` / `tasks.md`）の **書き換えは許容** され、成功時 `awaiting-design-review` に遷移する
  - **実装 PR (`claude/issue-<N>-impl-<slug>`)** で `needs-iteration` が付いた場合、watcher が次サイクルで Developer 役割の iteration を起動する。`docs/specs/<N>-<slug>/` 配下の **spec 書き換えは禁止** で、矛盾は PR 本文「確認事項」で指摘するに留める。成功時 `ready-for-review` に遷移する
  - **1 PR = design or impl のどちらか**（混在禁止）。1 PR で spec 編集と実装変更を同居させない（branch 名が両 pattern に合致するケースは watcher が `ambiguous` として skip する）
  - 設計 PR iteration は #112 以降デフォルト有効（`PR_ITERATION_DESIGN_ENABLED=true`）。`PR_ITERATION_DESIGN_ENABLED=false` を明示した watcher 環境では設計 PR iteration が無効になる
- **Phase D Auto Rebase Processor（`AUTO_REBASE_MODE=claude` opt-in 時のみ起動 / #17）**:
  - `needs-rebase` + approved な open PR について、Claude が rebase を試行し、変更ファイルが `MECHANICAL_PATHS` allowlist に閉じていれば既存 approve を維持して auto-merge へ、allowlist 外なら approving review を **review dismissal API** で剥がして `ready-for-review` に戻す
  - 失敗時（conflict 未解消 / timeout / push 失敗 / dismissal API 失敗）は `claude-failed` で人間にエスカレートし、`needs-rebase` を残置する。再試行は `claude-failed` ラベル除去まで自動では行われない
  - 既定 `AUTO_REBASE_MODE=off` のため、未設定環境では本機能は完全に no-op（NFR 1.1）。導入する場合は idd-claude README「Auto Rebase Processor (Phase D)」節を参照
- 各エージェントの成果物は `docs/specs/<番号>-<slug>/` 配下に保存する（Kiro / cc-sdd 互換）
  - `requirements.md`（PM）— EARS 形式の AC、numeric 階層 ID
  - `design.md`（Architect、条件付き）— File Structure Plan / Components and Interfaces / Traceability
  - `tasks.md`（Architect、条件付き）— `_Requirements:_` / `_Boundary:_` / `_Depends:_` / `(P)` アノテーション
  - `impl-notes.md`（Developer、補足）
  - `review-notes.md`（Reviewer、impl 系モードのみ）— 判定結果と Findings / 最終行 `RESULT: approve|reject`
- `<slug>` は Issue タイトルを lowercase・ハイフン区切り・40 文字以内に正規化した値。既存ディレクトリがあれば流用する

## エージェントが参照する共通ルール（`.claude/rules/`）

各エージェントは作業前に以下のルールを `Read` で読み込みます。

| ルールファイル | 参照エージェント | 役割 |
|---|---|---|
| `ears-format.md` | PM | AC の EARS 記法（When / If / While / Where / shall） |
| `requirements-review-gate.md` | PM | requirements.md の自己レビュー（Mechanical + 判断、最大 2 パス） |
| `design-principles.md` | Architect | design.md の必須セクションと詳細度の方針 |
| `design-review-gate.md` | Architect | design.md の自己レビュー（traceability / File Structure Plan 充填 / orphan 検出 / Budget overflow check） |
| `tasks-generation.md` | Architect / Developer | tasks.md のアノテーション規約と numeric ID 階層 |
| `issue-dependency.md` | PM / Triage / Architect | Issue 間依存・親子関係の canonical 記法（`Depends on:` / `Parent:` 他）と互換 alias マッピング |
| `feature-flag.md` | Developer / Reviewer | Feature Flag Protocol（**opt-in 時のみ Read**。本リポジトリの採否は後述の節を参照） |

ルール群は [cc-sdd](https://github.com/gotalab/cc-sdd)（MIT License, Copyright gotalab）から
adapt したものです。

---

## PR 品質チェック（PjM が PR 作成時に確認する項目）

- [ ] すべての受入基準に対応する実装がある
- [ ] 単体テストが追加・通過している（`go test ./...` / `npm test`）
- [ ] lint / format が通っている（`gofmt` / `go vet` / ESLint）
- [ ] 既存テストが壊れていない（CI `.github/workflows/ci.yml` が green）
- [ ] ドキュメントが更新されている（必要な場合）
- [ ] PR 本文に「確認事項」セクションがある（レビュワー判断ポイントを明示）

---

## 機密情報の扱い

- 本リポジトリでは以下の情報を **コミットしない／ログに残さない**
  - ユーザーの個人情報（メールアドレス・OAuth プロフィール等）やセッショントークン
  - 本番環境の認証情報（PostgreSQL 接続情報・Google OAuth client secret 等）
  - 外部 API の認証情報（はてなブックマーク API キー等）
- もし Issue 本文に機密情報が含まれていた場合、PM エージェントは実装を進めず
  `needs-decisions` で人間にエスカレーションすること
- フィードリーダー固有のセキュリティ前提として、外部 URL 取得は SSRF 対策（`safeurl`）、
  取得コンテンツは HTML サニタイズ（`bluemonday`）を経由させる方針を崩さないこと

---

## Feature Flag Protocol

> **デフォルトは opt-out です**。本節を削除する／空のままにする／値を `opt-in` 以外に
> する場合、自プロジェクトは Feature Flag Protocol を **採用しない**（= 通常の単一実装パス）と
> 解釈されます。誤って `enabled` 等の typo を書いても安全側（opt-out）に倒れます。

**採否**: opt-out

<!-- 採用する場合は上の行を `**採否**: opt-in` に変更し、規約詳細を確認してください -->
<!-- 規約詳細: `.claude/rules/feature-flag.md` -->
<!-- idd-claude:feature-flag-protocol opt-out -->

`Feature Flag Protocol` は **opt-in 制の規約**で、未完成機能を main にマージしても既存挙動を
壊さないようにする実装パターン（`if (flag) { 新挙動 } else { 旧挙動 }`）です。詳細は
`.claude/rules/feature-flag.md` を参照してください。

### この規約を採用するメリット

- 未完成機能を main にマージしても既存挙動を壊さない（リスク隔離）
- 段階的な機能リリースが可能（main 上で機能完成を待たずに細かく PR を merge できる）
- 不具合発生時に flag を false に倒すだけで切り戻し可能

### この規約を採用するデメリット

- flag 残存による技術債の管理コスト（クリーンアップ PR が別途必要）
- 両系統テストのメンテナンスコスト（同一スイートを flag-on / flag-off で 2 通り回す）

### 推奨ケース / 非推奨ケース

- **推奨**: 大規模機能で複数 PR をまたぐ実装、リリース日が確定している機能、未完成のまま main に
  載せたい機能
- **非推奨**: 単純な追加機能、テストが薄いプロジェクト、flag 削除 PR の起票運用が回らないチーム

---

## Spec 管理

GitHub Issue 駆動の idd-claude ワークフロー（PM / Architect / Developer / Reviewer / PjM の
自動連携）で進めます。各エージェントの成果物は **`docs/specs/<番号>-<slug>/`** に保存します
（詳細は前述「エージェント連携ルール」を参照）。

---

## 参考資料

- 各サブエージェントの詳細定義: `.claude/agents/*.md`
- 共通ルール: `.claude/rules/*.md`
- アーキテクチャ / 起動方法: `README.md`
- Triage プロンプト: `~/bin/triage-prompt.tmpl`
