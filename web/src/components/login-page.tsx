"use client";

import { Button } from "@/components/ui/button";
import { API_BASE_URL } from "@/lib/api";

/**
 * ログインページ
 *
 * Googleアカウントでのログイン/登録ボタンを表示し、
 * OAuthフローへの遷移を行う。
 * ログインと登録は同一フロー（初回OAuth認証=登録、2回目以降=ログイン）。
 */
export function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="w-full max-w-sm space-y-8 text-center">
        {/* アプリケーションヘッダー */}
        <div className="space-y-2">
          <h1 className="text-4xl font-bold">Feedman</h1>
          <p className="text-muted-foreground">RSS/Atom フィードリーダー</p>
        </div>

        {/* ログインボタン */}
        <div className="space-y-4">
          <Button asChild className="w-full" size="lg">
            <a href={`${API_BASE_URL}/auth/google/login`}>Googleアカウントでログイン</a>
          </Button>
          <p className="text-xs text-muted-foreground">
            初回ログイン時にアカウントが自動作成されます
          </p>
        </div>
      </div>
    </div>
  );
}
