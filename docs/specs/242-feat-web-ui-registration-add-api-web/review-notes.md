# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-8 timestamp=2026-07-29T05:29:51Z -->

## Reviewed Scope

- Branch: claude/issue-242-impl-feat-web-ui-registration-add-api-web
- HEAD commit: 69d6fa31a40f0e180d9c5a66d934ebdb6ea3f847
- Compared to: develop..HEAD
- 補足: 本 Issue は design-less impl（`tasks.md` / `design.md` 不在）。`_Boundary:_`
  アノテーションが存在しないため、境界判定は CLAUDE.md のレイヤ配置ガイド
  （`web/src/hooks/` / `web/src/components/`）に照らして実施した。Feature Flag
  Protocol は本 repo で opt-out のため flag 観点の確認は行わない。

## Verified Requirements

- 1.1 — `account-settings-dialog.tsx` `AccountSettingsBody` が `data` 取得成功時のみ
  `<PasskeyAddSection />` を描画 / test: account-settings-dialog.test.tsx
  「認証済みユーザーがダイアログを開くとパスキー追加登録セクションが表示されること」
- 1.2 — ダイアログ未展開・データ無しでは DOM に非在 / test: 同上
  「入口ボタンを押さない（ダイアログを開かない）と追加登録起動要素は表示されないこと」
- 1.3 — `isLoading` 中は loading プレースホルダに置換しセクション非描画 / test:
  「アカウント情報取得中はパスキー追加登録セクションが表示されないこと」
- 1.4 — `isError || !data` で `role=alert` に置換しセクション非描画 / test:
  「アカウント情報取得に失敗したときはパスキー追加登録セクションが表示されないこと」
- 1.5 — `passkey-add-section.tsx` `handleAdd()` → `mutate()` / test:
  passkey-add-section.test.tsx「起動要素をクリックすると mutation.mutate が呼ばれること」
- 2.1 — `apiClient.post("/api/passkey/registration/add/begin", {})`（空ボディ）/ test:
  use-passkey-add-registration.test.tsx「begin→create→finish で成功し…begin は空ボディで送信されること」
- 2.2 — `navigator.credentials.create(creationOptions)` / test: 同上（create 呼出を assert）
- 2.3 — `encodeAttestationResponse` → `/add/finish` に credential 送信 / test: 同上（finish 呼出を assert）
- 2.4 — `passkey-add-success`（`role=status`）バナー / test:
  passkey-add-section.test.tsx「成功時に成功フィードバックが視認できる形で表示されること」
- 2.5 — 成功時に `auth/me` を invalidate しない / test:
  use-passkey-add-registration.test.tsx「成功時に auth/me query を invalidate しないこと」
- 3.1 — `already_registered` → 専用 testid + 独自文言 / test: passkey-add-section.test.tsx
  「重複エラー時に他のエラーと区別できる文言・testid で提示されること」
- 3.2 — `InvalidStateError` を STEP_NAV_CREATE で catch し finish 未実行 / test:
  use-passkey-add-registration.test.tsx「同一 authenticator の重複時…finish を呼ばないこと」
- 3.3 — エラー表示後も `passkey-add-trigger` が残る / test: 3.1 のテスト内で trigger 残存を assert
- 3.4 — 重複=専用 testid/文言、他=共通 testid の汎用文言 / test:
  「汎用エラー %s は共通の汎用文言で提示され、専用の重複エラー枠が出ないこと」
- 4.1 — `cancelled` はエラー非表示 + `mutation.reset()` / test:
  「cancelled 発生時はエラー表示を出さず mutation.reset() を呼ぶ」+ hook のキャンセル分類テスト
- 4.2 — 4xx → `server_rejected` 汎用文言、内部詳細非反射 / test:
  「finish の %i は server_rejected として分類され、内部 body が message に混入しないこと」
- 4.3 — 5xx/TypeError/AbortError → `server_error`/`network_error`、起動要素残存 / test:
  hook の各分類テスト + section「汎用エラー %s は…再試行可能な状態が維持される」
- 4.4 — ボタン `disabled={isPending}`「登録中...」/ test:
  「mutation pending 中は起動要素が disabled になり文言が「登録中...」になること」
- 4.5 — 終了後 `isPending` false で再有効化 / test:
  「mutation 終了後は起動要素が再操作可能な状態に戻ること」「再試行時に成功通知が消え、mutate が再度呼ばれること」
- 4.6 — エラー分類のいずれでも session 不変（auth/me 非 invalidate）/ test:
  「エラー分類のいずれでも起動要素が残り…」+ hook 非 invalidate テスト
- 5.1 — セクションは `email` を条件にせず `data` 有無のみで表示 / test:
  「Google 由来（email 未設定含む）ユーザーでも同一位置にパスキー追加導線が表示されること」
- 5.2 — Web 責務として `auth/me` 非 invalidate（OAuth 紐付けはサーバ #216 で担保）/ test:
  hook「成功時に auth/me query を invalidate しないこと」
- 5.3 — 登録済み件数を判定材料にせず常時表示 / test:
  passkey-add-section.test.tsx「パスキー追加の起動要素とロスト対策説明を表示すること」
- 6.1 — `passkey-add-loss-prevention` にロスト対策説明文 / test: 同上（loss-prevention 文言を assert）
- 6.2 — 説明文は固定文字列で機密情報非含有 / test:
  「ロスト対策説明文に credential 識別子等の機密情報を含めないこと」
- NFR 1.1〜1.5 — 既存ファイルは additive 差込のみ（`use-passkey-registration` / `AccountInfoSection`
  / `WithdrawSection` 不変）。新規/変更 3 スイート 48 テスト green を再実行で確認
- NFR 2.1 — attestation/challenge 生値を mutation closure に閉じ込め console/storage/URL/message に非出力
- NFR 2.2 — `PasskeyAddRegistrationError` は `kind` のみ、`ApiError.body` を message/DOM に非反射 / test:
  hook「内部 body が message に混入しないこと」+ section「内部詳細（kind 名 / status）が DOM に反射しない」
- NFR 2.3 — `apiClient`（同一オリジン相対パス）経由 / 既存契約に依拠
- NFR 2.4 — `dangerouslySetInnerHTML` / `eval` / 動的 script なし（追加コードに非在）
- NFR 3.1 — `web/src/lib/webauthn.ts` の `decodeCreationOptions` / `encodeAttestationResponse` を
  再利用（同関数の存在を確認済み。重複実装なし）
- NFR 4.1 — `navigator.credentials` / `apiClient` を stub/mock、jsdom で完結（外部依存なし）

## Findings

なし

## Summary

design-less impl として Req 1〜6 / NFR 1〜4 の全 numeric ID が実装（hook / section / dialog 配線）
とテストの双方で観測でき、新規/変更 3 スイート 48 テストを reviewer 側で再実行して green を確認した。
変更は `web/src/hooks/` と `web/src/components/` に閉じ、既存ファイルは additive 差込のみで
CLAUDE.md のレイヤ配置に整合。AC 未カバー / missing test / boundary 逸脱いずれも検出せず。

RESULT: approve
