package logger

import (
	"io"
	"log/slog"
	"os"
)

// Setup はJSON構造化ログ出力のslog.Loggerを生成して返す。
// writerが指定された場合はそのwriterに出力する。
func Setup(w io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

// SetupDefault はJSON構造化ログ出力をグローバルロガーとして設定する。
// writerが指定された場合はそのwriterに出力する。
// 本番ではos.Stdoutを渡すことを想定している。
func SetupDefault(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	logger := Setup(w)
	slog.SetDefault(logger)
}
