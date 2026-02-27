"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { AlertCircle, Play, Trash2 } from "lucide-react";
import {
  useUpdateFetchInterval,
  useUnsubscribe,
  useResumeFeed,
} from "@/hooks/use-subscriptions";
import type { Subscription } from "@/types/feed";

/** SubscriptionSettings コンポーネントのプロパティ */
interface SubscriptionSettingsProps {
  /** 対象の購読データ */
  subscription: Subscription;
  /** 購読解除完了時のコールバック */
  onUnsubscribed: () => void;
}

/** フェッチ間隔の選択肢（30分刻み、30分-12時間） */
const FETCH_INTERVAL_OPTIONS = [
  { value: 30, label: "30分" },
  { value: 60, label: "1時間" },
  { value: 90, label: "1時間30分" },
  { value: 120, label: "2時間" },
  { value: 150, label: "2時間30分" },
  { value: 180, label: "3時間" },
  { value: 210, label: "3時間30分" },
  { value: 240, label: "4時間" },
  { value: 270, label: "4時間30分" },
  { value: 300, label: "5時間" },
  { value: 330, label: "5時間30分" },
  { value: 360, label: "6時間" },
  { value: 390, label: "6時間30分" },
  { value: 420, label: "7時間" },
  { value: 450, label: "7時間30分" },
  { value: 480, label: "8時間" },
  { value: 510, label: "8時間30分" },
  { value: 540, label: "9時間" },
  { value: 570, label: "9時間30分" },
  { value: 600, label: "10時間" },
  { value: 630, label: "10時間30分" },
  { value: 660, label: "11時間" },
  { value: 690, label: "11時間30分" },
  { value: 720, label: "12時間" },
];

/**
 * 購読設定と管理UIコンポーネント
 *
 * フェッチ間隔設定の変更、購読解除の確認ダイアログ、
 * 停止中フィードの再開ボタンを提供する。
 */
export function SubscriptionSettings({
  subscription,
  onUnsubscribed,
}: SubscriptionSettingsProps) {
  const [showUnsubscribeDialog, setShowUnsubscribeDialog] = useState(false);

  const updateInterval = useUpdateFetchInterval();
  const unsubscribe = useUnsubscribe();
  const resumeFeed = useResumeFeed();

  /** フェッチ間隔変更ハンドラ */
  const handleIntervalChange = (value: string) => {
    const minutes = parseInt(value, 10);
    updateInterval.mutate({
      subscriptionId: subscription.id,
      fetchIntervalMinutes: minutes,
    });
  };

  /** 購読解除確定ハンドラ */
  const handleUnsubscribe = () => {
    unsubscribe.mutate(subscription.id, {
      onSuccess: () => {
        setShowUnsubscribeDialog(false);
        onUnsubscribed();
      },
    });
  };

  /** フィード再開ハンドラ */
  const handleResume = () => {
    resumeFeed.mutate(subscription.feed_id);
  };

  const isStopped =
    subscription.feed_status === "stopped" ||
    subscription.feed_status === "error";

  return (
    <div className="space-y-4 p-4 border rounded-lg">
      {/* 停止中フィードの警告とエラーメッセージ */}
      {isStopped && (
        <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm">
          <AlertCircle className="w-4 h-4 text-destructive flex-shrink-0 mt-0.5" />
          <div>
            <p className="font-medium text-destructive">
              フィードのフェッチが停止しています
            </p>
            {subscription.error_message && (
              <p className="mt-1 text-muted-foreground">
                {subscription.error_message}
              </p>
            )}
          </div>
        </div>
      )}

      {/* フェッチ間隔設定 */}
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium whitespace-nowrap">
          更新間隔
        </label>
        <Select
          value={String(subscription.fetch_interval_minutes)}
          onValueChange={handleIntervalChange}
          disabled={updateInterval.isPending}
        >
          <SelectTrigger
            data-testid="fetch-interval-select"
            className="w-[180px]"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FETCH_INTERVAL_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={String(option.value)}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* アクションボタン */}
      <div className="flex items-center gap-2">
        {/* 停止中フィードの再開ボタン */}
        {isStopped && (
          <Button
            variant="outline"
            size="sm"
            data-testid="resume-button"
            onClick={handleResume}
            disabled={resumeFeed.isPending}
          >
            <Play className="w-4 h-4 mr-1" />
            {resumeFeed.isPending ? "再開中..." : "フェッチ再開"}
          </Button>
        )}

        {/* 購読解除ボタン */}
        <Button
          variant="destructive"
          size="sm"
          data-testid="unsubscribe-button"
          onClick={() => setShowUnsubscribeDialog(true)}
        >
          <Trash2 className="w-4 h-4 mr-1" />
          購読解除
        </Button>
      </div>

      {/* 購読解除確認ダイアログ */}
      <AlertDialog
        open={showUnsubscribeDialog}
        onOpenChange={setShowUnsubscribeDialog}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>購読を解除しますか？</AlertDialogTitle>
            <AlertDialogDescription>
              「{subscription.feed_title}」の購読を解除します。
              この操作は取り消せません。記事の既読・スター状態も削除されます。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>キャンセル</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleUnsubscribe}
              disabled={unsubscribe.isPending}
            >
              {unsubscribe.isPending ? "解除中..." : "購読解除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
