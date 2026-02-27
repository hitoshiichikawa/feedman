import { describe, it, expect } from "vitest";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui ユーティリティ関数のテスト
 *
 * Task 9.1: shadcn/ui の cn ユーティリティが正しく動作することを検証する
 */
describe("cn ユーティリティ関数", () => {
  it("単一のクラス名を返す", () => {
    expect(cn("text-red-500")).toBe("text-red-500");
  });

  it("複数のクラス名を結合する", () => {
    expect(cn("text-red-500", "bg-blue-500")).toBe("text-red-500 bg-blue-500");
  });

  it("条件付きクラス名を処理する", () => {
    expect(cn("base-class", false && "hidden", "visible")).toBe(
      "base-class visible"
    );
  });

  it("Tailwind の競合するクラスをマージする", () => {
    // tailwind-merge により後のクラスが優先される
    expect(cn("px-4", "px-8")).toBe("px-8");
  });

  it("undefined と null を無視する", () => {
    expect(cn("text-sm", undefined, null, "font-bold")).toBe(
      "text-sm font-bold"
    );
  });
});
