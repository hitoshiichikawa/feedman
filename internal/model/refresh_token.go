package model

import "time"

// RefreshTokenFamily は同一 refresh chain（rotation 系列）を束ねる family を表す。
//
// rotation で連なる複数の RefreshToken は同じ FamilyID を共有する。
// RevokedAt が non-nil の family は family 全体が revoke 済みとみなし、
// 配下の全 RefreshToken は使用不可になる（Req 3.4 の family 単位 revoke を表現）。
// UserID を直接保持することで、アカウント削除時の一括削除経路（Req 3.6）を
// family JOIN なしで簡潔に表現できる。
type RefreshTokenFamily struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	RevokedAt *time.Time // family 単位 revoke 用（nil なら有効）
}

// RefreshToken は rotation 系列の各世代の refresh token を表す。
//
// 平文の token はサーバー側で生成され、本構造体には絶対に保持しない（NFR 1.1）。
// 永続化・検索・比較は SHA-256 由来の固定長 hash 文字列（TokenHash）でのみ行う。
//
// rotation / revocation の semantics:
//   - RotatedAt が non-nil: 次世代 token に rotate 済み（直前世代扱い）。再利用すると
//     rotation chain の不正利用検知トリガとなり、後続 handler 側で family 全体の
//     revoke へ昇格させる想定（Req 3.3）。
//   - RevokedAt が non-nil: token 単位で revoke 済み（使用不可）。family 単位 revoke
//     でも本フィールドが set されるため、判定は本フィールド単独で完結する。
//   - Family の RevokedAt が non-nil の場合、配下の全 token は論理的に使用不可となる。
type RefreshToken struct {
	ID        string
	FamilyID  string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RotatedAt *time.Time // 次の token に rotate された時刻（nil なら最新世代）
	RevokedAt *time.Time // revoke 時刻（nil なら有効）
	CreatedAt time.Time
}
