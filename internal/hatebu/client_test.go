package hatebu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// --- Task 7.1: HatebuClient のテスト ---

func TestNewClient_ReturnsNonNil(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(http.DefaultClient, logger)
	if c == nil {
		t.Fatal("NewClient は nil を返してはならない")
	}
}

func TestClient_GetBookmarkCounts_SingleURL(t *testing.T) {
	// テスト用HTTPサーバー: 1つのURLに対してブックマーク数を返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("HTTPメソッド = %s, want GET", r.Method)
		}

		urls := r.URL.Query()["url"]
		if len(urls) != 1 {
			t.Errorf("URLパラメータ数 = %d, want 1", len(urls))
		}
		if urls[0] != "https://example.com/article1" {
			t.Errorf("URL = %s, want https://example.com/article1", urls[0])
		}

		resp := map[string]int{
			"https://example.com/article1": 42,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	counts, err := c.GetBookmarkCounts(context.Background(), []string{"https://example.com/article1"})
	if err != nil {
		t.Fatalf("GetBookmarkCounts がエラーを返した: %v", err)
	}

	if counts["https://example.com/article1"] != 42 {
		t.Errorf("ブックマーク数 = %d, want 42", counts["https://example.com/article1"])
	}
}

func TestClient_GetBookmarkCounts_MultipleURLs(t *testing.T) {
	// テスト用HTTPサーバー: 複数URLに対するブックマーク数を返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls := r.URL.Query()["url"]
		if len(urls) != 3 {
			t.Errorf("URLパラメータ数 = %d, want 3", len(urls))
		}

		resp := map[string]int{
			"https://example.com/a1": 10,
			"https://example.com/a2": 0,
			"https://example.com/a3": 100,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	urls := []string{
		"https://example.com/a1",
		"https://example.com/a2",
		"https://example.com/a3",
	}
	counts, err := c.GetBookmarkCounts(context.Background(), urls)
	if err != nil {
		t.Fatalf("GetBookmarkCounts がエラーを返した: %v", err)
	}

	if counts["https://example.com/a1"] != 10 {
		t.Errorf("a1 のブックマーク数 = %d, want 10", counts["https://example.com/a1"])
	}
	if counts["https://example.com/a2"] != 0 {
		t.Errorf("a2 のブックマーク数 = %d, want 0", counts["https://example.com/a2"])
	}
	if counts["https://example.com/a3"] != 100 {
		t.Errorf("a3 のブックマーク数 = %d, want 100", counts["https://example.com/a3"])
	}
}

func TestClient_GetBookmarkCounts_ZeroBookmarks_MissingFromResponse(t *testing.T) {
	// はてなAPIはブックマーク0件のURLをレスポンスに含めない場合がある
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// article2 はレスポンスに含めない（= 0件扱い）
		resp := map[string]int{
			"https://example.com/article1": 5,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	urls := []string{
		"https://example.com/article1",
		"https://example.com/article2",
	}
	counts, err := c.GetBookmarkCounts(context.Background(), urls)
	if err != nil {
		t.Fatalf("GetBookmarkCounts がエラーを返した: %v", err)
	}

	if counts["https://example.com/article1"] != 5 {
		t.Errorf("article1 のブックマーク数 = %d, want 5", counts["https://example.com/article1"])
	}
	// レスポンスに含まれないURLは0件として扱う
	if counts["https://example.com/article2"] != 0 {
		t.Errorf("article2 のブックマーク数 = %d, want 0", counts["https://example.com/article2"])
	}
}

func TestClient_GetBookmarkCounts_EmptyResponse(t *testing.T) {
	// レスポンスが空オブジェクトの場合
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	urls := []string{"https://example.com/no-bookmarks"}
	counts, err := c.GetBookmarkCounts(context.Background(), urls)
	if err != nil {
		t.Fatalf("GetBookmarkCounts がエラーを返した: %v", err)
	}

	if counts["https://example.com/no-bookmarks"] != 0 {
		t.Errorf("ブックマーク数 = %d, want 0", counts["https://example.com/no-bookmarks"])
	}
}

func TestClient_GetBookmarkCounts_EmptyURLList(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(http.DefaultClient, logger)

	counts, err := c.GetBookmarkCounts(context.Background(), []string{})
	if err != nil {
		t.Fatalf("空URLリストでエラーが返された: %v", err)
	}

	if len(counts) != 0 {
		t.Errorf("空URLリストの結果は空マップであるべき: got %d entries", len(counts))
	}
}

func TestClient_GetBookmarkCounts_NilURLList(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(http.DefaultClient, logger)

	counts, err := c.GetBookmarkCounts(context.Background(), nil)
	if err != nil {
		t.Fatalf("nil URLリストでエラーが返された: %v", err)
	}

	if len(counts) != 0 {
		t.Errorf("nil URLリストの結果は空マップであるべき: got %d entries", len(counts))
	}
}

func TestClient_GetBookmarkCounts_TooManyURLs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(http.DefaultClient, logger)

	// 51個のURLで呼び出す（上限は50）
	urls := make([]string, 51)
	for i := range urls {
		urls[i] = "https://example.com/article"
	}

	_, err := c.GetBookmarkCounts(context.Background(), urls)
	if err == nil {
		t.Fatal("51個のURLでエラーが返されるべき")
	}
	if !strings.Contains(err.Error(), "50") {
		t.Errorf("エラーメッセージに上限値50が含まれるべき: %s", err.Error())
	}
}

func TestClient_GetBookmarkCounts_HTTPError(t *testing.T) {
	// テスト用HTTPサーバー: 500エラーを返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	_, err := c.GetBookmarkCounts(context.Background(), []string{"https://example.com/article"})
	if err == nil {
		t.Fatal("HTTPエラー時にエラーが返されるべき")
	}
}

func TestClient_GetBookmarkCounts_InvalidJSON(t *testing.T) {
	// テスト用HTTPサーバー: 不正なJSONを返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	_, err := c.GetBookmarkCounts(context.Background(), []string{"https://example.com/article"})
	if err == nil {
		t.Fatal("不正JSONレスポンス時にエラーが返されるべき")
	}
}

func TestClient_GetBookmarkCounts_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	_, err := c.GetBookmarkCounts(ctx, []string{"https://example.com/article"})
	if err == nil {
		t.Fatal("キャンセルされたコンテキストでエラーが返されるべき")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled エラーであるべき: got %v", err)
	}
}

func TestClient_GetBookmarkCounts_LogsError(t *testing.T) {
	// テスト用HTTPサーバー: 500エラーを返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	_, _ = c.GetBookmarkCounts(context.Background(), []string{"https://example.com/article"})

	// エラーログが出力されていること
	logOutput := buf.String()
	if !strings.Contains(logOutput, "ERROR") {
		t.Errorf("APIエラー時にERRORレベルのログが記録されるべき: %s", logOutput)
	}
}

func TestClient_GetBookmarkCounts_50URLsExactly(t *testing.T) {
	// ちょうど50個のURLは許可される
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls := r.URL.Query()["url"]
		if len(urls) != 50 {
			t.Errorf("URLパラメータ数 = %d, want 50", len(urls))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	urls := make([]string, 50)
	for i := range urls {
		urls[i] = "https://example.com/article"
	}

	_, err := c.GetBookmarkCounts(context.Background(), urls)
	if err != nil {
		t.Fatalf("50個のURLでエラーが返されるべきではない: %v", err)
	}
}
