// Package model はドメインモデルを定義する。
package model

import "fmt"

// APIError は統一エラーフォーマットを表す。
// UIに表示する原因カテゴリと対処方法を含む。
// Details は任意の構造化追加情報を表し、429（クールダウン中）の retry_after_seconds など
// 既存 4 フィールド（Code / Message / Category / Action）では表現できない補足情報を載せる。
// nil の場合は JSON シリアライズ時に出力されない（omitempty 相当）。
type APIError struct {
	Code     string         // エラーコード
	Message  string         // エラーメッセージ
	Category string         // カテゴリ: auth, validation, feed, system
	Action   string         // ユーザー向け対処方法
	Details  map[string]any // 任意の構造化追加情報（429 等で retry_after_seconds 等を載せる）
}

// Error はerrorインターフェースを実装する。
func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// 定義済みエラーコード
const (
	ErrCodeFeedNotDetected      = "FEED_NOT_DETECTED"
	ErrCodeInvalidURL           = "INVALID_URL"
	ErrCodeSSRFBlocked          = "SSRF_BLOCKED"
	ErrCodeFetchFailed          = "FETCH_FAILED"
	ErrCodeParseFailed          = "PARSE_FAILED"
	ErrCodeSubscriptionLimit    = "SUBSCRIPTION_LIMIT"
	ErrCodeItemNotFound         = "ITEM_NOT_FOUND"
	ErrCodeInvalidFilter        = "INVALID_FILTER"
	ErrCodeSubscriptionNotFound = "SUBSCRIPTION_NOT_FOUND"
	ErrCodeInvalidFetchInterval = "INVALID_FETCH_INTERVAL"
	ErrCodeFeedNotStopped       = "FEED_NOT_STOPPED"
	ErrCodeUserNotFound         = "USER_NOT_FOUND"
	ErrCodeFeedFetchInProgress  = "FEED_FETCH_IN_PROGRESS"
	ErrCodeFeedCooldown         = "FEED_COOLDOWN"
	ErrCodeInvalidSearchQuery   = "INVALID_SEARCH_QUERY"
	ErrCodeFeedNotSubscribed    = "FEED_NOT_SUBSCRIBED"
	ErrCodeFeedHTTPError        = "FEED_HTTP_ERROR"
	// Issue #216: パスキー登録・認証で使用する固定エラーコード群。
	// 拒否メッセージにクライアント入力値・内部詳細を反射しない（NFR 1.3）。
	ErrCodeInvalidUsername      = "INVALID_USERNAME"
	ErrCodeUsernameTaken        = "USERNAME_TAKEN"
	ErrCodeRegistrationFailed   = "REGISTRATION_FAILED"
	ErrCodeAuthenticationFailed = "AUTHENTICATION_FAILED"
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

// NewFeedFetchInProgressError は対象フィードが別トランザクションでフェッチ中のため
// 行ロック取得に失敗したときのエラーを生成する。HTTP 409 にマップされる。
func NewFeedFetchInProgressError() *APIError {
	return &APIError{
		Code:     ErrCodeFeedFetchInProgress,
		Message:  "現在フェッチが進行中のためしばらく待ってから再試行してください。",
		Category: "feed",
		Action:   "現在フェッチが進行中のためしばらく待ってから再試行してください。",
	}
}

// NewFeedCooldownError は対象フィードが 10 分クールダウン中のため手動フェッチが
// 拒否されたときのエラーを生成する。HTTP 429 にマップされ、Details["retry_after_seconds"]
// に次回フェッチ可能になるまでの残り秒数（int）を載せる。
func NewFeedCooldownError(retryAfterSeconds int) *APIError {
	return &APIError{
		Code:     ErrCodeFeedCooldown,
		Message:  fmt.Sprintf("クールダウン中です。再試行まで残り %d 秒です。", retryAfterSeconds),
		Category: "feed",
		Action:   "最終成功時刻から10分経過するまで手動フェッチは実行できません。",
		Details: map[string]any{
			"retry_after_seconds": retryAfterSeconds,
		},
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

// NewInvalidUsernameError はパスキー登録要求のユーザー名形式が不正な場合のエラーを
// 生成する（Issue #216 / Req 1.5）。HTTP 400 にマップされる。
//
// クライアントの入力値（生 username）は反射しない（NFR 1.3）。message / action は
// 固定文言で、内部詳細（許容文字集合の詳細・長さ判定の内訳）も返さない。
func NewInvalidUsernameError() *APIError {
	return &APIError{
		Code:     ErrCodeInvalidUsername,
		Message:  "ユーザー名の形式が不正です。",
		Category: "validation",
		Action:   "ユーザー名は 3〜32 文字の半角英数字・アンダースコア・ハイフンのみで指定してください。",
	}
}

// NewUsernameTakenError はパスキー登録要求のユーザー名が既存ユーザーと重複した場合の
// エラーを生成する（Issue #216 / Req 1.4）。HTTP 409 にマップされる。
//
// 「重複した」という事実自体はクライアントが判別できる必要がある（Req 1.4）ため
// この APIError は他の登録拒否（uniform 化された REGISTRATION_FAILED）と区別する。
// 具体的な重複相手や既存 user の存在有無は返さない（NFR 1.3）。
func NewUsernameTakenError() *APIError {
	return &APIError{
		Code:     ErrCodeUsernameTaken,
		Message:  "指定されたユーザー名は既に使用されています。",
		Category: "validation",
		Action:   "別のユーザー名を指定してください。",
	}
}

// NewRegistrationFailedError はパスキー登録・追加登録の拒否事象を uniform 化した
// エラーを生成する（Issue #216 / Req 1.7 / 3.6 / 3.7）。HTTP 400 にマップされる。
//
// attestation 検証失敗・challenge 期限切れ・credential 重複などの拒否理由を
// 区別しない固定メッセージを返す（Req 3.6 の存在有無非開示にも整合）。
func NewRegistrationFailedError() *APIError {
	return &APIError{
		Code:     ErrCodeRegistrationFailed,
		Message:  "パスキー登録を完了できませんでした。",
		Category: "auth",
		Action:   "しばらく待ってから再度お試しください。",
	}
}

// NewAuthenticationFailedError はパスキー認証の拒否事象を uniform 化した
// エラーを生成する（Issue #216 / Req 2.5 / 2.6 / NFR 1.4）。HTTP 400 にマップされる。
//
// user / credential / challenge のいずれで拒否したかを区別できる情報を返さない
// （Req 2.6 の存在有無非開示）。counter 後退（NFR 1.4）も同じ APIError に合流する。
func NewAuthenticationFailedError() *APIError {
	return &APIError{
		Code:     ErrCodeAuthenticationFailed,
		Message:  "パスキー認証に失敗しました。",
		Category: "auth",
		Action:   "パスキーを選び直すか、しばらく待ってから再度お試しください。",
	}
}

// NewFeedHTTPError は検出対象 URL から非 2xx の HTTP ステータスを受信した場合の
// エラーを生成する（Issue #153）。FETCH_FAILED（レスポンス取得前の失敗）とは
// 区別され、レスポンス取得には成功したが HTTP エラー応答だったことを示す。
// Details["status_code"] に受信したステータスコード（int）を載せる。
//
// ユーザー向けメッセージには受信ステータスコードを付記し（Req 2.4）、
// 「サイト側のブロック / URL 間違い / 一時的にサーバが応答しない」といった
// 原因示唆と、ユーザーが取れる対処（別 URL を試す / 時間を置いて再試行する）を含める（Req 2.1）。
func NewFeedHTTPError(statusCode int) *APIError {
	return &APIError{
		Code:     ErrCodeFeedHTTPError,
		Message:  fmt.Sprintf("URLにアクセスしたところ HTTP %d が返されました。サイト側がアクセスをブロックしている、URL が間違っている、もしくは一時的にサーバが応答していない可能性があります。", statusCode),
		Category: "feed",
		Action:   "別のURL（フィード本体のURLや異なるページ）を試すか、時間を置いてから再度お試しください。",
		Details: map[string]any{
			"status_code": statusCode,
		},
	}
}
