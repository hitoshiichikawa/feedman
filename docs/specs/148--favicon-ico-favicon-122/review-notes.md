# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-148-impl--favicon-ico-favicon-122
- HEAD commit: 9d54565d1e7003f58de6b537f8c0aeb822f1f0d5
- Compared to: develop..HEAD
- 変更ファイル数: 8 / +1327 / -10
  - `internal/feed/favicon.go`（透明判定呼び出し追加 +24 行）
  - `internal/feed/favicon_transparency.go`（新規 +261 行 — 透明判定本体）
  - `internal/feed/favicon_transparency_test.go`（新規 +551 行）
  - `internal/feed/favicon_testimages_test.go`（新規 +223 行 — テスト fixture ヘルパー）
  - `internal/feed/favicon_test.go` / `favicon_fallback_test.go`（既存テストの fixture 差し替え）
  - `docs/specs/148--favicon-ico-favicon-122/{requirements,impl-notes}.md`

`design.md` および `tasks.md` は存在せず（design-less impl 経路）。Feature Flag Protocol は
本リポジトリの `CLAUDE.md` で `**採否**: opt-out` 宣言のため flag 細目は適用しない。

## Verified Requirements

- **1.1** 受領画像をデコードし全ピクセル alpha=0 を検証
  → `favicon_transparency.go::checkFaviconTransparency` + `isFullyTransparentImage` /
  `isFullyTransparentICO`。test: `TestCheckFaviconTransparency_FullyTransparentPNG` /
  `_FullyTransparentICO` / `_PNGEmbeddedTransparentICO`
- **1.2** 全面透明なら段階失敗扱い・favicon データを採用しない
  → `favicon.go` の透明判定ブロックで `nil, "", nil` 返却。test:
  `TestFetchFavicon_FullyTransparentICO_ReturnsNil` / `_FullyTransparentPNG_ReturnsNil`
- **1.3** alpha なし形式（JPEG 等）は透明判定対象外で成功扱い
  → `hasAlphaChannel` が `image/jpeg` 等で false 返却し早期 return。test:
  `TestCheckFaviconTransparency_JPEGIsNotChecked` / `TestHasAlphaChannel`
- **1.4** デコード不能（形式不明・破損）は段階失敗扱い
  → `errDecodeFailure` wrap → `favicon.go` で `nil, "", nil` 返却。test:
  `TestCheckFaviconTransparency_BrokenPNG` / `_BrokenICO`（3 サブテスト）/ `_EmptyBody` /
  `TestFetchFavicon_BrokenPNG_ReturnsNil`
- **1.5** 透明扱い後に後続フォールバック段階を継続
  → `FetchFaviconWithFallback` の段階列はそのまま温存（`favicon.go` で `nil` 返却 → 既存
  fallback ループが次段へ進む）。test:
  `TestFetchFaviconWithFallback_StageA_TransparentICO_FallsThroughToStageB`
- **2.1** 全段階失敗時に favicon=null として永続化
  → `FetchFaviconWithFallback` が `nil` 返却。test:
  `TestFetchFaviconWithFallback_AllStagesTransparent_ReturnsNil`（4 段階全透明時）
- **2.2** 有効段階で取得時は以降を試行しない
  → 同 `_FallsThroughToStageB` テストで `stageBHit.Load() == true` を verify し、段階 (b)
  で打ち切られ段階 (c) (d) が呼ばれないことを暗黙確認
- **2.3** UI 既定アイコン表示 — #122 で実装済み（フロントエンド `FeedFavicon` サブコンポーネント）。
  本 Issue の backend 変更により null 永続化が増えるため自動的に既定アイコン経路に乗る。
  対象外確認
- **3.1** 全面透明時の構造化ログ
  → `favicon.go` の `slog.Warn("favicon取得: 全面透明（段階失敗扱い）", "url", ..., "mime", ...,
  "reason", "transparent")`。url・段階ラベル相当（mime）・透明判定理由（reason=transparent）を
  記録。実装上で担保（既存 slog 出力テスト規約に倣う）
- **3.2** デコード失敗時の構造化ログ
  → `favicon.go` の `slog.Warn("favicon取得: デコード失敗（段階失敗扱い）", "url", ..., "mime", ...,
  "reason", "decode-failed", "error", ...)`。同上で実装担保
- **4.1** alpha 持ち形式で 1 ピクセル以上 alpha != 0 → 成功扱い
  → test: `TestCheckFaviconTransparency_PartiallyTransparentPNG` /
  `TestFetchFavicon_PartiallyTransparentPNG_ReturnsData` /
  `TestCheckFaviconTransparency_OpaquePNG` / `_OpaqueICO` / `_OpaqueGIF`
- **4.2** alpha なし形式（JPEG 等）は従来どおり成功扱い
  → test: `TestFetchFavicon_JPEG_ReturnsDataWithoutTransparencyCheck` /
  `TestCheckFaviconTransparency_SVGNotChecked`
- **4.3** 既永続化フィードへの再評価・再取得をしない
  → `RegisterFeed` の呼び出し経路を変更しておらず、`FetchFavicon` の起動契約も既存どおり。
  diff にスケジューラや再評価機構の追加なし
- **NFR 1.1** SSRF / ホスト検証 / タイムアウト / サイズ上限の維持
  → 既存 `TestFetchFaviconWithFallback_SSRFGuardBlocksAll` /
  `TestFaviconFetcher_FetchFavicon_LargeResponse` がそのまま温存され、`go test ./...` で pass
- **NFR 1.2** サイズ上限内画像にのみ透明判定
  → `favicon.go` の呼び出し位置は size チェック→mime チェック後に挿入。`git diff` で順序確認済み
- **NFR 2.1** 1 画像 100ms 以内
  → test: `TestCheckFaviconTransparency_Performance`（256x256 全面透明 PNG で計測）。
  `isFullyTransparentImage` は 1 ピクセルでも alpha != 0 を見つけた時点で早期終了
- **NFR 2.2** 取得成功候補のみデコード
  → `FetchFavicon` 内で HTTP 2xx / size / mime チェック後に `checkFaviconTransparency`
- **NFR 3.1** 永続化スキーマ・API レスポンス形状の不変
  → `model.Feed` / repository / API ハンドラへの変更が diff に存在しない

## Boundary 確認

design-less impl のため `tasks.md` の `_Boundary:_` 制約は存在しない。requirements.md の
スコープ（Feed Service の favicon 取得処理）に閉じているかを diff で確認:

- 変更ファイルはすべて `internal/feed/favicon*` 配下 + spec 文書のみ
- DB スキーマ / repository / handler / worker / web フロント / 共通基盤への波及なし
- スコープ逸脱なし

## Findings

なし

## Summary

Issue #148 のすべての AC（1.1〜1.5 / 2.1〜2.3 / 3.1〜3.2 / 4.1〜4.3）および NFR
（1.1〜1.2 / 2.1〜2.2 / 3.1）に対応する実装とテストが揃っており、`go test ./internal/feed/...`
も pass を確認。スコープは `internal/feed/favicon*` に閉じておりスコープ逸脱なし。
Feature Flag Protocol は opt-out のため flag 観点は不適用。

RESULT: approve
