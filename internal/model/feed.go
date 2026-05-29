// Package model はドメインモデルを定義する。
package model

import "time"

// Feed はRSS/Atomフィードを表す。
type Feed struct {
	ID                string
	FeedURL           string
	SiteURL           string
	Title             string
	FaviconData       []byte
	FaviconMime       string
	ETag              string
	LastModified      string
	FetchStatus       FetchStatus
	ConsecutiveErrors int
	ErrorMessage      string
	NextFetchAt       time.Time
	// LastSuccessfulFetchAt は直近のフェッチ成功時刻。
	// nil の場合は過去に成功実績がないことを表し、手動フェッチのクールダウン判定では非適用となる。
	// 自動ワーカー / 手動フェッチの双方の成功経路で更新される。
	LastSuccessfulFetchAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// FetchStatus はフィードのフェッチ状態を表す。
type FetchStatus string

const (
	// FetchStatusActive はアクティブなフェッチ状態。
	FetchStatusActive FetchStatus = "active"
	// FetchStatusStopped は停止されたフェッチ状態。
	FetchStatusStopped FetchStatus = "stopped"
	// FetchStatusError はエラーによるフェッチ停止状態。
	FetchStatusError FetchStatus = "error"
)

// Subscription はユーザーとフィードの購読関係を表す。
type Subscription struct {
	ID                   string
	UserID               string
	FeedID               string
	FetchIntervalMinutes int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
