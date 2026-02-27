# ============================================================
# Stage 1: ビルドステージ
# ============================================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# 依存ファイルを先にコピーしてキャッシュを活用
COPY go.mod go.sum ./
RUN go mod download

# ソースコード全体をコピー
COPY . .

# 静的リンクされたバイナリをビルド
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /feedman ./cmd/feedman

# ============================================================
# Stage 2: 実行ステージ（最小イメージ）
# ============================================================
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /feedman /feedman

# ポート公開（APIサーバーのデフォルトポート）
EXPOSE 8080

# サブコマンド（serve / worker）はCMDで切り替える
# デフォルトはAPIサーバーモード
ENTRYPOINT ["/feedman"]
CMD ["serve"]
