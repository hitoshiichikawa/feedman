package cleanup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// fakeDB はsql.DBのExecContextをモックするための構造体。
// テストではPostgreSQLを使わず、SQLクエリの内容と引数を検証する。
type fakeResult struct {
	rowsAffected int64
}

func (r *fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r *fakeResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// Executor インターフェースに対するモック実装
type mockExecutor struct {
	execCalled bool
	query      string
	args       []interface{}
	result     sql.Result
	err        error
}

func (m *mockExecutor) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	m.execCalled = true
	m.query = query
	m.args = args
	return m.result, m.err
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// --- RED: テストを先に書く ---

func TestNewCleanupJob_ReturnsNonNil(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	if job == nil {
		t.Fatal("NewCleanupJob は nil を返してはならない")
	}
}

func TestNewCleanupJob_SetsRetentionDays(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	if job.RetentionDays != 180 {
		t.Errorf("RetentionDays = %d, want 180", job.RetentionDays)
	}
}

func TestCleanupJob_Run_ExecutesDeleteQuery(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 5},
	}
	job := NewCleanupJob(mock, logger)

	err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() がエラーを返した: %v", err)
	}

	if !mock.execCalled {
		t.Fatal("ExecContext が呼び出されなかった")
	}

	// SQLクエリにDELETE FROM itemsが含まれること
	if !strings.Contains(mock.query, "DELETE FROM items") {
		t.Errorf("クエリに 'DELETE FROM items' が含まれていない: %s", mock.query)
	}

	// SQLクエリにcreated_atの条件が含まれること
	if !strings.Contains(mock.query, "created_at") {
		t.Errorf("クエリに 'created_at' 条件が含まれていない: %s", mock.query)
	}
}

func TestCleanupJob_Run_UsesIntervalParameter(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	// 引数に180日のinterval文字列が渡されること
	if len(mock.args) < 1 {
		t.Fatal("ExecContext に引数が渡されなかった")
	}

	argStr, ok := mock.args[0].(string)
	if !ok {
		t.Fatalf("第1引数が string ではない: %T", mock.args[0])
	}
	if argStr != "180 days" {
		t.Errorf("interval引数 = %q, want %q", argStr, "180 days")
	}
}

func TestCleanupJob_Run_LogsDeletedCount(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 42},
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	// ログ出力に削除件数が含まれること
	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	found := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if count, ok := entry["deleted_count"]; ok {
			if count == float64(42) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("ログに deleted_count=42 が記録されていない。ログ出力: %s", buf.String())
	}
}

func TestCleanupJob_Run_LogsRetentionDays(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 10},
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	found := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if days, ok := entry["retention_days"]; ok {
			if days == float64(180) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("ログに retention_days=180 が記録されていない。ログ出力: %s", buf.String())
	}
}

func TestCleanupJob_Run_ReturnsErrorOnDBFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: nil,
		err:    sql.ErrConnDone,
	}
	job := NewCleanupJob(mock, logger)

	err := job.Run(context.Background())
	if err == nil {
		t.Fatal("DBエラー時に Run() は nil でないエラーを返すべき")
	}

	if !strings.Contains(err.Error(), "sql: connection is already closed") {
		t.Errorf("エラーメッセージが期待と異なる: %v", err)
	}
}

func TestCleanupJob_Run_LogsErrorOnDBFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: nil,
		err:    sql.ErrConnDone,
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	// エラーログが出力されていること
	logOutput := buf.String()
	if !strings.Contains(logOutput, "ERROR") {
		t.Errorf("エラー時にERRORレベルのログが記録されていない。ログ出力: %s", logOutput)
	}
}

func TestCleanupJob_Run_Idempotent_ZeroRows(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	// 1回目の実行
	err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("1回目の Run() がエラーを返した: %v", err)
	}

	// 2回目の実行（冪等性: 削除対象がなくてもエラーにならない）
	err = job.Run(context.Background())
	if err != nil {
		t.Fatalf("2回目の Run() がエラーを返した: %v", err)
	}
}

func TestCleanupJob_Run_RespectsContext(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	// キャンセル済みコンテキストでの実行はDBのExecContextに委ねる
	// モックでは常に成功するが、実際のDBではコンテキストエラーが返る
	_ = job.Run(ctx)

	// ExecContextが呼ばれたことを確認（コンテキストはDB層に伝播する）
	if !mock.execCalled {
		t.Fatal("キャンセル済みコンテキストでもExecContextは呼び出されるべき")
	}
}

func TestCleanupJob_Run_LogsZeroDeletedCount(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	// 0件削除でもログが出力されること
	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	found := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if count, ok := entry["deleted_count"]; ok {
			if count == float64(0) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("0件削除時にもログに deleted_count=0 が記録されるべき。ログ出力: %s", buf.String())
	}
}

func TestCleanupJob_Run_LogsExecutionTime(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 3},
	}
	job := NewCleanupJob(mock, logger)

	_ = job.Run(context.Background())

	// 処理時間がログに含まれること
	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	found := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if _, ok := entry["duration_ms"]; ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ログに duration_ms が記録されていない。ログ出力: %s", buf.String())
	}
}

// TestCleanupJob_CustomRetentionDays はRetentionDaysをカスタマイズした場合のテスト。
func TestCleanupJob_CustomRetentionDays(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	mock := &mockExecutor{
		result: &fakeResult{rowsAffected: 0},
	}
	job := NewCleanupJob(mock, logger)
	job.RetentionDays = 90 // カスタム保持日数

	_ = job.Run(context.Background())

	argStr, ok := mock.args[0].(string)
	if !ok {
		t.Fatalf("第1引数が string ではない: %T", mock.args[0])
	}
	if argStr != "90 days" {
		t.Errorf("interval引数 = %q, want %q", argStr, "90 days")
	}
}
