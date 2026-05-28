# Requirements Document

## Introduction

`UpsertItems` はフィード取得後に記事をデータベースへ永続化する処理であり、記事 1 件ごとに最大 3 回の同一性判定 SELECT と 1 回の INSERT/UPDATE を逐次実行している。1 フィード 50 記事で最大約 200 回の DB 往復が発生し、フィード数・記事数の増加に対して処理時間が線形に悪化する N+1 構造を抱えている。本要件は、観測可能な永続化結果（`inserted` / `updated` 件数・同一性判定結果）を現状と差異なく保ちつつ、DB 往復回数を記事件数に比例させないバルク化へ改善することを目的とする。同一性判定の優先順位ロジックやサニタイズ / `content_hash` 計算アルゴリズムの変更は含まない。

## Requirements

### Requirement 1: バルク UPSERT による永続化結果の同等性

**Objective:** As a フィード取得処理の運用者, I want 記事をバルク UPSERT で永続化しても挿入・更新の結果が現状と変わらないこと, so that 性能改善後も記事の同一性判定と件数集計が信頼できる

#### Acceptance Criteria

1. When 新規記事と既存記事が混在したバッチを `UpsertItems` に渡したとき, the Item Upsert Service shall 新規記事の件数を `inserted` として、既存記事の件数を `updated` として返す
2. When 50 件の混在バッチ（新規 N 件・既存 M 件、N+M=50）を渡したとき, the Item Upsert Service shall `inserted` を N、`updated` を M として返す
3. When 既存記事と一致するバッチを処理したとき, the Item Upsert Service shall 当該記事を上書き更新し、対象記事の `id` を新規採番せず保持する
4. The Item Upsert Service shall バルク化前後で同一入力に対する同一性判定の結果（どの既存記事に一致するか／新規とみなすか）を変化させない
5. When 既存記事を更新するとき, the Item Upsert Service shall サニタイズ後のコンテンツ・サマリーと再計算した `content_hash` を保存する

### Requirement 2: DB 往復回数の定数オーダー化

**Objective:** As a システム運用者, I want 記事件数が増えても DB 往復回数が比例して増えないこと, so that フィード・記事のスケールに対して永続化処理がボトルネックにならない

#### Acceptance Criteria

1. When N 件の記事バッチを `UpsertItems` に渡したとき, the Item Upsert Service shall DB 往復回数を記事件数 N に比例させず定数オーダーに収める
2. While 記事件数が 1 件から 50 件に増加する状況で, the Item Upsert Service shall DB 往復回数を記事件数に比例して線形に増加させない

### Requirement 3: バッチ途中での DB エラー時の挙動

**Objective:** As a フィード取得処理の運用者, I want バッチ処理中に DB エラーが発生したときの挙動が明確に定義されていること, so that エラー時のデータ整合性と件数集計の意味を予測できる

#### Acceptance Criteria

1. If バッチの永続化中に DB エラーが発生したとき, the Item Upsert Service shall 当該バッチによる挿入・更新を全件ロールバックし、当該バッチの記事を 1 件も永続化しない
2. If バッチの永続化中に DB エラーが発生したとき, the Item Upsert Service shall `inserted` と `updated` をいずれも 0 として返し、エラーを返す
3. If 永続化中にエラーが発生したとき, the Item Upsert Service shall 発生元の原因エラーを wrap した error を返す
4. If エラーが発生したとき, the Item Upsert Service shall エラー内容を構造化ログに記録する

### Requirement 4: 境界値（0 件 / 1 件）の取り扱い

**Objective:** As a フィード取得処理の呼び出し元, I want 空バッチや単一記事バッチでも一貫した結果が返ること, so that バルク化によって境界ケースの挙動が変わらない

#### Acceptance Criteria

1. When `items` が 0 件（空スライス）で渡されたとき, the Item Upsert Service shall DB へアクセスせず早期 return し `(0, 0, nil)` を返す
2. When `items` が nil で渡されたとき, the Item Upsert Service shall DB へアクセスせず早期 return し `(0, 0, nil)` を返す
3. When `items` が 1 件の新規記事で渡されたとき, the Item Upsert Service shall 当該記事を挿入し `(1, 0, nil)` を返す
4. When `items` が 1 件の既存記事一致で渡されたとき, the Item Upsert Service shall 当該記事を更新し `(0, 1, nil)` を返す

## Non-Functional Requirements

### NFR 1: 後方互換性（公開シグネチャの維持）

1. The Item Upsert Service shall `UpsertItems` の公開シグネチャ（引数 `ctx`, `feedID`, `items` と戻り値 `inserted int, updated int, err error`）を変更しない
2. The Item Upsert Service shall フィード取得処理（呼び出し元）から見える戻り値の意味（`inserted` = 挿入件数、`updated` = 更新件数）を現状から変えない

### NFR 2: 同一性判定結果の不変性

1. The Item Upsert Service shall `content_hash` による同一性判定結果がバルク化前後で差異を生まないことを保証する
2. The Item Upsert Service shall 同一性判定の優先順位（`(feed_id, guid_or_id)` > `(feed_id, link)` > `content_hash`）の判定結果をバルク化前後で変えない

### NFR 3: 性能（往復回数の上限）

1. When 50 件の記事バッチを処理するとき, the Item Upsert Service shall DB 往復回数を記事件数（50）に比例しない定数オーダー（バッチ数に依存する固定回数）に抑える

## Out of Scope

- 3 段階同一性判定の優先順位ロジック自体の変更（判定順序・判定キーの追加削除）
- サニタイズアルゴリズムの変更
- `content_hash` の計算アルゴリズム（対象フィールド・ハッシュ関数）の変更
- `UpsertItems` の公開シグネチャ・戻り値の意味の変更
- 記事更新時の履歴保持（現状どおり上書きのみ。履歴テーブル等は対象外）
- フィード取得処理（worker / fetcher）側のバッチ分割戦略・並列化
- データベーススキーマ（テーブル定義・既存インデックス）の変更を前提とする要件化（必要なインデックス・制約の判断は design の領分）

## Open Questions

- バッチ途中の DB エラー時の挙動について、本要件では Requirement 3 で「全件ロールバック・件数 0・エラー返却」を方針として定義した。これは現状の「エラー発生時点までの件数を返しつつ即時 error を返す（部分成功カウントが残る）」挙動からの**意図的な変更**であり、永続化のアトミック性を優先したものである。現状挙動（部分成功カウントの温存）を維持すべき運用上の理由がある場合は人間判断を仰ぎたい。
- 同一バッチ内に「同一性判定上は同一」とみなされる記事が重複して含まれる場合の取り扱い（バッチ内重複の最終勝ち／先勝ち／件数カウント）について、Issue・既存実装ともに明示がない。現状の逐次処理では後続記事が先行記事を上書きし得るが、バルク化時の期待挙動を確定したい。
