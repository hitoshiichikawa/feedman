# 実装ノート（Issue #47: 単体試験カバレッジの細部改善）

## 概要

`internal/handler` / `internal/feed` / `internal/subscription` の未カバー分岐に対して
ユニットテストを **追加** した。実装コード（非テストファイル）は一切変更していない
（NFR 1.1: テストファイルの追加・拡張のみで完結）。

本 Issue は design-less impl（design.md / tasks.md なし）であり、`requirements.md` を
入力として実装した。

## 追加したテストと対応 AC

### 要件 1: handler のエラーステータスマッピング default 分岐（`internal/handler/feed_handler_test.go`）

| テスト関数 | 対応 AC | 観点 |
|---|---|---|
| `TestMapAPIErrorToHTTPStatus_KnownCodes` | 1.2 | 既知エラーコード 14 種が個別の HTTP ステータスへマッピングされる正常系（table-driven） |
| `TestMapAPIErrorToHTTPStatus_UnknownCode_ReturnsInternalServerError` | 1.1 | 未マップコードが default 分岐で HTTP 500 にフォールバック |
| `TestMapAPIErrorToHTTPStatus_EmptyCode_ReturnsInternalServerError` | 1.1 | 空コード（境界値）も default 分岐で HTTP 500 にフォールバック |

`mapAPIErrorToHTTPStatus` は private 関数のため、同一パッケージ内テストから直接呼び出して
検証した（HTTP リクエスト全体を経由せずマッピング表の単体ロジックを直接固定）。要件 1.2 の
「既知の正常系と default 経路を区別して検証する」を、別テスト関数として明確に分離した。

### 要件 2: feed パッケージの SSRF ガード有効/無効経路

#### detector（`internal/feed/detector_test.go`）

| テスト関数 | 対応 AC | 観点 |
|---|---|---|
| `TestNewFeedDetector_SSRFGuardEnabled_UsesSafeClient` | 2.1 | ガード有効時に `NewSafeClient`（SSRF 対策付きクライアント）経路が選択される |
| `TestNewFeedDetector_SSRFGuardDisabled_UsesPlainClient` | 2.2 | ガード無効(nil)時に素のクライアント（既定 `detectorTimeout`）経路が選択される |

#### favicon（`internal/feed/favicon_test.go`）

| テスト関数 | 対応 AC | 観点 |
|---|---|---|
| `TestNewFaviconFetcher_SSRFGuardEnabled_UsesSafeClient` | 2.3 | ガード有効時に `NewSafeClient` 経路が選択される |
| `TestNewFaviconFetcher_SSRFGuardDisabled_UsesPlainClient` | 2.4 | ガード無効(nil)時に素のクライアント（既定 `faviconTimeout`）経路が選択される |

SSRF 有効/無効の分岐本体は `newDetectorHTTPClient` / `newFaviconHTTPClient` の
`if ssrfGuard != nil { ... NewSafeClient ... } else { ... 素クライアント ... }` にある。
分岐を観察可能にするため、有効経路は既存 `mockSSRFGuard` の `newSafeClientCalls()` が
1 回呼ばれることで確認し、無効経路は生成クライアントの `Timeout` が定数（SSRF 対策なし
クライアントに設定される既定値）と一致することで確認した。いずれも `httptest` でテスト用
HTTP サーバを立て、実際の取得まで通して経路が観測可能な形にした（Issue 本文の推奨方針に準拠）。

### 要件 3: subscription の購読解除 nil 分岐とエラー経路（`internal/subscription/service_test.go`）

| テスト関数 | 対応 AC | 観点 |
|---|---|---|
| `TestService_Unsubscribe_NilItemStateRepo_SkipsItemStateDelete` | 3.1 | `itemStateRepo == nil` のとき記事状態削除をスキップし購読削除が成功 |
| `TestService_Unsubscribe_ItemStateDeleteError_PropagatesError` | 3.2 | `DeleteByUserAndFeed` がエラーを返すとエラーが伝播し、購読削除は実行されない |

### 要件 4: subscription のフェッチ再開における状態前提違反経路（`internal/subscription/service_test.go`）

| テスト関数 | 対応 AC | 観点 |
|---|---|---|
| `TestService_ResumeFetch_NotStopped_ReturnsFeedNotStoppedAndDoesNotUpdate` | 4.1 | 停止中でない（active）feed に対し `FEED_NOT_STOPPED` 専用エラーを返し、`UpdateFetchState` を呼ばない |

既存の `TestService_ResumeFetch_NotStopped_ReturnsError` は `err != nil` のみを確認していた
ため、AC 4.1 が要求する「専用エラーであること」「状態が更新されないこと」を明示的に検証する
テストを **別関数として追加**した（既存テストは変更していない）。

## AC とテストの対応（網羅確認）

| AC | 担保テスト |
|---|---|
| 1.1 | `TestMapAPIErrorToHTTPStatus_UnknownCode_ReturnsInternalServerError`, `TestMapAPIErrorToHTTPStatus_EmptyCode_ReturnsInternalServerError` |
| 1.2 | `TestMapAPIErrorToHTTPStatus_KnownCodes`（正常系）+ 上記 default 系（区別して検証） |
| 2.1 | `TestNewFeedDetector_SSRFGuardEnabled_UsesSafeClient` |
| 2.2 | `TestNewFeedDetector_SSRFGuardDisabled_UsesPlainClient` |
| 2.3 | `TestNewFaviconFetcher_SSRFGuardEnabled_UsesSafeClient` |
| 2.4 | `TestNewFaviconFetcher_SSRFGuardDisabled_UsesPlainClient` |
| 3.1 | `TestService_Unsubscribe_NilItemStateRepo_SkipsItemStateDelete` |
| 3.2 | `TestService_Unsubscribe_ItemStateDeleteError_PropagatesError` |
| 4.1 | `TestService_ResumeFetch_NotStopped_ReturnsFeedNotStoppedAndDoesNotUpdate` |
| NFR 1.1 | 実装コード（非テストファイル）は未変更。テストファイルのみ追加 |
| NFR 1.2 | `go test ./...` 全パッケージ成功（既存テストへの影響なし） |
| NFR 2.1 | 追加テストはすべて期待挙動が読み取れる関数名 + Arrange/Act/Assert 構造 |
| NFR 2.2 | HTTP 経路はモック SSRF ガード + `httptest`、nil 分岐・純粋ロジックは実物のまま検証 |

## 検証結果

- `go test ./...`: 全パッケージ ok（`internal/handler` / `internal/feed` / `internal/subscription` 含む）
- `go test -race ./internal/handler/ ./internal/feed/ ./internal/subscription/`: ok（データ競合なし）
- `go vet ./...`: 警告なし
- `gofmt -l`（追加・編集ファイル）:
  - `internal/feed/detector_test.go` / `internal/feed/favicon_test.go` / `internal/subscription/service_test.go`: クリーン
  - `internal/handler/feed_handler_test.go`: 後述「確認事項」参照

## 確認事項

- `internal/handler/feed_handler_test.go` は **本変更以前から** `gofmt -l` で
  リストされる既存の整形差分（92〜103 行付近・828 行付近のインデント）を持っている。
  `git show main:internal/handler/feed_handler_test.go` でも同じく検出されるため、本仕様の
  追加コードに起因するものではない。本仕様は「テスト追加のみ・既存挙動不変」が目的であり、
  当該既存行の整形差分修正は無関係なノイズ変更となるため **本 PR では触れていない**
  （追加した私のコード範囲は gofmt クリーン）。必要であれば別 Issue での整形修正を提案する。
- 要件 2 の SSRF 無効経路の観測手段として、`NewSafeClient` 非経由であることを「クライアントの
  `Timeout` が既定値と一致する」ことで間接的に確認している。`mockSSRFGuard.NewSafeClient` も
  `&http.Client{Timeout: timeout}` を返すため Timeout 値だけでは両経路を完全には判別できないが、
  無効経路では `guard` 自体が nil で `newSafeClientCalls()` を観測できないため、有効経路側で
  `newSafeClientCalls() == 1` を確認することで「無効時は NewSafeClient を経由しない」ことを
  相補的に担保している。実装挙動を変えない制約下での最大限の観測方法と判断した。

STATUS: complete
