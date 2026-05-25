import { describe, it, expect } from "vitest";
import {
  API_INTERNAL_URL_ENV,
  buildRewrites,
  resolveApiInternalUrl,
} from "./rewrites";

describe("buildRewrites", () => {
  it("base が与えられたとき /api/:path* と /auth/:path* の 2 ルールをプレフィックス保持で返すこと", () => {
    // Arrange
    const base = "http://api:8080";

    // Act
    const rules = buildRewrites(base);

    // Assert
    expect(rules).toEqual([
      { source: "/api/:path*", destination: "http://api:8080/api/:path*" },
      { source: "/auth/:path*", destination: "http://api:8080/auth/:path*" },
    ]);
  });

  it("base に末尾スラッシュが含まれるとき二重スラッシュにならないよう正規化すること", () => {
    // Arrange
    const base = "http://api:8080/";

    // Act
    const rules = buildRewrites(base);

    // Assert
    expect(rules).toEqual([
      { source: "/api/:path*", destination: "http://api:8080/api/:path*" },
      { source: "/auth/:path*", destination: "http://api:8080/auth/:path*" },
    ]);
  });

  it("base に複数の末尾スラッシュが含まれるときすべて除去すること", () => {
    // Arrange
    const base = "https://example.com///";

    // Act
    const rules = buildRewrites(base);

    // Assert
    expect(rules[0].destination).toBe("https://example.com/api/:path*");
    expect(rules[1].destination).toBe("https://example.com/auth/:path*");
  });
});

describe("resolveApiInternalUrl", () => {
  it("有効な値が与えられたとき末尾スラッシュを除去した base を返すこと", () => {
    // Arrange
    const env = { [API_INTERNAL_URL_ENV]: "http://api:8080" };

    // Act
    const result = resolveApiInternalUrl(env);

    // Assert
    expect(result).toBe("http://api:8080");
  });

  it("末尾スラッシュ付きの値が与えられたとき正規化された base を返すこと", () => {
    // Arrange
    const env = { [API_INTERNAL_URL_ENV]: "http://api:8080/" };

    // Act
    const result = resolveApiInternalUrl(env);

    // Assert
    expect(result).toBe("http://api:8080");
  });

  it("変数が未設定のとき throw し、メッセージに変数名を含むこと", () => {
    // Arrange
    const env = {};

    // Act & Assert
    expect(() => resolveApiInternalUrl(env)).toThrow(API_INTERNAL_URL_ENV);
  });

  it("空文字が与えられたとき throw し、メッセージに変数名を含むこと", () => {
    // Arrange
    const env = { [API_INTERNAL_URL_ENV]: "" };

    // Act & Assert
    expect(() => resolveApiInternalUrl(env)).toThrow(API_INTERNAL_URL_ENV);
  });

  it("空白のみの値が与えられたとき throw すること", () => {
    // Arrange
    const env = { [API_INTERNAL_URL_ENV]: "   " };

    // Act & Assert
    expect(() => resolveApiInternalUrl(env)).toThrow(API_INTERNAL_URL_ENV);
  });
});
