package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// envLogLevel は出力ログレベルを指定する環境変数名。
const envLogLevel = "LOG_LEVEL"

// defaultLevel は LOG_LEVEL が未設定・空文字・不正値のときに採用するデフォルトレベル。
// 本変更導入前のハードコード値（INFO）と等価であり、後方互換を維持する。
const defaultLevel = slog.LevelInfo

// invalidLevelWarnMsg は不正な LOG_LEVEL が指定されデフォルトへフォールバックした際に
// 出力する警告ログのメッセージ。config パッケージの環境変数パース失敗時の文言と揃える。
const invalidLevelWarnMsg = "環境変数のパースに失敗したためデフォルト値を採用します"

// resolveLevel は環境変数 LOG_LEVEL を読み取り、対応する slog.Level を返す。
// 未設定・空文字の場合は defaultLevel（INFO）を返し、後方互換を維持する。
// 許容値（DEBUG / INFO / WARN / ERROR）以外が指定された場合は defaultLevel を返し、
// invalid に true をセットする（呼び出し側で警告ログを出力させるため）。
// 値の解釈は大文字小文字を区別しない。
func resolveLevel(raw string) (level slog.Level, invalid bool) {
	if raw == "" {
		return defaultLevel, false
	}
	switch strings.ToUpper(raw) {
	case "DEBUG":
		return slog.LevelDebug, false
	case "INFO":
		return slog.LevelInfo, false
	case "WARN":
		return slog.LevelWarn, false
	case "ERROR":
		return slog.LevelError, false
	default:
		return defaultLevel, true
	}
}

// Setup はJSON構造化ログ出力のslog.Loggerを生成して返す。
// 出力レベルは起動時に環境変数 LOG_LEVEL から1回だけ決定する
// （未設定・空文字・不正値の場合は INFO へフォールバックする）。
// 不正値が指定された場合は INFO で起動を継続しつつ、フォールバックを示す警告ログを
// 同じ writer へ出力する（サイレント失敗を回避する）。
// writerが指定された場合はそのwriterに出力する。
func Setup(w io.Writer) *slog.Logger {
	level, invalid := resolveLevel(os.Getenv(envLogLevel))

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)

	if invalid {
		// フォールバックは INFO のため WARN レベルの本ログは必ず出力される。
		logger.Warn(invalidLevelWarnMsg,
			slog.String("key", envLogLevel),
			slog.String("value", os.Getenv(envLogLevel)),
			slog.String("default", defaultLevel.String()),
		)
	}

	return logger
}

// SetupDefault はJSON構造化ログ出力をグローバルロガーとして設定する。
// 出力レベルは Setup と同様に環境変数 LOG_LEVEL から決定する。
// writerが指定された場合はそのwriterに出力する。
// 本番ではos.Stdoutを渡すことを想定している。
func SetupDefault(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	logger := Setup(w)
	slog.SetDefault(logger)
}
