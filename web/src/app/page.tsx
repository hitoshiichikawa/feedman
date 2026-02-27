"use client";

import { AuthGuard } from "@/components/auth-guard";
import { AppShell } from "@/components/app-shell";

/**
 * メインページ
 *
 * AuthGuard で認証を確認し、認証済みの場合は AppShell（2ペインレイアウト）を表示する。
 * 未認証の場合は AuthGuard がログインページを表示する。
 */
export default function Home() {
  return (
    <AuthGuard>
      <AppShell />
    </AuthGuard>
  );
}
