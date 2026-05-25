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
