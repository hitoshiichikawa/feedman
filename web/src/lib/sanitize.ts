/**
 * 記事本文 HTML のクライアント側サニタイズユーティリティ。
 *
 * バックエンド（`internal/security/content_sanitizer.go` の bluemonday ポリシー）に続く
 * 多層防御（defense in depth）の最終層として、記事本文を DOM 挿入する直前に
 * フロントエンドでもサニタイズする。バックエンドのサニタイズが想定外に弱まった場合や
 * 経路を迂回したコンテンツが届いた場合でも、ブラウザ上で危険なスクリプトが実行されない
 * ことを保証する。
 *
 * 許可タグ・許可属性はバックエンド bluemonday ポリシーと整合する範囲に維持する:
 *   - 許可タグ: p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img
 *   - a タグ: href（target / rel は描画側で扱う）
 *   - img タグ: src（https スキームのみ）, alt
 *   - script, iframe, style 要素および on* イベント属性は除去
 *   - javascript: などの危険スキームは無効化
 *
 * DOMPurify はブラウザ DOM を前提とするため、サーバー（SSR）など `window` が無い環境では
 * サニタイズを行わず空文字列を返す。記事本文の描画は `"use client"` コンポーネントから
 * 行われ、クライアント側のハイドレーション後に正しくサニタイズ結果が描画される。
 */

import DOMPurify from "dompurify";

/**
 * フロント側サニタイズで保持を許可する HTML タグの集合。
 * バックエンド bluemonday ポリシー（content_sanitizer.go）の許可タグと整合する。
 */
const ALLOWED_TAGS = [
  "p",
  "br",
  "a",
  "ul",
  "ol",
  "li",
  "blockquote",
  "pre",
  "code",
  "strong",
  "em",
  "img",
] as const;

/**
 * フロント側サニタイズで保持を許可する属性の集合。
 * a タグの href、img タグの src / alt のみを許可する。
 * on* イベントハンドラ属性は許可リストに含めないことで除去される。
 */
const ALLOWED_ATTR = ["href", "src", "alt"] as const;

/**
 * 許可する URI スキームの集合（正規表現）。
 * https / http / mailto / tel と相対 URL を許可し、javascript: などの危険スキームは除外する。
 * 画像 src の https 限定はバックエンドポリシーに準拠するが、フロント層では描画維持を優先し
 * リンクの http も許容する（危険スキームの実行防止が本層の主目的）。
 */
const ALLOWED_URI_REGEXP = /^(?:(?:https?|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i;

/**
 * DOM が利用可能（ブラウザ環境）かどうかを判定する。
 *
 * DOMPurify はサニタイズに DOM（`window.document`）を必要とするため、SSR など
 * DOM が存在しない環境では `false` を返す。
 *
 * @returns DOM が利用可能なら `true`
 */
function isDomAvailable(): boolean {
  return typeof window !== "undefined" && typeof window.document !== "undefined";
}

/**
 * 記事本文 HTML をクライアント側でサニタイズし、DOM 挿入に安全な HTML 文字列を返す。
 *
 * `<script>` / `<iframe>` / `<style>` 要素、`on*` インラインイベントハンドラ属性、
 * `javascript:` などの危険なスキームを除去・無効化しつつ、許可タグ（段落・改行・リンク・
 * リスト・引用・整形済み・コード・強調・画像）を保持する。
 *
 * 同一入力に対して常に同一の結果を返す（冪等）。空文字列の入力には空文字列を返す。
 * DOM が利用できない環境（SSR）では空文字列を返す。
 *
 * @param rawHtml - サニタイズ対象の生の記事本文 HTML
 * @returns サニタイズ済みで DOM 挿入に安全な HTML 文字列
 */
export function sanitizeContentHtml(rawHtml: string): string {
  if (rawHtml === "") {
    return "";
  }

  if (!isDomAvailable()) {
    // DOM が無い環境（SSR）では生 HTML を挿入させないため空文字列を返す。
    // クライアント側ハイドレーション後に正しくサニタイズされた内容が描画される。
    return "";
  }

  return DOMPurify.sanitize(rawHtml, {
    ALLOWED_TAGS: [...ALLOWED_TAGS],
    ALLOWED_ATTR: [...ALLOWED_ATTR],
    ALLOWED_URI_REGEXP,
  });
}
