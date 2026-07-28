# Issue #236 実装ノート

## 概要

Web UI に「アカウント設定」への導線を追加し、既存の dead code だった `WithdrawDialog`
を配線した。同時に、`WithdrawDialog` が呼び出していた退会エンドポイントが未登録の
`/api/account` になっていた不整合を修正し、実 route である `DELETE /api/users/me` に
差し替えた。パスキーのみアカウント（`email` が空文字）でも情報表示・退会が破綻しない
状態にした。

## 追加 / 変更ファイル

| ファイル | 種別 | 責務 |
|---|---|---|
| `web/src/components/withdraw-dialog.tsx` | 変更 | 退会 API を `DELETE /api/users/me` に修正、失敗時のインライン通知を追加、多重送信防止（既存 `isPending` + `preventDefault`） |
| `web/src/components/withdraw-dialog.test.tsx` | 変更 | エンドポイント assertion 更新、失敗時 / 多重送信防止テストを追加（9 ケース） |
| `web/src/components/account-settings-dialog.tsx` | 新規 | ヘッダー配下ギアから開く Dialog、アカウント情報表示 + 退会導線を含む |
| `web/src/components/account-settings-dialog.test.tsx` | 新規 | 単体テスト（10 ケース） |
| `web/src/components/app-shell.tsx` | 変更 | ヘッダーに `<AccountSettingsDialog />` を配置（`LogoutButton` の隣） |
| `web/src/components/app-shell.test.tsx` | 変更 | `/auth/me` モック追加、入口表示 / ダイアログ内容テストを追加（3 ケース） |
| `web/src/components/auth-guard.test.tsx` | 変更 | 未認証時に入口が render されないこと / 認証時に render されることを検証（2 ケース） |

## エンドポイント修正（`/api/account` → `/api/users/me`）の裏取り

- 修正前: `WithdrawDialog` は `apiClient.delete("/api/account")` を呼んでいたが、
  `internal/handler/router.go` L377-L388 の `/api/users` ルート配下に `/account` は
  登録されていない。したがって既存の状態で配線しても 404 で退会不能だった
- 修正後: `internal/handler/router.go` L382 の `r.Delete("/me", userHandler.Withdraw)`
  に対応する `DELETE /api/users/me` を呼び出す
- テストでも旧 `/api/account` 呼び出しが発生していないことを回帰テストで担保
  （`withdraw-dialog.test.tsx` の「退会実行時に DELETE /api/users/me を送信すること」）

## AC マッピング

| AC | 対応コード | 対応テスト |
|---|---|---|
| Req 1.1 (認証済み画面から視認可能な入口) | `app-shell.tsx` ヘッダーの `<AccountSettingsDialog />` | `app-shell.test.tsx` "ヘッダーにアカウント設定入口が表示されること"、`auth-guard.test.tsx` "認証済み時はアカウント設定入口 ... render されること" |
| Req 1.2 (情報表示 + 退会導線を含む画面) | `account-settings-dialog.tsx` `AccountSettingsBody` | `account-settings-dialog.test.tsx` "入口ボタンをクリックするとアカウント情報表示領域と退会導線を含むダイアログが開くこと"、`app-shell.test.tsx` "アカウント設定入口クリックで ... ダイアログが開くこと" |
| Req 1.3 (未認証時に入口非表示) | 構造的保証: `AuthGuard` が `AppShell` を render しない | `auth-guard.test.tsx` "未認証時はアカウント設定入口 ... render されないこと" |
| Req 1.4 (明示的な閉鎖) | radix-ui `Dialog` の Esc / overlay click / close button（既定挙動） | `account-settings-dialog.test.tsx` "ダイアログはユーザー操作 (Esc) で明示的に閉じられること" |
| Req 2.1 (表示名表示) | `AccountInfoSection` 内 `data-testid="account-info-name"` | `account-settings-dialog.test.tsx` "表示名と email が表示されること"、`app-shell.test.tsx` "アカウント設定入口クリックで ..." |
| Req 2.2 (email 有設定時表示) | `AccountInfoSection` の `emailIsSet` true 分岐 | `account-settings-dialog.test.tsx` "表示名と email が表示されること" |
| Req 2.3 (email 空 → 「未設定」プレースホルダ) | `AccountInfoSection` の `emailIsSet` false 分岐、`EMAIL_UNSET_LABEL="未設定"` | `account-settings-dialog.test.tsx` "email が未設定（空文字）のとき「未設定」プレースホルダが表示され、空欄放置されないこと" |
| Req 2.4 (取得失敗時通知) | `AccountSettingsBody` の `isError` 分岐（`role="alert"`、空 UI にしない） | `account-settings-dialog.test.tsx` "アカウント情報の取得に失敗したときエラー通知が表示され、空 UI にならないこと" |
| Req 3.1 (退会起動要素の表示) | `WithdrawSection` 内蔵の `<WithdrawDialog />` の trigger button | `account-settings-dialog.test.tsx` "入口ボタンをクリックすると ... 退会導線を含むダイアログが開くこと" |
| Req 3.2 (確認ダイアログ表示 + 取消不能明示) | `WithdrawDialog` の `<AlertDialog>` + description | `withdraw-dialog.test.tsx` "退会ボタンをクリックすると確認ダイアログが開くこと"、"確認ダイアログに削除・取り消し不能である旨の警告文が表示されること" |
| Req 3.3 (キャンセルで送信しない) | `AlertDialogCancel` の既定 close 挙動 | `withdraw-dialog.test.tsx` "キャンセルボタンで退会要求が送信されないこと"、`account-settings-dialog.test.tsx` "退会確認ダイアログでキャンセルすると DELETE を送信せず ..." |
| Req 3.4 (現行エンドポイントと整合する削除要求) | `withdraw-dialog.tsx` `apiClient.delete("/api/users/me")` | `withdraw-dialog.test.tsx` "退会実行時に DELETE /api/users/me を送信すること"、`account-settings-dialog.test.tsx` "退会確定で DELETE /api/users/me が発行され ..." |
| Req 3.5 (成功時未認証遷移) | `withdraw-dialog.tsx` onSuccess: `queryClient.clear()` + `onWithdrawn?.()`、`account-settings-dialog.tsx` `handleWithdrawn = () => window.location.assign("/login")` | `withdraw-dialog.test.tsx` "退会成功時に onWithdrawn が呼ばれ ..."、`account-settings-dialog.test.tsx` "退会確定で DELETE ... 成功時に /login へリダイレクトされること" |
| Req 3.6 (失敗時セッション維持 + 通知) | `withdraw-dialog.tsx` の `mutation.isError` 表示（`role="alert"`）、onSuccess 分離により `queryClient.clear()`/`onWithdrawn` は失敗時に呼ばれない | `withdraw-dialog.test.tsx` "退会失敗時（サーバ 500）に ..."、"退会失敗時（ネットワーク到達不能）でも ..."、`account-settings-dialog.test.tsx` "退会失敗時に /login へリダイレクトせず ..." |
| Req 3.7 (email 空パスキーアカウントで同一フロー成功) | 退会フローは email に依存しない（`DELETE /api/users/me` は Cookie セッションで対象特定） | `account-settings-dialog.test.tsx` "email 未設定（空文字）のパスキーのみアカウントでも退会が同一フローで完了すること" |
| Req 3.8 (多重送信防止) | `AlertDialogAction` の `disabled={withdrawMutation.isPending}` + `event.preventDefault()` で close 抑止 | `withdraw-dialog.test.tsx` "応答待機中は退会確定ボタンが disabled で多重送信されないこと" |
| NFR 1.1 (2 ペイン挙動不変) | `app-shell.tsx` の変更範囲はヘッダー内のみで 2 ペイン分岐に触れていない | 既存 `app-shell.test.tsx` の 2 ペイン系ケース群（54 件が引き続き pass） |
| NFR 1.2 (購読設定ダイアログ挙動不変) | `SubscriptionSettingsDialog` の import / 呼び出しは変更していない | 既存 `app-shell.test.tsx` の「購読解除フロー (task 6)」ケース群が引き続き pass |
| NFR 1.3 (ヘッダー既存機能不変) | `LogoutButton` / `ThemeToggle` を残置、隣に追加 | `app-shell.test.tsx` "ヘッダーにアカウント設定入口が表示されること" 内で既存 2 ボタン存在も併せて assert |
| NFR 2.1〜2.3 (自動テストによる検証可能性) | 上記 Req 2.1〜2.4 / 3.4〜3.7 / 1.1 / 1.3 の各テストで担保 | ↑ 参照 |

## 検証結果

作業ディレクトリ: `/home/hitoshi/.issue-watcher/worktrees/hitoshiichikawa-feedman/slot-1/web`

| コマンド | 結果 | 備考 |
|---|---|---|
| `npx vitest run src/components/withdraw-dialog.test.tsx` | 9 pass / 9 total | エンドポイント修正 + 失敗ハンドリング + 多重送信防止 |
| `npx vitest run src/components/account-settings-dialog.test.tsx` | 10 pass / 10 total | 新規コンポーネントの単体テスト |
| `npx vitest run src/components/app-shell.test.tsx src/components/auth-guard.test.tsx src/components/account-settings-dialog.test.tsx src/components/withdraw-dialog.test.tsx` | 54 pass / 54 total | 変更に関連する 4 ファイル横串 |
| `npx vitest run`（全 web テスト） | **536 pass / 536 total（53 ファイル）** | 既存テストの regression なし |
| `npx eslint` | **0 errors** / 5 warnings（既存の `<img>` / 未使用変数警告のみ、本 PR で導入していない） | 本 PR の新規コードにエラー・警告なし |
| `npx next build` | pass（`.next` 生成 / route `/` は 75.5 kB / 5 static pages） | 型エラーなし、build 通過 |

Go 側は変更していないため `go test ./...` / `gofmt` / `go vet` は今回スコープ外
（`internal/handler/router.go` は route 確認のため Read のみ）。

## 実装上の判断

### AlertDialogAction の close 抑止
radix-ui `AlertDialogAction` は onClick 後に既定で dialog を close する。従来コードは
成功時 `queryClient.clear()` により全 query が消えて `AuthGuard` が login にリダイレクト
するため、実挙動としては close 動作が問題にならなかった。しかし失敗時の通知（Req 3.6）を
ダイアログ内に残す必要が出たため、`event.preventDefault()` で既定 close を抑止し、
成功時のみ明示的に `setOpen(false)`（`onSuccess` 内で実施）する構成に変更した。多重送信
防止の `disabled` 表示更新にも同構成が有利。

### AccountSettingsBody を internal function として抽出
`AccountSettingsDialog` は open 制御のみを担い、`useCurrentUser` の state 別分岐を
`AccountSettingsBody` に集約した。Dialog が閉じている間は radix-ui `Portal` により
`AccountSettingsBody` はマウントされないため、不要な `/auth/me` 発火を避ける効果もある
（既存キャッシュがある場合はネットワーク発火せず即座に render）。

### 既存 LogoutButton パターンとの整合
退会成功後のリダイレクトは `window.location.assign("/login")` で行う。これは既存
`LogoutButton` と同一の遷移方式で、Web の未認証入口を統一する。仮に SPA 内遷移
（`next/navigation` 経由）に変えると認証系 boundary で AuthGuard 再判定を挟むため
挙動は等価だが、既存パターンを踏襲した。

### email 未設定文言「未設定」の採用
requirements.md Open Questions で PM が採用した「未設定」文言をそのまま採用（別文言
案「(メールアドレス未登録)」は既存 UI コピー方針との整合が確認できなかったため
保守的に requirements の記述を採用）。将来的な文言変更は本 PR とは別 Issue で扱う。

### 取得失敗時の通知手段（Req 2.4 / 3.6）
Open Questions で「トースト / インラインエラー等」が保留されていたが、既存
`feed-register-dialog.tsx` / `passkey-signup-dialog.tsx` / `ManualRefreshBanner`
がすべて **`role="alert"` の inline エラー領域** を採用しているため、本 PR も同様の
パターンに揃えた。トーストコンポーネントは web/ に存在しない（`@testing-library/react`
以外に toast lib を導入していない）ため、新規依存を増やさない選択でもある。

## 確認事項（レビュワー判断ポイント）

以下は requirements 未規定 / Open Questions 相当の点を Developer 裁量で確定した箇所です。
requirements.md / 既存 spec は書き換えず本ノートに列挙します。

1. **UI 形式は Dialog を採用**
   Open Questions で「ヘッダー配下のポップオーバー / Dialog / 独立ページ」が保留され
   ていたが、既存 `SubscriptionSettingsDialog` と同じく shadcn/ui `Dialog` を採用した。
   独立ページ化した場合の URL 設計・戻り導線・Next.js App Router との整合が本 Issue
   スコープを超えるため、既存パターンとの整合を優先した。将来的な独立ページ化への
   移行障壁は低い（`AccountSettingsBody` を router page から呼び出せる形で分離済み）。

2. **アイコンは `UserCog`（lucide-react）を採用**
   既存で `UserCog` の利用箇所はなく、`LogoutButton` の `LogOut` アイコンと視覚的に
   区別しやすい形で選択した。設定系メニューのアイコンとしても慣習的。

3. **通知手段は inline `role="alert"` エリア（トースト非採用）**
   前述「実装上の判断」で述べたとおり、既存パターンとの整合とライブラリ追加回避のため
   inline 通知を選択。トーストへの将来的な統一が必要な場合は別 Issue で全画面横断的に
   実施する方針が妥当。

4. **`useCurrentUser` のキャッシュ透過性**
   AppShell 経由で本ダイアログを開いた場合、`useCurrentUser` は AuthGuard がすでに
   fetch 済みのキャッシュを直接返すため、通常運用ではロード表示は瞬時に消える。
   `staleTime` を超えた場合の refetch もバックグラウンドで走り、期間中は既存データが
   維持される（TanStack Query の既定挙動）。テスト側はキャッシュを持たない fresh
   な QueryClient で render するため、loading / error 状態の分岐を明示的に検証できる。

5. **`account-settings-trigger` の onClick 反応性**
   本 button は `DialogTrigger asChild` に委譲されているため、独自の onClick は
   実装せず radix-ui に open 制御を任せている。app-shell テストのボタン発火も
   `userEvent.click` を通じて radix-ui 内部 handler が動く前提でパスしている。

6. **退会失敗時に「確認ダイアログを開いたまま残す」設計**
   Req 3.6 は「セッション破棄せず失敗通知」だけを求めており、確認ダイアログを閉じるか
   残すかは規定されていない。ユーザーが「そのまま再試行できる」ことが自然と判断して
   dialog を開いたまま alert を表示する構成にした。閉じてほしいという別要求が出た
   場合は `mutation.isError` 判定を挙げる箇所で `setOpen(false)` を追加するだけで
   切替可能。

7. **未認証時の Req 1.3 は構造的保証で満たす**
   `AuthGuard` が未認証で children を render しないため、`AppShell` 経由の入口は
   自動的に非表示になる。この構造的保証を `auth-guard.test.tsx` で
   `AccountSettingsDialog` を children に置いたケースで検証した。個別に「ボタンが
   401 レスポンス時に自身で非表示化する」実装は追加していない（同ボタンは 2 か所
   から呼ばれる想定がないため）。

STATUS: complete
