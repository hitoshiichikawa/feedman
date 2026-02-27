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
import { createApiClient } from "@/lib/api";

/** WithdrawDialog コンポーネントのプロパティ */
interface WithdrawDialogProps {
  /** 退会完了時のコールバック（リダイレクト処理などに使用） */
  onWithdrawn?: () => void;
}

/**
 * 退会確認ダイアログ
 *
 * ユーザーが退会を確認・実行するためのダイアログコンポーネント。
 * DELETE /api/account を呼び出してアカウントを削除する。
 * 成功時に認証解除とリダイレクトを行う。
 */
export function WithdrawDialog({ onWithdrawn }: WithdrawDialogProps) {
  const [open, setOpen] = useState(false);
  const api = createApiClient();
  const queryClient = useQueryClient();

  const withdrawMutation = useMutation({
    mutationFn: () => api.delete("/api/account"),
    onSuccess: () => {
      // 全キャッシュをクリア（認証情報も含む）
      queryClient.clear();
      setOpen(false);
      // コールバックが指定されている場合に実行（リダイレクト処理など）
      onWithdrawn?.();
    },
  });

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

      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>退会しますか？</AlertDialogTitle>
            <AlertDialogDescription>
              退会すると、すべてのデータが削除されます。この操作は取り消せません。
              購読情報、記事の既読・スター状態がすべて失われます。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>キャンセル</AlertDialogCancel>
            <AlertDialogAction
              data-testid="withdraw-confirm"
              onClick={() => withdrawMutation.mutate()}
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
