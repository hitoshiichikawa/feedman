package hatebu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// --- Issue #12: レスポンスボディ読み込みサイズ上限のテスト ---

// buildJSONBodyOfExactSize は、指定URLのブックマーク数を含み、
// 全体のバイト長がちょうど targetSize になる有効なJSONレスポンスボディ
// （map[string]int としてパース可能）を生成する。
// パディング用キー（値は固定の int）のキー名の長さを調整して正確なサイズに合わせる。
// レスポンスは json.Marshal(map[string]int) と同形のため、本番のパースロジックで
// そのままデコードできる。
func buildJSONBodyOfExactSize(t *testing.T, reqURL string, count int, targetSize int) []byte {
	t.Helper()

	// JSON は手組みで構築し、バイト長を正確に制御する。
	// 形式: {"<reqURL>":<count>,"<padKey>":0}
	// padKey の文字数を調整して全体長を targetSize に合わせる。
	prefix := fmt.Sprintf("{%q:%d,", reqURL, count)
	suffix := `":0}`
	openQuote := `"`

	// padKey を空にした最小構成の長さ
	minSize := len(prefix) + len(openQuote) + len(suffix)
	if targetSize < minSize {
		t.Fatalf("targetSize %d が最小サイズ %d より小さい", targetSize, minSize)
	}

	padKeyLen := targetSize - minSize
	padKey := strings.Repeat("a", padKeyLen)

	body := prefix + openQuote + padKey + suffix
	if len(body) != targetSize {
		t.Fatalf("生成したJSONのサイズ = %d, want %d", len(body), targetSize)
	}

	// map[string]int としてパース可能であることを検証する（テストの前提条件）
	var sanity map[string]int
	if err := json.Unmarshal([]byte(body), &sanity); err != nil {
		t.Fatalf("生成したJSONが map[string]int にパースできない: %v", err)
	}
	return []byte(body)
}

func TestClient_GetBookmarkCounts_ExactlyMaxSize_ParsesSuccessfully(t *testing.T) {
	// 境界値: ちょうど 1 MiB（1,048,576 バイト）の有効JSONはエラーにならずパースされる（Req 4.1）
	const reqURL = "https://example.com/exact-boundary"
	body := buildJSONBodyOfExactSize(t, reqURL, 7, maxResponseBodySize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	counts, err := c.GetBookmarkCounts(context.Background(), []string{reqURL})
	if err != nil {
		t.Fatalf("上限ちょうど(%d バイト)のレスポンスでエラーが返された: %v", maxResponseBodySize, err)
	}
	if counts[reqURL] != 7 {
		t.Errorf("ブックマーク数 = %d, want 7", counts[reqURL])
	}
}

func TestClient_GetBookmarkCounts_OneByteOverMaxSize_ReturnsError(t *testing.T) {
	// 境界値: 上限を1バイト超過（1,048,577 バイト）したらエラーになる（Req 4.2 / Req 3.1）
	const reqURL = "https://example.com/one-byte-over"
	body := buildJSONBodyOfExactSize(t, reqURL, 7, maxResponseBodySize+1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	counts, err := c.GetBookmarkCounts(context.Background(), []string{reqURL})
	if err == nil {
		t.Fatal("上限を1バイト超過したレスポンスでエラーが返されるべき")
	}
	// 上限超過時はマップを返さない（nil）こと（Req 3.1 / Req 3.3）
	if counts != nil {
		t.Errorf("上限超過時はマップを返さず nil であるべき: got %v", counts)
	}
}

func TestClient_GetBookmarkCounts_OversizedBody_ReturnsErrorAndLogs(t *testing.T) {
	// 異常系: ボディが上限を大きく超えるとエラーが返り、ERRORログが出力される（Req 3.1 / 3.2 / 3.3）
	const reqURL = "https://example.com/oversized"
	body := buildJSONBodyOfExactSize(t, reqURL, 1, maxResponseBodySize*2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	c := NewClient(server.Client(), logger)
	c.endpoint = server.URL

	counts, err := c.GetBookmarkCounts(context.Background(), []string{reqURL})
	if err == nil {
		t.Fatal("上限を大きく超えるレスポンスでエラーが返されるべき")
	}
	// 不完全な切り詰めボディを正常結果として返さない（Req 3.3）
	if counts != nil {
		t.Errorf("上限超過時はマップを返さず nil であるべき: got %v", counts)
	}

	// 上限超過を示すERRORログが出力されていること（Req 3.2）
	logOutput := buf.String()
	if !strings.Contains(logOutput, "ERROR") {
		t.Errorf("上限超過時にERRORレベルのログが記録されるべき: %s", logOutput)
	}
	if !strings.Contains(logOutput, "上限") {
		t.Errorf("上限超過を示すログメッセージが出力されるべき: %s", logOutput)
	}
}
