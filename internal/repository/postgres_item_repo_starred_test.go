package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	_ "github.com/lib/pq"
)

// insertTestItem はテスト用記事を items テーブルに挿入し、生成された ID を返す。
// title / published_at を指定でき、スター記事一覧の並び順検証に利用する。
func insertTestItem(t *testing.T, db *sql.DB, feedID, title string, publishedAt time.Time) string {
	t.Helper()
	var itemID string
	err := db.QueryRow(
		`INSERT INTO items (feed_id, title, link, summary, published_at, fetched_at)
		 VALUES ($1, $2, $3, $4, $5, now()) RETURNING id`,
		feedID, title, "https://example.com/article/"+title, "サマリー: "+title, publishedAt,
	).Scan(&itemID)
	if err != nil {
		t.Fatalf("記事挿入に失敗: %v", err)
	}
	return itemID
}

// insertTestItemState はテスト用 item_states 行を挿入する。
// is_read / is_starred を指定でき、スター解除済み・既読既スター記事等の状態組合せを表現できる。
func insertTestItemState(t *testing.T, db *sql.DB, userID, itemID string, isRead, isStarred bool) {
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

// TestPostgresItemRepo_ListStarredByUser は ListStarredByUser が以下を満たすことを検証する。
//
//	(a) 自ユーザーのスター記事のみ返る
//	(b) 他ユーザーのスター記事が混入しない（NFR 2.1）
//	(c) published_at 降順
//	(d) 複数フィードにまたがる
//	(e) cursor 境界（指定時刻より前の記事のみ返る）
//	(f) スター 0 件で空スライス
//	(g) feed_title が正しく付与される（Requirement 2.4 / 4.10）
//
// テスト用 PostgreSQL に接続できない場合はスキップする。
func TestPostgresItemRepo_ListStarredByUser(t *testing.T) {
	ctx := context.Background()

	// 基準時刻（now より過去に置くことで cursor 境界・降順を制御する）
	base := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

	// (a) (c) (d) (g): 自ユーザーの複数フィードにまたがるスター記事が published_at 降順で返り、
	// 各行に feed_title が付与される。
	t.Run("自ユーザーの複数フィードのスター記事が降順で返り feed_title が付与される", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// Arrange: ユーザー 1 名と 2 フィード、それぞれに 1 件ずつスター記事を作成。
		user := insertTestUser(t, db, "starred-multi@example.com")
		feedA := insertTestFeedWithTitle(t, db, "https://example.com/feed-a.xml", "Feed A", "https://example.com/a", model.FetchStatusActive)
		feedB := insertTestFeedWithTitle(t, db, "https://example.com/feed-b.xml", "Feed B", "https://example.com/b", model.FetchStatusActive)

		// 公開日時を 3 段階に分けて並び順を検証可能にする。
		olderItem := insertTestItem(t, db, feedA, "old-article", base.Add(-2*time.Hour))
		middleItem := insertTestItem(t, db, feedB, "mid-article", base.Add(-1*time.Hour))
		newerItem := insertTestItem(t, db, feedA, "new-article", base)

		insertTestItemState(t, db, user, olderItem, false, true)
		insertTestItemState(t, db, user, middleItem, true, true) // is_read=true でもスターなら返る
		insertTestItemState(t, db, user, newerItem, false, true)

		// Act
		rows, err := repo.ListStarredByUser(ctx, user, time.Time{}, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert: 3 件返る
		if len(rows) != 3 {
			t.Fatalf("返却件数 = %d, want 3", len(rows))
		}
		// 降順: newer → middle → older
		wantOrder := []string{newerItem, middleItem, olderItem}
		for i, want := range wantOrder {
			if rows[i].ID != want {
				t.Errorf("rows[%d].ID = %q, want %q（published_at 降順違反）", i, rows[i].ID, want)
			}
		}
		// feed_title の付与: newer/older は Feed A、middle は Feed B
		wantTitles := []string{"Feed A", "Feed B", "Feed A"}
		for i, want := range wantTitles {
			if rows[i].FeedTitle != want {
				t.Errorf("rows[%d].FeedTitle = %q, want %q", i, rows[i].FeedTitle, want)
			}
		}
		// 全行が IsStarred=true
		for i, row := range rows {
			if !row.IsStarred {
				t.Errorf("rows[%d].IsStarred = false, want true（INNER JOIN 由来で常に true）", i)
			}
		}
		// is_read の取得: middle のみ既読
		if rows[0].IsRead || !rows[1].IsRead || rows[2].IsRead {
			t.Errorf("IsRead 値が不正: %v / %v / %v", rows[0].IsRead, rows[1].IsRead, rows[2].IsRead)
		}
	})

	// (b): 他ユーザーがスターした記事は一切返らない（NFR 2.1 / Requirement 4.9）。
	t.Run("他ユーザーのスター記事は返らない", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// Arrange: 2 ユーザー、両者が共通フィードの異なる記事にスター。
		userA := insertTestUser(t, db, "user-a@example.com")
		userB := insertTestUser(t, db, "user-b@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/shared.xml", "Shared Feed", "", model.FetchStatusActive)

		itemA := insertTestItem(t, db, feed, "article-a", base.Add(-1*time.Hour))
		itemB := insertTestItem(t, db, feed, "article-b", base)

		// userA は itemA のみスター、userB は itemB のみスター。
		insertTestItemState(t, db, userA, itemA, false, true)
		insertTestItemState(t, db, userB, itemB, false, true)

		// Act: userA の一覧を取得する。
		rows, err := repo.ListStarredByUser(ctx, userA, time.Time{}, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert: userA がスターした itemA のみ返り、itemB は混入しない。
		if len(rows) != 1 {
			t.Fatalf("userA の返却件数 = %d, want 1（他ユーザーのスター記事が混入してはならない）", len(rows))
		}
		if rows[0].ID != itemA {
			t.Errorf("rows[0].ID = %q, want %q", rows[0].ID, itemA)
		}
	})

	// (b) 拡張: スター解除済み記事（is_starred=false）は返らない。
	t.Run("スター解除済み記事は返らない", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// Arrange: スター付き 1 件 + スター解除済み 1 件。
		user := insertTestUser(t, db, "unstarred@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/mixed.xml", "Mixed Feed", "", model.FetchStatusActive)
		starred := insertTestItem(t, db, feed, "starred-article", base)
		unstarred := insertTestItem(t, db, feed, "unstarred-article", base.Add(-1*time.Hour))

		insertTestItemState(t, db, user, starred, false, true)
		insertTestItemState(t, db, user, unstarred, false, false) // 既読/スター無しの状態行

		// Act
		rows, err := repo.ListStarredByUser(ctx, user, time.Time{}, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert
		if len(rows) != 1 {
			t.Fatalf("返却件数 = %d, want 1（スター解除済みは除外されるべき）", len(rows))
		}
		if rows[0].ID != starred {
			t.Errorf("rows[0].ID = %q, want %q", rows[0].ID, starred)
		}
	})

	// (e): cursor 境界 — cursor 時刻より前 (i.published_at < cursor) の記事のみ返る。
	t.Run("cursor 指定時に当該時刻より前の記事のみ返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		user := insertTestUser(t, db, "cursor-boundary@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/cursor.xml", "Cursor Feed", "", model.FetchStatusActive)

		// 3 件: pubAtPast < pubAtMid < pubAtFuture
		pubAtPast := base.Add(-2 * time.Hour)
		pubAtMid := base.Add(-1 * time.Hour)
		pubAtFuture := base

		past := insertTestItem(t, db, feed, "past", pubAtPast)
		mid := insertTestItem(t, db, feed, "mid", pubAtMid)
		future := insertTestItem(t, db, feed, "future", pubAtFuture)
		insertTestItemState(t, db, user, past, false, true)
		insertTestItemState(t, db, user, mid, false, true)
		insertTestItemState(t, db, user, future, false, true)

		// Act: cursor = pubAtMid を指定（境界条件: i.published_at < pubAtMid のみ返る）
		rows, err := repo.ListStarredByUser(ctx, user, pubAtMid, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert: past のみ返る（mid 自身は < cursor を満たさないので除外）
		if len(rows) != 1 {
			t.Fatalf("cursor 適用時の件数 = %d, want 1", len(rows))
		}
		if rows[0].ID != past {
			t.Errorf("rows[0].ID = %q, want %q（past のみ返るべき）", rows[0].ID, past)
		}

		// 補足: cursor=zero では全件返ることを確認（境界の双方向確認）
		allRows, err := repo.ListStarredByUser(ctx, user, time.Time{}, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser (cursor=zero) returned error: %v", err)
		}
		if len(allRows) != 3 {
			t.Errorf("cursor=zero の件数 = %d, want 3", len(allRows))
		}
		// 念のため _ で参照することで future の使用とマッチする状況を明確化する。
		_ = future
	})

	// (f): スター 0 件で空スライス（Requirement 4.7 の repo 層側担保）。
	t.Run("スター記事が0件のとき空スライスが返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		// Arrange: ユーザーとフィードはあるが、当該ユーザーはスターを付与していない。
		user := insertTestUser(t, db, "no-star@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/no-star.xml", "No Star Feed", "", model.FetchStatusActive)
		item := insertTestItem(t, db, feed, "article", base)
		// スターを付与しない（item_states を挿入しない or is_starred=false を挿入する）。
		insertTestItemState(t, db, user, item, false, false)

		// Act
		rows, err := repo.ListStarredByUser(ctx, user, time.Time{}, 50)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert: 空スライス
		if len(rows) != 0 {
			t.Errorf("スター 0 件のときの返却件数 = %d, want 0", len(rows))
		}
	})

	// limit 境界: limit=2 で 3 件登録時に先頭 2 件のみ返る。
	t.Run("limit が指定件数で SQL レベルに反映される", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresItemRepo(db)

		user := insertTestUser(t, db, "limit@example.com")
		feed := insertTestFeedWithTitle(t, db, "https://example.com/limit.xml", "Limit Feed", "", model.FetchStatusActive)
		item1 := insertTestItem(t, db, feed, "limit-item1", base)
		item2 := insertTestItem(t, db, feed, "limit-item2", base.Add(-1*time.Hour))
		item3 := insertTestItem(t, db, feed, "limit-item3", base.Add(-2*time.Hour))
		insertTestItemState(t, db, user, item1, false, true)
		insertTestItemState(t, db, user, item2, false, true)
		insertTestItemState(t, db, user, item3, false, true)

		// Act: limit=2 を指定
		rows, err := repo.ListStarredByUser(ctx, user, time.Time{}, 2)
		if err != nil {
			t.Fatalf("ListStarredByUser returned error: %v", err)
		}

		// Assert: 先頭 2 件のみ返る（published_at 降順なので item1 / item2）
		if len(rows) != 2 {
			t.Errorf("limit=2 のときの返却件数 = %d, want 2", len(rows))
		}
		if len(rows) >= 2 {
			if rows[0].ID != item1 || rows[1].ID != item2 {
				t.Errorf("limit=2 の先頭2件 ID = [%q, %q], want [%q, %q]",
					rows[0].ID, rows[1].ID, item1, item2)
			}
		}
	})
}
