package item

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- テスト用モック ---

// mockItemRepo はテスト用のItemRepositoryモック。
type mockItemRepo struct {
	items         map[string]*model.Item // id -> item
	byFeedGUID    map[string]*model.Item // feedID+guid -> item
	byFeedLink    map[string]*model.Item // feedID+link -> item
	byContentHash map[string]*model.Item // feedID+hash -> item
	createCalls   int
	updateCalls   int

	// バルクメソッドの呼び出し回数（DB 往復が記事件数に比例しないことの検証用）。
	findBulkCalls int
	upsertCalls   int

	// 直近のバルク永続化内容。
	lastBulkCreated []*model.Item
	lastBulkUpdated []*model.Item

	lastCreatedItem *model.Item
	lastUpdatedItem *model.Item

	// findErr / upsertErr が non-nil の場合、それぞれのバルクメソッドがエラーを返す。
	findErr   error
	upsertErr error
}

func newMockItemRepo() *mockItemRepo {
	return &mockItemRepo{
		items:         make(map[string]*model.Item),
		byFeedGUID:    make(map[string]*model.Item),
		byFeedLink:    make(map[string]*model.Item),
		byContentHash: make(map[string]*model.Item),
	}
}

func (m *mockItemRepo) FindByID(_ context.Context, id string) (*model.Item, error) {
	item, ok := m.items[id]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (m *mockItemRepo) ListByFeed(_ context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
	return nil, nil
}

// ListStarredByUser はインターフェース充足のための最小スタブ。
// 本 task では Repository 層の実装と DB 結合テストのみがスコープであり、
// service 層への組み込みは task 2 で行うため、サービス層テストでは未使用。
func (m *mockItemRepo) ListStarredByUser(_ context.Context, _ string, _ time.Time, _ int) ([]repository.StarredItemRow, error) {
	return nil, nil
}

// ListNewAcrossFeeds は ItemRepository interface 適合のためのスタブ。
// upsert 経路のテストでは横断新着取得は対象外（Issue #121）のため、常に nil を返す。
func (m *mockItemRepo) ListNewAcrossFeeds(
	_ context.Context,
	_ string,
	_ time.Time,
	_ time.Time,
	_ string,
	_ int,
) ([]repository.CrossFeedItem, error) {
	return nil, nil
}

func (m *mockItemRepo) FindByFeedAndGUID(_ context.Context, feedID, guid string) (*model.Item, error) {
	key := feedID + "|" + guid
	item, ok := m.byFeedGUID[key]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (m *mockItemRepo) FindByFeedAndLink(_ context.Context, feedID, link string) (*model.Item, error) {
	key := feedID + "|" + link
	item, ok := m.byFeedLink[key]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (m *mockItemRepo) FindByContentHash(_ context.Context, feedID, contentHash string) (*model.Item, error) {
	key := feedID + "|" + contentHash
	item, ok := m.byContentHash[key]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (m *mockItemRepo) Create(_ context.Context, item *model.Item) error {
	m.createCalls++
	m.lastCreatedItem = item
	m.items[item.ID] = item
	if item.GuidOrID != "" {
		m.byFeedGUID[item.FeedID+"|"+item.GuidOrID] = item
	}
	if item.Link != "" {
		m.byFeedLink[item.FeedID+"|"+item.Link] = item
	}
	if item.ContentHash != "" {
		m.byContentHash[item.FeedID+"|"+item.ContentHash] = item
	}
	return nil
}

func (m *mockItemRepo) Update(_ context.Context, item *model.Item) error {
	m.updateCalls++
	m.lastUpdatedItem = item
	m.items[item.ID] = item
	return nil
}

// FindExistingForUpsert はバッチ化した既存記事取得をモックする。
// 内部の byFeed* マップから guid/link/hash 別の既存記事を一括で索引して返す。
// 実 DB 実装と同様、呼び出し回数は記事件数に依存せず 1 バッチあたり 1 回に収まる。
func (m *mockItemRepo) FindExistingForUpsert(
	_ context.Context,
	feedID string,
	guids, links, hashes []string,
) (*repository.ExistingItems, error) {
	m.findBulkCalls++
	if m.findErr != nil {
		return nil, m.findErr
	}

	result := &repository.ExistingItems{
		ByGUID:        make(map[string]*model.Item),
		ByLink:        make(map[string]*model.Item),
		ByContentHash: make(map[string]*model.Item),
	}
	for _, g := range guids {
		if item, ok := m.byFeedGUID[feedID+"|"+g]; ok {
			result.ByGUID[g] = item
		}
	}
	for _, l := range links {
		if item, ok := m.byFeedLink[feedID+"|"+l]; ok {
			result.ByLink[l] = item
		}
	}
	for _, h := range hashes {
		if item, ok := m.byContentHash[feedID+"|"+h]; ok {
			result.ByContentHash[h] = item
		}
	}
	return result, nil
}

// BulkUpsert はバルク永続化をモックする。
// upsertErr が設定されている場合はエラーを返し、内部状態を一切変更しない（全件ロールバック相当）。
func (m *mockItemRepo) BulkUpsert(_ context.Context, toCreate, toUpdate []*model.Item) error {
	m.upsertCalls++
	if m.upsertErr != nil {
		return m.upsertErr
	}

	m.lastBulkCreated = toCreate
	m.lastBulkUpdated = toUpdate

	for _, item := range toCreate {
		m.createCalls++
		m.lastCreatedItem = item
		m.items[item.ID] = item
		if item.GuidOrID != "" {
			m.byFeedGUID[item.FeedID+"|"+item.GuidOrID] = item
		}
		if item.Link != "" {
			m.byFeedLink[item.FeedID+"|"+item.Link] = item
		}
		if item.ContentHash != "" {
			m.byContentHash[item.FeedID+"|"+item.ContentHash] = item
		}
	}
	for _, item := range toUpdate {
		m.updateCalls++
		m.lastUpdatedItem = item
		m.items[item.ID] = item
	}
	return nil
}

// addExistingItem はテスト用の既存記事をモックに追加する。
func (m *mockItemRepo) addExistingItem(item *model.Item) {
	m.items[item.ID] = item
	if item.GuidOrID != "" {
		m.byFeedGUID[item.FeedID+"|"+item.GuidOrID] = item
	}
	if item.Link != "" {
		m.byFeedLink[item.FeedID+"|"+item.Link] = item
	}
	if item.ContentHash != "" {
		m.byContentHash[item.FeedID+"|"+item.ContentHash] = item
	}
}

// mockSanitizer はテスト用のContentSanitizerServiceモック。
type mockSanitizer struct {
	sanitizeCalls int
}

func (m *mockSanitizer) Sanitize(rawHTML string) string {
	m.sanitizeCalls++
	// テスト用: [sanitized] プレフィックスを付与して呼び出しを検証可能にする
	if rawHTML == "" {
		return ""
	}
	return "[sanitized]" + rawHTML
}

// --- 同一性判定テスト ---

// TestUpsertItems_IdentityByGUID はguid_or_idによる同一性判定（最優先）をテストする。
func TestUpsertItems_IdentityByGUID(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	existingItem := &model.Item{
		ID:       "existing-item-1",
		FeedID:   "feed-1",
		GuidOrID: "guid-123",
		Title:    "古いタイトル",
		Link:     "https://example.com/old",
		Content:  "古いコンテンツ",
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "guid-123",
			Title:    "新しいタイトル",
			Link:     "https://example.com/new",
			Content:  "<p>新しいコンテンツ</p>",
			Summary:  "新しいサマリー",
		},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if repo.updateCalls != 1 {
		t.Errorf("updateCalls = %d, want 1", repo.updateCalls)
	}
	// 既存記事が上書き更新されていること
	if repo.lastUpdatedItem.Title != "新しいタイトル" {
		t.Errorf("updated title = %q, want %q", repo.lastUpdatedItem.Title, "新しいタイトル")
	}
}

// TestUpsertItems_IdentityByLink はlinkによる同一性判定（第2優先）をテストする。
func TestUpsertItems_IdentityByLink(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	existingItem := &model.Item{
		ID:     "existing-item-2",
		FeedID: "feed-1",
		// GuidOrIDなし
		Link:    "https://example.com/article",
		Title:   "古いタイトル",
		Content: "古いコンテンツ",
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			// GuidOrIDなし -> linkで検索
			Link:    "https://example.com/article",
			Title:   "更新タイトル",
			Content: "<p>更新コンテンツ</p>",
			Summary: "更新サマリー",
		},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if repo.lastUpdatedItem.Title != "更新タイトル" {
		t.Errorf("updated title = %q, want %q", repo.lastUpdatedItem.Title, "更新タイトル")
	}
}

// TestUpsertItems_IdentityByContentHash はcontent_hashによる同一性判定（第3優先）をテストする。
func TestUpsertItems_IdentityByContentHash(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	// hash(title + published + summary) で一致させるための既存記事
	pubTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	existingItem := &model.Item{
		ID:          "existing-item-3",
		FeedID:      "feed-1",
		Title:       "同じタイトル",
		Summary:     "[sanitized]同じサマリー",
		PublishedAt: &pubTime,
		ContentHash: computeContentHash("同じタイトル", &pubTime, "[sanitized]同じサマリー"),
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			// GuidOrIDなし、Linkなし -> hashで検索
			Title:       "同じタイトル",
			Summary:     "同じサマリー",
			PublishedAt: &pubTime,
			Content:     "<p>新コンテンツ</p>",
		},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
}

// TestUpsertItems_IdentityPriority_GUIDOverLink はGUID判定がlink判定より優先されることをテストする。
func TestUpsertItems_IdentityPriority_GUIDOverLink(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	// guid_or_idで一致する記事を用意
	guidItem := &model.Item{
		ID:       "guid-item",
		FeedID:   "feed-1",
		GuidOrID: "guid-abc",
		Link:     "https://example.com/different-link",
		Title:    "GUID記事",
	}
	repo.addExistingItem(guidItem)

	// linkで一致する別の記事を用意
	linkItem := &model.Item{
		ID:     "link-item",
		FeedID: "feed-1",
		Link:   "https://example.com/target-link",
		Title:  "Link記事",
	}
	repo.addExistingItem(linkItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "guid-abc",                        // guidで一致
			Link:     "https://example.com/target-link", // linkでも別の記事に一致
			Title:    "更新タイトル",
			Content:  "<p>更新</p>",
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	// GUID記事が更新されるべき（link記事ではなく）
	if repo.lastUpdatedItem.ID != "guid-item" {
		t.Errorf("更新されたのはGUID記事であるべき。ID = %q, want %q", repo.lastUpdatedItem.ID, "guid-item")
	}
}

// TestUpsertItems_IdentityPriority_LinkOverHash はlink判定がhash判定より優先されることをテストする。
func TestUpsertItems_IdentityPriority_LinkOverHash(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	pubTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	// linkで一致する記事を用意
	linkItem := &model.Item{
		ID:     "link-item",
		FeedID: "feed-1",
		Link:   "https://example.com/article",
		Title:  "Link記事",
	}
	repo.addExistingItem(linkItem)

	// hashで一致する別の記事を用意
	hashItem := &model.Item{
		ID:          "hash-item",
		FeedID:      "feed-1",
		Title:       "ハッシュタイトル",
		Summary:     "[sanitized]ハッシュサマリー",
		PublishedAt: &pubTime,
		ContentHash: computeContentHash("ハッシュタイトル", &pubTime, "[sanitized]ハッシュサマリー"),
	}
	repo.addExistingItem(hashItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			// GuidOrIDなし -> linkで検索
			Link:        "https://example.com/article",
			Title:       "ハッシュタイトル",
			Summary:     "ハッシュサマリー",
			PublishedAt: &pubTime,
			Content:     "<p>コンテンツ</p>",
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	// Link記事が更新されるべき（Hash記事ではなく）
	if repo.lastUpdatedItem.ID != "link-item" {
		t.Errorf("更新されたのはLink記事であるべき。ID = %q, want %q", repo.lastUpdatedItem.ID, "link-item")
	}
}

// --- 新規記事挿入テスト ---

// TestUpsertItems_NewItem_Insert は新規記事が正しく挿入されることをテストする。
func TestUpsertItems_NewItem_Insert(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	pubTime := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "new-guid-1",
			Title:       "新規記事",
			Link:        "https://example.com/new-article",
			Content:     "<p>新規コンテンツ</p>",
			Summary:     "新規サマリー",
			Author:      "著者A",
			PublishedAt: &pubTime,
		},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted = %d, want 1", inserted)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0", updated)
	}
	if repo.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", repo.createCalls)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if created.FeedID != "feed-1" {
		t.Errorf("created.FeedID = %q, want %q", created.FeedID, "feed-1")
	}
	if created.GuidOrID != "new-guid-1" {
		t.Errorf("created.GuidOrID = %q, want %q", created.GuidOrID, "new-guid-1")
	}
	if created.Title != "新規記事" {
		t.Errorf("created.Title = %q, want %q", created.Title, "新規記事")
	}
	if created.Author != "著者A" {
		t.Errorf("created.Author = %q, want %q", created.Author, "著者A")
	}
	if created.IsDateEstimated {
		t.Error("published_atが設定されている場合、推定フラグはfalseであるべき")
	}
	if created.PublishedAt == nil || !created.PublishedAt.Equal(pubTime) {
		t.Errorf("created.PublishedAt = %v, want %v", created.PublishedAt, pubTime)
	}
}

// TestUpsertItems_NewItem_PublishedAtMissing_UsesFetchedAt はpublished_at未設定時にfetched_atを代用することをテストする。
func TestUpsertItems_NewItem_PublishedAtMissing_UsesFetchedAt(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "no-pubdate-guid",
			Title:    "日付なし記事",
			Link:     "https://example.com/no-date",
			Content:  "<p>コンテンツ</p>",
			// PublishedAt は nil
		},
	}

	inserted, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted = %d, want 1", inserted)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}

	// published_at未設定の場合、fetched_atが代用される
	if created.PublishedAt == nil {
		t.Fatal("published_atがnilであってはならない（fetched_atが代用されるべき）")
	}
	// fetched_atと同じ値が設定されているはず
	if !created.PublishedAt.Equal(created.FetchedAt) {
		t.Errorf("published_at(%v) should equal fetched_at(%v)", created.PublishedAt, created.FetchedAt)
	}
	// 推定フラグがtrueであること
	if !created.IsDateEstimated {
		t.Error("published_at未設定時はIsDateEstimatedがtrueであるべき")
	}
}

// --- サニタイズテスト ---

// TestUpsertItems_ContentIsSanitized は記事コンテンツにサニタイズが適用されることをテストする。
func TestUpsertItems_ContentIsSanitized(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "sanitize-test",
			Title:    "サニタイズテスト",
			Link:     "https://example.com/sanitize",
			Content:  "<script>alert('xss')</script><p>安全なコンテンツ</p>",
			Summary:  "<script>evil</script>サマリー",
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	if sanitizer.sanitizeCalls < 2 {
		t.Errorf("sanitize should be called for content and summary, got %d calls", sanitizer.sanitizeCalls)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	// モックのサニタイザーは[sanitized]プレフィックスを付与する
	if created.Content != "[sanitized]<script>alert('xss')</script><p>安全なコンテンツ</p>" {
		t.Errorf("content should be sanitized, got %q", created.Content)
	}
	if created.Summary != "[sanitized]<script>evil</script>サマリー" {
		t.Errorf("summary should be sanitized, got %q", created.Summary)
	}
}

// TestUpsertItems_EmptyContentNotSanitized は空コンテンツがサニタイズされないことをテストする。
func TestUpsertItems_EmptyContentNotSanitized(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "empty-content",
			Title:    "空コンテンツ",
			Content:  "",
			Summary:  "",
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if created.Content != "" {
		t.Errorf("空コンテンツはそのまま空であるべき、got %q", created.Content)
	}
	if created.Summary != "" {
		t.Errorf("空サマリーはそのまま空であるべき、got %q", created.Summary)
	}
}

// --- 上書き更新テスト ---

// TestUpsertItems_Update_OverwritesContent は既存記事の上書き更新で内容が正しく反映されることをテストする。
func TestUpsertItems_Update_OverwritesContent(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	pubTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	existingItem := &model.Item{
		ID:          "existing-1",
		FeedID:      "feed-1",
		GuidOrID:    "guid-update",
		Title:       "古いタイトル",
		Link:        "https://example.com/old",
		Content:     "古いコンテンツ",
		Summary:     "古いサマリー",
		Author:      "古い著者",
		PublishedAt: &pubTime,
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	newPubTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "guid-update",
			Title:       "新しいタイトル",
			Link:        "https://example.com/new",
			Content:     "<p>新しいコンテンツ</p>",
			Summary:     "新しいサマリー",
			Author:      "新しい著者",
			PublishedAt: &newPubTime,
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}

	u := repo.lastUpdatedItem
	if u == nil {
		t.Fatal("lastUpdatedItem should not be nil")
	}
	// 既存のIDが保持されること
	if u.ID != "existing-1" {
		t.Errorf("ID = %q, want %q (既存のIDが保持されるべき)", u.ID, "existing-1")
	}
	if u.Title != "新しいタイトル" {
		t.Errorf("Title = %q, want %q", u.Title, "新しいタイトル")
	}
	if u.Link != "https://example.com/new" {
		t.Errorf("Link = %q, want %q", u.Link, "https://example.com/new")
	}
	if u.Author != "新しい著者" {
		t.Errorf("Author = %q, want %q", u.Author, "新しい著者")
	}
	if u.PublishedAt == nil || !u.PublishedAt.Equal(newPubTime) {
		t.Errorf("PublishedAt = %v, want %v", u.PublishedAt, newPubTime)
	}
	// 上書き更新時はIsDateEstimatedがfalseであること（published_atが明示的に設定されている場合）
	if u.IsDateEstimated {
		t.Error("published_atが明示的に設定されている場合、IsDateEstimatedはfalseであるべき")
	}
}

// --- 複数記事の一括処理テスト ---

// TestUpsertItems_MultipleItems は複数記事の一括UPSERTをテストする。
func TestUpsertItems_MultipleItems(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	existingItem := &model.Item{
		ID:       "existing-multi",
		FeedID:   "feed-1",
		GuidOrID: "guid-existing",
		Title:    "既存記事",
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "guid-existing",
			Title:    "更新された既存記事",
			Content:  "<p>更新</p>",
		},
		{
			GuidOrID: "guid-new-1",
			Title:    "新規記事1",
			Content:  "<p>新規1</p>",
		},
		{
			GuidOrID: "guid-new-2",
			Title:    "新規記事2",
			Content:  "<p>新規2</p>",
		},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 2 {
		t.Errorf("inserted = %d, want 2", inserted)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
}

// --- 空の入力テスト ---

// TestUpsertItems_EmptyItems は空の記事リストに対してエラーなく0件を返すことをテストする。
func TestUpsertItems_EmptyItems(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", []model.ParsedItem{})
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0", updated)
	}
}

// TestUpsertItems_NilItems はnil記事リストに対してエラーなく0件を返すことをテストする。
func TestUpsertItems_NilItems(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", nil)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0", updated)
	}
}

// --- ContentHash計算テスト ---

// TestComputeContentHash_Deterministic は同一入力に対して同一ハッシュを返すことをテストする。
func TestComputeContentHash_Deterministic(t *testing.T) {
	pubTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hash1 := computeContentHash("タイトル", &pubTime, "サマリー")
	hash2 := computeContentHash("タイトル", &pubTime, "サマリー")

	if hash1 != hash2 {
		t.Errorf("同一入力に対してハッシュが一致すべき: %q != %q", hash1, hash2)
	}
	if hash1 == "" {
		t.Error("ハッシュが空であってはならない")
	}
}

// TestComputeContentHash_DifferentInputs は異なる入力に対して異なるハッシュを返すことをテストする。
func TestComputeContentHash_DifferentInputs(t *testing.T) {
	pubTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hash1 := computeContentHash("タイトル1", &pubTime, "サマリー")
	hash2 := computeContentHash("タイトル2", &pubTime, "サマリー")

	if hash1 == hash2 {
		t.Error("異なる入力に対してハッシュが異なるべき")
	}
}

// TestComputeContentHash_NilPublishedAt はpublished_atがnilの場合でもハッシュが計算されることをテストする。
func TestComputeContentHash_NilPublishedAt(t *testing.T) {
	hash := computeContentHash("タイトル", nil, "サマリー")
	if hash == "" {
		t.Error("ハッシュが空であってはならない")
	}
}

// --- 新規記事のID生成テスト ---

// TestUpsertItems_NewItem_HasValidID は新規記事にUUIDが付与されることをテストする。
func TestUpsertItems_NewItem_HasValidID(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "new-id-test",
			Title:    "ID生成テスト",
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if created.ID == "" {
		t.Error("新規記事のIDが空であってはならない")
	}
}

// --- 更新時のサニタイズテスト ---

// TestUpsertItems_Update_ContentIsSanitized は更新時にもコンテンツがサニタイズされることをテストする。
func TestUpsertItems_Update_ContentIsSanitized(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	existingItem := &model.Item{
		ID:       "sanitize-update",
		FeedID:   "feed-1",
		GuidOrID: "guid-sanitize-update",
		Title:    "古い",
		Content:  "古いコンテンツ",
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "guid-sanitize-update",
			Title:    "新しい",
			Content:  "<script>bad</script><p>good</p>",
			Summary:  "<iframe>bad</iframe>safe",
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	u := repo.lastUpdatedItem
	if u == nil {
		t.Fatal("lastUpdatedItem should not be nil")
	}
	// サニタイズされていることを確認
	if u.Content != "[sanitized]<script>bad</script><p>good</p>" {
		t.Errorf("updated content should be sanitized, got %q", u.Content)
	}
}

// --- ContentHash が保存されるテスト ---

// TestUpsertItems_NewItem_ContentHashSet は新規記事にcontent_hashが設定されることをテストする。
func TestUpsertItems_NewItem_ContentHashSet(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	pubTime := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "hash-test-guid",
			Title:       "ハッシュテスト",
			Summary:     "テストサマリー",
			PublishedAt: &pubTime,
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if created.ContentHash == "" {
		t.Error("新規記事のContentHashが空であってはならない")
	}
}

// --- FetchedAtが設定されるテスト ---

// TestUpsertItems_NewItem_FetchedAtSet は新規記事にFetchedAtが設定されることをテストする。
func TestUpsertItems_NewItem_FetchedAtSet(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "fetched-at-test",
			Title:    "FetchedAtテスト",
		},
	}

	before := time.Now()
	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	after := time.Now()

	created := repo.lastCreatedItem
	if created == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if created.FetchedAt.Before(before) || created.FetchedAt.After(after) {
		t.Errorf("FetchedAt = %v, expected between %v and %v", created.FetchedAt, before, after)
	}
}

// --- GUIDあり + GUID未検出 → linkフォールバック ---

// TestUpsertItems_GUIDNotFound_FallbackToLink はGUIDで検索して未検出の場合にlinkでフォールバックすることをテストする。
func TestUpsertItems_GUIDNotFound_FallbackToLink(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	// linkのみで一致する記事
	linkItem := &model.Item{
		ID:     "link-fallback-item",
		FeedID: "feed-1",
		Link:   "https://example.com/fallback",
		Title:  "Linkフォールバック",
	}
	repo.addExistingItem(linkItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "nonexistent-guid",             // GUIDでは見つからない
			Link:     "https://example.com/fallback", // linkで見つかる
			Title:    "更新タイトル",
			Content:  "<p>更新</p>",
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if repo.lastUpdatedItem.ID != "link-fallback-item" {
		t.Errorf("linkフォールバックで更新されるべき。ID = %q, want %q", repo.lastUpdatedItem.ID, "link-fallback-item")
	}
}

// TestUpsertItems_GUIDAndLinkNotFound_FallbackToHash はGUIDとlinkで未検出の場合にhashでフォールバックすることをテストする。
func TestUpsertItems_GUIDAndLinkNotFound_FallbackToHash(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	pubTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	hashItem := &model.Item{
		ID:          "hash-fallback-item",
		FeedID:      "feed-1",
		Title:       "ハッシュフォールバック",
		Summary:     "[sanitized]サマリー",
		PublishedAt: &pubTime,
		ContentHash: computeContentHash("ハッシュフォールバック", &pubTime, "[sanitized]サマリー"),
	}
	repo.addExistingItem(hashItem)

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "nonexistent-guid",
			Link:        "https://example.com/nonexistent",
			Title:       "ハッシュフォールバック",
			Summary:     "サマリー",
			PublishedAt: &pubTime,
			Content:     "<p>コンテンツ</p>",
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if repo.lastUpdatedItem.ID != "hash-fallback-item" {
		t.Errorf("hashフォールバックで更新されるべき。ID = %q, want %q", repo.lastUpdatedItem.ID, "hash-fallback-item")
	}
}

// --- 更新時のContentHashが更新されるテスト ---

// TestUpsertItems_Update_ContentHashUpdated は更新時にContentHashが再計算されることをテストする。
func TestUpsertItems_Update_ContentHashUpdated(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	oldPubTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	existingItem := &model.Item{
		ID:          "hash-update-item",
		FeedID:      "feed-1",
		GuidOrID:    "guid-hash-update",
		Title:       "古いタイトル",
		Summary:     "古いサマリー",
		PublishedAt: &oldPubTime,
		ContentHash: "old-hash",
	}
	repo.addExistingItem(existingItem)

	svc := NewItemUpsertService(repo, sanitizer)

	newPubTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "guid-hash-update",
			Title:       "新しいタイトル",
			Summary:     "新しいサマリー",
			PublishedAt: &newPubTime,
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	u := repo.lastUpdatedItem
	if u == nil {
		t.Fatal("lastUpdatedItem should not be nil")
	}
	if u.ContentHash == "old-hash" {
		t.Error("ContentHashが更新されるべき")
	}
	if u.ContentHash == "" {
		t.Error("ContentHashが空であってはならない")
	}
}

// --- バルク化に伴う AC 網羅テスト ---

// TestUpsertItems_Mixed50_Counts は50件の混在バッチ（新規N件・既存M件）で
// inserted=N / updated=M が返ることを検証する（Requirement 1.1, 1.2）。
func TestUpsertItems_Mixed50_Counts(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	const existingCount = 20 // M
	const newCount = 30      // N
	// 既存記事 M 件をリポジトリに用意する。
	for i := 0; i < existingCount; i++ {
		repo.addExistingItem(&model.Item{
			ID:       fmt.Sprintf("existing-%d", i),
			FeedID:   "feed-1",
			GuidOrID: fmt.Sprintf("existing-guid-%d", i),
			Title:    fmt.Sprintf("既存記事-%d", i),
		})
	}

	svc := NewItemUpsertService(repo, sanitizer)

	// 既存 M 件と新規 N 件を交互に混在させたバッチ（合計 50 件）。
	parsedItems := make([]model.ParsedItem, 0, existingCount+newCount)
	for i := 0; i < existingCount; i++ {
		parsedItems = append(parsedItems, model.ParsedItem{
			GuidOrID: fmt.Sprintf("existing-guid-%d", i),
			Title:    fmt.Sprintf("更新-%d", i),
			Content:  "<p>更新</p>",
		})
	}
	for i := 0; i < newCount; i++ {
		parsedItems = append(parsedItems, model.ParsedItem{
			GuidOrID: fmt.Sprintf("new-guid-%d", i),
			Title:    fmt.Sprintf("新規-%d", i),
			Content:  "<p>新規</p>",
		})
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != newCount {
		t.Errorf("inserted = %d, want %d", inserted, newCount)
	}
	if updated != existingCount {
		t.Errorf("updated = %d, want %d", updated, existingCount)
	}
}

// TestUpsertItems_RoundTripsConstant は記事件数を変えても DB 往復が定数オーダーに
// 収まることを検証する（Requirement 2.1, 2.2, NFR 3.1）。
func TestUpsertItems_RoundTripsConstant(t *testing.T) {
	cases := []int{1, 10, 50}

	for _, n := range cases {
		t.Run(fmt.Sprintf("%d件でも往復回数が定数のとき定数オーダーに収まる", n), func(t *testing.T) {
			repo := newMockItemRepo()
			sanitizer := &mockSanitizer{}
			svc := NewItemUpsertService(repo, sanitizer)

			parsedItems := make([]model.ParsedItem, 0, n)
			for i := 0; i < n; i++ {
				parsedItems = append(parsedItems, model.ParsedItem{
					GuidOrID: fmt.Sprintf("guid-%d", i),
					Title:    fmt.Sprintf("記事-%d", i),
				})
			}

			_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
			if err != nil {
				t.Fatalf("UpsertItems returned error: %v", err)
			}

			// バルクメソッドの呼び出しは記事件数に依存せず固定回数（各 1 回）であること。
			if repo.findBulkCalls != 1 {
				t.Errorf("findBulkCalls = %d, want 1 (記事件数 %d に依存しない定数)", repo.findBulkCalls, n)
			}
			if repo.upsertCalls != 1 {
				t.Errorf("upsertCalls = %d, want 1 (記事件数 %d に依存しない定数)", repo.upsertCalls, n)
			}
		})
	}
}

// TestUpsertItems_Update_PreservesID は既存記事更新時に id が新規採番されず保持されることを
// 検証する（Requirement 1.3）。
func TestUpsertItems_Update_PreservesID(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	repo.addExistingItem(&model.Item{
		ID:       "preserve-id-1",
		FeedID:   "feed-1",
		GuidOrID: "guid-preserve",
		Title:    "古い",
	})

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{
			GuidOrID: "guid-preserve",
			Title:    "新しい",
			Content:  "<p>新</p>",
		},
	}

	_, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if repo.lastUpdatedItem.ID != "preserve-id-1" {
		t.Errorf("更新後の id = %q, want %q（新規採番してはならない）", repo.lastUpdatedItem.ID, "preserve-id-1")
	}
}

// TestUpsertItems_Update_SanitizedContentAndHash は更新時にサニタイズ後コンテンツ・サマリーと
// 再計算した content_hash が保存されることを検証する（Requirement 1.5）。
func TestUpsertItems_Update_SanitizedContentAndHash(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	oldPubTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repo.addExistingItem(&model.Item{
		ID:          "sanitize-hash-1",
		FeedID:      "feed-1",
		GuidOrID:    "guid-sanitize-hash",
		Title:       "古い",
		Summary:     "古いサマリー",
		PublishedAt: &oldPubTime,
		ContentHash: "old-hash",
	})

	svc := NewItemUpsertService(repo, sanitizer)

	newPubTime := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	parsedItems := []model.ParsedItem{
		{
			GuidOrID:    "guid-sanitize-hash",
			Title:       "新しい",
			Content:     "<script>x</script><p>本文</p>",
			Summary:     "<iframe>y</iframe>要約",
			PublishedAt: &newPubTime,
		},
	}

	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	u := repo.lastUpdatedItem
	if u == nil {
		t.Fatal("lastUpdatedItem should not be nil")
	}
	if u.Content != "[sanitized]<script>x</script><p>本文</p>" {
		t.Errorf("更新コンテンツがサニタイズされるべき, got %q", u.Content)
	}
	if u.Summary != "[sanitized]<iframe>y</iframe>要約" {
		t.Errorf("更新サマリーがサニタイズされるべき, got %q", u.Summary)
	}
	// content_hash はサニタイズ後サマリーで再計算された値であること。
	want := computeContentHash("新しい", &newPubTime, "[sanitized]<iframe>y</iframe>要約")
	if u.ContentHash != want {
		t.Errorf("content_hash = %q, want %q（サニタイズ後サマリーで再計算）", u.ContentHash, want)
	}
}

// TestUpsertItems_DBError_FindReturnsZeroAndWrappedError は既存記事取得時のエラーで
// (0, 0, err) を返し、発生元エラーが wrap されることを検証する（Requirement 3.1, 3.2, 3.3）。
func TestUpsertItems_DBError_FindReturnsZeroAndWrappedError(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	sentinel := errors.New("find boom")
	repo.findErr = sentinel

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "guid-1", Title: "記事"},
		{GuidOrID: "guid-2", Title: "記事2"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err == nil {
		t.Fatal("エラーが返るべき")
	}
	if inserted != 0 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 0)", inserted, updated)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("発生元エラーが wrap されるべき: %v", err)
	}
	// 永続化が呼ばれていない（1 件も書き込んでいない）こと。
	if repo.upsertCalls != 0 {
		t.Errorf("upsertCalls = %d, want 0（取得失敗時は永続化しない）", repo.upsertCalls)
	}
}

// TestUpsertItems_DBError_UpsertRollsBackAndReturnsZero は永続化時のエラーで全件ロールバックし
// (0, 0, err) を返し発生元エラーが wrap されることを検証する（Requirement 3.1, 3.2, 3.3）。
func TestUpsertItems_DBError_UpsertRollsBackAndReturnsZero(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	sentinel := errors.New("upsert boom")
	repo.upsertErr = sentinel

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "guid-new-1", Title: "新規1"},
		{GuidOrID: "guid-new-2", Title: "新規2"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err == nil {
		t.Fatal("エラーが返るべき")
	}
	if inserted != 0 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 0)", inserted, updated)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("発生元エラーが wrap されるべき: %v", err)
	}
	// ロールバック相当: モックの内部状態に記事が永続化されていないこと。
	if len(repo.items) != 0 {
		t.Errorf("永続化された記事数 = %d, want 0（全件ロールバック）", len(repo.items))
	}
}

// TestUpsertItems_SingleNewItem は1件の新規記事で (1, 0, nil) を返すことを検証する
// （Requirement 4.3）。
func TestUpsertItems_SingleNewItem(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "single-new", Title: "単一新規"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (1, 0)", inserted, updated)
	}
}

// TestUpsertItems_SingleExistingItem は1件の既存記事一致で (0, 1, nil) を返すことを検証する
// （Requirement 4.4）。
func TestUpsertItems_SingleExistingItem(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	repo.addExistingItem(&model.Item{
		ID:       "single-existing",
		FeedID:   "feed-1",
		GuidOrID: "single-existing-guid",
		Title:    "既存",
	})

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "single-existing-guid", Title: "更新"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 || updated != 1 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 1)", inserted, updated)
	}
}

// TestUpsertItems_EmptyItems_NoDBAccess は空スライスでDBアクセスせず早期 return することを
// 検証する（Requirement 4.1）。
func TestUpsertItems_EmptyItems_NoDBAccess(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	svc := NewItemUpsertService(repo, sanitizer)

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", []model.ParsedItem{})
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 0)", inserted, updated)
	}
	if repo.findBulkCalls != 0 || repo.upsertCalls != 0 {
		t.Errorf("空入力時は DB へアクセスしてはならない: findBulkCalls=%d upsertCalls=%d", repo.findBulkCalls, repo.upsertCalls)
	}
}

// TestUpsertItems_NilItems_NoDBAccess はnilでDBアクセスせず早期 return することを
// 検証する（Requirement 4.2）。
func TestUpsertItems_NilItems_NoDBAccess(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	svc := NewItemUpsertService(repo, sanitizer)

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", nil)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 0)", inserted, updated)
	}
	if repo.findBulkCalls != 0 || repo.upsertCalls != 0 {
		t.Errorf("nil 入力時は DB へアクセスしてはならない: findBulkCalls=%d upsertCalls=%d", repo.findBulkCalls, repo.upsertCalls)
	}
}

// TestUpsertItems_BatchInternalDuplicate_LastWins はバッチ内で同一性判定上同一とみなされる
// 記事が重複する場合、最終要素が勝つ（後勝ち）ことを検証する（オーケストレーター確定事項）。
func TestUpsertItems_BatchInternalDuplicate_LastWins(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	svc := NewItemUpsertService(repo, sanitizer)

	// 同一 guid を持つ新規記事を 3 件並べる（DB には未存在）。
	parsedItems := []model.ParsedItem{
		{GuidOrID: "dup-guid", Title: "1番目"},
		{GuidOrID: "dup-guid", Title: "2番目"},
		{GuidOrID: "dup-guid", Title: "最後"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	// 事前 dedup により 1 件の新規としてのみ書き込まれる。
	if inserted != 1 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (1, 0)", inserted, updated)
	}
	// 後勝ち: 最終要素のタイトルが採用されること。
	if repo.lastCreatedItem == nil {
		t.Fatal("lastCreatedItem should not be nil")
	}
	if repo.lastCreatedItem.Title != "最後" {
		t.Errorf("採用されたタイトル = %q, want %q（後勝ち）", repo.lastCreatedItem.Title, "最後")
	}
}

// TestUpsertItems_BatchInternalDuplicate_ExistingLastWins は同一 guid の重複が既存記事に
// 一致する場合も最終要素が勝つことを検証する（オーケストレーター確定事項 / Requirement 1.4）。
func TestUpsertItems_BatchInternalDuplicate_ExistingLastWins(t *testing.T) {
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}

	repo.addExistingItem(&model.Item{
		ID:       "dup-existing",
		FeedID:   "feed-1",
		GuidOrID: "dup-existing-guid",
		Title:    "既存",
	})

	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "dup-existing-guid", Title: "更新1"},
		{GuidOrID: "dup-existing-guid", Title: "更新最後"},
	}

	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}
	if inserted != 0 || updated != 1 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 1)", inserted, updated)
	}
	if repo.lastUpdatedItem.Title != "更新最後" {
		t.Errorf("採用されたタイトル = %q, want %q（後勝ち）", repo.lastUpdatedItem.Title, "更新最後")
	}
	if repo.lastUpdatedItem.ID != "dup-existing" {
		t.Errorf("更新対象 id = %q, want %q", repo.lastUpdatedItem.ID, "dup-existing")
	}
}

// --- Task 4.1: WithMetrics によるアップサート件数記録のテスト ---

// mockMetricsCollector は metrics.MetricsCollector のテスト用モック。
// RecordItemsUpserted の呼び出し回数と最後の件数を保持する。
type mockMetricsCollector struct {
	itemsUpsertedCalls int
	lastItemsUpserted  int
}

func (m *mockMetricsCollector) RecordFetchSuccess(_ string)        {}
func (m *mockMetricsCollector) RecordFetchFailure(_, _ string)     {}
func (m *mockMetricsCollector) RecordParseFailure(_ string)        {}
func (m *mockMetricsCollector) RecordHTTPStatus(_ int)             {}
func (m *mockMetricsCollector) RecordFetchLatency(_ time.Duration) {}
func (m *mockMetricsCollector) RecordItemsUpserted(count int) {
	m.itemsUpsertedCalls++
	m.lastItemsUpserted = count
}

// 手動フェッチ系（Issue #115）は upsert サービスから呼ばれないが、
// MetricsCollector interface 充足のため no-op 実装する。
func (m *mockMetricsCollector) RecordManualFetchSuccess()          {}
func (m *mockMetricsCollector) RecordManualFetchFailure(_ string)  {}
func (m *mockMetricsCollector) RecordManualFetchCooldownRejected() {}
func (m *mockMetricsCollector) RecordManualFetchLockConflict()     {}

// TestUpsertItems_Metrics_RecordsUpsertedCount は UPSERT 成功時に
// 新規 + 更新の件数が RecordItemsUpserted に加算されることを検証する（Requirement 2.6）。
func TestUpsertItems_Metrics_RecordsUpsertedCount(t *testing.T) {
	// Arrange: 既存 1 件（更新対象）+ 新規 1 件 → inserted=1, updated=1
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	repo.addExistingItem(&model.Item{
		ID:       "existing-1",
		FeedID:   "feed-1",
		GuidOrID: "guid-existing",
		Title:    "古い",
	})
	mc := &mockMetricsCollector{}
	svc := NewItemUpsertService(repo, sanitizer, WithMetrics(mc))

	parsedItems := []model.ParsedItem{
		{GuidOrID: "guid-existing", Title: "更新後", Link: "https://example.com/u"},
		{GuidOrID: "guid-new", Title: "新規", Link: "https://example.com/n"},
	}

	// Act
	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	// Assert
	if inserted != 1 || updated != 1 {
		t.Fatalf("(inserted, updated) = (%d, %d), want (1, 1)", inserted, updated)
	}
	if mc.itemsUpsertedCalls != 1 {
		t.Errorf("RecordItemsUpserted 呼び出し回数 = %d, want 1", mc.itemsUpsertedCalls)
	}
	if mc.lastItemsUpserted != inserted+updated {
		t.Errorf("記録された件数 = %d, want %d（inserted+updated）", mc.lastItemsUpserted, inserted+updated)
	}
}

// TestUpsertItems_Metrics_NotRecordedOnError は BulkUpsert がエラー（ロールバック）の場合に
// RecordItemsUpserted を呼ばないことを検証する（Requirement 2.6）。
func TestUpsertItems_Metrics_NotRecordedOnError(t *testing.T) {
	// Arrange
	repo := newMockItemRepo()
	repo.upsertErr = errors.New("bulk upsert failed")
	sanitizer := &mockSanitizer{}
	mc := &mockMetricsCollector{}
	svc := NewItemUpsertService(repo, sanitizer, WithMetrics(mc))

	parsedItems := []model.ParsedItem{
		{GuidOrID: "guid-1", Title: "記事", Link: "https://example.com/1"},
	}

	// Act
	_, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems)

	// Assert
	if err == nil {
		t.Fatal("BulkUpsert エラー時はエラーを返すべき")
	}
	if mc.itemsUpsertedCalls != 0 {
		t.Errorf("エラー時の RecordItemsUpserted 呼び出し回数 = %d, want 0", mc.itemsUpsertedCalls)
	}
}

// TestUpsertItems_Metrics_EmptyInputNotRecorded は空入力（件数 0）のとき
// 早期 return により RecordItemsUpserted を呼ばないことを検証する。
func TestUpsertItems_Metrics_EmptyInputNotRecorded(t *testing.T) {
	// Arrange
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	mc := &mockMetricsCollector{}
	svc := NewItemUpsertService(repo, sanitizer, WithMetrics(mc))

	// Act
	inserted, updated, err := svc.UpsertItems(context.Background(), "feed-1", nil)
	if err != nil {
		t.Fatalf("UpsertItems returned error: %v", err)
	}

	// Assert
	if inserted != 0 || updated != 0 {
		t.Errorf("(inserted, updated) = (%d, %d), want (0, 0)", inserted, updated)
	}
	if mc.itemsUpsertedCalls != 0 {
		t.Errorf("空入力時の RecordItemsUpserted 呼び出し回数 = %d, want 0", mc.itemsUpsertedCalls)
	}
}

// TestNewItemUpsertService_DefaultMetricsIsNopAndNilSafe は WithMetrics を指定しない
// 既存 2 引数 call site でも no-op コレクタが既定値となり nil panic しないことを検証する。
func TestNewItemUpsertService_DefaultMetricsIsNopAndNilSafe(t *testing.T) {
	// Arrange
	repo := newMockItemRepo()
	sanitizer := &mockSanitizer{}
	svc := NewItemUpsertService(repo, sanitizer)

	parsedItems := []model.ParsedItem{
		{GuidOrID: "guid-1", Title: "記事", Link: "https://example.com/1"},
	}

	// Act + Assert: option 未指定でも記録呼び出しで panic せず正常完了する
	if _, _, err := svc.UpsertItems(context.Background(), "feed-1", parsedItems); err != nil {
		t.Fatalf("option 未指定の UpsertItems がエラーを返した: %v", err)
	}
}
