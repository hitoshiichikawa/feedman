# 実装ノート (Issue #242 追加パスキー登録 UI の Web 配線)

## Implementation Notes

本 Issue は design-less impl（Architect を介さず要件のみを入力）。既存 Issue #216 の
サーバ側 add endpoint (`POST /api/passkey/registration/add/begin` / `finish`) は Web の
既存 Cookie セッションでそのまま到達可能なため、バックエンド変更は含まない。

### 変更ファイル一覧

| ファイル | 責務 | 種別 |
|---|---|---|
| `web/src/hooks/use-passkey-add-registration.ts` | 追加パスキー登録の mutation chain（begin → create → finish）と 5 種のエラー分類（`cancelled` / `already_registered` / `server_rejected` / `server_error` / `network_error`） | 新規 |
| `web/src/hooks/use-passkey-add-registration.test.tsx` | フックの単体テスト（成功系・キャンセル・重複・サーバ拒否・ネットワーク断・decode 失敗・auth/me 非 invalidate） | 新規 |
| `web/src/components/passkey-add-section.tsx` | AccountSettings ダイアログ内の「パスキーを追加」セクション UI（起動要素・成功/エラー表示・ロスト対策説明） | 新規 |
| `web/src/components/passkey-add-section.test.tsx` | セクション UI の単体テスト（起動・状態表示・重複と汎用エラーの分離・cancelled の非表示・再試行） | 新規 |
| `web/src/components/account-settings-dialog.tsx` | `AccountSettingsBody` の `data` 取得成功後に `PasskeyAddSection` を差し込む配線 | 変更 |
| `web/src/components/account-settings-dialog.test.tsx` | AccountSettingsDialog レベルでの Req 1.1 / 1.3 / 1.4 / 5.1 / 未認証相当（ダイアログ未展開時）確認テストを追加 | 変更 |

上記に加え、本ドキュメント `docs/specs/242-feat-web-ui-registration-add-api-web/impl-notes.md`
を新規追加している。requirements.md / 既存 spec は書き換えていない。

### 各 AC / NFR とテストの対応

| Req / NFR | 実装 | テスト |
|---|---|---|
| 1.1 認証済み時に起動要素を表示 | `AccountSettingsBody` が `data` を得た時のみ `<PasskeyAddSection />` を描画 | `account-settings-dialog.test.tsx` "認証済みユーザーがダイアログを開くとパスキー追加登録セクションが表示されること" |
| 1.2 未認証時は表示しない | ダイアログを開かない限り DOM に存在しない構造で担保 | `account-settings-dialog.test.tsx` "入口ボタンを押さない（ダイアログを開かない）と追加登録起動要素は表示されないこと" |
| 1.3 取得前は確定的な操作可能状態にしない | `isLoading` 中は loading プレースホルダで置換し、セクションを描画しない | `account-settings-dialog.test.tsx` "アカウント情報取得中はパスキー追加登録セクションが表示されないこと" |
| 1.4 取得失敗時も同様 | `isError || !data` で `role=alert` に置換し、セクションを描画しない | `account-settings-dialog.test.tsx` "アカウント情報取得に失敗したときはパスキー追加登録セクションが表示されないこと" |
| 1.5 起動要素操作で登録フロー開始 | `passkey-add-section.tsx` の `handleAdd()` で `mutate()` を呼び出す | `passkey-add-section.test.tsx` "起動要素をクリックすると mutation.mutate が呼ばれること" |
| 2.1 追加登録用チャレンジ取得 | `apiClient.post("/api/passkey/registration/add/begin", {})` を空ボディで呼び出し | `use-passkey-add-registration.test.tsx` "begin→create→finish で成功し、認証系ceremony を呼ばず、begin は空ボディで送信されること" |
| 2.2 ブラウザのパスキー作成 UI 起動 | `navigator.credentials.create(creationOptions)` を呼び出し | 同上 |
| 2.3 credential 情報を確定処理へ提示 | `encodeAttestationResponse` で JSON 化して `/api/passkey/registration/add/finish` に送信 | 同上 |
| 2.4 成功を視認できる形で提示 | `passkey-add-success` testid の `role=status` バナー表示 | `passkey-add-section.test.tsx` "成功時に成功フィードバックが視認できる形で表示されること" |
| 2.5 認証済みセッション維持 | 成功時 `queryClient.invalidateQueries(auth/me)` を呼ばない | `use-passkey-add-registration.test.tsx` "成功時に auth/me query を invalidate しないこと" |
| 3.1 重複を認識できる形で提示 | `already_registered` kind → `passkey-add-error-already-registered` testid の独自文言（「この認証器はすでに...」）| `passkey-add-section.test.tsx` "重複エラー時に他のエラーと区別できる文言・testid で提示されること" |
| 3.2 重複時に finish を呼ばない | `navigator.credentials.create` の `InvalidStateError` を catch し STEP_NAV_CREATE で分類 → finish 未実行 | `use-passkey-add-registration.test.tsx` "同一 authenticator の重複時（InvalidStateError）は already_registered に分類し finish を呼ばないこと" |
| 3.3 別 authenticator で再試行できる状態を維持 | エラー表示後も起動要素は disabled=false で残る | `passkey-add-section.test.tsx` "重複エラー時に..." で `passkey-add-trigger` が残ることを確認 |
| 3.4 他のキャンセル・拒否理由と区別 | 重複は専用 testid + 専用文言、他エラーは共通 testid の汎用文言 | `passkey-add-section.test.tsx` "汎用エラー %s は共通の汎用文言で提示され、専用の重複エラー枠が出ないこと" |
| 4.1 キャンセル時に画面を壊さず再試行提示 | `NotAllowedError` / `AbortError` / `null` を `cancelled` に集約し、UI でエラー表示を出さず `mutation.reset()` | `use-passkey-add-registration.test.tsx` "create のユーザーキャンセル..." / `passkey-add-section.test.tsx` "cancelled 発生時はエラー表示を出さず..." |
| 4.2 サーバ拒否時に汎用エラー表示 | `ApiError` 4xx → `server_rejected` に集約、内部詳細を message に反射しない | `use-passkey-add-registration.test.tsx` "finish の %i は server_rejected として分類され..." |
| 4.3 ネットワーク断・サーバ内部エラー時に再試行導線 | `TypeError` / `AbortError` → `network_error`, `ApiError` 5xx → `server_error`, いずれも起動要素は残る | `use-passkey-add-registration.test.tsx` "finish の 5xx / fetch reject / AbortError..." / `passkey-add-section.test.tsx` "汎用エラー %s は..." |
| 4.4 pending 中の多重送信防止 | ボタン `disabled={isPending}`, 文言「登録中...」 | `passkey-add-section.test.tsx` "mutation pending 中は起動要素が disabled になり文言が「登録中...」になること" |
| 4.5 終了後に再操作可能に戻す | mutation の `isPending` が false に戻ることで再有効化。isSuccess のまま次回起動時は `reset()` + `mutate()` で成功通知を隠す | `passkey-add-section.test.tsx` "mutation 終了後は起動要素が再操作可能な状態に戻ること" / "再試行時に成功通知が消え、mutate が再度呼ばれること" |
| 4.6 キャンセル・失敗時のセッション維持 | mutation はサーバ拒否・断で session を変えない設計（`auth/me` を invalidate しない） | `passkey-add-section.test.tsx` "エラー分類のいずれでも起動要素が残り..." |
| 5.1 Google 由来ユーザーでも同一位置・同一操作性 | `PasskeyAddSection` は `user.email` を条件に取らず、`data` 有無だけで表示される | `account-settings-dialog.test.tsx` "Google 由来（email 未設定含む）ユーザーでも同一位置にパスキー追加導線が表示されること" |
| 5.2 Google 紐付けは解除・変更されない | 追加登録 chain はサーバ側で `user_id` に紐付く credential 追加のみで OAuth 紐付けを触らない（サーバ側 #216 実装）+ 成功時に `auth/me` を invalidate しない設計 | サーバ側は Issue #216 の既存テストで担保。Web 側は auth/me 非 invalidate（`use-passkey-add-registration.test.tsx`）で本 spec の Web 責務を担保 |
| 5.3 既存パスキー持ち・初回パスキーで同一導線 | セクションは登録済み件数を判定材料にしない | 導線が常に表示される点を `passkey-add-section.test.tsx` "パスキー追加の起動要素とロスト対策説明を表示すること" で担保 |
| 6.1 ロスト対策説明を表示 | `passkey-add-loss-prevention` testid で「別の端末や同期先を追加しておくと...」文言を表示 | `passkey-add-section.test.tsx` "パスキー追加の起動要素とロスト対策説明を表示すること" |
| 6.2 説明文に機密情報を含めない | 説明文は固定文字列、credential/user id/email 等の識別子キーワードを含まない | `passkey-add-section.test.tsx` "ロスト対策説明文に credential 識別子等の機密情報を含めないこと" |
| NFR 1.1〜1.4 既存機能非破壊 | 既存 `use-passkey-registration` / `AccountInfoSection` / `WithdrawSection` / 既存 fetch 動線は一切改変なし。追加コンポーネントのみを差し込み | 既存全 55 スイート 583 テストが green を維持 |
| NFR 1.5 既存テスト green 維持 | 上記のとおり全既存テストが green | `npm test` 全 pass |
| NFR 2.1 attestation・challenge 生値の非漏出 | フックの mutation 内 closure に閉じ込め、`console.*` / storage / URL / エラー message に一切書き出さない | ハンドラ・UI どちらのテストでも error.message に internal データが混入しないことを検査 |
| NFR 2.2 サーバ拒否詳細の非反射 | `PasskeyAddRegistrationError` は `kind` のみで区別、`ApiError.body` の内部詳細を message に含めない | `use-passkey-add-registration.test.tsx` "finish の %i は server_rejected として分類され、内部 body が message に混入しないこと" / UI 側でも kind 名が DOM に反射しないことを検査 |
| NFR 2.3 同一オリジンのみ | `apiClient`（`API_BASE_URL = ""`）経由で同一オリジン相対パス。他の URL を組み立てない | 既存 `apiClient` 契約に依拠（`web/src/lib/api.ts`） |
| NFR 2.4 CSP / sanitize 方針の非緩和 | 追加なし。dangerouslySetInnerHTML / eval / 動的 script 追加なし | 実装ファイルの grep で明確 |
| NFR 3.1 WebAuthn utility 再利用 | `decodeCreationOptions` / `encodeAttestationResponse` を `@/lib/webauthn` から import | フックのテストで両関数がモック経由で呼ばれることを確認 |
| NFR 4.1 テスト容易性（ブラウザ・外部依存なしで検証可能） | `navigator.credentials` と `apiClient` を vi.stubGlobal / vi.mock で置換、jsdom 環境で完結 | フック 20 テスト + UI 13 テスト + ダイアログ統合 15 テスト |

### 重要な設計判断

1. **`InvalidStateError` を `cancelled` と別 kind に分離**（Req 3 の中核要件）
   - 既存 `use-passkey-registration.ts` は `NotAllowedError` / `AbortError` / `InvalidStateError` を
     まとめて `CANCEL_ERROR_NAMES` セットで `cancelled` に集約している（新規作成では
     excludeCredentials が空のため、`InvalidStateError` の意味論的な役割が薄い）。
   - 追加登録では `excludeCredentials` に登録済み credential ID が含まれるため、同一 authenticator
     を選ぶと `InvalidStateError` がブラウザから明示的に発火する。本フックではこの意味を保存し、
     `already_registered` として **独立分類**する。
   - 既存フックの分類は **一切変更していない**（`use-passkey-registration.ts` の
     `PasskeyRegistrationErrorKind` は温存 / requirements.md「Out of Scope」でも明記）。

2. **finish の uncertain 状態を独立 kind に分けなかった理由**
   - 新規作成では finish 失敗が「サーバに commit されたか不明」と UX 上重要（登録済かどうかで
     次の導線が変わる）。
   - 追加登録では finish が失敗しても、再試行 begin が返す `excludeCredentials` に既に登録済み
     credential が含まれるため、同じ authenticator では `InvalidStateError` になり `already_registered`
     に自然に集約される。ユーザーは別 authenticator で再試行できる（Req 3.3 と整合）。そのため
     新規作成側の `registration_uncertain` 相当を持たず、`network_error` / `server_error` に集約した。

3. **`204 No Content` を安全に受け取る仕組み**
   - 既存 `apiClient` は 204/205 で `response.json()` を呼ばず `undefined` を返す契約
     （`web/src/lib/api.ts` L114-121）。add/finish は `apiClient.post<void>` として呼び出し、
     `ResponseParseError` を誘発しない。
   - 過去 Issue #213 系で 204 のパスがすでに担保されているため、本 spec で追加の仕組みは不要。

4. **成功後の再操作 UX**
   - 一度成功した後に「もう 1 台追加する」ケースを想定し、`isSuccess` のまま再クリック
     したときには `reset()` → `mutate()` を呼び、`useEffect(() => setDismissedSuccess(false), [isSuccess])`
     で新たな成功が来たら通知を再表示する設計にした。テスト観点上、mock hook で
     `isSuccess=true` のままの状態でも「クリックで mutate と reset が呼ばれる」ことを
     検査できる。

5. **成功時の `auth/me` 非 invalidate**（Req 2.5）
   - 追加登録はセッションを変えないため `queryClient.invalidateQueries({queryKey: ["auth","me"]})`
     を呼ばない。呼ぶとダイアログ外の UI（`AuthGuard` / ヘッダーのユーザ表示など）が再取得を
     走らせ「他画面の表示・既存機能を変化させない」に反する。テストで invalidate が呼ばれない
     ことを assert 済み。

### 実行したテスト結果

- **`npm test`**: 55 スイート / 583 テスト すべて pass（新規追加は 3 スイート合計 48 テスト。
  内訳: hook 20 / UI 13 / dialog 統合 15）。所要時間 ~81 秒。
- **`npm run lint`**: 新規/変更ファイル分の警告・エラー **なし**。既存の 5 件 warning（feed-favicon,
  search-results, theme-provider.test.tsx, app-state.test.tsx 由来）は本 PR 対象外。
- **`npm run build`**: production build 成功（`Compiled successfully` + `Generating static pages` 完了）。
- **`npx tsc --noEmit`**: 新規/変更ファイル分のエラー **0 件**。既存の TypeScript エラー（39 行、
  feed-list.test.tsx / starred-item-list.test.tsx / starred-nav-item.test.tsx / rewrites.test.ts）は
  本 PR 前後で件数変化なし = 本 PR とは無関係な既存問題。

### 確認事項

- **UI コピー・強調度**: requirements.md「Open Questions」で `design で確定` とされていた
  「ロスト対策説明の文言」「重複エラー文言」「成功フィードバック手段」は design がないため
  実装者判断で以下を採用:
  - ロスト対策: 「別の端末や同期先のパスキーを追加しておくと、片方の端末を失っても
    アカウントにアクセスできなくなるリスクを下げられます。」
  - 重複: 「この認証器はすでにアカウントに登録されています。別の端末や別のパスキー同期先を
    選んでください。」
  - 成功: `role=status` + 「パスキーを追加しました。」（インラインバナー）
  - 汎用エラー: `role=alert` + 「パスキーの追加に失敗しました。時間をおいて再度お試しください。」
  レビュワー判断でコピー・強調度は差し戻し可能。
- **UI 配置**: Account Settings ダイアログ内で `AccountInfoSection` の直下、`WithdrawSection` の
  直上に独立セクションとして配置した（退会と並列だが、ロスト対策文脈で登録直下が自然）。
  Open Questions の「独立セクション / 既存セクション周辺」は独立セクション案を採用。
- **追加登録件数の上限・可視化**: `Out of Scope`（credential 一覧 UI）に沿って登録済み件数を
  表示しない。連続追加時は成功通知を再表示する UX で担保しているが、ユーザーが「今何個
  登録されているか」を確認する導線は本 spec 対象外。将来の Issue で扱う想定。
- **既存 `use-passkey-registration.ts` は変更していない**（Out of Scope の明記に従う）。
  新規作成側で `InvalidStateError` を `cancelled` に含める現行挙動は、ログインフローの
  「作成前は excludeCredentials が空のため InvalidStateError は理論上発生しない」性質を
  前提としている。将来「作成前 InvalidStateError」観点を追加する場合は別 spec で。

STATUS: complete
