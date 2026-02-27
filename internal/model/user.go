// Package model はドメインモデルを定義する。
package model

import "time"

// User はサービス利用ユーザーを表す。
type User struct {
	ID        string
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Identity は外部IdPとの紐付け情報を表す。
// 将来的に複数のIdP（Google, GitHub等）に対応可能な構造。
type Identity struct {
	ID             string
	UserID         string
	Provider       string
	ProviderUserID string
	CreatedAt      time.Time
}

// Session はユーザーのログインセッションを表す。
type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}
