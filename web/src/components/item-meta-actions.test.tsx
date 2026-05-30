import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ItemMetaActions } from "./item-meta-actions";

/** デフォルト props 生成ヘルパー（テスト毎に必要な値のみ上書きする）。 */
function makeProps(overrides: Partial<Parameters<typeof ItemMetaActions>[0]> = {}) {
  return {
    itemId: "item-1",
    isStarred: false,
    hatebuCount: 0,
    hatebuFetchedAt: null as string | null,
    onToggleStar: vi.fn(),
    ...overrides,
  };
}

describe("ItemMetaActions コンポーネント", () => {
  describe("はてブ数表示分岐 (Req 1.3, 1.4)", () => {
    it("hatebuFetchedAt が null のときハイフン (-) を表示する", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-A",
        hatebuCount: 12,
        hatebuFetchedAt: null,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const counter = screen.getByTestId("item-hatebu-count-item-A");
      expect(counter).toHaveTextContent("-");
      expect(counter).not.toHaveTextContent("12");
    });

    it("hatebuFetchedAt が取得済みのとき hatebuCount の整数値を表示する", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-B",
        hatebuCount: 42,
        hatebuFetchedAt: "2026-02-27T09:00:00Z",
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const counter = screen.getByTestId("item-hatebu-count-item-B");
      expect(counter).toHaveTextContent("42");
    });
  });

  describe("0 件と未取得の区別表示 (Req 1.3, 1.4, 5.3, 5.4 / 境界値)", () => {
    it("hatebuFetchedAt が値あり かつ hatebuCount が 0 のとき 0 を表示する", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-zero",
        hatebuCount: 0,
        hatebuFetchedAt: "2026-02-27T09:00:00Z",
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert: 取得済みかつ 0 件のときは "0" を表示し "-" は表示しない
      const counter = screen.getByTestId("item-hatebu-count-item-zero");
      expect(counter).toHaveTextContent("0");
      expect(counter.textContent).not.toMatch(/-/);
    });

    it("hatebuFetchedAt が null のとき hatebuCount に関わらず ハイフン (-) を表示する", () => {
      // Arrange: 未取得時は hatebuCount の値を表示しない（区別表示）
      const props = makeProps({
        itemId: "item-unfetched",
        hatebuCount: 0,
        hatebuFetchedAt: null,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const counter = screen.getByTestId("item-hatebu-count-item-unfetched");
      expect(counter).toHaveTextContent("-");
    });
  });

  describe("スターアイコンの塗り分け (Req 1.5, 1.6)", () => {
    it("isStarred=true のとき塗りつぶし (黄色) アイコンを表示する", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-star-on",
        isStarred: true,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert: lucide Star を SVG として取得し、塗り分け className を検証
      const toggle = screen.getByTestId("item-star-toggle-item-star-on");
      const svg = toggle.querySelector("svg");
      expect(svg).not.toBeNull();
      expect(svg?.getAttribute("class")).toContain("fill-yellow-400");
      expect(svg?.getAttribute("class")).toContain("text-yellow-400");
    });

    it("isStarred=false のときアウトライン (muted-foreground) アイコンを表示する", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-star-off",
        isStarred: false,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const toggle = screen.getByTestId("item-star-toggle-item-star-off");
      const svg = toggle.querySelector("svg");
      expect(svg).not.toBeNull();
      expect(svg?.getAttribute("class")).toContain("text-muted-foreground");
      expect(svg?.getAttribute("class") ?? "").not.toContain("fill-yellow-400");
    });
  });

  describe("クリック時の挙動 (Req 2.1, 2.3 / NFR 2.1)", () => {
    it("クリック時に親要素の onClick へイベント伝播しない (stopPropagation)", async () => {
      // Arrange: 親 div に onClick を設定し、内部の Button クリックで発火しないことを検証
      const parentOnClick = vi.fn();
      const onToggleStar = vi.fn();
      const props = makeProps({
        itemId: "item-stop",
        isStarred: false,
        onToggleStar,
      });
      render(
        <div onClick={parentOnClick} data-testid="parent-row">
          <ItemMetaActions {...props} />
        </div>
      );
      const user = userEvent.setup();

      // Act
      await user.click(screen.getByTestId("item-star-toggle-item-stop"));

      // Assert: 親 onClick は呼ばれず、onToggleStar のみ発火する
      expect(parentOnClick).not.toHaveBeenCalled();
      expect(onToggleStar).toHaveBeenCalledTimes(1);
    });

    it("クリック時に onToggleStar(itemId, !isStarred) を発火する (未スター → 付与)", async () => {
      // Arrange
      const onToggleStar = vi.fn();
      const props = makeProps({
        itemId: "item-toggle-on",
        isStarred: false,
        onToggleStar,
      });
      render(<ItemMetaActions {...props} />);
      const user = userEvent.setup();

      // Act
      await user.click(screen.getByTestId("item-star-toggle-item-toggle-on"));

      // Assert: 次状態 true で callback が発火
      expect(onToggleStar).toHaveBeenCalledWith("item-toggle-on", true);
    });

    it("クリック時に onToggleStar(itemId, !isStarred) を発火する (スター済 → 解除)", async () => {
      // Arrange
      const onToggleStar = vi.fn();
      const props = makeProps({
        itemId: "item-toggle-off",
        isStarred: true,
        onToggleStar,
      });
      render(<ItemMetaActions {...props} />);
      const user = userEvent.setup();

      // Act
      await user.click(screen.getByTestId("item-star-toggle-item-toggle-off"));

      // Assert: 次状態 false で callback が発火
      expect(onToggleStar).toHaveBeenCalledWith("item-toggle-off", false);
    });
  });

  describe("aria-label / aria-pressed 状態整合 (NFR 1.1, 1.2)", () => {
    it("isStarred=true のとき aria-label=スターを解除する / aria-pressed=true となる", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-aria-on",
        isStarred: true,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const toggle = screen.getByTestId("item-star-toggle-item-aria-on");
      expect(toggle).toHaveAttribute("aria-label", "スターを解除する");
      expect(toggle).toHaveAttribute("aria-pressed", "true");
    });

    it("isStarred=false のとき aria-label=スターを付ける / aria-pressed=false となる", () => {
      // Arrange
      const props = makeProps({
        itemId: "item-aria-off",
        isStarred: false,
      });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert
      const toggle = screen.getByTestId("item-star-toggle-item-aria-off");
      expect(toggle).toHaveAttribute("aria-label", "スターを付ける");
      expect(toggle).toHaveAttribute("aria-pressed", "false");
    });
  });

  describe("32px ヒット領域 (NFR 1.3)", () => {
    it("Button size=icon-sm により size-8 (= 32px) のヒット領域を持つ", () => {
      // Arrange
      const props = makeProps({ itemId: "item-hit" });

      // Act
      render(<ItemMetaActions {...props} />);

      // Assert: Button cva の icon-sm variant は className に "size-8" を含む
      const toggle = screen.getByTestId("item-star-toggle-item-hit");
      expect(toggle.className).toContain("size-8");
      // 互換のため data-size 属性でも変えていないことを確認
      expect(toggle).toHaveAttribute("data-size", "icon-sm");
    });
  });
});
