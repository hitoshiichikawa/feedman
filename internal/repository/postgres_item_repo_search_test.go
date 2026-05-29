package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/database"
)

// このファイルは PostgresItemRepo.SearchByUserAndKeyword の DB 結合テスト。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。
// この Skip ガードの慣習は postgres_subscription_repo_db_test.go と統一している。

// searchTestDatabaseURL はテスト用のデータベース URL を返す。
func searchTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupItemSearchTestDB はテスト用データベースを準備し、マイグレーション適用済みの
// *sql.DB を返す。DB へ接続できない場合は t.Skip でテストをスキップする。
func setupItemSearchTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := searchTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	cleanupSQL := `
		DROP TABLE IF EXISTS user_cross_feed_views CASCADE;
		DROP TABLE IF EXISTS sessions CASCADE;
		DROP TABLE IF EXISTS user_settings CASCADE;
		DROP TABLE IF EXISTS item_states CASCADE;
		DROP TABLE IF EXISTS subscriptions CASCADE;
		DROP TABLE IF EXISTS items CASCADE;
		DROP TABLE IF EXISTS feeds CASCADE;
		DROP TABLE IF EXISTS identities CASCADE;
		DROP TABLE IF EXISTS users CASCADE;
		DROP TABLE IF EXISTS schema_migrations CASCADE;
	`
	if _, err := db.Exec(cleanupSQL); err != nil {
		db.Close()
		t.Fatalf("クリーンアップに失敗: %v", err)
	}

	if err := database.RunMigrations(dbURL); err != nil {
		db.Close()
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	return db
}

// insertTestUserForSearch はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForSearch(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// insertTestFeedForSearch はテスト用フィードを作成し、その ID を返す。
func insertTestFeedForSearch(t *testing.T, db *sql.DB, feedURL, title string) string {
	t.Helper()
	var feedID string
	err := db.QueryRow(
		`INSERT INTO feeds (feed_url, title) VALUES ($1, $2) RETURNING id`,
		feedURL, title,
	).Scan(&feedID)
	if err != nil {
		t.Fatalf("フィード挿入に失敗: %v", err)
	}
	return feedID
}

// insertTestSubscriptionForSearch はテスト用購読を作成する。
func insertTestSubscriptionForSearch(t *testing.T, db *sql.DB, userID, feedID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`,
		userID, feedID,
	)
	if err != nil {
		t.Fatalf("購読挿入に失敗: %v", err)
	}
}

// insertTestItem はテスト用記事を作成し、その ID を返す。
// title / content / publishedAt はテストごとに任意に指定可能。
// publishedAt にゼロ値を渡した場合は NULL として挿入する。
func insertTestItem(t *testing.T, db *sql.DB, feedID, title, content string, publishedAt time.Time) string {
	t.Helper()
	var itemID string
	var publishedAtArg interface{}
	if !publishedAt.IsZero() {
		publishedAtArg = publishedAt
	} else {
		publishedAtArg = nil
	}
	err := db.QueryRow(
		`INSERT INTO items (feed_id, title, content, published_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		feedID, title, content, publishedAtArg,
	).Scan(&itemID)
	if err != nil {
		t.Fatalf("記事挿入に失敗: %v", err)
	}
	return itemID
}

// insertTestItemState はテスト用記事状態を作成する。
func insertTestItemState(t *testing.T, db *sql.DB, userID, itemID string, isRead, isStarred bool) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO item_states (user_id, item_id, is_read, is_starred)
		 VALUES ($1, $2, $3, $4)`,
		userID, itemID, isRead, isStarred,
	)
	if err != nil {
		t.Fatalf("記事状態挿入に失敗: %v", err)
	}
}

// TestSearchByUserAndKeyword は PostgresItemRepo.SearchByUserAndKeyword の
// AC（Req 2.1, 2.2, 2.3, 2.4, 2.6, 3.1, 3.2, 3.4, 4.1, 5.3）を網羅する DB 結合テスト。
func TestSearchByUserAndKeyword(t *testing.T) {
	db := setupItemSearchTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresItemRepo(db)

	// Req 2.3: title がキーワードに部分一致する記事が返る
	t.Run("タイトル一致のとき記事が返る", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "title-match@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/title-match.xml", "Feed A")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		itemID := insertTestItem(t, db, feedID, "Golang Tutorial", "Some unrelated content", now)

		pattern := "%Golang%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 1 {
			t.Fatalf("検索ヒット件数が不正: got %d, want 1", len(hits))
		}
		if hits[0].ID != itemID {
			t.Errorf("ヒットした記事 ID が不正: got %q, want %q", hits[0].ID, itemID)
		}
		if hits[0].FeedTitle != "Feed A" {
			t.Errorf("FeedTitle が不正: got %q, want %q", hits[0].FeedTitle, "Feed A")
		}
	})

	// Req 2.3: content がキーワードに部分一致する記事が返る
	t.Run("本文一致のとき記事が返る", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "content-match@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/content-match.xml", "Feed B")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		itemID := insertTestItem(t, db, feedID, "Some unrelated title", "Body talks about Rust language", now)

		pattern := "%Rust%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 1 {
			t.Fatalf("検索ヒット件数が不正: got %d, want 1", len(hits))
		}
		if hits[0].ID != itemID {
			t.Errorf("ヒットした記事 ID が不正: got %q, want %q", hits[0].ID, itemID)
		}
	})

	// Req 2.3: title と content の両方が一致しても重複せず 1 件のみ返る
	t.Run("タイトルと本文の両方が一致するとき1件のみ返る", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "both-match@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/both-match.xml", "Feed C")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		itemID := insertTestItem(t, db, feedID, "Kotlin overview", "Kotlin is JVM language", now)

		pattern := "%Kotlin%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 1 {
			t.Fatalf("検索ヒット件数が不正（重複している可能性）: got %d, want 1", len(hits))
		}
		if hits[0].ID != itemID {
			t.Errorf("ヒットした記事 ID が不正: got %q, want %q", hits[0].ID, itemID)
		}
	})

	// Req 2.3: title / content のいずれにも一致しない記事は返らない
	t.Run("タイトルにも本文にも一致しないとき記事は返らない", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "no-match@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/no-match.xml", "Feed D")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		insertTestItem(t, db, feedID, "Title talks about cats", "Content talks about dogs", now)

		pattern := "%elephants%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 0 {
			t.Fatalf("不一致時にも記事が返却された: got %d, want 0", len(hits))
		}
	})

	// Req 2.6: ILIKE による大文字小文字区別なしの部分一致
	t.Run("大文字小文字が異なるとき部分一致する", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "case-insensitive@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/case.xml", "Feed E")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		// 記事は小文字、検索は大文字
		itemID := insertTestItem(t, db, feedID, "introduction to typescript", "details here", now)

		pattern := "%TYPESCRIPT%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 1 {
			t.Fatalf("ILIKE 大文字小文字区別なし部分一致がヒットしない: got %d, want 1", len(hits))
		}
		if hits[0].ID != itemID {
			t.Errorf("ヒットした記事 ID が不正: got %q, want %q", hits[0].ID, itemID)
		}
	})

	// Req 3.1 / 3.2: 他ユーザーの購読記事は返らない（クロスユーザー隔離）
	t.Run("他ユーザーの購読記事は返らない", func(t *testing.T) {
		userA := insertTestUserForSearch(t, db, "user-a@test.com")
		userB := insertTestUserForSearch(t, db, "user-b@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/cross-user.xml", "Feed F")
		// user A のみが購読
		insertTestSubscriptionForSearch(t, db, userA, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		insertTestItem(t, db, feedID, "Python tips", "About Python", now)

		pattern := "%Python%"
		// user B（購読していない）で検索 → 0 件
		hits, err := repo.SearchByUserAndKeyword(ctx, userB, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 0 {
			t.Fatalf("他ユーザーが購読中のフィードの記事が返却された: got %d, want 0", len(hits))
		}
	})

	// Req 3.4: 購読解除済みフィードの記事は返らない
	t.Run("購読解除済みフィードの記事は返らない", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "unsubscribed@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/unsubscribed.xml", "Feed G")
		// subscriptions レコードを作らない（あるいは作ってすぐ削除する）→ ここでは作らない
		// = 「過去に購読していたが現在は購読していない」のと同等の状態
		now := time.Now().UTC().Truncate(time.Second)
		insertTestItem(t, db, feedID, "Ruby on Rails", "Web framework", now)

		pattern := "%Rails%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 0 {
			t.Fatalf("購読していないフィードの記事が返却された: got %d, want 0", len(hits))
		}
	})

	// Req 2.4: 同 published_at の記事は id DESC で安定したソート順
	t.Run("同publishedAtのとき id DESC で安定したソート順を返す", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "stable-sort@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/stable-sort.xml", "Feed H")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		samePublishedAt := time.Now().UTC().Truncate(time.Second)
		itemID1 := insertTestItem(t, db, feedID, "Stable sort post 1", "matchme content", samePublishedAt)
		itemID2 := insertTestItem(t, db, feedID, "Stable sort post 2", "matchme content", samePublishedAt)

		pattern := "%matchme%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 2 {
			t.Fatalf("検索ヒット件数が不正: got %d, want 2", len(hits))
		}

		// ORDER BY published_at DESC, id DESC のため、id が大きい方が先
		var expectedFirst, expectedSecond string
		if itemID1 > itemID2 {
			expectedFirst, expectedSecond = itemID1, itemID2
		} else {
			expectedFirst, expectedSecond = itemID2, itemID1
		}
		if hits[0].ID != expectedFirst {
			t.Errorf("ソート順が不正 (1番目): got %q, want %q", hits[0].ID, expectedFirst)
		}
		if hits[1].ID != expectedSecond {
			t.Errorf("ソート順が不正 (2番目): got %q, want %q", hits[1].ID, expectedSecond)
		}
	})

	// Req 2.2: feedID 指定時は当該フィードの記事のみ返る（他購読フィードは混入しない）
	t.Run("feedID指定時は当該フィードの記事のみ返る", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "feed-scope@test.com")
		feedIDA := insertTestFeedForSearch(t, db, "https://example.com/scope-a.xml", "Feed I-A")
		feedIDB := insertTestFeedForSearch(t, db, "https://example.com/scope-b.xml", "Feed I-B")
		insertTestSubscriptionForSearch(t, db, userID, feedIDA)
		insertTestSubscriptionForSearch(t, db, userID, feedIDB)

		now := time.Now().UTC().Truncate(time.Second)
		itemA := insertTestItem(t, db, feedIDA, "Java basics", "Java content", now)
		insertTestItem(t, db, feedIDB, "Java advanced", "Java content B", now)

		pattern := "%Java%"
		// feed A のみに絞り込み
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, &feedIDA, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}

		if len(hits) != 1 {
			t.Fatalf("フィード内検索の件数が不正（他フィードが混入）: got %d, want 1", len(hits))
		}
		if hits[0].ID != itemA {
			t.Errorf("フィード内検索でヒットした記事 ID が不正: got %q, want %q", hits[0].ID, itemA)
		}
		if hits[0].FeedID != feedIDA {
			t.Errorf("フィード内検索でヒットした記事の FeedID が不正: got %q, want %q", hits[0].FeedID, feedIDA)
		}
	})

	// Req 5.3: 検索実行前後で item_states / items が変化しないことを検証する
	t.Run("検索実行前後で item_states と items が変化しない", func(t *testing.T) {
		userID := insertTestUserForSearch(t, db, "invariant@test.com")
		feedID := insertTestFeedForSearch(t, db, "https://example.com/invariant.xml", "Feed J")
		insertTestSubscriptionForSearch(t, db, userID, feedID)

		now := time.Now().UTC().Truncate(time.Second)
		itemID := insertTestItem(t, db, feedID, "Invariant check title", "matchme body", now)
		// item_states を明示的に挿入
		insertTestItemState(t, db, userID, itemID, true, true)

		// 検索前の state スナップショット
		var beforeIsRead, beforeIsStarred bool
		var beforeUpdatedAt time.Time
		err := db.QueryRow(
			`SELECT is_read, is_starred, updated_at FROM item_states WHERE user_id = $1 AND item_id = $2`,
			userID, itemID,
		).Scan(&beforeIsRead, &beforeIsStarred, &beforeUpdatedAt)
		if err != nil {
			t.Fatalf("検索前の item_states 取得に失敗: %v", err)
		}

		// 検索前の items スナップショット
		var beforeItemUpdatedAt time.Time
		var beforeHatebuCount int
		err = db.QueryRow(
			`SELECT updated_at, hatebu_count FROM items WHERE id = $1`,
			itemID,
		).Scan(&beforeItemUpdatedAt, &beforeHatebuCount)
		if err != nil {
			t.Fatalf("検索前の items 取得に失敗: %v", err)
		}

		pattern := "%matchme%"
		hits, err := repo.SearchByUserAndKeyword(ctx, userID, pattern, nil, "", time.Time{}, 100)
		if err != nil {
			t.Fatalf("SearchByUserAndKeyword がエラーを返した: %v", err)
		}
		if len(hits) != 1 {
			t.Fatalf("検索ヒット件数が不正: got %d, want 1", len(hits))
		}
		// 検索結果は state を反映する（既読 / スターが true）
		if !hits[0].IsRead {
			t.Errorf("検索結果 IsRead が状態を反映していない: got %v, want true", hits[0].IsRead)
		}
		if !hits[0].IsStarred {
			t.Errorf("検索結果 IsStarred が状態を反映していない: got %v, want true", hits[0].IsStarred)
		}

		// 検索後の state スナップショット
		var afterIsRead, afterIsStarred bool
		var afterUpdatedAt time.Time
		err = db.QueryRow(
			`SELECT is_read, is_starred, updated_at FROM item_states WHERE user_id = $1 AND item_id = $2`,
			userID, itemID,
		).Scan(&afterIsRead, &afterIsStarred, &afterUpdatedAt)
		if err != nil {
			t.Fatalf("検索後の item_states 取得に失敗: %v", err)
		}

		if afterIsRead != beforeIsRead {
			t.Errorf("検索により is_read が変化した: before=%v, after=%v", beforeIsRead, afterIsRead)
		}
		if afterIsStarred != beforeIsStarred {
			t.Errorf("検索により is_starred が変化した: before=%v, after=%v", beforeIsStarred, afterIsStarred)
		}
		if !afterUpdatedAt.Equal(beforeUpdatedAt) {
			t.Errorf("検索により item_states.updated_at が変化した: before=%v, after=%v", beforeUpdatedAt, afterUpdatedAt)
		}

		// 検索後の items スナップショット
		var afterItemUpdatedAt time.Time
		var afterHatebuCount int
		err = db.QueryRow(
			`SELECT updated_at, hatebu_count FROM items WHERE id = $1`,
			itemID,
		).Scan(&afterItemUpdatedAt, &afterHatebuCount)
		if err != nil {
			t.Fatalf("検索後の items 取得に失敗: %v", err)
		}
		if !afterItemUpdatedAt.Equal(beforeItemUpdatedAt) {
			t.Errorf("検索により items.updated_at が変化した: before=%v, after=%v", beforeItemUpdatedAt, afterItemUpdatedAt)
		}
		if afterHatebuCount != beforeHatebuCount {
			t.Errorf("検索により items.hatebu_count が変化した: before=%v, after=%v", beforeHatebuCount, afterHatebuCount)
		}
	})
}
