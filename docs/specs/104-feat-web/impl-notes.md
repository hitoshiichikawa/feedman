# 実装ノート: #104 記事一覧に概要を表示しタイムスタンプをタイトル右側に移動する

## 概要

記事一覧（右ペイン）に各記事の概要（summary）を表示し、公開日時（タイムスタンプ）を
タイトルの右側・同一行へ移動した。概要データは既にバックエンドのドメインモデル
（`model.Item.Summary`、サニタイズ済み）・リポジトリ取得結果に存在していたため、
サービス層〜ハンドラー〜API レスポンスへ伝播させるのみで実現できた。

## 変更ファイル一覧

### Backend（Go）

- `internal/item/service.go`
  - サービス層 `ItemSummary` 構造体に `Summary string` フィールドを追加
  - `ListItems` のサマリー変換で `Summary: item.Summary` をマッピング
  - `ItemDetail` は埋め込み `ItemSummary` が `Summary` を持つため重複していた独自
    `Summary` フィールドを撤去し、`ItemSummary.Summary` に一本化（GetItem は引き続き
    `item.Summary` を設定）
- `internal/handler/item_handler.go`
  - `itemSummaryResponse` 構造体に `Summary string` `json:"summary"`（omitempty なし）を追加。
    空概要は空文字列で返り、フィールド自体は省略しない
  - `itemDetailResponse` の独自 `Summary` フィールドを撤去（埋め込み `itemSummaryResponse`
    の `summary` に一本化）
- `internal/handler/service_adapter.go`
  - `ListItems` のレスポンス変換で `Summary: it.Summary` をマッピング
- `internal/item/service_test.go`（テスト追加）
- `internal/handler/item_handler_test.go`（テスト追加）

### Frontend（Next.js / TypeScript）

- `web/src/types/item.ts`
  - `ItemSummary` 型に `summary: string` を追加。`ItemDetail` 側の重複 `summary` を撤去
    （継承元 `ItemSummary` に一本化）
- `web/src/components/item-list.tsx`
  - `ItemRow` を改修:
    - 公開日時（推定フラグ含む）をタイトルと同一行の右側へ移動
    - タイトル直下に概要を `text-xs text-muted-foreground line-clamp-2` で表示
    - 概要が空（trim 後 0 文字）のときは概要領域を描画しない
    - `min-w-0`（タイトル span）/ `flex-shrink-0`・`whitespace-nowrap`（日時 span）で
      狭幅時の重なり・はみ出しを防止
  - 既読時の不透明度低下（`opacity-60`）は `button` 全体に適用済みのため概要にも一貫して効く
- `web/src/components/item-list.test.tsx`（テスト追加 + mock データに `summary` 付与）
- `web/src/hooks/use-items.test.tsx`（mock データに `summary` 付与。`ItemSummary` 型が必須化
  されたため型エラー回避のための追従）

## 各 AC への対応とテスト

| AC | 内容 | 担保テスト |
|---|---|---|
| 1.1 | 一覧レスポンスに概要を含める | `item`: `TestItemService_ListItems_IncludesSummary` / `handler`: `TestItemHandler_ListItems_IncludesSummary` |
| 1.2 | 一覧と詳細で概要を同一値で返す | `item`: `TestItemService_SummaryConsistentBetweenListAndDetail` |
| 1.3 | 空概要は空文字列（フィールド省略しない） | `item`: `TestItemService_ListItems_IncludesSummary`（空ケース）/ `handler`: `TestItemHandler_ListItems_IncludesSummary`（summary キー存在 + 空文字列を検証） |
| 1.4 | 既存フィールドを変更せず保持 | `handler`: `TestItemHandler_ListItems_PreservesExistingFields` |
| 2.1 | タイトル直下に概要表示 | `web`: 「概要があるとき記事行のタイトル直下に概要が表示されること」 |
| 2.2 | 概要をタイトルより小さいフォント | `web`: 「概要テキストがタイトルより小さく薄い配色で表示されること」（`text-xs`） |
| 2.3 | 概要を薄い配色で区別 | `web`: 同上（`text-muted-foreground`） |
| 2.4 | 空概要は概要領域を描画しない | `web`: 「概要が空のとき概要領域を描画しないこと」 |
| 2.5 | 既読表現を概要にも一貫適用 | `button` 全体の `opacity-60` で担保（既存テスト「既読記事は視覚的に区別されること」が data-read を検証。概要は同一 button 内のため透過対象に含まれる） |
| 3.1 | 上限超で打ち切り + 省略 | `web`: 「概要が最大2行で省略されるよう line-clamp-2 が適用されること」 |
| 3.2 | 概要表示領域を最大2行に制限 | `web`: 同上（`line-clamp-2`） |
| 3.3 | 短い概要は打ち切らず全体表示 | `line-clamp-2` は上限行数までしか省略しないため短文は全表示。「概要があるとき…表示されること」で全文一致を検証 |
| 4.1 | 公開日時をタイトル同一行の右側に | `web`: 「公開日時がタイトルと同一行の右側に配置されること」（title-row が time を含み、summary は含まない） |
| 4.2 | 推定フラグを公開日時に隣接維持 | `web`: 「推定日付の記事では推定フラグが日時に隣接して表示されること」 |
| 4.3 | 狭幅時に重なり・はみ出しを起こさない | `min-w-0` + `flex-shrink-0` + `whitespace-nowrap` + `line-clamp` によるレイアウトで担保（自動テストでは CSS レイアウト寸法まで検証しないため実装で対応） |
| 4.4 | 日時を縮小しすぎず判読可能に保つ | 日時 span に `whitespace-nowrap` を付与し折り返し・切り詰めによる判読不能を回避（実装で対応） |
| 5.1 | 表示件数を減らさない | `defaultItemsPerPage`（50）・ページサイズは不変（既存挙動を変更していない） |
| 5.2 | 無限スクロール挙動維持 | 既存テスト「無限スクロール用のsentinelが存在すること」（変更なし） |
| 5.3 | 記事選択・展開の既存挙動維持 | 既存テスト「記事をクリックするとonSelectItemが呼ばれること」（変更なし） |
| NFR 1.1 | 既存項目名・型を変更せず追加のみ | `TestItemHandler_ListItems_PreservesExistingFields` |
| NFR 1.2 | 概要を認識しない既存クライアントを破綻させない | フィールド追加のみで既存項目を変更しないため後方互換（`PreservesExistingFields` で既存項目不変を担保） |
| NFR 2.1 | フロントで生 DOM へ未サニタイズ HTML を注入しない | 概要は `<p>{item.summary}</p>` のテキストノードとして描画（`dangerouslySetInnerHTML` 不使用）。実装で担保 |
| NFR 3.1 | 概要を2行以内に制限し視認件数の半減を防ぐ | `line-clamp-2`（3.2 と同一テストで担保） |

## 実行したテスト結果

### Backend（実行環境に Go あり）

- `gofmt -l`（変更ファイル）: 差分なし（クリーン）
- `go vet ./internal/item/... ./internal/handler/...`: パス
- `go test ./internal/item/... ./internal/handler/...`: **ok**（全パス）

### Frontend（未実行 / 環境制約）

- 本実行環境には Node.js / npm が **インストールされていない**ため、
  `cd web && npm test`（vitest）・`npm run lint`・`npm run build` を **実行できなかった**。
- 代替として TypeScript の型整合（`summary` 必須化に伴う全 `ItemSummary` / `ItemDetail`
  リテラルへの `summary` 付与）と、テスト期待値（testid / クラス名 / role）と実装の対応を
  手動レビューで確認済み。
- CI（`.github/workflows/ci.yml`）側で `npm test` が実行されるため、最終的な green は CI で
  担保される想定。

## 確認事項

1. **frontend テスト未実行（環境制約）**: 上記のとおり本ローカル環境に Node.js が無く、
   `npm test` / `npm run lint` / `npm run build` を実行できていない。レビュー時 / CI 上での
   vitest・ESLint・next build の green 確認を依頼したい。
2. **既存の `internal/repository` パッケージのテストビルド失敗（本 PR 起因ではない）**:
   `go test ./...` を流すと `internal/repository/postgres_subscription_repo_db_test.go` で
   `insertTestUser` / `insertTestFeed` / `insertTestSubscription` の重複宣言・シグネチャ不一致
   によるビルドエラーが出る。これは本ブランチを `git stash` してクリーンツリーでも再現する
   **既存の問題**（直近 merge 済み PR 由来）であり、本 Issue のスコープ外。本 Issue では
   touch していない。別 Issue での修正を提案する。
3. **概要の省略行数（Open Questions 対応）**: requirements.md の Open Questions にある
   「1 行固定か 2 行か」は未確定だが、要件本文が「最大 2 行」（Req 3.2）と整理しているため
   `line-clamp-2`（最大 2 行）で実装した。デザイン上 1 行固定の確定値があれば再調整が必要。
4. **狭幅時の優先表示（Open Questions 対応）**: タイトルと日時が同一行に収まらない場合の
   優先表示方針（タイトル優先 / 日時折り返し等）は要件上 design 判断に委ねられている。
   本実装ではタイトルを `flex-1 min-w-0`（縮小・省略可）、日時を `flex-shrink-0`
   `whitespace-nowrap`（折り返さず判読維持）として、日時を優先的に維持しつつタイトル側を
   省略する方針を採用した（Req 4.4 の判読性維持を優先）。

## 補足

- Feature Flag Protocol は本リポジトリで opt-out のため、flag 裏実装は行っていない（通常の
  単一実装パス）。
- backend と frontend を別コミットに分割した（`feat(api):` / `feat(web):`）。

STATUS: complete
