# 実装メモ（Issue #46: フェッチ間隔バリデーション）

## 概要

フェッチ間隔（30〜720 分・30 分刻み）の境界値バリデーションをサービス層
（`internal/subscription/service.go` の `UpdateSettings`）に集約し、ハンドラー層
（`internal/handler/subscription_handler.go`）の二重実装を解消した。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/subscription/service.go` | `UpdateSettings` 入口に境界値バリデーションを追加。`isValidFetchInterval` ヘルパーと定数（`fetchIntervalMin=30` / `fetchIntervalMax=720` / `fetchIntervalStep=30`）を新設。不正値時は `model.NewInvalidFetchIntervalError(minutes)` を返し `subRepo.UpdateFetchInterval` を呼ばない |
| `internal/subscription/service_test.go` | `TestService_UpdateSettings_BoundaryValues` を table-driven で追加（受理／拒否・更新呼び出し有無・更新後購読情報を検証） |
| `internal/handler/subscription_handler.go` | `UpdateSettings` ハンドラーの事前バリデーションを削除。未使用となった `isValidFetchInterval` ヘルパーを削除。検証はサービス層へ一本化 |
| `internal/handler/subscription_handler_test.go` | 不正値テストをサービスモックが `INVALID_FETCH_INTERVAL` を返す形へ追従。境界値テストは 400 応答とエラー本文 `code` を検証 |

## 採用方針

- バリデーションの正をサービス層に一元化（要件 3）。ハンドラーは検証を持たず、サービスが
  返す `INVALID_FETCH_INTERVAL`（`model.APIError`）を既存の `handleServiceError` →
  `mapAPIErrorToHTTPStatus` 経由で HTTP 400 にマップする経路へ統一した。
- `mapAPIErrorToHTTPStatus`（`internal/handler/feed_handler.go:305`）は既に
  `ErrCodeInvalidFetchInterval` を 400 にマップ済みであることを確認。ハンドラーから
  バリデーションを外しても 400 応答とエラー本文の `code` は維持される（要件 2.2/2.3）。
- バリデーションは購読の存在/所有者チェックより**前**に配置した。不正値時はリポジトリ
  更新（`UpdateFetchInterval`）が一切走らないことをテストで担保（要件 2.4）。
- 判定ロジック自体は既存ハンドラーの式（`>=30 && <=720 && %30==0`）と同一で、許容範囲は
  変更していない（NFR 1 後方互換）。マジックナンバーは定数化（CLAUDE.md コード規約）。

## 受入基準とテストの対応

| Req ID | 内容 | 担保テスト |
|---|---|---|
| 1.1 | 30 分で受理 | `TestService_UpdateSettings_BoundaryValues/下限(30)のとき受理` |
| 1.2 | 60 分で受理 | `.../中間値(60)のとき受理` |
| 1.3 | 90 分で受理 | `.../中間値(90)のとき受理` |
| 1.4 | 720 分で受理 | `.../上限(720)のとき受理` |
| 1.5 | 29 分で拒否 | `.../下限未満(29)のとき拒否` |
| 1.6 | 721 分で拒否 | `.../上限超過(721)のとき拒否` |
| 1.7 | 31 分で拒否 | `.../刻み違反(31)のとき拒否` |
| 1.8 | 45 分で拒否 | `.../刻み違反(45)のとき拒否` |
| 1.9 | 0 分で拒否 | `.../ゼロ(0)のとき拒否` |
| 1.10 | 負値で拒否 | `.../負値(-30)のとき拒否` |
| 2.1 | `INVALID_FETCH_INTERVAL` を返す | `TestService_UpdateSettings_BoundaryValues`（拒否ケースで error code を検証） |
| 2.2 | サービスエラー時 HTTP 400 | `TestSubscriptionHandler_UpdateSettings_BoundaryValues` / `InvalidInterval_TooLow` / `TooHigh` / `NotMultipleOf30` |
| 2.3 | エラー本文に `INVALID_FETCH_INTERVAL` | `TestSubscriptionHandler_UpdateSettings_BoundaryValues`（`code` 検証） |
| 2.4 | 拒否時に更新しない | `TestService_UpdateSettings_BoundaryValues`（拒否ケースで `updateCalled == false`） |
| 3.1 | サービス層で検証 | `TestService_UpdateSettings_BoundaryValues`（service 直接呼び出し） |
| 3.2 | ハンドラーの二重実装を解消 | `isValidFetchInterval` 削除 + 既存ハンドラーテストがサービスモック経由で 400 を維持 |
| 3.3 | 正当値で同一の更新後情報を返す | `TestService_UpdateSettings_BoundaryValues`（受理ケースで `FetchIntervalMinutes` / `FeedTitle` を検証） |
| NFR 1.1/1.2 | 後方互換 | 既存 `TestSubscriptionHandler_UpdateSettings_ValidIntervals` / `Success` が緑、許容範囲式不変 |
| NFR 2.1 | 各境界値を単体テストで検証可能 | `TestService_UpdateSettings_BoundaryValues`（29/30/31/45/60/90/720/721/0/負値） |

## テスト実行結果

- `go vet ./...`: 問題なし
- `go test ./internal/subscription/... ./internal/handler/...`: ok（両パッケージ green）
- `go test ./...`: 全パッケージ ok
- `gofmt`: 本変更で touch したファイルは整形済み（リポジトリには本 Issue 対象外の既存
  未整形ファイルが残存するが、本 Issue のスコープ外のため変更していない）

## 確認事項

- なし。要件・既存実装・エラーマッピングから受入基準を確定でき、設計矛盾は検出されなかった。
- Feature Flag Protocol は `CLAUDE.md` で `opt-out` のため通常フローで実装した（flag 不使用）。

STATUS: complete
