// Package model はドメインモデルを定義する。
package model

import "fmt"

// APIError は統一エラーフォーマットを表す。
// UIに表示する原因カテゴリと対処方法を含む。
type APIError struct {
	Code     string // エラーコード
	Message  string // エラーメッセージ
	Category string // カテゴリ: auth, validation, feed, system
	Action   string // ユーザー向け対処方法
}

// Error はerrorインターフェースを実装する。
func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// 定義済みエラーコード
const (
	ErrCodeFeedNotDetected       = "FEED_NOT_DETECTED"
	ErrCodeInvalidURL            = "INVALID_URL"
	ErrCodeSSRFBlocked           = "SSRF_BLOCKED"
	ErrCodeFetchFailed           = "FETCH_FAILED"
	ErrCodeParseFailed           = "PARSE_FAILED"
	ErrCodeSubscriptionLimit     = "SUBSCRIPTION_LIMIT"
	ErrCodeItemNotFound          = "ITEM_NOT_FOUND"
	ErrCodeInvalidFilter         = "INVALID_FILTER"
	ErrCodeSubscriptionNotFound  = "SUBSCRIPTION_NOT_FOUND"
	ErrCodeInvalidFetchInterval  = "INVALID_FETCH_INTERVAL"
	ErrCodeFeedNotStopped        = "FEED_NOT_STOPPED"
	ErrCodeUserNotFound          = "USER_NOT_FOUND"
	ErrCodeInvalidSearchQuery    = "INVALID_SEARCH_QUERY"
	ErrCodeFeedNotSubscribed     = "FEED_NOT_SUBSCRIBED"
)

// NewItemNotFoundError は記事未検出エラーを生成する。
func NewItemNotFoundError(itemID string) *APIError {
	return &APIError{
		Code:     ErrCodeItemNotFound,
		Message:  fmt.Sprintf("指定された記事が見つかりません: %s", itemID),
		Category: "feed",
		Action:   "記事IDを確認してください。",
	}
}

// NewInvalidFilterError は無効なフィルタエラーを生成する。
func NewInvalidFilterError(filter string) *APIError {
	return &APIError{
		Code:     ErrCodeInvalidFilter,
		Message:  fmt.Sprintf("無効なフィルタです: %s", filter),
		Category: "validation",
		Action:   "フィルタには all、unread、starred のいずれかを指定してください。",
	}
}

// NewFeedNotDetectedError はフィード未検出エラーを生成する。
func NewFeedNotDetectedError(url string) *APIError {
	return &APIError{
		Code:     ErrCodeFeedNotDetected,
		Message:  fmt.Sprintf("指定されたURLからRSS/Atomフィードを検出できませんでした: %s", url),
		Category: "feed",
		Action:   "RSS/AtomフィードのURLを直接入力するか、フィードが公開されているページのURLを確認してください。",
	}
}

// NewInvalidURLError は無効なURLエラーを生成する。
func NewInvalidURLError(reason string) *APIError {
	return &APIError{
		Code:     ErrCodeInvalidURL,
		Message:  fmt.Sprintf("無効なURLです: %s", reason),
		Category: "validation",
		Action:   "正しいURL形式（http:// または https:// で始まるURL）を入力してください。",
	}
}

// NewSSRFBlockedError はSSRFブロックエラーを生成する。
func NewSSRFBlockedError() *APIError {
	return &APIError{
		Code:     ErrCodeSSRFBlocked,
		Message:  "セキュリティポリシーにより、指定されたURLへのアクセスがブロックされました。",
		Category: "validation",
		Action:   "公開されているWebサイトのURLを入力してください。ローカルネットワークやプライベートIPへのアクセスは許可されていません。",
	}
}

// NewFetchFailedError はフェッチ失敗エラーを生成する。
func NewFetchFailedError(reason string) *APIError {
	return &APIError{
		Code:     ErrCodeFetchFailed,
		Message:  fmt.Sprintf("URLの取得に失敗しました: %s", reason),
		Category: "feed",
		Action:   "URLが正しいか確認し、しばらく待ってから再度お試しください。",
	}
}

// NewParseFailedError はパース失敗エラーを生成する。
func NewParseFailedError() *APIError {
	return &APIError{
		Code:     ErrCodeParseFailed,
		Message:  "フィードの解析に失敗しました。",
		Category: "feed",
		Action:   "有効なRSS/Atomフィードかどうか確認してください。",
	}
}

// NewSubscriptionLimitError は購読上限エラーを生成する。
func NewSubscriptionLimitError() *APIError {
	return &APIError{
		Code:     ErrCodeSubscriptionLimit,
		Message:  "購読数が上限（100件）に達しています。",
		Category: "feed",
		Action:   "不要な購読を解除してから、新しいフィードを登録してください。",
	}
}

// NewDuplicateSubscriptionError は既に購読済みのフィードを再度登録しようとした場合のエラーを生成する。
func NewDuplicateSubscriptionError() *APIError {
	return &APIError{
		Code:     "DUPLICATE_SUBSCRIPTION",
		Message:  "このフィードは既に購読しています。",
		Category: "feed",
		Action:   "購読一覧から該当フィードを確認してください。",
	}
}

// NewSubscriptionNotFoundError は購読が見つからない場合のエラーを生成する。
func NewSubscriptionNotFoundError(subscriptionID string) *APIError {
	return &APIError{
		Code:     ErrCodeSubscriptionNotFound,
		Message:  fmt.Sprintf("指定された購読が見つかりません: %s", subscriptionID),
		Category: "feed",
		Action:   "購読IDを確認してください。",
	}
}

// NewInvalidFetchIntervalError はフェッチ間隔が無効な場合のエラーを生成する。
func NewInvalidFetchIntervalError(minutes int) *APIError {
	return &APIError{
		Code:     ErrCodeInvalidFetchInterval,
		Message:  fmt.Sprintf("無効なフェッチ間隔です: %d分", minutes),
		Category: "validation",
		Action:   "フェッチ間隔は30分から720分（12時間）の範囲で、30分刻みで指定してください。",
	}
}

// NewFeedNotStoppedError はフィードが停止状態でない場合のエラーを生成する。
func NewFeedNotStoppedError() *APIError {
	return &APIError{
		Code:     ErrCodeFeedNotStopped,
		Message:  "フィードは停止中ではありません。",
		Category: "feed",
		Action:   "再開はフェッチが停止しているフィードに対してのみ実行できます。",
	}
}

// NewUserNotFoundError はユーザーが見つからない場合のエラーを生成する。
func NewUserNotFoundError() *APIError {
	return &APIError{
		Code:     ErrCodeUserNotFound,
		Message:  "ユーザーが見つかりません。",
		Category: "auth",
		Action:   "ログインし直してください。",
	}
}

// NewInvalidSearchQueryError は記事検索のクエリパラメータが不正な場合のエラーを生成する。
// reason には cursor 形式不正 / feed_id UUID パース失敗 / クエリ長超過などの具体的な
// 原因を渡す。Category は "validation" であり、handler 層で 400 BadRequest に変換される。
func NewInvalidSearchQueryError(reason string) *APIError {
	return &APIError{
		Code:     ErrCodeInvalidSearchQuery,
		Message:  fmt.Sprintf("検索クエリが無効です: %s", reason),
		Category: "validation",
		Action:   "検索キーワードや検索条件を見直してください。",
	}
}

// NewFeedNotSubscribedError は記事検索の feed_id 指定先を当該ユーザーが
// 購読していない場合のエラーを生成する。Category は "authorization" であり、
// handler 層で 403 Forbidden に変換される。
func NewFeedNotSubscribedError(feedID string) *APIError {
	return &APIError{
		Code:     ErrCodeFeedNotSubscribed,
		Message:  fmt.Sprintf("指定されたフィードを購読していません: %s", feedID),
		Category: "authorization",
		Action:   "購読中のフィードを指定するか、横断検索を利用してください。",
	}
}
