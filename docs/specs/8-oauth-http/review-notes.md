# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-8-impl-oauth-http
- HEAD commit: 630f5d9237ea23fc7940a343e65241ae19c95483
- Compared to: develop..HEAD
- 種別: design-less impl（design.md / tasks.md 不在。AC カバレッジは requirements.md と差分で判定）
- Feature Flag Protocol: CLAUDE.md `**採否**: opt-out` のため flag 観点は適用せず、通常 3 カテゴリ判定のみ

## Verified Requirements

- 1.1 — `exchangeToken` の送信を `http.DefaultClient.Do` → `p.httpClient.Do` に変更（google_oauth.go:131）。`TestGoogleOAuthProvider_DefaultHTTPTimeout` / `_ExchangeCode_TokenTimeout` で担保
- 1.2 — `fetchUserInfo` の送信を `p.httpClient.Do` に変更（google_oauth.go:166）。`_DefaultHTTPTimeout` / `_ExchangeCode_UserInfoTimeout` で担保
- 1.3 — `defaultOAuthHTTPTimeout = 10 * time.Second` を定数化（google_oauth.go:22）。`_DefaultHTTPTimeout` が `httpClient.Timeout == defaultOAuthHTTPTimeout` および `== 10*time.Second` を検証
- 2.1 — タイムアウト超過でリクエスト打ち切り + エラー返却。`_ExchangeCode_TokenTimeout` / `_UserInfoTimeout`（遅延ハンドラ + 短縮 Timeout で err != nil を検証）
- 2.2 — トークン交換タイムアウトを `%w` で wrap し呼び出し側へ伝播（google_oauth.go:133, 98）。`_ExchangeCode_TokenTimeout` で検証（silent fail なし）
- 2.3 — ユーザー情報取得タイムアウトを `%w` で wrap し伝播（google_oauth.go:168, 104）。`_ExchangeCode_UserInfoTimeout` で検証
- 2.4 — 無期限待機なし。両タイムアウトテストが 0.05s で完了することで担保（実装前は無期限ハング）
- 3.1 — 正常応答時のアクセストークン取得は既存 `_ExchangeCode_Success`（green 維持。正常系ロジック不変）
- 3.2 — 正常応答時のユーザー情報（ProviderUserID / Email / Name / Provider）取得は `_ExchangeCode_Success`（green 維持）
- 3.3 — HTTP エラーステータスのエラー返却は既存 `_ExchangeCode_TokenError` / `_UserInfoError`（green 維持。ステータス判定ロジック不変）
- NFR 1.1 — 上限 10 秒。`_DefaultHTTPTimeout` + タイムアウト系テストで担保
- NFR 1.2 — タイムアウト時のリソース解放は `http.Client.Timeout`（標準ライブラリ挙動）。タイムアウト系テストがハングせず完了することで間接担保
- NFR 2.1 — 正常系 OAuth フローの観測可能挙動を維持。`http.DefaultClient`（グローバル）は不変、`NewGoogleOAuthProvider` シグネチャ不変。success / GetLoginURL テスト green
- NFR 2.2 — TokenURL / UserInfoURL 上書きを継続サポート。全 ExchangeCode 系テストが上書きで動作（green）

## Findings

なし

## Summary

OAuth Provider の token 交換・user info 取得を明示的タイムアウト付き専用クライアントへ移行し、Requirement 1〜3 と NFR 1〜2 の全 numeric ID が実装または既存テストでカバーされている。新規挙動（タイムアウト）には対応テスト 2 件が追加され、`go test -count=1 ./internal/auth/...` で全テスト green を確認。変更は `internal/auth/google_oauth.go` と同梱テストに閉じており Out of Scope の侵食なし。

RESULT: approve