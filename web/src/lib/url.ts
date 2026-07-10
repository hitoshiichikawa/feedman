/**
 * フィード由来 URL を `href` に出力する前の scheme 検証ユーティリティ。
 *
 * 記事の `link` はフィード（攻撃者が制御しうる外部サイト）由来の値であり、バックエンドの
 * bluemonday / DOMPurify による本文サニタイズの経路を通らない。`javascript:` や `data:` の
 * URL を `<a href>` に出力するとクリック時に任意スクリプトが実行されうるため（保存型 XSS）、
 * バックエンドの取り込み時検証（`internal/security/url_guard.go` の SafeExternalURL）に続く
 * 多層防御の最終層として、描画直前にもフロントエンドで scheme を検証する。
 *
 * 許可するのは絶対 URL の http / https のみ。それ以外（危険スキーム・相対 URL・
 * プロトコル相対 URL・空値）は安全な `"#"` に無害化する。
 */

/** http / https の絶対 URL のみを許可する正規表現（スキーム部のみ判定）。 */
const SAFE_URL_SCHEME = /^https?:\/\//i;

/**
 * フィード由来 URL を href 用に検証する。http/https のみ許可し、危険・不正な URL は
 * `"#"` を返す。
 *
 * @param url 検証対象の URL（null / undefined 許容）
 * @returns 安全な URL、または無害化された `"#"`
 */
export function safeFeedUrl(url: string | null | undefined): string {
  if (!url) {
    return "#";
  }
  const trimmed = url.trim();
  if (SAFE_URL_SCHEME.test(trimmed)) {
    return trimmed;
  }
  return "#";
}
