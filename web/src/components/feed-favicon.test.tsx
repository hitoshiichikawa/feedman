import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { FeedFavicon } from "./feed-favicon";

describe("FeedFavicon", () => {
  it("faviconURL が指定されているとき <img> 要素で favicon を描画すること", () => {
    render(
      <FeedFavicon
        feedId="feed-1"
        faviconURL="data:image/png;base64,iVBORw0KGgo="
        feedTitle="Example Feed"
      />
    );

    const img = screen.getByTestId("feed-favicon-feed-1");
    expect(img.tagName).toBe("IMG");
    expect(img).toHaveAttribute("src", "data:image/png;base64,iVBORw0KGgo=");
    expect(img).toHaveAttribute("alt", "Example Feed のアイコン");
    // fallback は描画されない
    expect(
      screen.queryByTestId("feed-favicon-fallback-feed-1")
    ).not.toBeInTheDocument();
  });

  it("faviconURL が null のとき Rss 代替アイコン（fallback）を描画すること", () => {
    render(
      <FeedFavicon
        feedId="feed-2"
        faviconURL={null}
        feedTitle="No Icon Feed"
      />
    );

    const fallback = screen.getByTestId("feed-favicon-fallback-feed-2");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveAttribute("aria-label", "No Icon Feed のアイコン");
    expect(fallback).toHaveAttribute("role", "img");
    // <img> は描画されない
    expect(screen.queryByTestId("feed-favicon-feed-2")).not.toBeInTheDocument();
  });

  it("faviconURL が空文字のとき Rss 代替アイコン（fallback）を描画すること（境界値）", () => {
    render(
      <FeedFavicon feedId="feed-3" faviconURL="" feedTitle="Empty URL Feed" />
    );

    expect(
      screen.getByTestId("feed-favicon-fallback-feed-3")
    ).toBeInTheDocument();
    expect(screen.queryByTestId("feed-favicon-feed-3")).not.toBeInTheDocument();
  });

  it("<img> の onError 発火後に Rss 代替アイコンに切り替わること", () => {
    render(
      <FeedFavicon
        feedId="feed-4"
        faviconURL="https://example.com/broken.ico"
        feedTitle="Broken Feed"
      />
    );

    const img = screen.getByTestId("feed-favicon-feed-4");
    expect(img.tagName).toBe("IMG");

    // onError を発火
    fireEvent.error(img);

    // <img> が消えて fallback が描画される
    expect(screen.queryByTestId("feed-favicon-feed-4")).not.toBeInTheDocument();
    expect(
      screen.getByTestId("feed-favicon-fallback-feed-4")
    ).toBeInTheDocument();
  });
});
