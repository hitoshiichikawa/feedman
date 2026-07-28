# Implementation Notes — Issue #235: Web ログアウトの fetch/303 事故修正

## 実装アプローチの選定

要件定義には実装方針が規定されておらず、Developer が Requirement 1〜6 の AC を満たす形で
判断する契約になっている。以下の 3 候補を比較検討し、(A) を採用した。

### 候補比較

| 案 | 内容 | 採否 | 理由 |
|---|---|---|---|
| (A) サーバ side content negotiation + クライアント Accept 明示 | サーバは Accept ヘッダで JSON クライアントを識別し 204 を返す。fetch 側は `Accept: application/json` を送信する。 | **採用** | Req 6.2（form POST 経路の後方互換）を破らずに直接的に問題を解消できる。分岐が明快で仕様も自然。 |
| (B) fetch を `redirect: "manual"` にして 303 を明示的にハンドリング | クライアント側のみで完結。サーバは変更しない。 | 不採用 | 303 レスポンスをクライアントで手動処理する分岐が煩雑。ユーザー側から見えるログアウトフローに「サーバは 303 で応答する」実装詳細が漏出する。 |
| (C) サーバを常に 204 に変更 | 単純化されるがフォーム POST 経路の挙動を変える。 | 不採用 | Req 6.2（form POST 経路がある場合の挙動維持）に抵触する可能性が残る。 |

### (A) 採用理由と副次効果

- **プロトコル的にも自然**: HTTP の content negotiation は Accept ヘッダを識別条件として使うのが
  標準的な設計。クライアントを明示的な JSON クライアントとして宣言し、サーバは JSON クライアントに
  対しては 204 で「セッション破棄済み・遷移はクライアントで」というレスポンスを返す。
- **JSON API クライアント全体の防衛**: `apiClient` に `Accept: application/json` を追加する
  ことで、他エンドポイントが将来同様の content negotiation を導入した際にも自動的に整合する。
- **既存 form POST 経路への影響ゼロ**: ブラウザ form POST は Accept に
  `text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8` を送るため、
  `acceptsJSON` は false を返し、これまで通り 303 + Location が返る。

## 変更内容（コミット単位）

### コミット 1: `fix(auth): JSON クライアントには 204 を返し fetch リダイレクト事故を防ぐ`

- `internal/handler/auth_handler.go`
  - `Logout` を content-negotiation 対応に変更（Accept: application/json → 204、それ以外 → 303）
  - 新規プライベート関数 `acceptsJSON(accept string) bool` を追加（カンマ区切り parse、
    パラメータ除去、`application/json` を大小無視で完全一致比較）
- `internal/handler/auth_handler_test.go` にサーバ側テストを追加:
  - `TestAuthHandler_Logout_JSONAccept_ReturnsNoContent` — セッションあり + Accept: application/json → 204
  - `TestAuthHandler_Logout_JSONAccept_NoSession_ReturnsNoContent` — セッションなし + Accept: application/json → 204
  - `TestAuthHandler_Logout_FormPost_StillRedirects` — Accept 不在 / text/html / text/plain → 303（table-driven）

### コミット 2: `fix(web/api): apiClient に Accept: application/json ヘッダを付与する`

- `web/src/lib/api.ts` — `request<T>` のデフォルトヘッダに `Accept: application/json` を追加
- `web/src/lib/api.test.ts` — 既存の全 `toHaveBeenCalledWith` 期待値を新ヘッダに合わせて更新。
  追加テスト:
  - `Accept ヘッダ（Issue #235）` describe > `全メソッドが Accept: application/json を送信すること` —
    5 メソッド全経路の regression net
- `web/src/hooks/use-feeds.test.tsx` — `toHaveBeenCalledWith` 期待値を新ヘッダに合わせて更新

### コミット 3: `fix(web/logout): 遷移先を実在する `/` に修正しエラー時のフィードバックを追加`

- `web/src/components/logout-button.tsx`
  - 遷移先を `/login`（存在しない）→ `/`（AuthGuard 経由でログイン画面表示）に修正
  - mutation エラー時に `role="alert"` のエラー表示を追加（固定文言。内部詳細は反射しない）
  - コメントで Requirement 4 / 5.1 / 5.2 / 5.4 との対応を明示
- `web/src/components/logout-button.test.tsx` — 既存テスト維持 + AC 対応テスト追加:
  - `/` 遷移 / `/login` 非遷移（Req 4.1 / 4.2）
  - QueryClient キャッシュクリア（Req 3.1）
  - 多重クリック抑止（Req 1.4）
  - ネットワーク失敗 / 5xx でのエラー表示 + 再活性化（Req 5.1 / 5.2）
  - 内部詳細（スタック / code / status）を alert に露出しない（Req 5.4）
- `web/src/hooks/use-auth.test.tsx` — `useLogout` に対する追加テスト:
  - Accept: application/json 送信の regression net（Issue #235 の core）
  - mutation success 時に `queryClient.clear()` が走ることを実キャッシュエントリの
    消滅で確認（Req 3.1）

## 各 Requirement / AC のカバレッジ

| AC ID | 内容（要旨） | 担保箇所（テスト） |
|---|---|---|
| 1.1 | 1 クリックでサーバセッション破棄要求 | `TestAuthHandler_Logout_JSONAccept_ReturnsNoContent` の `service.Logout` 呼び出し確認 |
| 1.2 | 追加操作なしでログイン画面表示 | `logout-button.test.tsx` の `ログアウト成功後は `/` に遷移し...` |
| 1.3 | ログアウト後に保護ビュー再取得不可 | `logout-button.test.tsx` の `QueryClient のキャッシュがクリアされること` + `use-auth.test.tsx` の `useLogout は成功時に QueryClient のキャッシュをクリア...` （キャッシュ消滅により保護ビューは新規 `/auth/me` 401 まで再取得できない） |
| 1.4 | 進行中の多重クリック抑止 | `logout-button.test.tsx` の `ログアウト処理中はボタンが非活性となり多重クリックを抑止する...` |
| 2.1 / 2.2 / 2.3 | Google / パスキー / 認証方式不問 | 実装上 `useLogout` は認証方式を判別せず一律 `POST /auth/logout` を呼ぶ設計。既存の `TestAuthHandler_Logout_Success_ClearsCookieAndRedirects` および新規 JSON 系テストが認証方式非依存で成立することで担保。認証方式別の分岐が存在しないことは `use-auth.ts` と `logout-button.tsx` の実装（分岐なし）で構造的に保証。 |
| 3.1 | クライアント側キャッシュクリア | `logout-button.test.tsx` の `QueryClient のキャッシュがクリアされること` + `use-auth.test.tsx` の同名テスト |
| 3.2 | 別ユーザー切替時に前ユーザーデータ非表示 | 3.1 のキャッシュクリアで成立する派生 AC。`queryClient.clear()` により全キャッシュが除去されるため、後続ユーザーが同一ブラウザで表示するデータは新規 fetch から始まる。 |
| 4.1 / 4.2 | 遷移先は実在パスでログイン画面 | `logout-button.test.tsx` の `ログアウト成功後は `/` に遷移し、`/login` には遷移しない...` |
| 4.3 | 非実在パスへは遷移しない | 上記テストで `mockAssign` が `/login` で呼ばれないことを検証 |
| 5.1 | ネットワーク失敗時のフィードバック | `logout-button.test.tsx` の `ネットワーク到達失敗時はエラー表示を提示し、ボタンを再操作可能に戻すこと` |
| 5.2 | 5xx でのフィードバック | `logout-button.test.tsx` の `サーバ 5xx 応答時はエラー表示を提示し、ボタンを再操作可能に戻すこと` |
| 5.3 | セッション期限切れ時もログイン画面到達 | サーバ実装により、`session.Logout` が失敗しても Cookie クリア + 204/303 を返す。すなわちクライアント側からは常に成功として観察され、ログイン画面に遷移する。`TestAuthHandler_Logout_JSONAccept_NoSession_ReturnsNoContent` がこの経路（session cookie 不在時にも 204）を担保。 |
| 5.4 | エラー表示に内部詳細を反射しない | `logout-button.test.tsx` の `エラー表示にサーバ応答の内部詳細（メッセージ / status 数値）を反射しないこと` |
| 6.1 | Cookie 属性・CSRF 前提維持 | `TestAuthHandler_Logout_JSONAccept_ReturnsNoContent` で HttpOnly / SameSite / MaxAge=-1 を assert。既存の `TestAuthHandler_Logout_Success_ClearsCookieAndRedirects` も保持。 |
| 6.2 | 従来 form POST 経路の互換性維持 | `TestAuthHandler_Logout_FormPost_StillRedirects` が Accept ヘッダ不在 / text/html / text/plain の 3 パターンで従来通りの 303 + Location を担保 |
| 6.3 | 対象外機能の既存挙動維持 | 全 Go / Web テストスイート pass（528 web + 全 Go handler）で担保 |
| 6.4 | ログイン画面の要素・導線に変化なし | `login-page.test.tsx` 等の既存テストが引き続き pass（本 spec で `login-page.tsx` は無変更） |
| NFR 1.1 | セッション ID・Cookie 値・PII をコンソール / エラー / URL に残さない | エラー表示は固定文言のみ（`5.4` テストで担保）。`onSuccess` の `window.location.assign("/")` はクエリ文字列なし |
| NFR 1.2 | ログアウト応答本文にセッション識別子 / PII を含めない | サーバは 204 No Content（本文なし）または 303 + Location（本文なし）を返すため構造的に成立 |
| NFR 2.1 / 2.2 / 2.3 | テスト容易性（正常系 / 異常系 / キャッシュ状態を実物到達なしで検証可能） | 追加した web テスト群は `mockFetch` / `window.location` モックで完結。Go テストは `httptest.NewRecorder` で完結 |

## 実装上の判断

- **API client への Accept 追加の副作用スコープ**: `apiClient` の変更は全 API リクエストに影響
  するが、既存の JSON API エンドポイントは content-type 分岐を行っていないため実挙動は不変。
  スキャン結果 `grep -r Content-Type.*application/json` で確認したテスト（api.test.ts /
  use-feeds.test.tsx）のみを最小限に更新した。
- **`acceptsJSON` の parse は defensive**: RFC 7231 の Accept ヘッダは q-value / パラメータ /
  ワイルドカードを含むが、本ユースケースは「fetch クライアントが `Accept: application/json` を
  明示的に送るかどうか」の 2 択判定なので、`application/*` や `*/*` の合致は敢えて **false** に
  倒している。これにより form POST ブラウザの通常 Accept で誤判定が起きない。
- **エラー表示のスタイル**: `text-destructive` / `text-xs` は shadcn/ui のテーマトークンをその
  まま活用。既存の UI 規約に沿う。
- **`role="alert"` の採用**: スクリーンリーダーに即座に読み上げられ、Requirement 5.1 / 5.2 が
  求める「ユーザーに失敗した旨を認識可能な表示」を満たしやすい。

## 実行コマンドと結果

- `go test ./...` — 全パッケージ pass（handler / auth / middleware / worker 等すべて OK）
- `go vet ./internal/handler/...` — 出力なし（clean）
- `gofmt -l internal/handler/auth_handler.go internal/handler/auth_handler_test.go` — 出力なし
  （本 PR で変更したファイルのみ gofmt-clean。事前に存在した format 差分は本 spec の scope 外
  として温存）
- `cd web && npm test -- --run` — 52 files / 528 tests pass
- `cd web && npm run lint` — errors: 0（warnings 5 は既存の pre-existing、本 PR で追加 / 修正
  したファイルに warning は無い）
- `cd web && npm run build` — production build 成功（`/` route 76.3 kB / First Load 201 kB。
  logout-button のバンドル寸法に増加なし）

## 変更ファイル一覧

- `internal/handler/auth_handler.go`（Logout ハンドラの content negotiation 対応 / `strings` import 追加 / `acceptsJSON` 追加）
- `internal/handler/auth_handler_test.go`（3 テスト追加）
- `web/src/lib/api.ts`（Accept ヘッダ追加）
- `web/src/lib/api.test.ts`（既存 assert 更新 + 1 テスト追加）
- `web/src/hooks/use-feeds.test.tsx`（既存 assert 更新）
- `web/src/hooks/use-auth.test.tsx`（2 テスト追加）
- `web/src/components/logout-button.tsx`（遷移先修正 + エラー表示追加）
- `web/src/components/logout-button.test.tsx`（既存テスト整備 + 6 テスト追加）

## 確認事項

- なし。要件定義（`requirements.md`）と Issue 本文から実装方針の判断材料が十分に得られており、
  仕様の解釈で PM / Architect に差し戻すべき曖昧箇所は発見しなかった。
- pre-existing の gofmt 差分（`internal/handler/crossfeed_handler_test.go` /
  `feed_handler_test.go` / `item_search_handler.go` / `router_full_test.go`）は本 spec の
  scope 外として温存した（Req 6.3 の「対象外の機能の既存挙動を変化させない」に合致するよう、
  format-only の書き換えを追加しない）。

STATUS: complete
