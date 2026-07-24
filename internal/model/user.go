// Package model はドメインモデルを定義する。
package model

import "time"

// User はサービス利用ユーザーを表す。
//
// Username / UsernameNormalized はパスキー（WebAuthn）新規登録ユーザー向けに
// Issue #216 で追加されたフィールドである。Google 由来ユーザーは空文字（未設定）の
// ままとし、既存挙動と互換性を保つ（NFR 2.1 / 2.2）。
// UsernameNormalized は lowercase 変換した canonical 形式で、DB 上は部分 UNIQUE
// 制約（NULL 同士は許容）で一意性を担保する。
type User struct {
	ID                 string
	Email              string
	Name               string
	Username           string
	UsernameNormalized string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
