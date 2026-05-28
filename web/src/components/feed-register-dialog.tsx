"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import type { ApiErrorResponse, FeedRegistrationResponse } from "@/types/api";
import { ApiError } from "@/lib/api";

/** FeedRegisterDialog のプロパティ */
interface FeedRegisterDialogProps {
  /** フィード登録成功時のコールバック */
  onRegistered: (feed: FeedRegistrationResponse) => void;
}

/** ダイアログの状態（登録成功時は即座にダイアログを閉じるため success phase は持たない） */
type DialogState =
  | { phase: "input" }
  | { phase: "loading" }
  | { phase: "error"; error: ApiErrorResponse };

/**
 * フィード登録ダイアログ
 *
 * URL入力欄1つでフィード登録を行うダイアログ。
 * 登録成功時はダイアログを自動で閉じ、左ペインのフィード一覧キャッシュを無効化する。
 * エラー表示（フィード未検出、購読上限到達、想定外エラー）を原因カテゴリと対処方法付きで表示する。
 */
export function FeedRegisterDialog({ onRegistered }: FeedRegisterDialogProps) {
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [dialogState, setDialogState] = useState<DialogState>({
    phase: "input",
  });

  const queryClient = useQueryClient();

  const registerMutation = useMutation({
    mutationFn: async (inputUrl: string) => {
      return apiClient.post<FeedRegistrationResponse>("/api/feeds", {
        url: inputUrl,
      });
    },
    onSuccess: (data) => {
      // 登録成功時はダイアログを即座に閉じる（要件 1.1 / 2.1〜2.3）
      setOpen(false);
      // フィード一覧のキャッシュを無効化（要件 1.2）
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
      // 呼び出し元コンポーネントへの登録完了通知（要件 1.3）
      onRegistered(data);
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.body) {
        const apiError = error.body as ApiErrorResponse;
        setDialogState({
          phase: "error",
          error: apiError,
        });
      } else {
        setDialogState({
          phase: "error",
          error: {
            code: "UNKNOWN",
            message: "予期しないエラーが発生しました",
            category: "system",
            action: "しばらく時間をおいてから再度お試しください",
          },
        });
      }
    },
  });

  /** ダイアログを開いた時の初期化（要件 1.4 / 4.2） */
  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
    if (isOpen) {
      setUrl("");
      setDialogState({ phase: "input" });
      registerMutation.reset();
    }
  };

  /** フィード登録を実行 */
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim()) return;
    setDialogState({ phase: "loading" });
    registerMutation.mutate(url.trim());
  };

  const isSubmitting = dialogState.phase === "loading";

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          <Plus className="w-4 h-4" />
          フィード追加
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>フィードを登録</DialogTitle>
          <DialogDescription>
            WebサイトまたはフィードのURLを入力してください
          </DialogDescription>
        </DialogHeader>

        {/* エラー表示 */}
        {dialogState.phase === "error" && (
          <div
            className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
            role="alert"
          >
            <p className="font-medium text-destructive">
              {dialogState.error.message}
            </p>
            <p className="mt-1 text-muted-foreground">
              {dialogState.error.action}
            </p>
          </div>
        )}

        {/* 入力フォーム（input / loading / error 時に表示。success phase は廃止済み） */}
        <form onSubmit={handleSubmit}>
          <div className="space-y-4">
            <Input
              type="url"
              placeholder="https://example.com"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              disabled={isSubmitting}
              autoFocus
            />
          </div>
          <DialogFooter className="mt-4">
            <Button type="submit" disabled={!url.trim() || isSubmitting}>
              {isSubmitting ? "登録中..." : "登録"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
