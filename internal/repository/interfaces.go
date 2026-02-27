// Package repository はデータ永続化のインターフェースを定義する。
package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// UserRepository はユーザーデータの永続化インターフェース。
type UserRepository interface {
	// FindByID は指定IDのユーザーを取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.User, error)

	// CreateWithIdentity はユーザーとidentityを同一トランザクションで作成する。
	CreateWithIdentity(ctx context.Context, user *model.User, identity *model.Identity) error

	// DeleteByID は指定IDのユーザーを削除する。
	// 関連するidentities、user_settingsはCASCADE削除される。
	DeleteByID(ctx context.Context, id string) error
}

// IdentityRepository は外部IdP紐付け情報の永続化インターフェース。
type IdentityRepository interface {
	// FindByProviderAndProviderUserID はproviderとprovider_user_idでidentityを検索する。
	// 見つからない場合はnilを返す。
	FindByProviderAndProviderUserID(ctx context.Context, provider, providerUserID string) (*model.Identity, error)
}

// SessionRepository はセッションデータの永続化インターフェース。
type SessionRepository interface {
	// Create はセッションを作成する。
	Create(ctx context.Context, session *model.Session) error
	// FindByID は指定IDのセッションを取得する。期限切れの場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.Session, error)
	// DeleteByID は指定IDのセッションを削除する。
	DeleteByID(ctx context.Context, id string) error
	// DeleteByUserID は指定ユーザーの全セッションを削除する。
	DeleteByUserID(ctx context.Context, userID string) error
}

// FeedRepository はフィードデータの永続化インターフェース。
type FeedRepository interface {
	// FindByID は指定IDのフィードを取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.Feed, error)

	// FindByFeedURL はフィードURLでフィードを検索する。見つからない場合はnilを返す。
	FindByFeedURL(ctx context.Context, feedURL string) (*model.Feed, error)

	// Create はフィードを作成する。
	Create(ctx context.Context, feed *model.Feed) error

	// Update はフィード情報を更新する。
	Update(ctx context.Context, feed *model.Feed) error

	// UpdateFavicon はフィードのfaviconデータを更新する。
	UpdateFavicon(ctx context.Context, feedID string, faviconData []byte, faviconMime string) error

	// ListDueForFetch はフェッチ対象のフィードを取得する。
	// next_fetch_at <= now() かつ fetch_status = 'active' かつ購読者が存在するフィードを
	// FOR UPDATE SKIP LOCKEDで排他的に取得する。
	ListDueForFetch(ctx context.Context) ([]*model.Feed, error)

	// UpdateFetchState はフィードのフェッチ状態を更新する。
	// fetch_status、consecutive_errors、error_message、next_fetch_at、etag、last_modifiedを更新する。
	UpdateFetchState(ctx context.Context, feed *model.Feed) error
}

// SubscriptionRepository は購読データの永続化インターフェース。
type SubscriptionRepository interface {
	// FindByID は指定IDの購読を取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.Subscription, error)

	// FindByUserAndFeed はユーザーIDとフィードIDで購読を検索する。見つからない場合はnilを返す。
	FindByUserAndFeed(ctx context.Context, userID, feedID string) (*model.Subscription, error)

	// CountByUserID はユーザーの購読数を返す。
	CountByUserID(ctx context.Context, userID string) (int, error)

	// Create は購読を作成する。
	Create(ctx context.Context, subscription *model.Subscription) error

	// ListByUserID はユーザーの購読一覧を返す。
	ListByUserID(ctx context.Context, userID string) ([]*model.Subscription, error)

	// MinFetchIntervalByFeedID は指定フィードの全購読者の中で最小のfetch_interval_minutesを返す。
	// 購読者が存在しない場合は0とエラーを返す。
	MinFetchIntervalByFeedID(ctx context.Context, feedID string) (int, error)

	// UpdateFetchInterval は購読のフェッチ間隔を更新する。
	UpdateFetchInterval(ctx context.Context, id string, minutes int) error

	// Delete は指定IDの購読を削除する。
	Delete(ctx context.Context, id string) error

	// DeleteByUserID はユーザーの全購読を削除する。
	DeleteByUserID(ctx context.Context, userID string) error

	// ListByUserIDWithFeedInfo はユーザーの購読一覧をフィード情報と未読数付きで返す。
	ListByUserIDWithFeedInfo(ctx context.Context, userID string) ([]SubscriptionWithFeedInfo, error)
}

// ItemRepository は記事データの永続化インターフェース。
// 記事の同一性判定（3段階の優先順位）とCRUD操作を提供する。
type ItemRepository interface {
	// FindByID は指定IDの記事を取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.Item, error)

	// FindByFeedAndGUID はfeed_idとguid_or_idで記事を検索する。
	// 同一性判定の最優先手段。見つからない場合はnilを返す。
	FindByFeedAndGUID(ctx context.Context, feedID, guid string) (*model.Item, error)

	// FindByFeedAndLink はfeed_idとlinkで記事を検索する。
	// 同一性判定の第2優先手段。見つからない場合はnilを返す。
	FindByFeedAndLink(ctx context.Context, feedID, link string) (*model.Item, error)

	// FindByContentHash はfeed_idとcontent_hashで記事を検索する。
	// 同一性判定の第3優先手段（hash(title+published+summary)）。見つからない場合はnilを返す。
	FindByContentHash(ctx context.Context, feedID, contentHash string) (*model.Item, error)

	// ListByFeed はフィードの記事一覧をユーザーの状態とJOINして取得する。
	// published_at降順でカーソルベースページネーションを使用する。
	// cursorがゼロ値の場合は先頭から取得する。
	// filter: "all"=全件, "unread"=未読のみ, "starred"=スターのみ
	ListByFeed(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error)

	// Create は新規記事を作成する。
	Create(ctx context.Context, item *model.Item) error

	// Update は既存記事を上書き更新する。履歴は保持しない。
	Update(ctx context.Context, item *model.Item) error
}

// HatebuItemRepository ははてなブックマーク取得に必要な記事データ操作のインターフェース。
type HatebuItemRepository interface {
	// ListNeedingHatebuFetch ははてなブックマーク数の取得が必要な記事を取得する。
	// hatebu_fetched_at IS NULL（未取得）を優先し、次にhatebu_fetched_atが古い順に処理する。
	ListNeedingHatebuFetch(ctx context.Context, limit int) ([]*model.Item, error)

	// UpdateHatebuCount は記事のはてなブックマーク数と取得日時を更新する。
	UpdateHatebuCount(ctx context.Context, itemID string, count int, fetchedAt time.Time) error
}

// ItemStateRepository はユーザーごとの記事状態（既読/スター）の永続化インターフェース。
type ItemStateRepository interface {
	// FindByUserAndItem はユーザーIDと記事IDで記事状態を取得する。見つからない場合はnilを返す。
	FindByUserAndItem(ctx context.Context, userID, itemID string) (*model.ItemState, error)

	// Upsert は記事状態を冪等にUPSERTする。
	// nilフィールドは変更せず、既存の値を維持する部分更新を行う。
	Upsert(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error)

	// DeleteByUserAndFeed はユーザーIDとフィードIDに関連する記事状態を全て削除する。
	DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error

	// DeleteByUserID はユーザーIDに関連する全ての記事状態を削除する。
	DeleteByUserID(ctx context.Context, userID string) error
}

// SubscriptionWithFeedInfo は購読とフィード情報、未読数を結合した構造体。
type SubscriptionWithFeedInfo struct {
	model.Subscription
	FeedTitle    string
	FeedURL      string
	FaviconData  []byte
	FaviconMime  string
	FetchStatus  model.FetchStatus
	ErrorMessage string
	UnreadCount  int
}

// UserRepository の拡張メソッド用。
// DeleteByIDはUserRepository内に追加する。

// TxBeginner はトランザクション開始用のインターフェース。
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}
