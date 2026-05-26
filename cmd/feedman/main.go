// Command feedman は Feedman バックエンド（api / worker）のエントリポイントである。
//
// 起動ロジックそのものは internal/app に実装されており、本パッケージは
// os.Args を既存の app.Run へ委譲し、戻り値の error を stderr 出力と
// プロセス終了コードに変換するだけの薄いラッパーである。サブコマンド
// （serve / worker / migrate / healthcheck）の解釈は app.ParseCommand が
// 担うため、本パッケージでは独自の引数解釈ロジックを持たない。
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/hitoshi/feedman/internal/app"
)

// runner は起動機構（app.Run 相当）のシグネチャを表す。
// テスト時に fake を注入してサーバ／DB を起動せずに run の挙動を検証するための型である。
type runner func(w io.Writer, args []string) error

// run はアプリを起動し、プロセス終了コードを返す。
//
// 起動機構 r に args をそのまま委譲し、r が error を返した場合はそのメッセージを
// stderr へ出力して 1 を返す。error が nil の場合は 0 を返す。main から分離することで、
// サーバ／DB を起動せずにエラー→stderr 出力＋非ゼロ終了の挙動を単体テストできるようにする。
func run(stdout, stderr io.Writer, args []string, r runner) int {
	if err := r(stdout, args); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args[1:], app.Run))
}
