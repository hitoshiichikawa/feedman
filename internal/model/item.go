// Package model はドメインモデルを定義する。
package model

import "time"

// Item はフィードから取得した記事を表す。
type Item struct {
	ID                 string
	FeedID             string
	GuidOrID           string
	Title              string
	Link               string
	Content            string // サニタイズ済みHTML
	Summary            string // サニタイズ済み
	Author             string
	PublishedAt        *time.Time
	IsDateEstimated    bool
	FetchedAt          time.Time
	ContentHash        string
	HatebuCount        int
	HatebuFetchedAt    *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ItemWithState は記事とユーザーごとの状態（既読/スター）を結合したモデル。
// item_statesテーブルとLEFT JOINして取得される。
type ItemWithState struct {
	Item
	IsRead    bool
	IsStarred bool
}

// ItemFilter は記事一覧のフィルタ種別を表す。
type ItemFilter string

const (
	// ItemFilterAll は全記事を表示するフィルタ。
	ItemFilterAll ItemFilter = "all"
	// ItemFilterUnread は未読記事のみを表示するフィルタ。
	ItemFilterUnread ItemFilter = "unread"
	// ItemFilterStarred はスター付き記事のみを表示するフィルタ。
	ItemFilterStarred ItemFilter = "starred"
)

// ItemState はユーザーごとの記事状態（既読/スター）を表す。
type ItemState struct {
	ID        string
	UserID    string
	ItemID    string
	IsRead    bool
	IsStarred bool
	ReadAt    *time.Time
	StarredAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ParsedItem はフィードパーサーから取得した未保存の記事データを表す。
// ワーカーがフィードをパースした後、ItemUpsertServiceに渡される。
type ParsedItem struct {
	GuidOrID    string
	Title       string
	Link        string
	Content     string     // 未サニタイズのHTML
	Summary     string     // 未サニタイズ
	Author      string
	PublishedAt *time.Time
}
