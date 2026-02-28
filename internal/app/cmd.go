package app

// Command はアプリケーションの起動モードを表す。
type Command string

const (
	// CommandServe はAPIサーバーモードで起動することを示す。
	CommandServe Command = "serve"
	// CommandWorker はワーカーモードで起動することを示す。
	CommandWorker Command = "worker"
	// CommandMigrate はデータベースマイグレーションを実行することを示す。
	CommandMigrate Command = "migrate"
	// CommandHealthcheck はヘルスチェックを実行することを示す。
	// distroless環境でのDockerヘルスチェック用。
	CommandHealthcheck Command = "healthcheck"
)

// ParseCommand はコマンドライン引数からサブコマンドを解析する。
// 引数が空またはサポート外のコマンドの場合はCommandServeを返す。
func ParseCommand(args []string) Command {
	if len(args) == 0 {
		return CommandServe
	}

	switch args[0] {
	case "worker":
		return CommandWorker
	case "serve":
		return CommandServe
	case "migrate":
		return CommandMigrate
	case "healthcheck":
		return CommandHealthcheck
	default:
		return CommandServe
	}
}
