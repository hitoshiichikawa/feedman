package feedman_test

import (
	"os"
	"strings"
	"testing"
)

func TestDockerfileExists(t *testing.T) {
	_, err := os.Stat("Dockerfile")
	if err != nil {
		t.Fatalf("Dockerfile should exist: %v", err)
	}
}

func TestDockerfileMultiStageBuild(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}
	content := string(data)

	// マルチステージビルドの確認: ビルドステージと実行ステージが存在すること
	if !strings.Contains(content, "FROM golang:") {
		t.Error("Dockerfile should contain a Go builder stage (FROM golang:)")
	}

	// 最終ステージは軽量イメージであること
	lines := strings.Split(content, "\n")
	var lastFrom string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FROM ") {
			lastFrom = trimmed
		}
	}
	if !strings.Contains(lastFrom, "gcr.io/distroless") && !strings.Contains(lastFrom, "alpine") && !strings.Contains(lastFrom, "scratch") {
		t.Errorf("final stage should use a minimal base image (distroless/alpine/scratch), got: %s", lastFrom)
	}
}

func TestDockerfileBinaryName(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}
	content := string(data)

	// バイナリ名がfeedmanであること
	if !strings.Contains(content, "feedman") {
		t.Error("Dockerfile should build a binary named 'feedman'")
	}
}

func TestDockerfileEntrypoint(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}
	content := string(data)

	// ENTRYPOINTまたはCMDでfeedmanバイナリを起動すること
	if !strings.Contains(content, "ENTRYPOINT") && !strings.Contains(content, "CMD") {
		t.Error("Dockerfile should contain ENTRYPOINT or CMD")
	}
}

func TestDockerComposeExists(t *testing.T) {
	_, err := os.Stat("docker-compose.yml")
	if err != nil {
		t.Fatalf("docker-compose.yml should exist: %v", err)
	}
}

func TestDockerComposeServices(t *testing.T) {
	data, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	// 3コンテナ構成: api, worker, db
	requiredServices := []string{"api:", "worker:", "db:"}
	for _, svc := range requiredServices {
		if !strings.Contains(content, svc) {
			t.Errorf("docker-compose.yml should contain service %q", svc)
		}
	}
}

func TestDockerComposePostgres(t *testing.T) {
	data, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	// PostgreSQLイメージを使用していること
	if !strings.Contains(content, "postgres:") {
		t.Error("docker-compose.yml should use PostgreSQL image")
	}
}

func TestDockerComposeWorkerCommand(t *testing.T) {
	data, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	// workerサービスがworkerサブコマンドで起動すること
	if !strings.Contains(content, "worker") {
		t.Error("docker-compose.yml worker service should use 'worker' subcommand")
	}
}

func TestDockerComposeNetworks(t *testing.T) {
	data, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	// ネットワーク設定が存在すること（egress制限用）
	if !strings.Contains(content, "networks:") {
		t.Error("docker-compose.yml should define networks for egress control")
	}

	// 内部ネットワークの定義（internal: true）
	if !strings.Contains(content, "internal: true") {
		t.Error("docker-compose.yml should define an internal network (internal: true) for egress restriction")
	}
}

func TestDockerComposeWorkerHasExternalNetwork(t *testing.T) {
	data, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	// ワーカーコンテナのみ外部通信を許可するネットワーク構成
	// "external"ネットワークの定義が存在すること
	if !strings.Contains(content, "external") {
		t.Error("docker-compose.yml should define an external network for worker egress")
	}
}
