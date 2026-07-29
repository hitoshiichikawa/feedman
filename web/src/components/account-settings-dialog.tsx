"use client";

import { useState } from "react";
import { UserCog } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useCurrentUser } from "@/hooks/use-auth";
import { WithdrawDialog } from "@/components/withdraw-dialog";
import { PasskeyAddSection } from "@/components/passkey-add-section";
import type { User } from "@/types/auth";

/**
 * 未設定 email 表示用プレースホルダ（Req 2.3）。
 *
 * requirements.md の Open Questions に従い「未設定」を採用する。空欄放置しない
 * ことで、パスキーのみ登録アカウントでも自分がどの主体でログインしているかを
 * 明確に把握できるようにする。
 */
const EMAIL_UNSET_LABEL = "未設定";

/**
 * 退会成功時のリダイレクト先。
 *
 * 既存 LogoutButton と同じ `/login` へ遷移させ、Web の未認証入口を統一する
 * （Req 3.5 / NFR 1.3 の既存ヘッダー機能非破壊性と整合）。
 */
const LOGIN_URL = "/login";

/**
 * アカウント設定ダイアログ
 *
 * ヘッダー配下のギア型トリガーから開かれる Dialog。中身は以下 2 セクション:
 *
 * 1. 現在ログイン中ユーザーの表示名 / email（未設定なら「{@link EMAIL_UNSET_LABEL}」）
 * 2. 退会導線としての {@link WithdrawDialog}
 *
 * `useCurrentUser` の状態（loading / error / success）で本文を出し分けし、
 * 取得失敗時には role="alert" のエラー表示に切替える（Req 2.4）。
 * open state はローカル state のみで管理し、ヘッダー側の状態と切り離す。
 */
export function AccountSettingsDialog() {
  const [open, setOpen] = useState(false);

  /**
   * 退会成功時のリダイレクト。既存 LogoutButton と同一の遷移方式を採用し、
   * QueryClient 側のキャッシュクリアは {@link WithdrawDialog} 内で完了済み（Req 3.5）。
   */
  const handleWithdrawn = () => {
    window.location.assign(LOGIN_URL);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          data-testid="account-settings-trigger"
          aria-label="アカウント設定"
        >
          <UserCog className="w-4 h-4" />
          アカウント
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>アカウント設定</DialogTitle>
          <DialogDescription className="sr-only">
            ログイン中のアカウント情報の確認と退会操作を行います。
          </DialogDescription>
        </DialogHeader>
        <AccountSettingsBody onWithdrawn={handleWithdrawn} />
      </DialogContent>
    </Dialog>
  );
}

/** AccountSettingsBody の props */
interface AccountSettingsBodyProps {
  /** 退会完了時のコールバック（親のリダイレクト処理） */
  onWithdrawn: () => void;
}

/**
 * アカウント設定ダイアログの本文。
 *
 * useCurrentUser の状態別に描画を切替える。取得失敗時は role="alert" のエラー
 * 表示で「空 UI にしない」ことを保証する（Req 2.4）。
 */
function AccountSettingsBody({ onWithdrawn }: AccountSettingsBodyProps) {
  const { data, isLoading, isError } = useCurrentUser();

  if (isLoading) {
    return (
      <div
        data-testid="account-settings-loading"
        className="text-sm text-muted-foreground"
      >
        読み込み中...
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div
        data-testid="account-settings-error"
        role="alert"
        className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
      >
        <p className="font-medium text-destructive">
          アカウント情報の取得に失敗しました
        </p>
        <p className="mt-1 text-muted-foreground">
          通信状況を確認して、ダイアログを閉じてから再度お試しください。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <AccountInfoSection user={data} />
      {/*
       * 追加パスキー登録セクション (Issue #242 / Req 1)。
       * `data` が取得成功したときのみ本要素が描画されるため、未認証・取得前・取得失敗時は
       * 構造で自動的に非表示になる（Req 1.2 / 1.3 / 1.4）。
       */}
      <PasskeyAddSection />
      <WithdrawSection onWithdrawn={onWithdrawn} />
    </div>
  );
}

/** ユーザー情報表示セクションの props */
interface AccountInfoSectionProps {
  /** useCurrentUser で取得したユーザー情報 */
  user: User;
}

/**
 * アカウント情報表示セクション（Req 2.1〜2.3 / Issue #241 Req 3.1〜3.4）。
 *
 * 表示名を必ず表示し、username は非 null / 非空のときのみ描画する（null / 空文字は
 * 要素そのものを DOM に出さない / Req 3.3）。email は空文字なら「未設定」ラベルに
 * 差替える（既存挙動 / Req 3.4 の非破壊性）。
 */
function AccountInfoSection({ user }: AccountInfoSectionProps) {
  const emailIsSet = user.email !== "";

  return (
    <section
      data-testid="account-info-section"
      className="space-y-2 rounded-lg border p-4"
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-xs font-medium text-muted-foreground">
          表示名
        </span>
        <span
          data-testid="account-info-name"
          className="text-sm font-medium break-all"
        >
          {user.name}
        </span>
      </div>
      {/*
       * username 表示行（Issue #241 / Req 3.2 / Req 3.3）。
       * `user.username != null && user.username !== ""` を満たすときのみ描画し、
       * null / 空文字なら要素そのものを DOM に出さない（email 未設定時の代替ラベル
       * のようなプレースホルダは出さない）。表示位置は表示名行の直後 / email 行の
       * 直前（Req 3 の Objective「ログイン主体識別」に沿い、表示名 = 人間可読ラベル、
       * username = システム上の識別子を隣接表示する意図）。
       */}
      {user.username != null && user.username !== "" && (
        <div className="flex items-baseline justify-between gap-3">
          <span className="text-xs font-medium text-muted-foreground">
            ユーザー名
          </span>
          <span
            data-testid="account-info-username"
            className="text-sm font-medium break-all"
          >
            {user.username}
          </span>
        </div>
      )}
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-xs font-medium text-muted-foreground">
          メールアドレス
        </span>
        {emailIsSet ? (
          <span
            data-testid="account-info-email"
            className="text-sm font-medium break-all"
          >
            {user.email}
          </span>
        ) : (
          <span
            data-testid="account-info-email-unset"
            className="text-sm font-medium text-muted-foreground italic"
          >
            {EMAIL_UNSET_LABEL}
          </span>
        )}
      </div>
    </section>
  );
}

/** WithdrawSection の props */
interface WithdrawSectionProps {
  /** 退会完了時のコールバック */
  onWithdrawn: () => void;
}

/**
 * 退会導線セクション（Req 3.1）。
 *
 * 実際の確認ダイアログと DELETE 送信は {@link WithdrawDialog} が担う。本セクション
 * はコンテキスト説明と入口配置のみを担当し、責務を単一に保つ。
 */
function WithdrawSection({ onWithdrawn }: WithdrawSectionProps) {
  return (
    <section
      data-testid="account-withdraw-section"
      className="space-y-2 rounded-lg border p-4"
    >
      <h3 className="text-sm font-medium">アカウントの削除</h3>
      <p className="text-xs text-muted-foreground">
        退会するとアカウントとすべてのデータが削除されます。
      </p>
      <div className="pt-1">
        <WithdrawDialog onWithdrawn={onWithdrawn} />
      </div>
    </section>
  );
}
