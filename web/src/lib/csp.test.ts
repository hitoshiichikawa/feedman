import { describe, it, expect } from "vitest";
import {
  CSP_HEADER_NAME,
  buildCspDirectives,
  buildContentSecurityPolicy,
} from "./csp";

/**
 * CSP 文字列値をパースして「ディレクティブ名 → 値配列」のマップに変換するテスト用ヘルパー。
 * Assert を読みやすくするための補助で、テスト対象の実装ロジックは含まない。
 */
function parseCsp(value: string): Map<string, string[]> {
  const map = new Map<string, string[]>();
  for (const part of value.split(";")) {
    const trimmed = part.trim();
    if (trimmed === "") continue;
    const [name, ...values] = trimmed.split(/\s+/);
    map.set(name, values);
  }
  return map;
}

describe("CSP_HEADER_NAME", () => {
  it("HTTP ヘッダーの正準名 Content-Security-Policy であること", () => {
    // Arrange / Act / Assert
    expect(CSP_HEADER_NAME).toBe("Content-Security-Policy");
  });
});

describe("buildContentSecurityPolicy", () => {
  it("production 環境のとき default-src 'self' を含むこと（Req 1.3 / 2.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("default-src")).toEqual(["'self'"]);
  });

  it("default-src にワイルドカード * を既定値として許可しないこと（Req 2.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("default-src")).not.toContain("*");
  });

  it("connect-src を production では 'self' のみに限定すること（Req 2.2）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("connect-src")).toEqual(["'self'"]);
  });

  it("font-src を 'self' に限定し外部フォント CDN を許可しないこと（Req 2.3 / 4.4）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("font-src")).toEqual(["'self'"]);
  });

  it("script-src に 'unsafe-inline' を含めブートストラップ inline script を許可すること（Req 5.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("script-src")).toContain("'unsafe-inline'");
  });

  it("style-src に 'unsafe-inline' を含め inline style を許可すること（Req 5.2）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("style-src")).toContain("'unsafe-inline'");
  });

  it("production 環境のとき script-src に 'unsafe-eval' を含めないこと（Req 5.3 / NFR 3）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("script-src")).not.toContain("'unsafe-eval'");
  });

  it("production 環境のとき connect-src に websocket（ws:）を含めないこと（Req 2 / 5.3）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("connect-src")).not.toContain("ws:");
  });

  it("development 環境のとき HMR 用に script-src へ 'unsafe-eval' を追加すること（Req 5.3）", () => {
    // Arrange
    const env = { NODE_ENV: "development" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("script-src")).toContain("'unsafe-eval'");
  });

  it("development 環境のとき HMR 用に connect-src へ websocket（ws:）を追加すること（Req 5.3）", () => {
    // Arrange
    const env = { NODE_ENV: "development" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("connect-src")).toContain("ws:");
  });

  it("img-src ディレクティブを明示的に定義すること（Req 6.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.has("img-src")).toBe(true);
  });

  it("img-src で HTTPS 外部画像と data URI を許可しつつワイルドカード * は許可しないこと（Req 6.2）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("img-src")).toEqual(["'self'", "data:", "https:"]);
    expect(csp.get("img-src")).not.toContain("*");
  });

  it("img-src の画像許可が script-src へ波及しないこと（Req 6.3）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    // img-src で許可した https: / data: が script-src に混入していないことを検証する。
    expect(csp.get("script-src")).not.toContain("https:");
    expect(csp.get("script-src")).not.toContain("data:");
  });

  it("OWASP 推奨の堅牢化として object-src 'none' / base-uri 'self' / frame-ancestors 'none' / form-action 'self' を含むこと（Req 2）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    expect(csp.get("object-src")).toEqual(["'none'"]);
    expect(csp.get("base-uri")).toEqual(["'self'"]);
    expect(csp.get("frame-ancestors")).toEqual(["'none'"]);
    expect(csp.get("form-action")).toEqual(["'self'"]);
  });

  it("ディレクティブをセミコロン区切りの 1 行文字列として直列化すること（NFR 2.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const value = buildContentSecurityPolicy(env);

    // Assert
    expect(value).toContain("; ");
    expect(value.startsWith("default-src 'self'")).toBe(true);
    expect(value).not.toContain("\n");
  });

  it("NODE_ENV が未設定（空入力）のとき dev 相当の緩いポリシー（'unsafe-eval' 含む）を生成すること（境界）", () => {
    // Arrange
    const env = {};

    // Act
    const csp = parseCsp(buildContentSecurityPolicy(env));

    // Assert
    // NODE_ENV 未設定は production ではないため dev 相当（安全側ではなく機能維持側に倒す。HMR 環境想定）。
    expect(csp.get("script-src")).toContain("'unsafe-eval'");
  });
});

describe("buildCspDirectives", () => {
  it("production 環境のとき全ての必須ディレクティブを含むマップを返すこと（Req 1.3 / 2 / 6.1）", () => {
    // Arrange
    const env = { NODE_ENV: "production" };

    // Act
    const directives = buildCspDirectives(env);

    // Assert
    const expectedKeys = [
      "default-src",
      "script-src",
      "style-src",
      "img-src",
      "font-src",
      "connect-src",
      "object-src",
      "base-uri",
      "frame-ancestors",
      "form-action",
    ];
    for (const key of expectedKeys) {
      expect(directives.has(key)).toBe(true);
    }
  });
});
