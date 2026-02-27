package item

import (
	"context"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- テスト用モック ---

// mockItemRepo はテスト用のItemRepositoryモック。
type mockItemRepo struct {
	items           map[string]*model.Item // id -> item
	byFeedGUID      map[string]*model.Item // feedID+guid -> item
	byFeedLink      map[string]*model.Item // feedID+link -> item
	byContentHash   map[string]*model.Item // feedID+hash -> item
	createCalls     int
	updateCalls     int
	lastCreatedItem *model.Item
	lastUpdatedItem *model.Item
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
			Link:     "https://example.com/target-link",  // linkでも別の記事に一致
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
			GuidOrID: "nonexistent-guid", // GUIDでは見つからない
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
