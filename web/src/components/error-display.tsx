"use client";

import { AlertCircle } from "lucide-react";
import type { ApiErrorResponse } from "@/types/api";

/** カテゴリごとの表示ラベルマップ */
const CATEGORY_LABELS: Record<string, string> = {
  auth: "認証エラー",
  validation: "入力エラー",
  feed: "フィードエラー",
  system: "システムエラー",
};

/** ErrorDisplay のプロパティ（個別指定） */
interface ErrorDisplayPropsIndividual {
  /** エラーカテゴリ（auth, validation, feed, system） */
  category: string;
  /** エラーメッセージ */
  message: string;
  /** ユーザー向け対処方法 */
  action: string;
  error?: never;
}

/** ErrorDisplay のプロパティ（ApiErrorResponseオブジェクト指定） */
interface ErrorDisplayPropsObject {
  /** APIエラーレスポンスオブジェクト */
  error: ApiErrorResponse;
  category?: never;
  message?: never;
  action?: never;
}

type ErrorDisplayProps = ErrorDisplayPropsIndividual | ErrorDisplayPropsObject;

/**
 * エラー表示コンポーネント
 *
 * 原因カテゴリと対処方法を提示する再利用可能なエラー表示コンポーネント。
 * 個別のプロパティまたはApiErrorResponseオブジェクトのどちらでも指定可能。
 */
export function ErrorDisplay(props: ErrorDisplayProps) {
  const category = props.error ? props.error.category : props.category;
  const message = props.error ? props.error.message : props.message;
  const action = props.error ? props.error.action : props.action;

  const categoryLabel = CATEGORY_LABELS[category] ?? "エラー";

  return (
    <div
      role="alert"
      className="rounded-md border border-destructive/50 bg-destructive/10 p-4"
    >
      <div className="flex items-start gap-3">
        <AlertCircle className="w-5 h-5 text-destructive flex-shrink-0 mt-0.5" />
        <div className="space-y-1">
          <p
            data-testid="error-category"
            className="text-xs font-medium text-destructive uppercase tracking-wide"
          >
            {categoryLabel}
          </p>
          <p className="text-sm font-medium text-destructive">{message}</p>
          <p className="text-sm text-muted-foreground">{action}</p>
        </div>
      </div>
    </div>
  );
}
