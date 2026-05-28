# 実装メモ: Issue #12 はてブ API レスポンスの読み込みサイズ上限

## 変更概要

`internal/hatebu/client.go` の `GetBookmarkCounts` に、はてブ API レスポンスボディの
読み込みサイズ上限（1 MiB）を追加した。上限内の正常レスポンスは従来どおりブックマーク数
マップを返し、上限超過レスポンスはサイレント切り詰めではなくエラーとして検出・記録する。
公開シグネチャ・JSON パース・0 件補完・非 200 / 通信失敗の既存エラー処理は変更していない。

## 採用した上限値と定数名

- 上限値: **1 MiB = 1,048,576 バイト**（`1 * 1024 * 1024`）
- 定数名: `maxResponseBodySize`（`client.go` 冒頭の const ブロック、`defaultEndpoint` /
  `maxURLsPerRequest` と同じ箇所に追加）
- マジックナンバーは定数化し、リテラルの散在を排除（NFR 2）

## 上限超過の判定方式

`io.LimitReader(resp.Body, maxResponseBodySize)` でそのまま `io.ReadAll` すると「ちょうど
上限」と「上限超過」を区別できない（どちらも上限ちょうどのバイト数で打ち切られる）。
そこで **上限 +1 バイトまで読み込み**（`io.LimitReader(resp.Body, maxResponseBodySize+1)`）、
読み込み長が上限を超えた（`len(body) > maxResponseBodySize`）場合のみ上限超過と判定する。
これにより以下を満たす:

- ちょうど 1,048,576 バイト → `len(body) == maxResponseBodySize` でパス（Req 4.1）
- 1,048,577 バイト（1 バイト超過）→ `len(body) == maxResponseBodySize+1 > maxResponseBodySize`
  でエラー（Req 4.2）

上限超過時は既存ログ慣習（`c.logger.Error(...)` + `slog` 構造化フィールド）に倣って
ERROR ログを出力し（`max_response_body_size` / `url_count` を付与）、`fmt.Errorf` で
日本語メッセージのエラーを返す。マップは返さず `nil` を返す（Req 3.1 / 3.2 / 3.3）。

メモリ消費は `LimitReader` により最大でも上限 +1 バイトに抑えられる（NFR 1）。

## 追加テスト一覧（`internal/hatebu/client_test.go`）

- `TestClient_GetBookmarkCounts_ExactlyMaxSize_ParsesSuccessfully`
  — ちょうど 1 MiB の有効 JSON がエラーにならずパースされる（境界値 / Req 4.1）
- `TestClient_GetBookmarkCounts_OneByteOverMaxSize_ReturnsError`
  — 1 バイト超過でエラー、マップは nil（境界値 / Req 3.1 / Req 4.2）
- `TestClient_GetBookmarkCounts_OversizedBody_ReturnsErrorAndLogs`
  — 上限を大きく超える（2 MiB）レスポンスでエラー + ERROR ログ + 上限超過メッセージ、
    マップは nil（異常系 / Req 3.1 / 3.2 / 3.3）
- ヘルパー `buildJSONBodyOfExactSize`
  — `map[string]int` としてパース可能な JSON を **正確なバイト数**で生成（パディングキー名
    の長さで 1 バイト単位調整。生成後に実バイト長と map パース可否を assert する）

## 受入基準とテストの対応

| Req ID | 担保するテスト |
|---|---|
| 1.1 / 1.2 / 1.3 | `ExactlyMaxSize_ParsesSuccessfully`（上限内全量読込）+ 上限定数の存在 |
| 2.1 | `SingleURL` / `MultipleURLs`（既存。上限内正常パース） |
| 2.2 | `ZeroBookmarks_MissingFromResponse`（既存。0 件補完） |
| 2.3 | `EmptyURLList` / `NilURLList`（既存。API 非呼び出し空マップ） |
| 2.4 | 既存全テスト（戻り値型 `map[string]int` 不変）+ シグネチャ未変更 |
| 3.1 | `OneByteOverMaxSize_ReturnsError` / `OversizedBody_ReturnsErrorAndLogs`（エラー + nil） |
| 3.2 | `OversizedBody_ReturnsErrorAndLogs`（ERROR ログ + 上限超過メッセージ） |
| 3.3 | `OversizedBody_ReturnsErrorAndLogs`（切り詰めボディを正常返却しない / counts==nil） |
| 4.1 | `ExactlyMaxSize_ParsesSuccessfully` |
| 4.2 | `OneByteOverMaxSize_ReturnsError` |
| 5.1 | `HTTPError` / `LogsError`（既存。非 200 ステータスは上限処理に到達せずエラー） |
| 5.2 | `ContextCancelled`（既存。通信失敗は上限処理に到達せずエラー） |
| NFR 1 | `LimitReader` により最大読込量を上限 +1 バイトに抑制（`OversizedBody` で間接担保） |
| NFR 2 | 定数 `maxResponseBodySize` 化（コードレビューで担保） |
| NFR 3 | 公開シグネチャ不変（既存全テストが無改変で pass） |

## 実装上の判断

- 上限処理の挿入位置はボディ読み取り（`io.ReadAll`）箇所のみとし、非 200 ステータス
  チェック（client.go:84-90）より後段に配置。これにより Req 5.1 / 5.2（既存エラー処理が
  上限処理に到達せず先行する）を構造的に保証している。
- エラーメッセージ・ログメッセージは既存実装の慣習に合わせ日本語ベースで記述した。

## 確認事項

- なし（requirements.md の Open Questions も「なし」。要件は確定済み）。
- スコープ外メモ: `internal/hatebu/batch.go` は本変更着手前から `gofmt -l` で差分が報告される
  状態だったが、本 Issue の対象外（Out of Scope）かつ本変更で触れていないため未修正。
  別 Issue として整形 PR を検討する余地がある（派生タスク候補）。

STATUS: complete
