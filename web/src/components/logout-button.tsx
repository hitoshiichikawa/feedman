"use client";

import { LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useLogout } from "@/hooks/use-auth";

/**
 * ログアウトボタン
 *
 * POST /auth/logout を呼び出してセッションを破棄し、Feedman Web のルート `/` に
 * 遷移する。未認証状態で `/` にアクセスすると `AuthGuard` が `LoginPage` を表示
 * するため、ユーザーにはログイン画面が提示される（Issue #235 Requirement 4）。
 *
 * 従来は存在しない `/login` に遷移していたため 404 に落ちていた。
 *
 * 失敗時（ネットワーク到達失敗 / 5xx）はエラーメッセージを表示し、ボタンを
 * 再操作可能な状態に戻す（Requirement 5.1 / 5.2）。ボタンは処理中のみ非活性
 * となり、多重クリックによる重複要求を抑止する（Requirement 1.4）。
 */
export function LogoutButton() {
  const logoutMutation = useLogout();

  const handleLogout = () => {
    logoutMutation.mutate(undefined, {
      onSuccess: () => {
        // セッション破棄後にルート `/` へ遷移する。
        // 未認証で `/` にアクセスすると AuthGuard がログイン画面を表示する
        // （Feedman Web のルーティングには `/login` は存在しない）。
        window.location.assign("/");
      },
    });
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <Button
        variant="ghost"
        size="sm"
        onClick={handleLogout}
        disabled={logoutMutation.isPending}
      >
        <LogOut className="w-4 h-4" />
        ログアウト
      </Button>
      {logoutMutation.isError && (
        <p
          role="alert"
          className="text-xs text-destructive"
          data-testid="logout-error"
        >
          ログアウトに失敗しました。時間をおいて再度お試しください。
        </p>
      )}
    </div>
  );
}
