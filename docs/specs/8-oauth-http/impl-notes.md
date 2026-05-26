# 実装ノート: OAuth HTTP クライアントにタイムアウトを設定する (Issue #8)

## 実装サマリ

`GoogleOAuthProvider` のトークン交換（`exchangeToken`）とユーザー情報取得（`fetchUserInfo`）が
`http.DefaultClient.Do` を使用しており、クライアントレベルのタイムアウトを持たないため、
上流無応答時にリクエストが無期限ハングしリソースが滞留し得た。本実装で両リクエストを、
明示的なタイムアウトを持つ専用 HTTP クライアント経由に変更した。

### 変更点

- `internal/auth/google_oauth.go`
  - タイムアウト定数 `defaultOAuthHTTPTimeout = 10 * time.Second` を追加（マジックナンバー回避 / R1.3）。
  - `GoogleOAuthProvider` に `httpClient *http.Client` フィールドを追加。トークン交換・ユーザー
    情報取得の両リクエストで **同一クライアントを共有**する。
  - `NewGoogleOAuthProvider` で `&http.Client{Timeout: defaultOAuthHTTPTimeout}` を初期化。
    既存シグネチャ（`config GoogleOAuthConfig` のみ）は変更せず後方互換を維持（NFR 2.1）。
  - `exchangeToken` / `fetchUserInfo` 内の `http.DefaultClient.Do(req)` を `p.httpClient.Do(req)`
    に変更（R1.1 / R1.2）。`http.DefaultClient`（グローバル）は変更していない。

### 採用したクライアント生成方式

- **provider フィールド注入方式**を採用（`tasks` で推奨された方式）。理由:
  - 両リクエストが同一の明示的タイムアウト付きクライアントを自然に共有できる。
  - テストから `provider.httpClient.Timeout` を上書きでき、テスト容易性が高い（同一パッケージ内
    アクセス）。
  - 既存の `internal/hatebu/client.go` も struct に `httpClient *http.Client` を保持する慣習と整合。
- タイムアウト定数名: `defaultOAuthHTTPTimeout` / 値: `10 * time.Second`。

## 各 AC とテストの対応表

| AC | 内容 | 担保テスト |
|---|---|---|
| R1.1 | トークン交換は明示的タイムアウト付きクライアント経由で送信 | `TestGoogleOAuthProvider_DefaultHTTPTimeout`（クライアント存在 + 共有実装）、`TestGoogleOAuthProvider_ExchangeCode_TokenTimeout` |
| R1.2 | ユーザー情報取得は明示的タイムアウト付きクライアント経由で送信 | `TestGoogleOAuthProvider_DefaultHTTPTimeout`、`TestGoogleOAuthProvider_ExchangeCode_UserInfoTimeout` |
| R1.3 | タイムアウトを 10 秒に設定 | `TestGoogleOAuthProvider_DefaultHTTPTimeout`（`defaultOAuthHTTPTimeout == 10*time.Second` を検証） |
| R2.1 | タイムアウト超過時はリクエストを打ち切りエラーを返す | `TestGoogleOAuthProvider_ExchangeCode_TokenTimeout` / `_UserInfoTimeout`（遅延ハンドラ + 短縮タイムアウトでエラー検証） |
| R2.2 | トークン交換タイムアウトをエラー伝播（silent fail にしない） | `TestGoogleOAuthProvider_ExchangeCode_TokenTimeout` |
| R2.3 | ユーザー情報取得タイムアウトをエラー伝播（silent fail にしない） | `TestGoogleOAuthProvider_ExchangeCode_UserInfoTimeout` |
| R2.4 | 無期限待機させない | `TestGoogleOAuthProvider_ExchangeCode_TokenTimeout` / `_UserInfoTimeout`（タイムアウトで完了することを検証。実装前は無期限ハングしていた） |
| R3.1 | 正常応答時に従来どおりアクセストークン取得 | `TestGoogleOAuthProvider_ExchangeCode_Success`（既存 / green 維持） |
| R3.2 | 正常応答時に従来どおりユーザー情報取得 | `TestGoogleOAuthProvider_ExchangeCode_Success`（既存 / green 維持） |
| R3.3 | HTTP エラーステータスを従来どおりエラー返却 | `TestGoogleOAuthProvider_ExchangeCode_TokenError` / `_UserInfoError`（既存 / green 維持） |
| NFR 1.1 | 上限 10 秒で終了 | `TestGoogleOAuthProvider_DefaultHTTPTimeout`、`_TokenTimeout` / `_UserInfoTimeout` |
| NFR 1.2 | タイムアウト時にリソース解放 | `http.Client.Timeout` による接続クローズ（標準ライブラリ挙動）。タイムアウト系テストでハングせず完了することで間接的に担保 |
| NFR 2.1 | 正常系 OAuth フローの観測可能挙動を維持 | `TestGoogleOAuthProvider_ExchangeCode_Success` / `TestGoogleOAuthProvider_GetLoginURL_ContainsRequiredParams`（既存 / green 維持） |
| NFR 2.2 | テスト用エンドポイント上書き（TokenURL / UserInfoURL）を継続サポート | 全 ExchangeCode 系テストが `TokenURL` / `UserInfoURL` 上書きで動作（green 維持） |

## テスト方針メモ

- タイムアウト系テストは `httptest.NewServer` のハンドラを channel `<-released` で待機させ、
  provider の `httpClient.Timeout` を `50ms` に短縮して 10 秒待たずにタイムアウトを再現。
- `httptest.Server.Close()` は in-flight ハンドラの完了を待つため、クリーンアップ順序を誤ると
  デッドロックする。`t.Cleanup` の LIFO 実行順を利用し、`Server.Close` を先に登録、
  `close(released)` を後に登録（= Close より先に解放される）してハンドラリーク・デッドロックを防止。
  - 初回実装でこの順序を誤りテストが 600s タイムアウトで FAIL → 順序修正で green を確認（Red→Green）。

## 実行確認

- `go test ./internal/auth/...`: PASS（追加 2 テスト 0.05s / 0.06s、全 auth テスト 0.114s）
- `go test ./...`: 全パッケージ PASS（既存破壊なし）
- `go build ./...`: 成功
- `gofmt -l internal/auth/google_oauth.go internal/auth/google_oauth_test.go`: 差分なし
- `go vet ./internal/auth/...`: 問題なし
- 使用 toolchain: go1.25.4（`/home/hitoshi/.local/go/bin/go`。go.mod が go>=1.25 を要求するため
  システム既定の go1.22 ではなくこちらを使用）

## 確認事項

なし。
- Feature Flag Protocol は本リポジトリ CLAUDE.md で `**採否**: opt-out` のため通常フローで実装。
- requirements.md / design.md / tasks.md は本 impl ステージでは書き換えていない（design.md /
  tasks.md は本 Issue では未生成 = design-less impl）。
