package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// TestQueryItemsByColumn_RejectsUnknownColumn は、allowlist 外のカラム名が渡された場合に
// DB へアクセスする前にエラーを返すことを検証する（#177 防御的 SQL インジェクション対策）。
// db は nil でよい（allowlist チェックは DB アクセス前に行われる）。
func TestQueryItemsByColumn_RejectsUnknownColumn(t *testing.T) {
	repo := &PostgresItemRepo{db: nil}
	dest := make(map[string]*model.Item)

	err := repo.queryItemsByColumn(
		context.Background(),
		"feed-1",
		"title; DROP TABLE items", // allowlist 外（攻撃的なカラム名）
		[]string{"v1"},
		dest,
		func(i *model.Item) string { return i.ID },
	)

	if err == nil {
		t.Fatal("allowlist 外のカラム名でエラーが返らなかった")
	}
	if !strings.Contains(err.Error(), "不正なカラム名") {
		t.Errorf("想定外のエラー: %v", err)
	}
}

// TestQueryItemsByColumn_AllowsKnownColumns は、同一性判定で使う 3 カラムが allowlist に
// 含まれることを検証する（実 IN 句クエリは DB 結合テストでカバーされるため、ここでは
// allowlist 自体の網羅を DB 非依存で確認する）。
func TestQueryItemsByColumn_AllowsKnownColumns(t *testing.T) {
	for _, col := range []string{"guid_or_id", "link", "content_hash"} {
		if !allowedItemQueryColumns[col] {
			t.Errorf("既知カラム %q が allowlist に含まれていない", col)
		}
	}
}
