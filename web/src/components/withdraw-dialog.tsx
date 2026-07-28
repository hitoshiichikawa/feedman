"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { UserMinus } from "lucide-react";
import { apiClient } from "@/lib/api";

/** WithdrawDialog コンポーネントのプロパティ */
interface WithdrawDialogProps {
  /**
   * 退会完了時のコールバック。
   *
   * 退会が成功した後（キャッシュクリア済み）に呼ばれる。呼び出し元は通常この
   * コールバックでログイン画面等への遷移を行う（Req 3.5）。
   */
  onWithdrawn?: () => void;
}

/**
 * 退会確認ダイアログ
 *
 * ユーザーが退会を確認・実行するためのダイアログコンポーネント。
 * `DELETE /api/users/me` を呼び出してアカウントを削除する（Req 3.4）。
 * 成功時に認証キャッシュを破棄し、`onWithdrawn` コールバックで呼び出し元へ
 * 完了を通知する（Req 3.5）。失敗時はセッションを維持したままダイアログ内で
 * エラーメッセージを表示する（Req 3.6）。応答待機中は退会確定操作を非活性化して
 * 多重送信を防止する（Req 3.8）。
 */
export function WithdrawDialog({ onWithdrawn }: WithdrawDialogProps) {
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();

  const withdrawMutation = useMutation({
    mutationFn: () => apiClient.delete("/api/users/me"),
    onSuccess: () => {
      // 全キャッシュをクリア（認証情報も含む / Req 3.5）
      queryClient.clear();
      setOpen(false);
      // コールバックが指定されている場合に実行（リダイレクト処理など / Req 3.5）
      onWithdrawn?.();
    },
    // onError は宣言せず、mutation.isError を UI 側で参照して失敗表示に切替える。
    // Req 3.6: 失敗時にセッション（cookie / query cache）を破棄しないため、
    // onError 内で queryClient.clear() や onWithdrawn を呼ばないこと（既存 onSuccess の
    // 分離に依拠して二重呼び出しを防ぐ）。
  });

  /** 確認ダイアログの open/close 制御。閉じる際は前回のエラー表示状態も一緒にリセットする。 */
  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
    if (!isOpen) {
      withdrawMutation.reset();
    }
  };

  return (
    <>
      <Button
        variant="destructive"
        size="sm"
        data-testid="withdraw-trigger"
        onClick={() => setOpen(true)}
      >
        <UserMinus className="w-4 h-4 mr-1" />
        退会
      </Button>

      <AlertDialog open={open} onOpenChange={handleOpenChange}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>退会しますか？</AlertDialogTitle>
            <AlertDialogDescription>
              退会すると、すべてのデータが削除されます。この操作は取り消せません。
              購読情報、記事の既読・スター状態がすべて失われます。
            </AlertDialogDescription>
          </AlertDialogHeader>

          {/* 失敗時の通知（Req 3.6）。mutation.isError が真のときのみ表示し、
              セッション破棄・リダイレクトは行わない。再試行は「退会する」ボタンから可能。 */}
          {withdrawMutation.isError && (
            <div
              data-testid="withdraw-error"
              role="alert"
              className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
            >
              <p className="font-medium text-destructive">
                退会に失敗しました
              </p>
              <p className="mt-1 text-muted-foreground">
                通信状況を確認して、しばらくしてから再度お試しください。
              </p>
            </div>
          )}

          <AlertDialogFooter>
            <AlertDialogCancel disabled={withdrawMutation.isPending}>
              キャンセル
            </AlertDialogCancel>
            <AlertDialogAction
              data-testid="withdraw-confirm"
              onClick={(event) => {
                // radix-ui の AlertDialogAction は既定で close するため、mutation の
                // pending / error 状態を UI に反映するには close をキャンセルする必要がある。
                event.preventDefault();
                withdrawMutation.mutate();
              }}
              disabled={withdrawMutation.isPending}
            >
              {withdrawMutation.isPending ? "処理中..." : "退会する"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
