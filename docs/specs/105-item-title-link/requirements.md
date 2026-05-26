# Requirements Document

## Introduction

ユーザーが RSS リーダーで記事タイトルを見たとき、一次情報ソース（元のウェブサイト）へ直接遷移したいケースが頻繁にあります。
現在は詳細画面の小さな「元記事を開く」リンクからのみ遷移可能ですが、記事一覧のタイトルおよび記事詳細画面の大きなタイトル自体を直リンク化（新規タブ表示）することで、ナビゲーションの利便性を高めます。

## Requirements

### Requirement 1: 記事一覧タイトルからの元記事直リンク

**Objective:** As a ユーザー, I want 記事一覧のタイトルをクリックして直接元のウェブサイトの記事を開きたい, so that 詳細を開く手間を省いてスムーズに情報収集ができる

#### Acceptance Criteria

1. Where the system renders the article row in the list, the article title shall be wrapped in a hyperlink (`<a>` tag) pointing to the original article URL (`item.link`).
2. When the user clicks the title link in the row, the system shall open the URL in a new browser tab (`target="_blank"`, `rel="noopener noreferrer"`).
3. When the user clicks the title link, the system shall prevent the row click event (which selects/expands the row) from triggering (event propagation shall be stopped via `e.stopPropagation()`).
4. When the user hovers over the title in the list, the system shall display the pointer cursor and apply an underline hover state.

### Requirement 2: 記事詳細タイトルからの元記事直リンク

**Objective:** As a ユーザー, I want 記事詳細画面のタイトルをクリックして元のウェブサイトの記事を開きたい, so that 詳細画面を読んだ後にソース元へ直感的にジャンプできる

#### Acceptance Criteria

1. Where the system displays the article title in the detail panel, the title shall be wrapped in a hyperlink pointing to `item.link` opening in a new tab.
2. When the user hovers over the detail title, the system shall show an underline style.

## Non-Functional Requirements

### NFR 1: セキュリティ

1. All external links generated shall include `rel="noopener noreferrer"` to prevent tab-nabbing vulnerabilities.

## Out of Scope

- タイトル横に外部リンクアイコンを追加するなどの過剰な装飾（一覧の幅を保つため）。

## Open Questions

- なし
