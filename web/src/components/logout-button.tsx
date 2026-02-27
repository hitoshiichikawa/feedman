"use client";

import { LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useLogout } from "@/hooks/use-auth";

/**
 * ログアウトボタン
 *
 * POST /auth/logout を呼び出してセッションを破棄し、
 * ログインページにリダイレクトする。
 */
export function LogoutButton() {
  const logoutMutation = useLogout();

  const handleLogout = () => {
    logoutMutation.mutate(undefined, {
      onSuccess: () => {
        // セッション破棄後にログインページへリダイレクト
        window.location.assign("/login");
      },
    });
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={handleLogout}
      disabled={logoutMutation.isPending}
    >
      <LogOut className="w-4 h-4" />
      ログアウト
    </Button>
  );
}
