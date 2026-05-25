# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-46-impl--30-720-30
- HEAD commit: c13c207
- Compared to: 4595bde..HEAD（base: main 4595bde）

## Verified Requirements

- 1.1 — `internal/subscription/service_test.go` `TestService_UpdateSettings_BoundaryValues/下限(30)のとき受理`（30 分で受理。`updateCalled==true` / 結果非 nil を検証）
- 1.2 — 同上 `/中間値(60)のとき受理`
- 1.3 — 同上 `/中間値(90)のとき受理`
- 1.4 — 同上 `/上限(720)のとき受理`
- 1.5 — 同上 `/下限未満(29)のとき拒否`（`*model.APIError` / code 検証）
- 1.6 — 同上 `/上限超過(721)のとき拒否`
- 1.7 — 同上 `/刻み違反(31)のとき拒否`
- 1.8 — 同上 `/刻み違反(45)のとき拒否`
- 1.9 — 同上 `/ゼロ(0)のとき拒否`
- 1.10 — 同上 `/負値(-30)のとき拒否`
- 2.1 — `internal/subscription/service.go:107` で `model.NewInvalidFetchIntervalError(minutes)` を返却。テストで `apiErr.Code == model.ErrCodeInvalidFetchInterval` を assert
- 2.2 — `internal/handler/feed_handler.go:305` で `ErrCodeInvalidFetchInterval → http.StatusBadRequest`。`handleServiceError`（feed_handler.go:268）経由でマップ。`TestSubscriptionHandler_UpdateSettings_BoundaryValues` / `_InvalidInterval_TooLow` / `_TooHigh` / `_NotMultipleOf30` で 400 を検証
- 2.3 — `TestSubscriptionHandler_UpdateSettings_BoundaryValues` がエラー本文を decode し `code == INVALID_FETCH_INTERVAL` を assert（subscription_handler_test.go:705）
- 2.4 — `internal/subscription/service.go` でバリデーションを `FindByID` より前に配置。`TestService_UpdateSettings_BoundaryValues` の拒否ケースで `updateCalled == false`（`UpdateFetchInterval` 未呼び出し）を assert
- 3.1 — `Service.UpdateSettings` 入口でバリデーション実施。service を直接呼ぶ単体テストで検証
- 3.2 — `internal/handler/subscription_handler.go` からハンドラー側の事前バリデーションと `isValidFetchInterval` ヘルパーを削除（diff で確認）。既存ハンドラーテストはサービスモック経由で 400 を維持
- 3.3 — `TestService_UpdateSettings_BoundaryValues` の受理ケースで `result.FetchIntervalMinutes` / `result.FeedTitle` を検証し、本変更導入前と同一の更新後購読情報を返すことを担保
- NFR 1.1/1.2 — 判定式 `>=30 && <=720 && %30==0` は旧ハンドラー実装と同一（定数化のみ）。許容範囲不変。既存 `_ValidIntervals` / `_Success` テストが green
- NFR 2.1 — 29/30/31/45/60/90/720/721/0/負値 の全境界値が table-driven 単体テストで検証可能

## Findings

なし

## Summary

Requirement 1（境界値 10 AC）/ 2（エラー契約・非更新）/ 3（サービス層集約）および NFR 1/2 のすべてが
実装とテストで網羅されている。拒否時の `UpdateFetchInterval` 未呼び出しも service テストで担保。
変更は `internal/subscription/` と `internal/handler/` に閉じており Out of Scope への逸脱なし。
`go test ./internal/subscription/... ./internal/handler/...` および `go vet` ともに green。

RESULT: approve
