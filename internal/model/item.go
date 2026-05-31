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

// ItemSearchHit は記事検索結果 1 件の DB レベル射影を表すモデル。
// items を subscriptions / feeds / item_states と JOIN した SELECT 結果を保持し、
// 検索結果カードに必要な記事サマリ・所属フィードのタイトル・favicon の生バイトと
// MIME タイプ・ユーザー固有の既読 / スター状態をまとめて運ぶ。
//
// サービス層でアプリケーション向けの表現（例: favicon を data URL に整形した
// ItemSearchSummary）へ変換する前段の生データを担うのが本構造体の責務であり、
// 既存 ItemWithState と同様にリポジトリ層が直接生成して返す。
type ItemSearchHit struct {
	ID              string
	FeedID          string
	FeedTitle       string
	FaviconData     []byte
	FaviconMime     string
	Title           string
	Link            string
	Summary         string // サニタイズ済みの概要テキスト
	PublishedAt     time.Time
	IsDateEstimated bool
	IsRead          bool
	IsStarred       bool
	HatebuCount     int
	// HatebuFetchedAt は items.hatebu_fetched_at の値を保持する。
	// NULL（未取得）の場合は nil、取得済みの場合はポインタ経由で時刻を持つ。
	// 通常一覧 / スター横断一覧 API と表示挙動を統一するため、検索 API でも
	// 「未取得 → '-' / 取得済み → 数値」をフロントが判定するための情報源として利用する。
	HatebuFetchedAt *time.Time
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
