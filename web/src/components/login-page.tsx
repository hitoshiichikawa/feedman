"use client";

import { useCallback, useState } from "react";
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
  // review #6: アカウント作成後に session 合流失敗・WebAuthn キャンセル等が起きたとき、
  // Dialog は閉じられ本フラグが立つ。「アカウントは作成済みなのでログインへ」という復旧
  // バナーをログイン画面に表示し、ユーザーを「パスキーでログイン」導線へ誘導する
  // （Requirement 3.4: 2 ペイン UI に遷移させず、ログイン画面に復帰させ案内を残す）。
  const [accountCreatedNeedsLogin, setAccountCreatedNeedsLogin] =
    useState(false);

  // 新規作成をやり直すために Dialog を開くときは、古い復旧バナーを消してから開く。
  // review #3: Dialog の effect 依存に渡る callback を安定化させ、親の再 render で
  // effect が不必要に再発火するのを防ぐ（`onAccountCreatedNeedsLogin` も同様）。
  const openSignup = useCallback(() => {
    setAccountCreatedNeedsLogin(false);
    setSignupOpen(true);
  }, []);

  // review #3: inline arrow を毎 render 生成すると、Dialog の effect 依存が render ごとに
  // 変わり effect が再発火し得る。useCallback で参照を安定化する。
  const handleAccountCreatedNeedsLogin = useCallback(() => {
    setAccountCreatedNeedsLogin(true);
  }, []);

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

          {/* アカウント作成後・ログイン未完了時の復旧バナー（review #3 / #6 / Requirement 3.4）。
              Req 3.4 の「汎用的な失敗表示」として、作成は完了した一方でログイン処理に失敗した
              旨を汎用文言で提示し、復旧導線（パスキーでログイン）へ誘導する。 */}
          {accountCreatedNeedsLogin && (
            <div
              className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
              role="alert"
            >
              <p className="font-medium text-destructive">
                アカウントは作成されましたが、ログイン処理に問題が発生しました。下の「パスキーでログイン」からログインしてください。
              </p>
            </div>
          )}

          {/* パスキー導線（capability に応じて自動的に表示・非表示） */}
          <PasskeyButtons onSignupClick={openSignup} />
        </div>

        {/* パスキー新規作成 Dialog（open state は親が管理） */}
        <PasskeySignupDialog
          open={signupOpen}
          onOpenChange={setSignupOpen}
          onAccountCreatedNeedsLogin={handleAccountCreatedNeedsLogin}
        />
      </div>
    </div>
  );
}
