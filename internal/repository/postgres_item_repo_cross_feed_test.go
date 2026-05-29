package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	_ "github.com/lib/pq"
)

// このファイルは PostgresItemRepo.ListNewAcrossFeeds の DB 結合テスト。
// Issue #121 / Req 2.1, 2.2, 2.3, 4.2 / NFR 1.1 / NFR 1.2 の検証を担う。
//
// テスト用 PostgreSQL に接続できない場合は setupListDueTestDB 経由で
// 自動的にスキップされる（既存の DB integration test 群と統一）。
// ヘルパ名は他テスト群（postgres_item_repo_starred_test.go の insertStarredTestItem 等）と
// 衝突しないよう insertCrossFeedTestItem などの接頭辞付き命名を採用している。

// insertCrossFeedTestItem はテスト用記事を items テーブルに挿入し、生成された ID を返す。
// title / published_at を指定でき、横断新着一覧の並び順検証に利用する。
// published_at は NOT NULL ではないが、本テストでは並び順検証のため常に明示的に渡す。
func insertCrossFeedTestItem(t *testing.T, db *sql.DB, feedID, title string, publishedAt time.Time) string {
	t.Helper()
	var itemID string
	err := db.QueryRow(
		`INSERT INTO items (feed_id, title, link, summary, published_at, fetched_at)
		 VALUES ($1, $2, $3, $4, $5, now()) RETURNING id`,
		feedID, title, "https://example.com/cross-feed/"+title, "summary: "+title, publishedAt,
	).Scan(&itemID)
	if err != nil {
		t.Fatalf("記事挿入に失敗: %v", err)
	}
	return itemID
}

// insertCrossFeedTestItemState はテスト用 item_states 行を挿入する。
// LEFT JOIN 経路の検証のため、明示的に既読・スター状態を指定可能。
func insertCrossFeedTestItemState(t *testing.T, db *sql.DB, userID, itemID string, isRead, isStarred bool) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO item_states (user_id, item_id, is_read, is_starred)
		 VALUES ($1, $2, $3, $4)`,
		userID, itemID, isRead, isStarred,
	)
	if err != nil {
		t.Fatalf("item_states 挿入に失敗: %v", err)
	}
}

// updateCrossFeedFeedFavicon は feeds テーブルの favicon_data / favicon_mime を更新する。
// favicon 取得経路（Req 3.2）の検証のため、テストごとに任意の値を設定可能。
func updateCrossFeedFeedFavicon(t *testing.T, db *sql.DB, feedID string, data []byte, mime string) {
	t.Helper()
	_, err := db.Exec(
		`UPDATE feeds SET favicon_data = $2, favicon_mime = $3 WHERE id = $1`,
		feedID, data, mime,
	)
	if err != nil {
		t.Fatalf("favicon 更新に失敗: %v", err)
	}
}

// TestPostgresItemRepo_ListNewAcrossFeeds は ListNewAcrossFeeds が以下を満たすことを検証する。
//
//	(a) 2 フィード購読 + 6 記事の環境で、sinceTime 以後の記事のみが
//	    published_at DESC, id DESC で取得される（Req 2.1, 2.2, 4.2）
//	(b) 同一 published_at で id 降順タイブレーク（Req 2.3）
//	(c) cursor 指定時に複合キー境界 (published_at, id) < (cursor) で次ページが返る
//	    （Req 2.3 / 設計の cursor 仕様）
//	(d) 補足: feed_title / favicon / IsRead / IsStarred が正しく付与される（Req 3.1, 3.2）
//	(e) 補足: 他ユーザーが購読していないフィードの記事は混入しない（NFR 1.2 / Req 5.1）
//
// テスト用 PostgreSQL に接続できない場合はスキップする。
func TestPostgresItemRepo_ListNewAcrossFeeds(t *testing.T) {
	ctx := context.Background()

	// 基準時刻（now より過去に置くことで sinceTime 境界・降順を制御する）
	base := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

	// (a): 2 フィード購読 + 各 3 件（合計 6 件）で sinceTime 以後の記事のみが
	// published_at DESC, id DESC で取得される。
	t.Run("2 フィード購読 + 6 記事で sinceTime 以後のみ published_at 降順で返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// Arrange: 1 ユーザーが 2 フィードを購読し、各フィードに 3 記事ずつ存在。
		user := insertTestUser(t, db, "cross-feed-basic@example.com")
		feedA := insertTestFeedWithTitle(t, db, "https://example.com/feed-a.xml", "Feed A", "https://example.com/a", model.FetchStatusActive)
		feedB := insertTestFeedWithTitle(t, db, "https://example.com/feed-b.xml", "Feed B", "https://example.com/b", model.FetchStatusActive)
		insertTestSubscription(t, db, user, feedA)
		insertTestSubscription(t, db, user, feedB)

		// sinceTime をこの時刻に設定する（後述の境界）。
		sinceTime := base

		// sinceTime 以前の記事（除外されるべき 3 件）
		_ = insertCrossFeedTestItem(t, db, feedA, "old-a-1", base.Add(-3*time.Hour))
		_ = insertCrossFeedTestItem(t, db, feedB, "old-b-1", base.Add(-2*time.Hour))
		_ = insertCrossFeedTestItem(t, db, feedA, "old-a-2", base.Add(-1*time.Hour))
		// sinceTime ちょうど（境界、> なので除外されるべき）
		_ = insertCrossFeedTestItem(t, db, feedA, "boundary-a", base)
		// sinceTime 以後の記事（含まれるべき 3 件、published_at 昇順に作成）
		newer1 := insertCrossFeedTestItem(t, db, feedA, "new-a-1", base.Add(1*time.Hour))
		newer2 := insertCrossFeedTestItem(t, db, feedB, "new-b-1", base.Add(2*time.Hour))
		newer3 := insertCrossFeedTestItem(t, db, feedA, "new-a-2", base.Add(3*time.Hour))

		// Act: cursor なしで先頭から取得（limit は余裕を持って 50）
		rows, err := repo.ListNewAcrossFeeds(ctx, user, sinceTime, time.Time{}, "", 50)
		if err != nil {
			t.Fatalf("ListNewAcrossFeeds returned error: %v", err)
		}

		// Assert: sinceTime より厳密に大きい 3 件のみ返る（境界 boundary-a は除外）
		if len(rows) != 3 {
			t.Fatalf("返却件数 = %d, want 3（sinceTime ちょうどは除外、より新しい 3 件のみ）", len(rows))
		}
		// published_at DESC 順: newer3 → newer2 → newer1
		wantOrder := []string{newer3, newer2, newer1}
		for i, want := range wantOrder {
			if rows[i].ID != want {
				t.Errorf("rows[%d].ID = %q, want %q（published_at 降順違反）", i, rows[i].ID, want)
			}
		}

		// (d): feed_title が正しく付与される
		wantTitles := []string{"Feed A", "Feed B", "Feed A"}
		for i, want := range wantTitles {
			if rows[i].FeedTitle != want {
				t.Errorf("rows[%d].FeedTitle = %q, want %q", i, rows[i].FeedTitle, want)
			}
		}

		// (d): item_states 未挿入のため IsRead / IsStarred はいずれも false（LEFT JOIN COALESCE）
		for i, row := range rows {
			if row.IsRead || row.IsStarred {
				t.Errorf("rows[%d].IsRead/IsStarred = %v/%v, want false/false（item_states 未挿入時）",
					i, row.IsRead, row.IsStarred)
			}
		}
	})

	// (b): 同一 published_at で id 降順タイブレーク（Req 2.3）
	t.Run("同一 published_at で id 降順タイブレークが効く", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		user := insertTestUser(t, db, "cross-feed-tiebreak@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/tiebreak.xml", "Tiebreak Feed", "", model.FetchStatusActive)
		insertTestSubscription(t, db, user, feed)

		// 同一 published_at で 3 件挿入する。
		// UUID は ランダム生成のため、挿入順 ≠ id 順となるケースを許容する。
		samePub := base.Add(1 * time.Hour)
		id1 := insertCrossFeedTestItem(t, db, feed, "tie-1", samePub)
		id2 := insertCrossFeedTestItem(t, db, feed, "tie-2", samePub)
		id3 := insertCrossFeedTestItem(t, db, feed, "tie-3", samePub)

		// Act
		rows, err := repo.ListNewAcrossFeeds(ctx, user, base, time.Time{}, "", 50)
		if err != nil {
			t.Fatalf("ListNewAcrossFeeds returned error: %v", err)
		}

		// Assert: 3 件返り、id 降順で並ぶ
		if len(rows) != 3 {
			t.Fatalf("返却件数 = %d, want 3", len(rows))
		}

		// id 降順を期待する: 3 件の id を降順ソートした順序と一致するはず
		ids := []string{id1, id2, id3}
		wantIDOrder := sortDescending(ids)
		for i, want := range wantIDOrder {
			if rows[i].ID != want {
				t.Errorf("rows[%d].ID = %q, want %q（id 降順タイブレーク違反）", i, rows[i].ID, want)
			}
		}

		// すべて same published_at であることも確認
		for i, row := range rows {
			if row.PublishedAt == nil || !row.PublishedAt.Equal(samePub) {
				t.Errorf("rows[%d].PublishedAt = %v, want %v", i, row.PublishedAt, samePub)
			}
		}
	})

	// (c): cursor 指定時に複合キー境界で次ページが返る（Req 2.3 / cursor 仕様）
	t.Run("cursor 指定時に複合キー境界で次ページが返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		user := insertTestUser(t, db, "cross-feed-cursor@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/cursor.xml", "Cursor Feed", "", model.FetchStatusActive)
		insertTestSubscription(t, db, user, feed)

		// 4 件: pub1 < pub2 = pub2' < pub3
		pub1 := base.Add(1 * time.Hour)
		pub2 := base.Add(2 * time.Hour)
		pub3 := base.Add(3 * time.Hour)

		id1 := insertCrossFeedTestItem(t, db, feed, "c-1", pub1)
		// 同一 published_at の 2 件は id 降順タイブレークを利用
		id2a := insertCrossFeedTestItem(t, db, feed, "c-2a", pub2)
		id2b := insertCrossFeedTestItem(t, db, feed, "c-2b", pub2)
		id3 := insertCrossFeedTestItem(t, db, feed, "c-3", pub3)

		// 1 ページ目: limit=2 で先頭 2 件を取得
		firstPage, err := repo.ListNewAcrossFeeds(ctx, user, base, time.Time{}, "", 2)
		if err != nil {
			t.Fatalf("first page error: %v", err)
		}
		if len(firstPage) != 2 {
			t.Fatalf("first page 件数 = %d, want 2", len(firstPage))
		}
		// 期待: id3（pub3） → 同一 pub2 内で id 降順の先頭
		if firstPage[0].ID != id3 {
			t.Errorf("firstPage[0].ID = %q, want %q（id3 が降順先頭）", firstPage[0].ID, id3)
		}

		// 1 ページ目の末尾を cursor として 2 ページ目を取得
		cursorPub := *firstPage[1].PublishedAt
		cursorID := firstPage[1].ID
		secondPage, err := repo.ListNewAcrossFeeds(ctx, user, base, cursorPub, cursorID, 50)
		if err != nil {
			t.Fatalf("second page error: %v", err)
		}

		// 2 ページ目: cursor 境界 (published_at, id) < (cursorPub, cursorID) を満たす残り 2 件
		// firstPage[1] が id2a または id2b のどちらか（id 降順依存）。残りはもう片方 + id1。
		// いずれにせよ 2 件返り、境界自身は含まれない。
		if len(secondPage) != 2 {
			t.Fatalf("second page 件数 = %d, want 2", len(secondPage))
		}
		// 末尾は最古の id1（pub1）
		if secondPage[1].ID != id1 {
			t.Errorf("secondPage[1].ID = %q, want %q（最古の id1 が末尾）", secondPage[1].ID, id1)
		}

		// 境界自身（firstPage[1]）が secondPage に含まれていないこと
		boundaryID := firstPage[1].ID
		for i, row := range secondPage {
			if row.ID == boundaryID {
				t.Errorf("secondPage[%d].ID = %q が境界 cursor 自身と一致（重複してはならない）", i, row.ID)
			}
		}

		// 全ページ合算で重複なく 4 件揃うことを確認
		seen := map[string]bool{}
		for _, r := range firstPage {
			seen[r.ID] = true
		}
		for _, r := range secondPage {
			seen[r.ID] = true
		}
		for _, want := range []string{id1, id2a, id2b, id3} {
			if !seen[want] {
				t.Errorf("ID %q が両ページに現れない（cursor で欠落している）", want)
			}
		}
	})

	// (e): 他ユーザーの購読フィードに属する記事は混入しない（NFR 1.2 / Req 5.1 の repo 層側担保）
	t.Run("他ユーザーが購読していないフィードの記事は混入しない", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// userA は feedA のみ購読、userB は feedB のみ購読
		userA := insertTestUser(t, db, "cross-feed-iso-a@example.com")
		userB := insertTestUser(t, db, "cross-feed-iso-b@example.com")
		feedA := insertTestFeedWithTitle(t, db, "https://example.com/iso-a.xml", "Iso A", "", model.FetchStatusActive)
		feedB := insertTestFeedWithTitle(t, db, "https://example.com/iso-b.xml", "Iso B", "", model.FetchStatusActive)
		insertTestSubscription(t, db, userA, feedA)
		insertTestSubscription(t, db, userB, feedB)

		itemA := insertCrossFeedTestItem(t, db, feedA, "iso-a-article", base.Add(1*time.Hour))
		_ = insertCrossFeedTestItem(t, db, feedB, "iso-b-article", base.Add(2*time.Hour))

		// Act: userA で取得
		rows, err := repo.ListNewAcrossFeeds(ctx, userA, base, time.Time{}, "", 50)
		if err != nil {
			t.Fatalf("ListNewAcrossFeeds returned error: %v", err)
		}

		// Assert: userA が購読する feedA の記事のみ返る
		if len(rows) != 1 {
			t.Fatalf("userA の返却件数 = %d, want 1（他ユーザーの購読フィードは混入してはならない）", len(rows))
		}
		if rows[0].ID != itemA {
			t.Errorf("rows[0].ID = %q, want %q", rows[0].ID, itemA)
		}
	})

	// (d) 追加: item_states 既読・スター状態と favicon が正しく付与される
	t.Run("既読 スター状態と favicon が正しく付与される", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		user := insertTestUser(t, db, "cross-feed-state@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/state.xml", "Stateful Feed", "", model.FetchStatusActive)
		insertTestSubscription(t, db, user, feed)

		// favicon を設定
		faviconBytes := []byte{0x89, 0x50, 0x4E, 0x47} // PNG マジックバイト断片
		updateCrossFeedFeedFavicon(t, db, feed, faviconBytes, "image/png")

		readItem := insertCrossFeedTestItem(t, db, feed, "read", base.Add(1*time.Hour))
		starredItem := insertCrossFeedTestItem(t, db, feed, "starred", base.Add(2*time.Hour))
		untouchedItem := insertCrossFeedTestItem(t, db, feed, "untouched", base.Add(3*time.Hour))

		insertCrossFeedTestItemState(t, db, user, readItem, true, false)
		insertCrossFeedTestItemState(t, db, user, starredItem, false, true)
		// untouchedItem は item_states 未挿入（LEFT JOIN 経路）

		// Act
		rows, err := repo.ListNewAcrossFeeds(ctx, user, base, time.Time{}, "", 50)
		if err != nil {
			t.Fatalf("ListNewAcrossFeeds returned error: %v", err)
		}

		if len(rows) != 3 {
			t.Fatalf("返却件数 = %d, want 3", len(rows))
		}

		// published_at DESC: untouched → starred → read
		// untouched
		if rows[0].ID != untouchedItem {
			t.Errorf("rows[0].ID = %q, want untouched %q", rows[0].ID, untouchedItem)
		}
		if rows[0].IsRead || rows[0].IsStarred {
			t.Errorf("rows[0] untouched: IsRead/IsStarred = %v/%v, want false/false", rows[0].IsRead, rows[0].IsStarred)
		}
		// starred
		if rows[1].ID != starredItem {
			t.Errorf("rows[1].ID = %q, want starred %q", rows[1].ID, starredItem)
		}
		if rows[1].IsRead || !rows[1].IsStarred {
			t.Errorf("rows[1] starred: IsRead/IsStarred = %v/%v, want false/true", rows[1].IsRead, rows[1].IsStarred)
		}
		// read
		if rows[2].ID != readItem {
			t.Errorf("rows[2].ID = %q, want read %q", rows[2].ID, readItem)
		}
		if !rows[2].IsRead || rows[2].IsStarred {
			t.Errorf("rows[2] read: IsRead/IsStarred = %v/%v, want true/false", rows[2].IsRead, rows[2].IsStarred)
		}

		// favicon が全行に伝播される
		for i, row := range rows {
			if row.FaviconMime != "image/png" {
				t.Errorf("rows[%d].FaviconMime = %q, want image/png", i, row.FaviconMime)
			}
			if len(row.FaviconData) != len(faviconBytes) {
				t.Errorf("rows[%d].FaviconData length = %d, want %d", i, len(row.FaviconData), len(faviconBytes))
			}
		}
	})
}

// sortDescending は文字列スライスのコピーを降順ソートして返す。
// テスト内でランダム UUID の id 降順タイブレーク期待値を組み立てるために用いる。
func sortDescending(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	// 単純な O(n^2) 選択ソート（テスト用、n は小さい）。
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] > out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
