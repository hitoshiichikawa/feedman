# 実装メモ — Issue #13 退会処理（Withdraw）の DB トランザクション原子化

## 採用した設計方式（transaction-aware 化の手法）

`database/sql` の `*sql.DB` と `*sql.Tx` が共通して満たす **`DBTX` querier 抽象**
（`ExecContext` / `QueryContext` / `QueryRowContext`）を `internal/repository/tx.go` に導入し、
各リポジトリの削除メソッドを transaction-aware 化した。Go の定石（querier インターフェースで
DB ハンドルとトランザクションを統一的に扱う）に従っている。

### レイヤごとの変更点

1. **repository 層（`internal/repository/`）**
   - `tx.go`: `DBTX` querier 抽象、`SQLTx`（`*sql.Tx` ラッパー + `Querier()` / `Commit()` /
     `Rollback()`）、`SQLTxBeginner`（`*sql.DB` から `BeginTx(ctx)` でトランザクション開始）を追加。
   - 各リポジトリに共有トランザクション上で実行する `*Exec` バリアントを追加し、
     既存メソッドはそれに `r.db` を渡して委譲する形にリファクタした（挙動は不変）:
     - `PostgresItemStateRepo.DeleteByUserIDExec(ctx, q DBTX, userID)`
     - `PostgresSubscriptionRepo.DeleteByUserIDExec(ctx, q DBTX, userID)`
     - `PostgresSessionRepo.DeleteByUserIDExec(ctx, q DBTX, userID)`
     - `PostgresUserRepo.DeleteByIDExec(ctx, q DBTX, id)`（`rowsAffected == 0` でエラーを返す既存挙動を維持）
   - 既存の `DeleteByUserID` / `DeleteByID` は `*Exec` に `r.db` を渡すだけになり、
     これらを使う他の呼び出し元（購読解除など）の挙動は完全に等価。

2. **user 層（`internal/user/service.go`）**
   - `Tx`（`Commit` / `Rollback`）・`TxBeginner`（`BeginTx(ctx) (Tx, error)`）抽象と、
     トランザクション対応 deleter インターフェース（`TxUserDeleter` / `TxSessionDeleter` /
     `TxSubscriptionDeleter` / `TxItemStateDeleter`）を導入。`*sql.Tx` に直接依存せず
     抽象に依存させることで、DB なしの単体テストで fake に差し替え可能にした。
   - `Service` に `txBeginner` 系フィールドを追加。`NewServiceWithTx(...)` でトランザクション
     パスを組み立てる。`Withdraw` は `txBeginner != nil` のとき単一トランザクション上で
     item_states → subscriptions → sessions → user の順に削除し、**全成功時のみ Commit、
     途中失敗時は defer 経由で全 Rollback** する。
   - 既存の `NewService(...)` は後方互換のため温存し、`txBeginner == nil` のときは旧来の
     逐次削除パス（`withdrawLegacy`）にフォールバックする（既存のモックベース単体テスト互換）。

3. **app 層（`internal/app/withdraw_wiring.go`）**
   - `user` 層は `repository` を import しているため、`repository` 側で `user.Tx*` を
     実装すると import cycle になる。これを避けるため、両者を結ぶアダプタを `app` 層に配置した。
   - `txBeginnerAdapter` が `*repository.SQLTxBeginner` を `user.TxBeginner` に適合させ、
     各 `tx*DeleterAdapter` が `user.Tx`（実体は `*repository.SQLTx`）から `Querier()` で
     `DBTX` を取り出してリポジトリの `*Exec` メソッドへ委譲する。
   - `app.go` の本番配線を `user.NewService(...)` から `newTxUserService(...)` に変更し、
     `repository.NewSQLTxBeginner(db)` を渡してトランザクションパスを有効化した。

### nil チェックの維持

旧実装の `s.stateDeleter != nil` 等の nil ガードは、トランザクションパス（`withdrawTx`）でも
`s.txStateDeleter != nil` 等として維持している（`userDeleter` は存在確認に必須のため非 nil 前提）。

## 各 AC とテストの対応

| AC | 内容 | 担保するテスト |
|---|---|---|
| 1.1 | 全削除ステップ成功時に item_states/subscriptions/sessions/user を全削除 | `TestService_Withdraw`（legacy）/ `TestService_Withdraw_Tx_CommitsOnSuccess`（tx・削除順序も検証） |
| 1.2 | user 削除に連動して CASCADE 対象（identities/user_settings）を削除 | `PostgresUserRepo.DeleteByIDExec` が `DELETE FROM users` を実行（CASCADE は DB スキーマ責務）。`TestService_Withdraw_Tx_CommitsOnSuccess` で user 削除呼び出しを検証 |
| 1.3 | feeds と items を削除せず残存 | 削除対象テーブルに feeds/items を含めないことで担保（削除呼び出しは 4 対象のみ）。`TestService_Withdraw_Tx_CommitsOnSuccess` の `rec.order` が `[item_states, subscriptions, sessions, user]` のみであることを検証 |
| 1.4 | エラーを返さず正常終了 | `TestService_Withdraw_Tx_CommitsOnSuccess`（err == nil を検証） |
| 2.1 | 削除ステップ失敗時に同一処理内の全削除を取り消す | `TestService_Withdraw_Tx_RollsBackOnDeleteError`（rolledBack == true / committed == false） |
| 2.2 | 失敗時に退会前と同一状態のまま残す | 単一トランザクション + Rollback で担保。`TestService_Withdraw_Tx_RollsBackOnDeleteError`（失敗後の後続削除 user が呼ばれないこと + ロールバックを検証） |
| 2.3 | 発生したエラーを呼び出し元へ返す | `TestService_Withdraw_Tx_RollsBackOnDeleteError`（`errors.Is` で wrap 検証）/ `TestService_Withdraw_Tx_CommitError` / `TestService_Withdraw_Tx_BeginError` |
| 3.1 | 存在しないユーザーで `UserNotFound` を返す | `TestService_Withdraw_UserNotFound`（legacy）/ `TestService_Withdraw_Tx_UserNotFound`（`ErrCodeUserNotFound` を検証） |
| 3.2 | 存在しないユーザーで削除を確定しない | `TestService_Withdraw_Tx_UserNotFound`（beginCalled == false / committed == false / 削除 0 件） |
| 4.1 | 関連データ 0 件でもエラーを返さず完了 | `TestService_Withdraw_Tx_NoRelatedData`（err == nil） |
| 4.2 | 関連データ無しユーザーの user（+ CASCADE）削除 | `TestService_Withdraw_Tx_NoRelatedData`（committed == true / user 削除呼び出しを検証） |
| NFR 1.1 | `Withdraw` シグネチャ不変 | `Withdraw(ctx, userID) error` を維持（変更なし）。既存 handler テスト群（`go test ./...`）が green |
| NFR 1.2 | 削除対象テーブル + CASCADE 集合不変 | 削除対象 4 テーブル + CASCADE（identities/user_settings）を維持。`TestService_Withdraw_Tx_CommitsOnSuccess` の `rec.order` で検証 |
| NFR 1.3 | 成功時の最終削除結果が不変 | 同上（同一テーブル群が削除済み） |
| NFR 2.1 | 子→親の削除順序（外部キー制約順守） | `TestService_Withdraw_Tx_CommitsOnSuccess`（`[item_states, subscriptions, sessions, user]` の順序を厳密検証） |
| NFR 3.1 | 退会開始ログ（user_id 含む）1 件 | `withdrawTx` で `slog.Info("退会処理を開始します", user_id)` を出力（既存挙動踏襲） |
| NFR 3.2 | 退会完了ログ（user_id 含む）1 件 | `withdrawTx` で Commit 成功後に `slog.Info("退会処理が完了しました", user_id)` を出力 |

## 後方互換の担保方法

- `Withdraw(ctx, userID) error` のシグネチャを変更していない（NFR 1.1）。
- 削除順序（item_states → subscriptions → sessions → user）・CASCADE 対象・削除する
  テーブル集合を変更していない（NFR 1.2 / 2.1 / Out of Scope）。
- リポジトリの既存メソッド（`DeleteByUserID` / `DeleteByID`）は `*Exec` バリアントに
  `r.db` を渡して委譲するのみで、購読解除など他経路の挙動は完全に等価。
- 旧 `NewService(...)` を温存し、トランザクション抽象が未配線（`txBeginner == nil`）の場合は
  旧来の逐次削除パスにフォールバックする。これにより既存のモックベース単体テストも無改変で通る。
- 退会 API エンドポイントのレスポンス仕様・HTTP ステータスは変更していない（Out of Scope）。

## テスト戦略（結合 or 単体とその理由）

**単体テスト（fake/stub querier + fake トランザクション）を採用した。**

理由:
- 本リポジトリの既存 `internal/repository/*_test.go` は実 PostgreSQL に接続せず、インター
  フェース充足やモデル構築の「コンセプト／単体テスト」のみで構成されている。テスト用
  PostgreSQL のセットアップ機構（接続ヘルパ・スキップ条件・`testdata/` フィクスチャ等）は
  リポジトリ内に存在しない。このため DB 結合テストを新規に持ち込むと、この Issue のスコープ
  外のテスト基盤整備（DSN 取得・migration 適用・CI への DB サービス追加）が必要になる。
- ロールバック／コミットの境界はサービス層の制御フロー（成功時 Commit・失敗時 Rollback・
  存在しないユーザーでトランザクション未開始）に集約されている。`TxBeginner` / `Tx` を
  抽象化したことで、fake が Commit / Rollback の呼び出しを観測でき、トランザクション境界・
  ロールバック発生・コミット未発生を単体テストで確実に検証できる。
- 削除順序（NFR 2.1）も各 deleter の fake が呼び出し順序を記録することで厳密に検証している。

CASCADE（identities / user_settings の連動削除）の実挙動は DB スキーマ（外部キー制約）に
依存するため単体テストでは直接検証していない。これは本変更で導入したものではなく既存挙動の
踏襲であり、`DELETE FROM users` を同一トランザクション上で実行する点のみ検証している。
実 DB を用いた CASCADE / ロールバックの E2E 検証は、テスト用 PostgreSQL 基盤の整備を伴う
別 Issue として切り出すことを推奨する（下記「派生タスク」参照）。

## 確認事項（レビュワー判断ポイント）

1. **DB 結合テストの不在**: 上記理由により、実 PostgreSQL を用いたロールバック / CASCADE の
   結合テストは本 PR に含めていない（既存テスト基盤に実 DB 接続の枠組みが無いため）。要件は
   「テスト用 DB が利用できない環境では単体テストでも可」としており、それに従った。実 DB での
   検証が必須と判断される場合は、テスト基盤整備を含む別 Issue 化を提案する。
2. **トランザクション分離レベル**: `BeginTx` は `opts = nil`（PostgreSQL 既定の READ COMMITTED）で
   開始している。退会は単一ユーザーの削除のみで分離レベル引き上げの要件は無いと判断したが、
   要件外の前提のため明記する。
3. **import cycle 回避の配置**: `user → repository` の既存依存があるため、`user.Tx*` を満たす
   アダプタを `repository` ではなく `app` 層に配置した。これは設計判断であり、専用の adapter
   パッケージへ切り出す選択肢もあるが、既存の配線が `app.go` に集約されている慣習に合わせた。

## 派生タスク（次 Issue 候補）

- テスト用 PostgreSQL 基盤（接続ヘルパ / migration 適用 / CI サービス）を整備し、退会処理の
  ロールバック・CASCADE 連動削除を実 DB で検証する結合テストを追加する。

## 変更ファイル一覧

- `internal/repository/tx.go`（新規）
- `internal/repository/tx_test.go`（新規）
- `internal/repository/postgres_user_repo.go`
- `internal/repository/postgres_session_repo.go`
- `internal/repository/postgres_subscription_repo.go`
- `internal/repository/postgres_item_state_repo.go`
- `internal/user/service.go`
- `internal/user/service_test.go`
- `internal/app/app.go`
- `internal/app/withdraw_wiring.go`（新規）
- `internal/app/withdraw_wiring_test.go`（新規）

## 検証コマンドと結果

- `go test ./...` — 全パッケージ pass（実 DB を用いた skip テストは存在しない＝skip なし）
- `gofmt -l`（対象パッケージ）— 差分なし
- `go vet ./...` — 警告なし
- `go build ./...` — 成功

STATUS: complete
