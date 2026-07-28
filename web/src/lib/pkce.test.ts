import { describe, it, expect } from "vitest";
import { generatePkcePair, deriveCodeChallengeS256 } from "./pkce";

describe("generatePkcePair", () => {
  // RFC 7636 §4.1 の許容文字種と長さ（43〜128 文字 unreserved）。base64url は
  // `[A-Za-z0-9_-]` の部分集合のため本正規表現にも一致する。
  const VERIFIER_RE = /^[A-Za-z0-9._~-]{43,128}$/;
  // RFC 7636 §4.2 の base64url 化 SHA-256 (32 バイト → 43 文字 無 padding)。
  const CHALLENGE_RE = /^[A-Za-z0-9_-]{43}$/;

  it("code_verifier が RFC 7636 §4.1 の許容文字種と 43 文字長に一致すること（3 サンプル）", async () => {
    for (let i = 0; i < 3; i++) {
      // Arrange & Act
      const pair = await generatePkcePair();

      // Assert
      expect(pair.codeVerifier).toMatch(VERIFIER_RE);
      expect(pair.codeVerifier.length).toBe(43);
    }
  });

  it("code_challenge が RFC 7636 §4.2 の base64url 43 文字に一致すること（3 サンプル）", async () => {
    for (let i = 0; i < 3; i++) {
      // Arrange & Act
      const pair = await generatePkcePair();

      // Assert
      expect(pair.codeChallenge).toMatch(CHALLENGE_RE);
    }
  });

  it("生成値は base64 padding `=` を含まないこと", async () => {
    // Arrange & Act
    const pair = await generatePkcePair();

    // Assert
    expect(pair.codeVerifier).not.toContain("=");
    expect(pair.codeChallenge).not.toContain("=");
  });

  it("生成値は base64url 化されており `+` と `/` を含まないこと", async () => {
    // Arrange & Act
    const pair = await generatePkcePair();

    // Assert
    expect(pair.codeVerifier).not.toMatch(/[+/]/);
    expect(pair.codeChallenge).not.toMatch(/[+/]/);
  });

  it("複数回呼ぶと毎回異なる code_verifier を返すこと（乱数性の最低限確認）", async () => {
    // Arrange
    const verifiers = new Set<string>();

    // Act
    for (let i = 0; i < 3; i++) {
      const pair = await generatePkcePair();
      verifiers.add(pair.codeVerifier);
    }

    // Assert
    expect(verifiers.size).toBe(3);
  });
});

describe("deriveCodeChallengeS256", () => {
  it("RFC 7636 Appendix B の既知ベクトルと一致すること（SHA-256 派生の正しさ）", async () => {
    // Arrange: RFC 7636 Appendix B の固定 code_verifier → 期待 code_challenge (S256)。
    // これはサーバ側 `sha256.Sum256([]byte(verifier))` と対称の計算が
    // Web 側で実現されていることの互換性テストでもある。
    const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk";
    const expected = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM";

    // Act
    const actual = await deriveCodeChallengeS256(verifier);

    // Assert
    expect(actual).toBe(expected);
  });
});
