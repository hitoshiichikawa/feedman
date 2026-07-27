import { describe, it, expect, afterEach, vi } from "vitest";
import { isPasskeyBrowserSupported } from "./passkey-capability";

describe("isPasskeyBrowserSupported", () => {
  afterEach(() => {
    // 各テストで window.PublicKeyCredential を stub した後に元に戻す
    vi.unstubAllGlobals();
  });

  it("window.PublicKeyCredential が function として存在するとき true を返すこと", () => {
    // Arrange: PublicKeyCredential を function として差し込む
    const fakePkc = function PublicKeyCredential() {};
    vi.stubGlobal("PublicKeyCredential", fakePkc);

    // Act
    const supported = isPasskeyBrowserSupported();

    // Assert
    expect(supported).toBe(true);
  });

  it("window.PublicKeyCredential が undefined のとき false を返すこと", () => {
    // Arrange: PublicKeyCredential を明示的に undefined へ
    vi.stubGlobal("PublicKeyCredential", undefined);

    // Act
    const supported = isPasskeyBrowserSupported();

    // Assert
    expect(supported).toBe(false);
  });

  it("window.PublicKeyCredential が function 以外（object）のとき false を返すこと", () => {
    // Arrange: object を差し込む（typeof === "object"）
    vi.stubGlobal("PublicKeyCredential", { some: "object" });

    // Act
    const supported = isPasskeyBrowserSupported();

    // Assert
    expect(supported).toBe(false);
  });

  it("SSR 環境相当（window が undefined）のとき false を返すこと", () => {
    // Arrange: SSR 環境相当として window 自体を undefined 化する。
    // これにより `typeof window === "undefined"` の early return 分岐を通す。
    vi.stubGlobal("window", undefined);

    // Act
    const supported = isPasskeyBrowserSupported();

    // Assert
    expect(supported).toBe(false);
  });
});
