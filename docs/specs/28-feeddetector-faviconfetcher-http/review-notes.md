# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-28-impl-feeddetector-faviconfetcher-http
- HEAD commit: 9cffcd4556b1552c4af9e2c09e8cdd5b2d991406
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `FeedDetector.httpClient` をコンストラクタで 1 回生成し `getHTTPClient()` がそれを返す（detector.go）。`TestFeedDetector_GetHTTPClient_ReusesSameInstanceWithGuard` / `..._WithoutGuard`
- 1.2 — リクエスト都度の生成ロジックを排除（旧 `getHTTPClient` 内の生成削除）。`TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest`（NewSafeClient 呼び出し ≤1 回を検証）
- 1.3 — クライアント再利用＝コネクションプール共有。同上テストで担保
- 2.1 — 検出ロジック不変。3 回検出で結果一致を検証（`TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest`）+ 既存 `TestDetectFeedURL_*`
- 2.2 — 未検出エラー安定性。`TestFeedDetector_DetectFeedURL_NotDetectedResultStable`（異常系）
- 3.1 — `FaviconFetcher.httpClient` をコンストラクタで 1 回生成（favicon.go）。`TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard` / `..._WithoutGuard`
- 3.2 — favicon 側もリクエスト都度生成を排除。`TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest`
- 3.3 — 再利用によるプール共有。同上テスト
- 4.1 — 取得ロジック不変。3 回取得でデータ長・MIME 一致を検証（同上）+ 既存 `TestFaviconFetcher_FetchFavicon_Success`
- 4.2 — 取得失敗(404)時 nil・空 MIME・エラーなしの安定性。`TestFaviconFetcher_FetchFavicon_FailureResultStable`（異常系）
- 5.1 — SSRF ガード有効時のクライアント再利用後もブロック維持。`TestFeedDetector_SSRFBlocked_StableAfterReuse`
- 5.2 — 同上（favicon）。`TestFaviconFetcher_SSRFBlocked_StableAfterReuse`
- 5.3 — 禁止先ブロックの非退行。同上 + 既存 `TestDetectFeedURL_SSRFBlocked`
- 5.4 — 同上（favicon）+ 既存 `TestFaviconFetcher_FetchFavicon_SSRFBlocked`
- 5.5 — `ValidateURL` がリクエスト都度呼ばれることを検証（detector.go:312 / favicon.go:68 のパス不変）。DNS 解決後 IP 検証は `internal/security`（Out of Scope）が担い、本変更は呼び出しパス・`NewSafeClient` 経由クライアントを変更していない
- 6.1 — `detectorTimeout=10s` / `detectorMaxResponseSize=5MB`（定数化のみ、値不変。インライン `maxBodySize` も同定数に統一）。`TestFeedDetector_GetHTTPClient_TimeoutPreserved`
- 6.2 — `faviconTimeout=5s` / `maxFaviconSize=2MB`（既存定数をそのまま使用）。`TestFaviconFetcher_GetHTTPClient_TimeoutPreserved`
- NFR 1.1 / 1.2 — 公開シグネチャ（`NewFeedDetector` / `NewFaviconFetcher` / 各メソッド・`SSRFValidator` IF）無変更。`go build ./...` 成功 + 既存全テスト pass
- NFR 2.1 — クライアントをコンストラクタで即時生成し read-only フィールドとして保持。`TestFeedDetector_Concurrent_NoDataRace` / `TestFaviconFetcher_Concurrent_NoDataRace`（`go test -race` で PASS を確認）

## Findings

なし

## Summary

本 Issue は design-less impl（tasks.md / design.md 不在）のため AC カバレッジは requirements.md と既存コードの突き合わせで判定した。変更は in-scope の `internal/feed`（detector.go / favicon.go と各テスト）のみで Out of Scope の `internal/worker/fetch` / `internal/security` には触れていない（boundary 逸脱なし）。全 numeric AC（1.1〜6.2 / NFR 1.1, 1.2, 2.1）に対応する実装と正常系・異常系・境界値・並行のテストが追加され、`go test -race ./internal/feed/...` および `go build ./...` の green を確認した。Feature Flag Protocol は opt-out のため flag 観点は適用外。

RESULT: approve
