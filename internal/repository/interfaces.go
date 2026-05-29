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

	// LockFeedForUpdateNowait は指定フィード行に対し非ブロッキング排他ロック（FOR UPDATE NOWAIT）を取得する。
	// 既に別トランザクションがロックを保持している場合は ErrFeedLocked を返し、待機しない。
	// 取得したロックは tx の COMMIT / ROLLBACK で自動解放される。
	// 対象 ID のフィードが存在しないときは (nil, nil) を返す（FindByID と同パターン）。
	LockFeedForUpdateNowait(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error)

	// UpdateLastSuccessfulFetchAt は指定フィードの last_successful_fetch_at を更新する。
	// 自動ワーカーの成功経路と手動フェッチの成功経路の双方から呼ばれる共有更新メソッド。
	UpdateLastSuccessfulFetchAt(ctx context.Context, feedID string, at time.Time) error
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

	// ListStarredByUser は指定ユーザーがスター付与した記事を全フィード横断・published_at降順で取得する。
	// items と item_states と feeds を INNER JOIN し、feed_title を付与する。
	// cursor がゼロ値の場合は先頭から取得する。
	// 返却スライス内の全行は s.user_id = userID AND s.is_starred = true を満たし、
	// 他ユーザーのスター記事は一切含まれない（NFR 2.1）。
	ListStarredByUser(ctx context.Context, userID string, cursor time.Time, limit int) ([]StarredItemRow, error)

	// ListNewAcrossFeeds はユーザーの全購読フィードから sinceTime より後の記事を横断取得する。
	// items × subscriptions × feeds × item_states を 1 クエリで JOIN し、N+1 を回避する。
	// cursorPublishedAt がゼロ値かつ cursorItemID が空文字の場合は cursor なし扱いで先頭から取得する。
	// 非ゼロ値時は (i.published_at, i.id) < (cursorPublishedAt, cursorItemID) のタプル比較で
	// 安定したページネーションを行う。
	// 戻り値は published_at DESC, id DESC で決定論的に並ぶ。limit は SQL の LIMIT にそのまま反映され、
	// 呼び出し側が limit+1 件を要求して HasMore 判定を行う前提（Issue #121 / Req 2.1, 2.2, 2.3, 4.2）。
	ListNewAcrossFeeds(
		ctx context.Context,
		userID string,
		sinceTime time.Time,
		cursorPublishedAt time.Time,
		cursorItemID string,
		limit int,
	) ([]CrossFeedItem, error)

	// Create は新規記事を作成する。
	Create(ctx context.Context, item *model.Item) error

	// Update は既存記事を上書き更新する。履歴は保持しない。
	Update(ctx context.Context, item *model.Item) error

	// FindExistingForUpsert は同一性判定に必要な既存記事を一括取得する。
	// guids / links / hashes は当該バッチに含まれる guid_or_id / link / content_hash の
	// 候補集合であり、それぞれを定数回（合計 3 回）のバッチ SELECT で引く。
	// 記事件数に比例した DB 往復を発生させないための一括取得手段。
	// 戻り値の ExistingItems は呼び出し側の 3 段階優先順位判定に用いる。
	FindExistingForUpsert(ctx context.Context, feedID string, guids, links, hashes []string) (*ExistingItems, error)

	// BulkUpsert は新規記事の一括 INSERT と既存記事の一括 UPDATE を単一トランザクションで実行する。
	// 途中でエラーが発生した場合は当該バッチを全件ロールバックし、1 件も永続化しない。
	// toCreate / toUpdate のいずれかが空でも安全に動作する。
	BulkUpsert(ctx context.Context, toCreate, toUpdate []*model.Item) error
}

// StarredItemRow は全フィード横断スター記事一覧の 1 行分のデータを表す。
// model.ItemWithState（記事 + ユーザー状態）にフィードタイトルを併記する。
// Requirement 2.4 / 4.10 によりフロントエンドで「どのフィードの記事か」を表示するため、
// items と feeds の INNER JOIN で feed_title を 1 段で取得する。
type StarredItemRow struct {
	model.ItemWithState
	// FeedTitle は当該記事が所属するフィードのタイトル（feeds.title）。
	FeedTitle string
}

// CrossFeedItem はフィード横断新着一覧の 1 行分のデータを表す。
// model.ItemWithState（記事 + ユーザー状態）に発信元フィードのタイトルと favicon を併記する。
// Issue #121 / Req 3.1, 3.2 によりフロントエンドで「どのフィードの記事か」と
// favicon バッジを表示するため、items / feeds / item_states を 1 段で JOIN して取得する。
type CrossFeedItem struct {
	model.ItemWithState
	// FeedTitle は当該記事が所属するフィードのタイトル（feeds.title）。
	FeedTitle string
	// FaviconData は当該フィードの favicon バイナリ。未設定の場合は nil（空スライス）。
	FaviconData []byte
	// FaviconMime は当該フィードの favicon の MIME タイプ。未設定の場合は空文字列。
	FaviconMime string
}

// ExistingItems は同一性判定のための既存記事を guid_or_id / link / content_hash 別に索引した結果。
// いずれのマップも feed_id 単位で取得済みの既存記事を保持する。
type ExistingItems struct {
	// ByGUID は guid_or_id をキーとする既存記事マップ。
	ByGUID map[string]*model.Item
	// ByLink は link をキーとする既存記事マップ。
	ByLink map[string]*model.Item
	// ByContentHash は content_hash をキーとする既存記事マップ。
	ByContentHash map[string]*model.Item
}

// ItemSearchRepository は記事検索向けの DB アクセス（横断検索 / フィード内検索の両モード）を提供する。
// 既存 ItemRepository とは別インターフェースとして公開し、検索専用の射影モデル
// model.ItemSearchHit を直接返す。実装上は PostgresItemRepo にメソッドを追加することで
// 単一の DB ハンドルを共有する。
type ItemSearchRepository interface {
	// SearchByUserAndKeyword は当該ユーザーが購読中のフィードに属する記事から、
	// title または content がキーワードに部分一致するものを取得する。
	//
	// feedID が nil の場合は横断検索（購読中フィード全体）、非 nil の場合は当該フィードに
	// 限定したフィード内検索を行う。pattern は ILIKE に渡す '%escaped%' 形式の文字列を
	// 呼び出し側で組み立てて渡す（LIKE メタ文字 %, _, \ のエスケープ責務は呼び出し側）。
	//
	// cursorPublishedAt がゼロ値の場合はカーソル条件を WHERE から外し先頭から取得する。
	// 非ゼロ値の場合は (published_at, id) < (cursorPublishedAt, cursorID) のタプル比較で
	// 安定したページネーションを行う。limit は実取得件数（HasMore 判定は呼び出し側で
	// limit+1 件取得して行うため、本メソッドはその件数をそのまま LIMIT に適用する）。
	SearchByUserAndKeyword(
		ctx context.Context,
		userID, pattern string,
		feedID *string,
		cursorID string,
		cursorPublishedAt time.Time,
		limit int,
	) ([]model.ItemSearchHit, error)
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

// UserCrossFeedViewRepository は「最後にフィード横断新着一覧を開いた時刻」の永続化インターフェース。
// ユーザーごとに 1 行を保持し、未読判定の基準時刻として用いる（Issue #121 / Req 4.1, 4.3, 4.5）。
type UserCrossFeedViewRepository interface {
	// Get は当該ユーザーの記録を取得する。未登録の場合は (nil, nil) を返す。
	Get(ctx context.Context, userID string) (*model.UserCrossFeedView, error)

	// Upsert は user_id をキーに last_seen_at を冪等に上書き保存する。
	// 既存行が存在しなければ新規挿入し、存在すれば last_seen_at と updated_at を更新する。
	Upsert(ctx context.Context, userID string, lastSeenAt time.Time) error
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
