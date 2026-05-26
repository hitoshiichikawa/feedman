import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * web/Dockerfile の runner ステージに、Next.js standalone server を全 interface へ
 * bind させるための `ENV HOSTNAME=0.0.0.0` と待ち受けポートを明示する
 * `ENV PORT=3000` が組み込まれていることを検証する回帰ガード。
 *
 * Next.js standalone server（.next/standalone/server.js）は `process.env.HOSTNAME` の
 * 値に bind するため、Docker が既定でコンテナ ID を設定する `HOSTNAME` を image 側で
 * `0.0.0.0` に上書きする必要がある（Issue #97 / Req 2.1: image 自己完結性）。
 *
 * docker build + curl のフル E2E は unit 環境で実行できないため、本テストは
 * Dockerfile の runner ステージに当該 ENV が存在することを機械的にロックする。
 */

// vitest は cwd = web/ で実行されるため、Dockerfile は web/ 直下に存在する。
const dockerfilePath = path.resolve(process.cwd(), "Dockerfile");

/**
 * Dockerfile から最後の `FROM ... AS runner` 行以降（= runner ステージ本体）を
 * 切り出して返す。ビルドステージ等の誤検出を避けるため、runner ステージのみを対象にする。
 */
function extractRunnerStage(dockerfile: string): string {
  const lines = dockerfile.split("\n");
  const runnerStartIndex = lines.findLastIndex((line) =>
    /^\s*FROM\s+.+\s+AS\s+runner\s*$/i.test(line),
  );

  if (runnerStartIndex === -1) {
    throw new Error("runner ステージ（FROM ... AS runner）が Dockerfile に見つからない");
  }

  return lines.slice(runnerStartIndex).join("\n");
}

describe("web/Dockerfile runner ステージ", () => {
  const dockerfile = readFileSync(dockerfilePath, "utf-8");
  const runnerStage = extractRunnerStage(dockerfile);

  it("runner ステージ（FROM ... AS runner）が存在するとき切り出せること", () => {
    // Arrange / Act は describe スコープで実施済み

    // Assert
    expect(runnerStage).toMatch(/^\s*FROM\s+.+\s+AS\s+runner/i);
  });

  it("runner ステージに ENV HOSTNAME=0.0.0.0 が設定されているとき全 interface bind を image 自己完結で担保できること", () => {
    // Arrange / Act は describe スコープで実施済み

    // Assert
    expect(runnerStage).toMatch(/^\s*ENV\s+HOSTNAME=0\.0\.0\.0\s*$/m);
  });

  it("runner ステージに ENV PORT=3000 が設定されているとき待ち受けポートが image に明示されていること", () => {
    // Arrange / Act は describe スコープで実施済み

    // Assert
    expect(runnerStage).toMatch(/^\s*ENV\s+PORT=3000\s*$/m);
  });

  it("ビルドステージ（deps / builder）には HOSTNAME/PORT を設定しないこと（runner ステージ限定の上書き）", () => {
    // Arrange
    const lines = dockerfile.split("\n");
    const runnerStartIndex = lines.findLastIndex((line) =>
      /^\s*FROM\s+.+\s+AS\s+runner\s*$/i.test(line),
    );
    const beforeRunner = lines.slice(0, runnerStartIndex).join("\n");

    // Act / Assert
    expect(beforeRunner).not.toMatch(/^\s*ENV\s+HOSTNAME=/m);
    expect(beforeRunner).not.toMatch(/^\s*ENV\s+PORT=/m);
  });
});
