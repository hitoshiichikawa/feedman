import { describe, it, expect } from "vitest";
import { sanitizeContentHtml } from "./sanitize";

describe("sanitizeContentHtml", () => {
  // Req 2.1: <script> 要素の除去
  it("script要素が含まれるとき当該要素を除去すること", () => {
    // Arrange
    const input = "<p>本文</p><script>alert('xss')</script>";

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("<script");
    expect(result).not.toContain("alert('xss')");
  });

  // Req 2.2: <iframe> 要素の除去
  it("iframe要素が含まれるとき当該要素を除去すること", () => {
    // Arrange
    const input = '<p>本文</p><iframe src="https://evil.example.com"></iframe>';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("<iframe");
  });

  // Req 2.2: <style> 要素の除去
  it("style要素が含まれるとき当該要素を除去すること", () => {
    // Arrange
    const input = "<style>body{display:none}</style><p>本文</p>";

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("<style");
    expect(result).not.toContain("display:none");
  });

  // Req 2.3: on* インラインイベントハンドラ属性の除去
  it("onerror属性が含まれるとき当該属性を除去すること", () => {
    // Arrange
    const input = '<img src="https://example.com/a.png" onerror="alert(1)" alt="a">';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("onerror");
    expect(result).not.toContain("alert(1)");
  });

  // Req 2.3: on* インラインイベントハンドラ属性の除去（onclick など別の on* 属性も対象）
  it("onclick属性が含まれるとき当該属性を除去すること", () => {
    // Arrange
    const input = '<p onclick="steal()">本文</p>';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("onclick");
    expect(result).not.toContain("steal()");
  });

  // Req 2.4: javascript: スキームの無効化・除去
  it("aタグのhrefにjavascriptスキームが含まれるとき当該属性を無効化または除去すること", () => {
    // Arrange
    const input = '<a href="javascript:alert(1)">クリック</a>';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).not.toContain("javascript:");
  });

  // Req 3.1: 許可タグ（段落・改行・リスト・引用・整形済み・コード・強調）の保持
  it("許可タグのみで構成されるとき当該タグを保持して描画すること", () => {
    // Arrange
    const input =
      "<p>段落<br><strong>強調</strong><em>斜体</em></p>" +
      "<ul><li>項目</li></ul><blockquote>引用</blockquote>" +
      "<pre><code>code</code></pre>";

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).toContain("<p>");
    expect(result).toContain("<br");
    expect(result).toContain("<strong>強調</strong>");
    expect(result).toContain("<em>斜体</em>");
    expect(result).toContain("<ul>");
    expect(result).toContain("<li>項目</li>");
    expect(result).toContain("<blockquote>");
    expect(result).toContain("<pre>");
    expect(result).toContain("<code>code</code>");
  });

  // Req 3.2: 許可された URL 属性を持つリンクの保持
  it("https URLを持つリンクが含まれるとき当該リンクを保持して描画すること", () => {
    // Arrange
    const input = '<a href="https://example.com/article">記事</a>';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).toContain('href="https://example.com/article"');
    expect(result).toContain("記事");
  });

  // Req 3.2: 許可された URL 属性を持つ画像の保持
  it("https srcを持つ画像が含まれるとき当該画像を保持して描画すること", () => {
    // Arrange
    const input = '<img src="https://example.com/image.png" alt="画像">';

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).toContain('src="https://example.com/image.png"');
    expect(result).toContain('alt="画像"');
  });

  // Req 1.3: 空文字列は空のまま（境界値）
  it("空文字列のとき空文字列を返すこと", () => {
    // Arrange
    const input = "";

    // Act
    const result = sanitizeContentHtml(input);

    // Assert
    expect(result).toBe("");
  });

  // Req 1.4 / NFR: 冪等性（同一入力に同一出力）
  it("同一入力に対して常に同一のサニタイズ結果を返すこと", () => {
    // Arrange
    const input =
      '<p>本文<a href="https://example.com">リンク</a></p>' +
      '<script>alert(1)</script>';

    // Act
    const first = sanitizeContentHtml(input);
    const second = sanitizeContentHtml(input);
    // サニタイズ済み出力を再度通しても変化しない（多層適用での安定性）
    const reapplied = sanitizeContentHtml(first);

    // Assert
    expect(first).toBe(second);
    expect(reapplied).toBe(first);
  });
});
