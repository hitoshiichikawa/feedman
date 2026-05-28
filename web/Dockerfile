# ============================================================
# Stage 1: 依存インストール
# ============================================================
FROM node:20-alpine AS deps

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

# ============================================================
# Stage 2: ビルド
# ============================================================
FROM node:20-alpine AS builder

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# 単一オリジン化により API の URL はビルド時に焼き込まない（build-once）。
# ブラウザは常に同一オリジン相対パスで API を呼び、内部 API への転送は
# 実行時設定 API_INTERNAL_URL を参照する rewrites が担う。
RUN npm run build

# ============================================================
# Stage 3: 実行（最小イメージ）
# ============================================================
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Next.js standalone server（server.js）は process.env.HOSTNAME へ bind する。
# Docker は既定でコンテナ ID を HOSTNAME に設定するため、image 側で 0.0.0.0 に
# 上書きして全 interface へ bind させる（外部 environment 無しでも公開ポート経由で
# 到達可能にする image 自己完結性）。compose / docker run -e の HOSTNAME 指定が
# あれば Docker の env 優先順位でこの ENV を上書きできる。
ENV HOSTNAME=0.0.0.0
# 待ち受けポートを明示する（EXPOSE 3000 と整合）。environment で上書き可能。
ENV PORT=3000

# Next.js のスタンドアロン出力を利用（node ユーザーで所有）
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
COPY --from=builder --chown=node:node /app/public ./public

# 起動前に API_INTERNAL_URL を検証する fail-fast entrypoint を server.js と同階層へ配置する。
# entrypoint は ./server.js を import するため、standalone root（/app）に置く。
COPY --from=builder --chown=node:node /app/server-entrypoint.mjs ./server-entrypoint.mjs

USER node

EXPOSE 3000

# entrypoint が API_INTERNAL_URL を検証し、通過時のみ server.js を起動する（fail-fast）。
CMD ["node", "server-entrypoint.mjs"]
