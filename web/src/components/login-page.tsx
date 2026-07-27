"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { PasskeyButtons } from "@/components/passkey-buttons";
import { PasskeySignupDialog } from "@/components/passkey-signup-dialog";
import { API_BASE_URL } from "@/lib/api";

/**
 * ログインページ
 *
 * Google アカウントでのログイン/登録ボタンに加え、パスキー導線
 * （「パスキーでログイン」「アカウント新規作成」）とパスキー新規作成 Dialog を提示する。
 * パスキー導線は `usePasskeyCapability()` の判定で自動的に表示・非表示が切り替わるため、
 * 未対応環境（サーバ非提供 / ブラウザ非対応）では Google 単体構成に静かに縮退する
 * （Requirement 5.1 / 5.2 / 5.3 / 6.1 / 6.4 の縮退時互換）。
 */
export function LoginPage() {
  // Signup Dialog の open state を親（本コンポーネント）で管理する制御コンポーネントとして
  // `PasskeySignupDialog` を利用する（Dialog 自体はアンマウントされないが、
  // 「閉じたら次回は空欄で開く」UX は Dialog 内部の mutation state に依存する）。
  const [signupOpen, setSignupOpen] = useState(false);

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

          {/* パスキー導線（capability に応じて自動的に表示・非表示） */}
          <PasskeyButtons onSignupClick={() => setSignupOpen(true)} />
        </div>

        {/* パスキー新規作成 Dialog（open state は親が管理） */}
        <PasskeySignupDialog open={signupOpen} onOpenChange={setSignupOpen} />
      </div>
    </div>
  );
}
