package model

import "time"

// UserCrossFeedView はユーザーごとのフィード横断新着一覧の最終閲覧時刻を表す。
// 「最後に横断一覧を開いた時刻」を保持し、未読判定の基準として用いる。
type UserCrossFeedView struct {
	UserID     string
	LastSeenAt time.Time
	UpdatedAt  time.Time
}
