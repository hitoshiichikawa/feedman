"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SubscriptionSettings } from "@/components/subscription-settings";
import type { Subscription } from "@/types/feed";

/** SubscriptionSettingsDialog のプロパティ */
interface SubscriptionSettingsDialogProps {
  /** ダイアログの open 制御。親（AppShell）から渡される */
  open: boolean;
  /**
   * 対象の購読データ。`null` のときは内容を描画しない（防御的ガード）。
   * `open` が true でも `subscription === null` なら何も描画しない。
   */
  subscription: Subscription | null;
  /**
   * ダイアログの open 状態が変化したときのコールバック。
   * Esc / Cancel / 外側クリックなどユーザー操作で閉じられたときに `false` で呼ばれる（AC 2.5）。
   */
  onOpenChange: (open: boolean) => void;
  /**
   * 購読解除成功時のコールバック。解除された subscription の `feed_id` を引数で受け取る。
   * AppShell 側で「解除されたフィードが現在右ペインに選択されていれば clear」分岐を行うため（AC 4.4）。
   */
  onUnsubscribed: (unsubscribedFeedId: string) => void;
}

/**
 * SubscriptionSettings を Dialog でラップする thin wrapper。
 *
 * フォーカストラップ / Esc 閉鎖 / 外側クリックでの閉鎖は radix-ui の既定挙動に依拠する（NFR 2.2）。
 * `open` 制御は親に委譲し、本コンポーネントは状態を持たない。
 */
export function SubscriptionSettingsDialog({
  open,
  subscription,
  onOpenChange,
  onUnsubscribed,
}: SubscriptionSettingsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>フィードの設定</DialogTitle>
          <DialogDescription className="sr-only">
            フィードの更新間隔の変更、フェッチ再開、購読解除を行います。
          </DialogDescription>
        </DialogHeader>
        {subscription !== null && (
          <SubscriptionSettings
            subscription={subscription}
            onUnsubscribed={(feedId) => {
              onUnsubscribed(feedId);
              onOpenChange(false);
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
