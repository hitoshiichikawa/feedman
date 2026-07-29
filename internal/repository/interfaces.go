// Package repository はデータ永続化のインターフェースを定義する。
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// ErrAuthCodeNotUsable は AuthCodeRepository.MarkUsed の対象 auth_code が
// 見つからない・期限切れ・既に使用済みのいずれかで、使用済み確定を成功させられない
// ことを示す sentinel error（Req 2.6）。
// メッセージには code_hash や user_id 等の機密値を含めない（NFR 1.2）。
var ErrAuthCodeNotUsable = errors.New("auth_code is not usable")

// ErrRefreshTokenAlreadyRotated は RefreshTokenRepository.MarkRotated の対象
// refresh_token が既に rotation 済みのため、再度 rotated として確定できないことを
// 示す sentinel error（Req 3.3）。
// メッセージには token_hash や user_id 等の機密値を含めない（NFR 1.2）。
var ErrRefreshTokenAlreadyRotated = errors.New("refresh_token already rotated")

// ErrChallengeNotUsable は PasskeyChallengeRepository.MarkConsumed の対象 challenge が
// 見つからない・二重消費・期限切れのいずれかで、消費確定を成功させられないことを示す
// sentinel error（Issue #216 / Req 4.3 / 4.4）。
// メッセージには challenge_hash や user_id 等の機密値を含めない（NFR 1.2）。
var ErrChallengeNotUsable = errors.New("passkey challenge is not usable")

// ErrCredentialAlreadyRegistered は PasskeyCredentialRepository.Create が
// credential_id UNIQUE 制約違反により保存を失敗したことを示す sentinel error
// （Issue #216 / Req 3.6 / 1.7）。上位レイヤは Req 3.6 の防衛線として
// ErrRegistrationFailed に正規化する。メッセージには credential_id や user_id 等の
// 機密値を含めない（NFR 1.2）。
var ErrCredentialAlreadyRegistered = errors.New("passkey credential already registered")

// ErrUsernameTaken は UserRepository.CreateUserOnly が username_normalized UNIQUE
// 制約違反により保存を失敗したことを示す sentinel error（Issue #216 / Req 1.4）。
// メッセージには username 等の入力値を含めない（NFR 1.2）。
var ErrUsernameTaken = errors.New("username already taken")

// UserRepository はユーザーデータの永続化インターフェース。
type UserRepository interface {
	// FindByID は指定IDのユーザーを取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.User, error)

	// CreateWithIdentity はユーザーとidentityを同一トランザクションで作成する。
	CreateWithIdentity(ctx context.Context, user *model.User, identity *model.Identity) error

	// DeleteByID は指定IDのユーザーを削除する。
	// 関連するidentities、user_settingsはCASCADE削除される。
	DeleteByID(ctx context.Context, id string) error

	// FindByNormalizedUsername は正規化済みユーザー名（lowercase）でユーザーを検索する
	// （Issue #216 / Req 1.4）。見つからない場合は (nil, nil) を返す。
	// 呼び出し側は非空 normalized のみを渡す前提（DB 側の部分 UNIQUE INDEX は
	// username_normalized IS NOT NULL を対象にしており、NULL 同士は衝突しない）。
	FindByNormalizedUsername(ctx context.Context, normalized string) (*model.User, error)

	// CreateUserOnly は identity を持たないユーザー（パスキー新規登録ユーザー）の
	// users 行のみを INSERT する（Issue #216 / Req 1.2 / 1.6）。
	// username_normalized の UNIQUE 制約違反時は ErrUsernameTaken を返す（Req 1.4）。
	// email 空文字を許容し、リカバリ用メールなしのユーザー作成に対応する（Req 1.6）。
	CreateUserOnly(ctx context.Context, u *model.User) error
}

// PasskeyCredentialRepository はパスキー credential（公開鍵・credential_id・
// sign counter 等）の永続化インターフェース（Issue #216 / Req 1.2 / 3.2 / 3.4 / 3.6 /
// 7.1〜7.5 / NFR 1.1）。
//
// 保存対象は検証に必要な情報のみ（NFR 1.1）で、パスキー本体の秘密情報は保持しない。
// エラーメッセージには credential_id や public_key 等の機密値を含めない（NFR 1.2）。
type PasskeyCredentialRepository interface {
	// Create は credential を新規保存する（Req 1.2 / 3.2）。
	// credential_id の UNIQUE 制約違反時は ErrCredentialAlreadyRegistered を返す
	// （Req 3.6 の防衛線）。
	Create(ctx context.Context, c *model.PasskeyCredential) error

	// FindByCredentialID は credential_id で credential を検索する（Req 2.2 / 3.6）。
	// 見つからない場合は (nil, nil) を返す。
	FindByCredentialID(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error)

	// ListByUserID は当該 user に紐付く全 credential を返す（Req 3.4 / 3.1 の excludeCredentials 用途）。
	// 存在しない場合は空スライスを返す。
	ListByUserID(ctx context.Context, userID string) ([]*model.PasskeyCredential, error)

	// UpdateAuthenticationState は認証 ceremony 成功時に当該 credential の
	// sign_count / backup_state / last_used_at の 3 列を更新する（Issue #234 /
	// Req 4.1, 4.2, 4.3）。旧 UpdateSignCount の後継で、認証時の credential 属性
	// 最新化ポリシー（Req 4.2 / 4.3）を interface レベルで担保する。
	//
	// backup_eligible は本メソッドで **一切更新しない**（引数にも SQL SET 句にも
	// 含めない）。これにより Req 4.3「BE は再認証時に上書き更新しない = 初回登録時の
	// 値を不変で保持する」を SQL レベルで構造的に保証する。
	//
	// 対象レコードが存在しない場合はエラーにせず 0 rows で成功する（呼び出し側が
	// 事前に FindByCredentialID で存在確認する既存流儀に整合）。
	// メッセージには credential_id / public_key / user_id 等の機密値を含めない
	// （NFR 1.2）。
	UpdateAuthenticationState(
		ctx context.Context,
		id string,
		signCount uint32,
		backupState bool,
		lastUsedAt time.Time,
	) error

	// DeleteByUserID は当該ユーザーに紐付く全 passkey_credentials を削除する
	// （Issue #216 / Req 7.1 / 7.4）。対象 0 件でも成功する（冪等）。
	DeleteByUserID(ctx context.Context, userID string) error

	// DeleteByUserIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
	// 当該ユーザーに紐付く全 passkey_credentials を削除する（Req 7.2 / 7.3）。
	// 退会 tx（user.Service.withdrawTx）に統合するための共有 tx 対応版
	// （PostgresAuthCodeRepo.DeleteByUserIDExec / PostgresSessionRepo.DeleteByUserIDExec と同型）。
	DeleteByUserIDExec(ctx context.Context, q DBTX, userID string) error
}

// PasskeyChallengeRepository はパスキー challenge の永続化インターフェース
// （Issue #216 / Req 4.1 / 4.2 / 4.3 / 4.4）。
//
// 生 challenge 値は保存せず、SHA-256 hex（challenge_hash）のみを保持する（NFR 1.2）。
// MarkConsumed は UPDATE の atomic 判定により単回利用を保証する（Req 4.3）。
type PasskeyChallengeRepository interface {
	// Create は challenge を新規保存する（Req 4.1 / 4.2）。
	// ch.ChallengeHash / ch.Kind / ch.SessionData / ch.ExpiresAt は呼び出し側で
	// 確定済みであること。ch.ID が空文字 / ch.CreatedAt が zero-value の場合は
	// DB 側デフォルト（gen_random_uuid() / now()）を採用する。
	Create(ctx context.Context, ch *model.PasskeyChallenge) error

	// FindByHash は challenge_hash に一致するレコードを 1 件返す。
	// 見つからない場合は (nil, nil) を返す（既存 FindByHash パターンに整合）。
	FindByHash(ctx context.Context, hash string) (*model.PasskeyChallenge, error)

	// FindByID は id（PK / opaque challenge_id）でレコードを 1 件返す。
	// 見つからない場合は (nil, nil) を返す。ChallengeStore.Consume が client の
	// 提示する opaque challenge_id から challenge を逆引きするために用いる
	// （tasks.md task 3 スコープ調整）。
	FindByID(ctx context.Context, id string) (*model.PasskeyChallenge, error)

	// MarkConsumed は当該 ID の challenge を consumed = true に遷移させる（Req 4.3）。
	//
	// レコードが以下のいずれかに該当する場合は ErrChallengeNotUsable を返し、
	// 永続化状態は変更しない（Req 4.4）:
	//   - 既に consumed = true（二重消費）
	//   - expires_at <= now()（期限切れ）
	//   - id に一致するレコードが存在しない
	//
	// UPDATE 文の WHERE 句で consumed / expires_at をまとめて判定することで、
	// 並行アクセス下でも race を起こさず単回利用を保証する（PostgresAuthCodeRepo.MarkUsed と同流儀）。
	MarkConsumed(ctx context.Context, id string) error
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

// AuthCodeRepository は native auth の一時認可コード永続化操作を公開する。
//
// 親 Issue #163 / 本 spec #164 で導入される native auth フローのうち、OAuth callback
// が発行する一時 auth_code の保存・hash 一致参照・単回利用確定のみを公開する。
// 平文 code は引数にも戻り値にも一切含めない（NFR 1.1 / 1.2）。
type AuthCodeRepository interface {
	// Create は AuthCode を新規保存する。
	// code.CodeHash / code.UserID / code.PKCEChallenge / code.ExpiresAt は
	// 呼び出し側で確定済みであること（Req 2.3）。
	Create(ctx context.Context, code *model.AuthCode) error

	// FindByHash は code_hash に一致する未削除レコードを 1 件返す。
	// 見つからない場合は (nil, nil) を返す（既存 FindByID パターンに整合、Req 2.4）。
	FindByHash(ctx context.Context, codeHash string) (*model.AuthCode, error)

	// MarkUsed は当該 ID の auth_code を used = true に遷移させる（Req 2.5）。
	// 当該レコードが 1) 既に used = true, 2) expires_at <= now(), 3) 存在しない の
	// いずれかの場合は ErrAuthCodeNotUsable を返し、永続化状態は変更しない（Req 2.6）。
	MarkUsed(ctx context.Context, id string) error

	// DeleteByUserID は当該ユーザーに属する全ての auth_code を削除する（Issue #170 Req 1.1）。
	// 対象 0 件でも成功する（冪等）。FK ON DELETE CASCADE で users 削除時にも到達するが、
	// 退会フローからの明示的削除経路として提供する（RefreshTokenRepository.DeleteByUserID と対）。
	DeleteByUserID(ctx context.Context, userID string) error
}

// RefreshTokenRepository は native auth の refresh token / family 永続化操作を公開する。
//
// 親 Issue #163 / 本 spec #164 で導入される native auth フローのうち、refresh token の
// 発行・rotation・family 単位 revoke・ユーザー単位削除のみを公開する。平文 token は
// 引数にも戻り値にも一切含めない（NFR 1.1 / 1.2）。
//
// 本 interface は 1 メソッド 1 SQL を基本とし、複数操作の atomic 性が必要な
// orchestration（rotation = 旧 token rotate + 新 token create）は呼び出し側に委ねる。
type RefreshTokenRepository interface {
	// CreateFamily は新規 family を保存する。token 発行の前に呼ぶ（Req 3.2）。
	CreateFamily(ctx context.Context, family *model.RefreshTokenFamily) error

	// CreateToken は family に属する refresh token を保存する（Req 3.1, 3.2）。
	// token.TokenHash / FamilyID / UserID / ExpiresAt は呼び出し側で確定済みであること。
	CreateToken(ctx context.Context, token *model.RefreshToken) error

	// FindByHash は token_hash に一致する 1 件を返す（Req 3.5）。
	// 見つからない場合は (nil, nil) を返す。
	// 戻り値の token が RotatedAt / RevokedAt を持つかは呼び出し側で判定する。
	FindByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error)

	// MarkRotated は当該 ID の refresh_token の rotated_at を set する（Req 3.3）。
	// 既に rotated_at が set 済みの場合は ErrRefreshTokenAlreadyRotated を返す。
	MarkRotated(ctx context.Context, id string, rotatedAt time.Time) error

	// RevokeFamily は当該 family を revoked にし、family 配下の全 token の revoked_at を
	// 一括で set する（Req 3.4）。当該 family が既に revoked の場合は冪等に成功する
	// （二重 revoke 安全）。
	RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error

	// DeleteByUserID は当該ユーザーに属する全ての refresh_token と family を削除する
	// （Req 3.6）。FK ON DELETE CASCADE で users 削除時にも到達するが、明示的削除経路
	// （アカウント削除フロー以外の運用削除）も提供する。
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
