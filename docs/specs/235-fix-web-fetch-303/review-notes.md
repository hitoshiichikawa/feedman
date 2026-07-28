# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-28T20:32:00Z -->

## Reviewed Scope

- Branch: claude/issue-235-impl-fix-web-fetch-303
- HEAD commit: 426143f1ae6bd3b43d2948914433d124837a35be
- Compared to: develop..HEAD
- 備考: 本 Issue は `tasks.md` / `design.md` を持たない design-less impl。`_Boundary:_` アノテーションは存在せず、boundary 判定は requirements.md が示す scope（`auth_handler.go` の Logout / `web/src/lib/api.ts` / `web/src/components/logout-button.tsx`）との照合で実施。Feature Flag Protocol は `opt-out`（flag 観点は非適用）。

## Verified Requirements

- 1.1 — `auth_handler.go:248` Logout が `h.service.Logout` でセッション破棄要求を発行。`TestAuthHandler_Logout_JSONAccept_ReturnsNoContent`（`logoutCalledWith` を assert）
- 1.2 — `logout-button.tsx` onSuccess → `window.location.assign("/")`（AuthGuard 経由でログイン画面）。`logout-button.test.tsx` の「ログアウト成功後は `/` に遷移し...」
- 1.3 — `use-auth.ts` onSuccess の `queryClient.clear()` で全キャッシュ除去 + サーバセッション破棄。`logout-button.test.tsx` / `use-auth.test.tsx` のキャッシュクリアテスト
- 1.4 — `logout-button.tsx` `disabled={logoutMutation.isPending}`。`logout-button.test.tsx` の「ログアウト処理中はボタンが非活性となり多重クリックを抑止する」（2 回目クリックで fetch 未発火を assert）
- 2.1 / 2.2 / 2.3 — `useLogout` / `Logout` ともに認証方式で分岐しない単一経路。分岐不在が構造的に保証され、1.1〜1.3 の各テストが認証方式非依存で成立
- 3.1 — `use-auth.ts` `queryClient.clear()`。`use-auth.test.tsx`「useLogout は成功時に QueryClient のキャッシュをクリア」+ `logout-button.test.tsx` の実キャッシュ消滅確認
- 3.2 — 3.1 の `clear()`（全キャッシュ除去）で成立する派生 AC。クリア後は後続ユーザーが新規 fetch から開始
- 4.1 / 4.3 — 遷移先を `/login`（非実在）→ `/` に修正。`logout-button.test.tsx` で `assign("/")` かつ `not.toHaveBeenCalledWith("/login")` を assert
- 4.2 — 未認証で `/` → AuthGuard が LoginPage を提示（`login-page.tsx` は無変更で導線維持）
- 5.1 — network reject → `mutation.isError` → `role="alert"` 表示 + ボタン再活性。`logout-button.test.tsx`「ネットワーク到達失敗時はエラー表示...再操作可能に戻す」
- 5.2 — `api.ts` 非 OK 応答で `ApiError` throw → エラー表示。`logout-button.test.tsx`「サーバ 5xx 応答時...」
- 5.3 — `Logout` はセッション未存在/破棄失敗でも Cookie クリア + 204 を返す。`TestAuthHandler_Logout_JSONAccept_NoSession_ReturnsNoContent`
- 5.4 — エラー表示は固定文言のみ。`logout-button.test.tsx`「内部詳細（メッセージ / status 数値）を反射しない」（stack / code / 500 を not.toContain）
- 6.1 — 両分岐で Cookie 属性（HttpOnly / SameSite=Lax / MaxAge=-1）を維持。`TestAuthHandler_Logout_JSONAccept_ReturnsNoContent` で assert
- 6.2 — Accept 非 JSON は従来通り 303 + Location。`TestAuthHandler_Logout_FormPost_StillRedirects`（Accept 不在 / text/html / text/plain の table-driven）
- 6.3 — `api.ts` の変更は Accept ヘッダ追加のみで既存 JSON エンドポイント挙動は不変。全 Go / web スイート pass
- 6.4 — `login-page.tsx` 無変更、ログイン画面の要素・導線に変化なし
- NFR 1.1 / 1.2 — エラー表示は固定文言・遷移 URL にクエリなし（5.4 テスト）/ 204・303 はボディなし（構造的）
- NFR 2.1 / 2.2 / 2.3 — 追加テストは `mockFetch` / `window.location` モック・`httptest.NewRecorder` で完結し実物到達不要

## Findings

なし

## Summary

design-less impl として requirements.md の全 numeric AC（1.1〜6.4 / NFR 1・2）に対応する実装とテストを確認。変更は Logout ハンドラの content negotiation・apiClient の Accept 付与・logout-button の遷移先修正 + エラー表示に限定され scope 内。reviewer 再実行で Go handler tests / web logout tests（39 passed）とも green。

RESULT: approve
