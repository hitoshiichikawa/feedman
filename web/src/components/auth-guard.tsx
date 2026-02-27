"use client";

import { useCurrentUser } from "@/hooks/use-auth";
import { LoginPage } from "@/components/login-page";
import { ApiError } from "@/lib/api";

/** AuthGuard のプロパティ */
interface AuthGuardProps {
  children: React.ReactNode;
}

/**
 * 認証ガードコンポーネント
 *
 * GET /auth/me で現在のユーザー情報を確認し、
 * 認証済みの場合は子コンポーネントを表示する。
 * 未認証の場合はログインページを表示する。
 */
export function AuthGuard({ children }: AuthGuardProps) {
  const { data, isLoading, isError, error } = useCurrentUser();

  // 認証確認中はローディング表示
  if (isLoading) {
    return (
      <div
        data-testid="auth-loading"
        className="flex min-h-screen items-center justify-center"
      >
        <div className="text-muted-foreground">読み込み中...</div>
      </div>
    );
  }

  // 未認証の場合はログインページへリダイレクト
  if (isError) {
    const isUnauthorized =
      error instanceof ApiError && error.status === 401;

    if (isUnauthorized) {
      return (
        <div data-testid="auth-redirect">
          <LoginPage />
        </div>
      );
    }

    // その他のエラー（ネットワークエラー等）の場合もログインページを表示
    return (
      <div data-testid="auth-redirect">
        <LoginPage />
      </div>
    );
  }

  // 認証済み: ユーザーデータが取得できた場合は子コンポーネントを表示
  if (data) {
    return <>{children}</>;
  }

  // フォールバック
  return null;
}
