// Package hatebu ははてなブックマーク連携機能を提供する。
// はてなブックマークAPIの呼び出しとブックマーク数のバッチ取得ジョブを含む。
package hatebu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

const (
	// defaultEndpoint ははてなブックマーク一括取得APIのエンドポイント。
	defaultEndpoint = "https://bookmark.hatenaapis.com/count/entries"
	// maxURLsPerRequest は1リクエストあたりの最大URL数。
	maxURLsPerRequest = 50
)

// Client ははてなブックマークAPIのクライアント。
// 一括取得エンドポイントを使用して複数URLのブックマーク数を取得する。
type Client struct {
	httpClient *http.Client
	logger     *slog.Logger
	endpoint   string // テスト用にエンドポイントを差し替え可能
}

// NewClient はClient の新しいインスタンスを生成する。
func NewClient(httpClient *http.Client, logger *slog.Logger) *Client {
	return &Client{
		httpClient: httpClient,
		logger:     logger,
		endpoint:   defaultEndpoint,
	}
}

// GetBookmarkCounts は複数URLのはてなブックマーク数を一括取得する。
// URLリストは最大50件まで。レスポンスに含まれないURLは0件として扱う。
// 取得失敗時はエラーを返す（呼び出し元が前回値維持を判断する）。
func (c *Client) GetBookmarkCounts(ctx context.Context, urls []string) (map[string]int, error) {
	// 空リストの場合は空マップを返す
	if len(urls) == 0 {
		return make(map[string]int), nil
	}

	// URL数の上限チェック
	if len(urls) > maxURLsPerRequest {
		return nil, fmt.Errorf("URLの数が上限を超えています: %d > %d", len(urls), maxURLsPerRequest)
	}

	// リクエストURL構築
	reqURL, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("エンドポイントURLのパースに失敗しました: %w", err)
	}

	q := reqURL.Query()
	for _, u := range urls {
		q.Add("url", u)
	}
	reqURL.RawQuery = q.Encode()

	// HTTPリクエスト作成
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("HTTPリクエストの作成に失敗しました: %w", err)
	}
	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")

	// HTTPリクエスト実行
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("はてなブックマークAPIの呼び出しに失敗しました",
			slog.String("error", err.Error()),
			slog.Int("url_count", len(urls)),
		)
		return nil, err
	}
	defer resp.Body.Close()

	// HTTPステータスチェック
	if resp.StatusCode != http.StatusOK {
		c.logger.Error("はてなブックマークAPIがエラーステータスを返しました",
			slog.Int("http_status", resp.StatusCode),
			slog.Int("url_count", len(urls)),
		)
		return nil, fmt.Errorf("はてなブックマークAPIがステータス %d を返しました", resp.StatusCode)
	}

	// レスポンスボディ読み取り
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("レスポンスボディの読み取りに失敗しました",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("レスポンスボディの読み取りに失敗しました: %w", err)
	}

	// JSONデコード
	var result map[string]int
	if err := json.Unmarshal(body, &result); err != nil {
		c.logger.Error("はてなブックマークAPIのレスポンスのパースに失敗しました",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("レスポンスJSONのパースに失敗しました: %w", err)
	}

	// レスポンスに含まれないURLは0件として補完する
	counts := make(map[string]int, len(urls))
	for _, u := range urls {
		if count, ok := result[u]; ok {
			counts[u] = count
		} else {
			counts[u] = 0
		}
	}

	return counts, nil
}
