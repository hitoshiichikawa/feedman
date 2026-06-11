import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { formatRelativeDate } from "./date";

describe("formatRelativeDate", () => {
  // 現在時刻を固定し、相対表現の各分岐を決定論的に検証する。
  const NOW = new Date("2026-06-10T12:00:00Z");

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("1時間未満のとき「1時間以内」を返すこと", () => {
    const d = new Date(NOW.getTime() - 30 * 60 * 1000); // 30分前
    expect(formatRelativeDate(d)).toBe("1時間以内");
  });

  it("24時間未満のとき「N時間前」を返すこと", () => {
    const d = new Date(NOW.getTime() - 5 * 60 * 60 * 1000); // 5時間前
    expect(formatRelativeDate(d)).toBe("5時間前");
  });

  it("7日未満のとき「N日前」を返すこと", () => {
    const d = new Date(NOW.getTime() - 3 * 24 * 60 * 60 * 1000); // 3日前
    expect(formatRelativeDate(d)).toBe("3日前");
  });

  it("7日以上前のとき ja-JP の絶対日付を返すこと", () => {
    const d = new Date(NOW.getTime() - 30 * 24 * 60 * 60 * 1000); // 30日前
    const got = formatRelativeDate(d);
    // 「N時間前」「N日前」「1時間以内」のいずれにも該当しない絶対日付であること
    expect(got).not.toMatch(/時間前|日前|時間以内/);
    expect(got).toContain("年");
    expect(got).toContain("月");
  });

  it("境界: ちょうど24時間前のとき「N日前」側に入ること", () => {
    const d = new Date(NOW.getTime() - 24 * 60 * 60 * 1000); // 24時間前 = 1日前
    expect(formatRelativeDate(d)).toBe("1日前");
  });
});
