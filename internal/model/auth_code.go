package model

import "time"

// AuthCode は native auth の使い捨て認可コード状態を表す。
//
// 平文の code はサーバー側で生成され、本構造体には絶対に保持しない（NFR 1.1）。
// 永続化・検索・比較は SHA-256 由来の固定長 hash 文字列（CodeHash）でのみ行う。
type AuthCode struct {
	ID            string    // UUID
	CodeHash      string    // SHA-256 hex（lowercase, 固定長）
	UserID        string    // users.id への FK
	PKCEChallenge string    // S256 challenge（生文字列。client が送る base64url 値）
	ExpiresAt     time.Time // 呼び出し側から渡された絶対時刻
	Used          bool      // true なら確定済み（単回利用後）
	CreatedAt     time.Time
}
