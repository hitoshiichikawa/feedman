import { describe, it, expect } from "vitest";
import { safeFeedUrl } from "./url";

describe("safeFeedUrl", () => {
  // 正常系: http/https はそのまま維持する
  it("https URL のときそのまま返すこと", () => {
    expect(safeFeedUrl("https://example.com/article")).toBe("https://example.com/article");
  });

  it("http URL のときそのまま返すこと", () => {
    expect(safeFeedUrl("http://example.com/article")).toBe("http://example.com/article");
  });

  it("大文字スキームの URL も許可すること", () => {
    expect(safeFeedUrl("HTTPS://example.com")).toBe("HTTPS://example.com");
  });

  it("前後の空白を除去して返すこと", () => {
    expect(safeFeedUrl("  https://example.com  ")).toBe("https://example.com");
  });

  // 異常系: 危険スキームを "#" へ無害化する
  it("javascript スキームのとき # を返すこと", () => {
    expect(safeFeedUrl("javascript:alert(1)")).toBe("#");
  });

  it("大文字混在の javascript スキームのとき # を返すこと", () => {
    expect(safeFeedUrl("JaVaScRiPt:alert(1)")).toBe("#");
  });

  it("先頭空白付きの javascript スキームのとき # を返すこと", () => {
    expect(safeFeedUrl("  javascript:alert(1)")).toBe("#");
  });

  it("data スキームのとき # を返すこと", () => {
    expect(safeFeedUrl("data:text/html,<script>alert(1)</script>")).toBe("#");
  });

  // 境界値: 相対・プロトコル相対・空・null/undefined
  it("相対 URL のとき # を返すこと", () => {
    expect(safeFeedUrl("/path/to/article")).toBe("#");
  });

  it("プロトコル相対 URL のとき # を返すこと", () => {
    expect(safeFeedUrl("//evil.example.com")).toBe("#");
  });

  it("空文字列のとき # を返すこと", () => {
    expect(safeFeedUrl("")).toBe("#");
  });

  it("null のとき # を返すこと", () => {
    expect(safeFeedUrl(null)).toBe("#");
  });

  it("undefined のとき # を返すこと", () => {
    expect(safeFeedUrl(undefined)).toBe("#");
  });
});
