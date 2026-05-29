# 実装ノート — Issue #148 透明/空の `/favicon.ico` を成功扱いし favicon フォールバック・既定アイコンが効かない

## 採用した透明判定方式

### デコード経路と対応形式

| MIME | デコード経路 | 対応 |
|---|---|---|
| `image/png` | Go 標準 `image/png`（blank import で `image.Decode` に登録） | 全ピクセル alpha=0 を透明と判定 |
| `image/gif` | Go 標準 `image/gif`（blank import） | 全ピクセル alpha=0 を透明と判定 |
| `image/x-icon` / `image/vnd.microsoft.icon` / `image/ico` | 自前 ICO デコーダ（`internal/feed/favicon_transparency.go`） | 後述 |
| `image/jpeg` | 透明判定対象外（要件 1.3 / 4.2） | 従来どおり成功扱い |
| `image/svg+xml` | 透明判定対象外（XML テキストで Go の image パッケージで扱えない） | 従来どおり成功扱い |
| `image/bmp` / `image/webp` | 透明判定対象外（標準デコーダ未登録、追加依存を避けるため範囲外） | 従来どおり成功扱い |

### ICO の扱い（自前デコード）

Go 標準ライブラリには ICO デコーダが無いため、軽量な自前パーサを実装した
（要件 1.1 を満たす精度で「最初の ICONDIRENTRY」のみ検査）。追加依存ライブラリは
入れていない（`go.mod` 変更なし）。

ICO のレイアウト:

1. **ICONDIR**（6 bytes）: `reserved` + `type` + `count` を検証（type=1, count>0）
2. **ICONDIRENTRY**（16 bytes）: 最初のエントリの `imageOffset` と `bytesInRes` を取得
3. 画像データブロック:
   - **PNG 内包**（先頭 8 バイトが PNG マジック `89 50 4E 47 0D 0A 1A 0A`）→ `image.Decode` で PNG として解釈し alpha 走査
   - **DIB（BMP ヘッダなし版）32bpp** → ピクセル配列の alpha バイト（オフセット 3）を直接走査
   - **24bpp 等 alpha なし BMP** → 透明判定対象外として false 返却（要件 4.2 と同様の扱い）

「最初のエントリのみ検査」設計は要件 Out of Scope の「複数解像度の同一画像」を
考慮した妥当な近似（典型 favicon.ico は単一画像 or 同一画像の複数サイズで、
最初のエントリの透明性が全体の透明性を代表する）。

### NFR 2.2 の充足

透明判定は「HTTP 2xx + `image/*` MIME + サイズ上限内」を満たした画像のみに対して実行する。
`FetchFavicon` 内の既存 mime チェック・size チェックの **直後**に `checkFaviconTransparency`
呼び出しを挿入したため、SSRF / size / mime のいずれかで失格する画像はデコードされない。

### NFR 1.1 の維持

SSRF ガード（`safeurl`）・サイズ上限（2MB）・タイムアウト（5s）は既存実装をそのまま温存。
透明判定追加に伴うネットワーク層の変更はない。

## 追加 / 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/feed/favicon.go` | `FetchFavicon` の MIME チェック直後に `checkFaviconTransparency` 呼び出しを追加。透明 / デコード失敗時は `nil, ""` 返却し構造化ログに `reason=transparent` / `reason=decode-failed` を記録 |
| `internal/feed/favicon_transparency.go` | 新規。`checkFaviconTransparency` / `hasAlphaChannel` / `isFullyTransparentImage` / `isFullyTransparentICO` / `isFullyTransparentICOBMP` 実装 |
| `internal/feed/favicon_transparency_test.go` | 新規。透明判定単体テスト + `FetchFavicon` 経由統合テスト + `FetchFaviconWithFallback` 段階制御テスト + 性能テスト |
| `internal/feed/favicon_testimages_test.go` | 新規。テスト用画像生成ヘルパー（`newOpaquePNG` / `newFullyTransparentPNG` / `newPartiallyTransparentPNG` / `newOpaqueGIF` / `newOpaqueICO` / `newFullyTransparentICO` / `newPNGEmbeddedICO` / `newJPEGLikeBytes`） |
| `internal/feed/favicon_test.go` | 既存テストの修正。`pngData := []byte{0x89, 0x50, 0x4E, 0x47, ...}`（8 バイトのマジックのみ）→ `newOpaquePNG(t, ...)` に置換（透明判定でデコードされるため有効な PNG が必要） |
| `internal/feed/favicon_fallback_test.go` | `pngBody` 初期化を `mustGenerateOpaquePNG` 経由に変更。`TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds` が `image/x-icon` MIME で PNG バイトを返していた箇所を `newOpaqueICO(t, 8, 8)` に修正 |

`service_test.go` / `service_async_test.go` の `mockFaviconFetcher` 内で
`0x89 0x50 0x4E 0x47` を使う箇所は **モック実装**で `FetchFavicon` を経由しないため
変更不要（mock が直接 `data []byte` を返す）。

## 追加テスト概要と AC 対応

### Requirement 1: 全面透明 favicon の取得失敗判定

| AC | テスト |
|---|---|
| 1.1 受領画像をデコードし全ピクセル alpha=0 を検証 | `TestCheckFaviconTransparency_FullyTransparentPNG`, `TestCheckFaviconTransparency_FullyTransparentICO`, `TestCheckFaviconTransparency_PNGEmbeddedTransparentICO` |
| 1.2 全面透明なら段階失敗扱い | `TestFetchFavicon_FullyTransparentICO_ReturnsNil`, `TestFetchFavicon_FullyTransparentPNG_ReturnsNil` |
| 1.3 alpha なし形式（JPEG）は透明判定対象外 | `TestCheckFaviconTransparency_JPEGIsNotChecked`, `TestFetchFavicon_JPEG_ReturnsDataWithoutTransparencyCheck`, `TestHasAlphaChannel` |
| 1.4 デコード失敗は段階失敗扱い | `TestCheckFaviconTransparency_BrokenPNG`, `TestCheckFaviconTransparency_BrokenICO`, `TestCheckFaviconTransparency_EmptyBody`, `TestFetchFavicon_BrokenPNG_ReturnsNil` |
| 1.5 透明扱い後に後続段階を継続実行 | `TestFetchFaviconWithFallback_StageA_TransparentICO_FallsThroughToStageB` |

### Requirement 2: フォールバック完走後の挙動

| AC | テスト |
|---|---|
| 2.1 全段階失敗で favicon=null として永続化 | `TestFetchFaviconWithFallback_AllStagesTransparent_ReturnsNil`（呼び出し側 `service.go` の永続化は #122 既存テストで担保） |
| 2.2 有効段階で取得時は以降を試行しない | `TestFetchFaviconWithFallback_StageA_TransparentICO_FallsThroughToStageB` で `stageBHit.Load() == true` を検証（=有効段階で打ち切り） |
| 2.3 UI 既定アイコン表示 | #122 で実装済み（フロントエンド `web/src/components/feed-list.tsx` の `FeedFavicon` サブコンポーネント）。本 Issue の Backend 変更により null 永続化が増えるため、自動的に既定アイコン経路に乗る |

### Requirement 3: 透明判定の可観測性

| AC | テスト・実装根拠 |
|---|---|
| 3.1 全面透明時に構造化ログ記録 | `favicon.go` の `slog.Warn("favicon取得: 全面透明（段階失敗扱い）", "url", ..., "mime", ..., "reason", "transparent")` |
| 3.2 デコード失敗時に構造化ログ記録 | `favicon.go` の `slog.Warn("favicon取得: デコード失敗（段階失敗扱い）", "url", ..., "mime", ..., "reason", "decode-failed", "error", ...)` |

ログ出力自体の機械テストは行わず実装上で担保（既存 `slog` テストも本 repo では行っていない方針と整合）。

### Requirement 4: 既存正常ケースの非リグレッション

| AC | テスト |
|---|---|
| 4.1 1 ピクセル以上 alpha != 0 で成功扱い | `TestCheckFaviconTransparency_PartiallyTransparentPNG`, `TestFetchFavicon_PartiallyTransparentPNG_ReturnsData`, `TestCheckFaviconTransparency_OpaquePNG`, `TestCheckFaviconTransparency_OpaqueICO`, `TestCheckFaviconTransparency_OpaqueGIF` |
| 4.2 alpha なし形式（JPEG）は従来どおり成功扱い | `TestFetchFavicon_JPEG_ReturnsDataWithoutTransparencyCheck`, `TestCheckFaviconTransparency_SVGNotChecked` |
| 4.3 既永続化フィードへの再評価・再取得を行わない | `FetchFavicon` の呼び出しタイミングは変更しておらず、`RegisterFeed` 経路のみで起動する（既存 service.go の構造を温存） |

### Non-Functional Requirements

| NFR | テスト・実装根拠 |
|---|---|
| NFR 1.1 SSRF / ホスト検証 / タイムアウト上限 / サイズ上限の維持 | 既存 `TestFetchFaviconWithFallback_SSRFGuardBlocksAll` / `TestFaviconFetcher_FetchFavicon_LargeResponse` が引き続き pass |
| NFR 1.2 サイズ上限内画像にのみ透明判定 | `FetchFavicon` 内で size チェック → mime チェック → 透明判定の順なので、超過時はデコード前に弾かれる |
| NFR 2.1 1 画像あたり 100ms 以内 | `TestCheckFaviconTransparency_Performance`（256x256 全面透明 PNG で計測） |
| NFR 2.2 取得成功候補のみデコード | `FetchFavicon` 内で HTTP 2xx / size / mime チェック → 透明判定の順で配置 |
| NFR 3.1 永続化スキーマ・API レスポンス形状の不変 | `model.Feed` / `repository.FeedRepository.UpdateFavicon` のシグネチャは変更していない |

## 実行コマンド

| コマンド | 結果 |
|---|---|
| `go test ./...` | 全パッケージ `ok` |
| `go test -race ./internal/feed/...` | `ok` race なし |
| `go vet ./...` | warnings なし |
| `gofmt -l internal/feed/` | 出力なし（clean） |

## 確認事項

- **WebP / BMP の透明判定**: WebP（VP8L / VP8X の alpha チャネル）と BMP（32bpp）は
  Go 標準ライブラリにデコーダが無く、追加依存（`golang.org/x/image/bmp` 等）を避けるため
  本実装では透明判定対象外（`hasAlphaChannel` で false 返却）とした。要件は「alpha
  チャネルを持ち得る形式をデコードして判定」とあり厳密には WebP / BMP も対象だが、
  favicon 実態として ICO / PNG / GIF が圧倒的多数のため、追加依存を入れずに ICO / PNG /
  GIF のみカバーする実装方針を採用した。WebP / BMP の透明 favicon が実世界で問題化した
  場合は派生 Issue で `golang.org/x/image/{bmp,webp}` の追加を検討する。
- **SVG の扱い**: SVG は XML テキストで Go の image パッケージで扱えないため透明判定
  対象外とした（要件 4.2 と同様の取り扱い）。
- **ICO の「最初のエントリのみ検査」設計**: 典型的な favicon.ico は単一画像 or 同一画像の
  複数解像度のため、最初のエントリの透明性が全体の透明性を代表すると見なせる近似。
  全エントリを検査する厳密実装は複雑度に対して得るものが少ないと判断した。
- **既存テストの修正**: PNG マジック 8 バイトのみで成功判定していた既存テスト 7 箇所
  （`favicon_test.go` 5 箇所 / `favicon_fallback_test.go` 1 箇所 + `pngBody` 初期化）を
  有効な PNG（or ICO）に差し替えた。これは「透明判定でデコードできない画像は段階失敗
  扱い（要件 1.4）」の仕様変更に伴うテスト fixture の追従であり、実装側を緩める変更では
  ない（テスト規約「テストを通すためにテスト側を書き換えて弱めることの禁止」には該当しない）。
- **`pngBody` のパッケージ初期化**: `var pngBody = mustGenerateOpaquePNG(4, 4)` で
  package init 時に PNG を生成している。`testing.T` を取らない `generateOpaquePNGForInit`
  経由で実装。失敗時は panic で test runner が拾う（テストファイルなので production 影響なし）。

## 補足ノート

- **要件で曖昧だった点とその解釈**: なし（要件 Open Questions 節で「なし（判定基準は人間決定により
  全面透明のみで確定）」と明記されている）。
- **追加した依存**: なし（Go 標準 `image` / `image/png` / `image/gif` / `image/color` /
  `encoding/binary` のみ使用）。
- **次の Issue として切り出すべき派生タスク**:
  - 既存 staging / 本番 DB に永続化されている透明 favicon データの修復（要件 Out of Scope 明示）
  - WebP / BMP の透明 favicon が実世界で問題化した場合の追加依存検討
  - 部分透明・高透明率・極小サイズ・単色塗りつぶし等の追加判定（要件 Out of Scope 明示）

STATUS: complete
