import type { NextConfig } from "next";
import {
  buildRewrites,
  resolveApiInternalUrlForRewrites,
} from "./src/lib/rewrites";

const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=()",
  },
];

const nextConfig: NextConfig = {
  output: "standalone",
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: securityHeaders,
      },
    ];
  },
  // 同一オリジン proxy: /api/* と /auth/* を内部 API（API_INTERNAL_URL）へ server-side 転送する。
  // API_INTERNAL_URL は server-side のみで参照し、ブラウザバンドルには含めない。
  // rewrites() はビルド時にも評価されるため、ここでは throw しない解決関数を使い
  // ビルドを env 非依存で完了させる（Req 1.1 / 5.3）。実行時の未設定 fail-fast は
  // server-entrypoint.mjs（resolveApiInternalUrl）が担う。
  async rewrites() {
    return buildRewrites(resolveApiInternalUrlForRewrites(process.env));
  },
};

export default nextConfig;
