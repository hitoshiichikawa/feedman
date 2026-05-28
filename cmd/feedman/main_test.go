package main

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

// Test_run は run 関数の終了コード・stderr 出力・args 委譲の挙動を検証する。
// 実際のサーバ／DB を起動せず、注入した fake runner で挙動を観測する。
func Test_run(t *testing.T) {
	t.Run("runner が nil を返すとき終了コード 0 で stderr に何も書かれない", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		okRunner := func(w io.Writer, args []string) error { return nil }

		// Act
		code := run(&stdout, &stderr, []string{"serve"}, okRunner)

		// Assert
		if code != 0 {
			t.Errorf("run() = %d, want 0", code)
		}
	})

	t.Run("runner が nil を返すとき stderr が空である", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		okRunner := func(w io.Writer, args []string) error { return nil }

		// Act
		run(&stdout, &stderr, []string{"serve"}, okRunner)

		// Assert
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
	})

	t.Run("runner が error を返すとき終了コード 1 を返す", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		errRunner := func(w io.Writer, args []string) error {
			return errors.New("initialization failed")
		}

		// Act
		code := run(&stdout, &stderr, []string{"serve"}, errRunner)

		// Assert
		if code != 1 {
			t.Errorf("run() = %d, want 1", code)
		}
	})

	t.Run("runner が error を返すとき stderr にエラーメッセージが出力される", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		errRunner := func(w io.Writer, args []string) error {
			return errors.New("initialization failed")
		}

		// Act
		run(&stdout, &stderr, []string{"serve"}, errRunner)

		// Assert
		got := stderr.String()
		want := "initialization failed\n"
		if got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})

	t.Run("受け取った args を改変せず runner にそのまま委譲する", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		argsIn := []string{"worker", "--flag", "value"}
		var captured []string
		captureRunner := func(w io.Writer, args []string) error {
			captured = args
			return nil
		}

		// Act
		run(&stdout, &stderr, argsIn, captureRunner)

		// Assert
		if !reflect.DeepEqual(captured, argsIn) {
			t.Errorf("runner received args = %v, want %v", captured, argsIn)
		}
	})

	t.Run("空 args をそのまま runner に委譲する", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		argsIn := []string{}
		var captured []string
		var called bool
		captureRunner := func(w io.Writer, args []string) error {
			called = true
			captured = args
			return nil
		}

		// Act
		run(&stdout, &stderr, argsIn, captureRunner)

		// Assert
		if !called {
			t.Fatal("runner was not called")
		}
		if !reflect.DeepEqual(captured, argsIn) {
			t.Errorf("runner received args = %v, want %v", captured, argsIn)
		}
	})

	t.Run("stdout を runner に委譲する", func(t *testing.T) {
		// Arrange
		var stdout, stderr bytes.Buffer
		var captured io.Writer
		captureRunner := func(w io.Writer, args []string) error {
			captured = w
			return nil
		}

		// Act
		run(&stdout, &stderr, []string{"serve"}, captureRunner)

		// Assert
		if captured != io.Writer(&stdout) {
			t.Errorf("runner received writer = %p, want stdout %p", captured, &stdout)
		}
	})
}
