import { describe, it, expect } from "vitest";
import {
  base64urlToArrayBuffer,
  arrayBufferToBase64url,
  decodeCreationOptions,
  decodeRequestOptions,
  encodeAttestationResponse,
  encodeAssertionResponse,
} from "./webauthn";

/** ASCII 文字列を ArrayBuffer に組み立てる（テスト用ヘルパ）。 */
function stringToArrayBuffer(s: string): ArrayBuffer {
  const bytes = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) {
    bytes[i] = s.charCodeAt(i);
  }
  return bytes.buffer;
}

/** ArrayBuffer を ASCII 文字列に戻す（テスト用ヘルパ）。 */
function arrayBufferToString(buf: ArrayBuffer): string {
  return String.fromCharCode(...new Uint8Array(buf));
}

describe("arrayBufferToBase64url", () => {
  it("0 バイト入力を空文字にエンコードすること", () => {
    // Arrange
    const buf = new ArrayBuffer(0);

    // Act
    const result = arrayBufferToBase64url(buf);

    // Assert
    expect(result).toBe("");
  });

  it("RFC 4648 §10 の 'M' (1 バイト) を 'TQ' にエンコードすること（padding 除去）", () => {
    // Arrange
    const buf = stringToArrayBuffer("M");

    // Act
    const result = arrayBufferToBase64url(buf);

    // Assert: 標準 base64 は "TQ==" だが base64url は padding を落として "TQ"
    expect(result).toBe("TQ");
  });

  it("RFC 4648 §10 の 'Ma' (2 バイト) を 'TWE' にエンコードすること（padding 除去）", () => {
    // Arrange
    const buf = stringToArrayBuffer("Ma");

    // Act
    const result = arrayBufferToBase64url(buf);

    // Assert: 標準 base64 は "TWE=" だが base64url は padding を落として "TWE"
    expect(result).toBe("TWE");
  });

  it("RFC 4648 §10 の 'Man' (3 バイト) を 'TWFu' にエンコードすること", () => {
    // Arrange: 3 バイトはちょうど 4 base64 文字（padding 不要）
    const buf = stringToArrayBuffer("Man");

    // Act
    const result = arrayBufferToBase64url(buf);

    // Assert
    expect(result).toBe("TWFu");
  });

  it("Uint8Array 入力も ArrayBuffer と同一に扱うこと", () => {
    // Arrange
    const bytes = new Uint8Array([0x4d, 0x61, 0x6e]); // "Man"

    // Act
    const result = arrayBufferToBase64url(bytes);

    // Assert
    expect(result).toBe("TWFu");
  });

  it("31 バイト境界値を 42 文字 base64url にエンコードすること（padding 除去確認）", () => {
    // Arrange: 31 バイト（(31*8)/6 = 41.33 → 42 文字 body + "==" padding）
    const bytes = new Uint8Array(31);
    for (let i = 0; i < 31; i++) bytes[i] = i;

    // Act
    const result = arrayBufferToBase64url(bytes);

    // Assert
    expect(result.length).toBe(42);
    expect(result).not.toContain("=");
    expect(result).not.toMatch(/[+/]/);
  });

  it("32 バイト境界値を 43 文字 base64url にエンコードすること", () => {
    // Arrange: 32 バイト（(32*8)/6 = 42.67 → 43 文字 body + "=" padding）
    const bytes = new Uint8Array(32);
    for (let i = 0; i < 32; i++) bytes[i] = i;

    // Act
    const result = arrayBufferToBase64url(bytes);

    // Assert
    expect(result.length).toBe(43);
    expect(result).not.toContain("=");
  });

  it("`+` / `/` の代わりに `-` / `_` を出力すること（base64url 変換）", () => {
    // Arrange: 0xf8 → 標準 base64 "+A==" → base64url "-A"
    const bytes = new Uint8Array([0xf8]);

    // Act
    const result = arrayBufferToBase64url(bytes);

    // Assert
    expect(result).toBe("-A");
    expect(result).not.toContain("+");
    expect(result).not.toContain("/");
  });

  it("`/` を含むバイト列を `_` に変換してエンコードすること", () => {
    // Arrange: 0xff, 0xff, 0xff → 標準 base64 "////" → base64url "____"
    const bytes = new Uint8Array([0xff, 0xff, 0xff]);

    // Act
    const result = arrayBufferToBase64url(bytes);

    // Assert
    expect(result).toBe("____");
    expect(result).not.toContain("/");
  });
});

describe("base64urlToArrayBuffer", () => {
  it("空文字を 0 バイト ArrayBuffer にデコードすること", () => {
    // Act
    const result = base64urlToArrayBuffer("");

    // Assert
    expect(result.byteLength).toBe(0);
  });

  it("RFC 4648 §10 'TQ' を 'M' (1 バイト) にデコードすること", () => {
    // Act
    const result = base64urlToArrayBuffer("TQ");

    // Assert
    expect(arrayBufferToString(result)).toBe("M");
  });

  it("RFC 4648 §10 'TWE' を 'Ma' (2 バイト) にデコードすること", () => {
    // Act
    const result = base64urlToArrayBuffer("TWE");

    // Assert
    expect(arrayBufferToString(result)).toBe("Ma");
  });

  it("RFC 4648 §10 'TWFu' を 'Man' (3 バイト) にデコードすること", () => {
    // Act
    const result = base64urlToArrayBuffer("TWFu");

    // Assert
    expect(arrayBufferToString(result)).toBe("Man");
  });

  it("`-` / `_` を base64 の `+` / `/` に変換してデコードすること", () => {
    // Arrange: base64url "-A" → 標準 base64 "+A" → 1 バイト 0xf8
    // Act
    const result = base64urlToArrayBuffer("-A");

    // Assert
    expect(new Uint8Array(result)[0]).toBe(0xf8);
  });

  it("padding 付き入力も許容してデコードすること（互換性）", () => {
    // Arrange: 一部サーバ実装が padding を残す可能性への防衛的許容
    // Act
    const result = base64urlToArrayBuffer("TQ==");

    // Assert
    expect(arrayBufferToString(result)).toBe("M");
  });

  it("非 string 入力に対し TypeError を throw すること（形式不正のドメイン境界 throw）", () => {
    // Assert
    expect(() => base64urlToArrayBuffer(123 as unknown as string)).toThrow(TypeError);
    expect(() => base64urlToArrayBuffer(null as unknown as string)).toThrow(TypeError);
    expect(() => base64urlToArrayBuffer(undefined as unknown as string)).toThrow(TypeError);
  });

  it("round-trip: 32 バイト任意データを encode → decode で復元すること", () => {
    // Arrange: 決定論的な擬似ランダム 32 バイト
    const original = new Uint8Array(32);
    for (let i = 0; i < 32; i++) original[i] = (i * 7 + 13) & 0xff;

    // Act
    const encoded = arrayBufferToBase64url(original);
    const decoded = new Uint8Array(base64urlToArrayBuffer(encoded));

    // Assert
    expect(decoded.length).toBe(original.length);
    for (let i = 0; i < original.length; i++) {
      expect(decoded[i]).toBe(original[i]);
    }
  });
});

describe("decodeCreationOptions", () => {
  it("challenge を base64url → ArrayBuffer に変換すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: {
          id: "dXNlcl9pZA", // "user_id"
          name: "alice",
          displayName: "Alice",
        },
        challenge: "TWFu", // "Man"
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      },
    };

    // Act
    const result = decodeCreationOptions(raw);

    // Assert
    expect(result.publicKey?.challenge).toBeInstanceOf(ArrayBuffer);
    expect(arrayBufferToString(result.publicKey!.challenge as ArrayBuffer)).toBe("Man");
  });

  it("user.id を base64url → ArrayBuffer に変換すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: {
          id: "TWFu", // "Man"
          name: "alice",
          displayName: "Alice",
        },
        challenge: "dXNlcl9pZA",
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      },
    };

    // Act
    const result = decodeCreationOptions(raw);
    const userId = (result.publicKey!.user as PublicKeyCredentialUserEntity).id as ArrayBuffer;

    // Assert
    expect(userId).toBeInstanceOf(ArrayBuffer);
    expect(arrayBufferToString(userId)).toBe("Man");
  });

  it("excludeCredentials[].id を base64url → ArrayBuffer に変換すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: { id: "dXNlcl9pZA", name: "a", displayName: "A" },
        challenge: "dXNlcl9pZA",
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
        excludeCredentials: [
          { type: "public-key", id: "TQ" }, // "M"
          { type: "public-key", id: "TWE" }, // "Ma"
        ],
      },
    };

    // Act
    const result = decodeCreationOptions(raw);
    const excludes = result.publicKey!.excludeCredentials!;

    // Assert
    expect(excludes).toHaveLength(2);
    expect(excludes[0].id).toBeInstanceOf(ArrayBuffer);
    expect(arrayBufferToString(excludes[0].id as ArrayBuffer)).toBe("M");
    expect(arrayBufferToString(excludes[1].id as ArrayBuffer)).toBe("Ma");
    expect(excludes[0].type).toBe("public-key");
    expect(excludes[1].type).toBe("public-key");
  });

  it("他フィールド（rp / user.name / pubKeyCredParams / authenticatorSelection / attestation / timeout）を透過的にコピーすること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: {
          id: "dXNlcl9pZA",
          name: "alice",
          displayName: "Alice Wonder",
        },
        challenge: "TWFu",
        pubKeyCredParams: [
          { type: "public-key", alg: -7 },
          { type: "public-key", alg: -257 },
        ],
        authenticatorSelection: {
          residentKey: "required",
          userVerification: "preferred",
        },
        attestation: "none",
        timeout: 60000,
      },
    };

    // Act
    const result = decodeCreationOptions(raw);
    const pk = result.publicKey!;

    // Assert: 変換対象外は原値と shallow に一致
    expect(pk.rp).toEqual({ id: "example.com", name: "Example" });
    expect((pk.user as PublicKeyCredentialUserEntity).name).toBe("alice");
    expect((pk.user as PublicKeyCredentialUserEntity).displayName).toBe("Alice Wonder");
    expect(pk.pubKeyCredParams).toEqual([
      { type: "public-key", alg: -7 },
      { type: "public-key", alg: -257 },
    ]);
    expect(pk.authenticatorSelection).toEqual({
      residentKey: "required",
      userVerification: "preferred",
    });
    expect(pk.attestation).toBe("none");
    expect(pk.timeout).toBe(60000);
  });

  it("excludeCredentials が省略されているとき変換対象なし（undefined のまま）", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: { id: "dXNlcl9pZA", name: "a", displayName: "A" },
        challenge: "TWFu",
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      },
    };

    // Act
    const result = decodeCreationOptions(raw);

    // Assert
    expect(result.publicKey?.excludeCredentials).toBeUndefined();
  });

  it("publicKey が欠落しているとき TypeError を throw すること", () => {
    // Assert
    expect(() => decodeCreationOptions({})).toThrow(TypeError);
    expect(() => decodeCreationOptions(null)).toThrow(TypeError);
    expect(() => decodeCreationOptions("bad")).toThrow(TypeError);
    expect(() => decodeCreationOptions({ publicKey: null })).toThrow(TypeError);
  });

  it("publicKey.challenge が文字列でないとき TypeError を throw すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        user: { id: "dXNlcl9pZA", name: "a", displayName: "A" },
        challenge: 123,
        pubKeyCredParams: [],
      },
    };

    // Assert
    expect(() => decodeCreationOptions(raw)).toThrow(TypeError);
  });

  it("publicKey.user が欠落しているとき TypeError を throw すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        rp: { id: "example.com", name: "Example" },
        challenge: "TWFu",
        pubKeyCredParams: [],
      },
    };

    // Assert
    expect(() => decodeCreationOptions(raw)).toThrow(TypeError);
  });
});

describe("decodeRequestOptions", () => {
  it("challenge を base64url → ArrayBuffer に変換すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        challenge: "TWFu",
      },
    };

    // Act
    const result = decodeRequestOptions(raw);

    // Assert
    expect(result.publicKey?.challenge).toBeInstanceOf(ArrayBuffer);
    expect(arrayBufferToString(result.publicKey!.challenge as ArrayBuffer)).toBe("Man");
  });

  it("allowCredentials[].id を base64url → ArrayBuffer に変換すること", () => {
    // Arrange
    const raw = {
      publicKey: {
        challenge: "TWFu",
        allowCredentials: [
          { type: "public-key", id: "TQ" },
          { type: "public-key", id: "TWE" },
        ],
      },
    };

    // Act
    const result = decodeRequestOptions(raw);
    const allows = result.publicKey!.allowCredentials!;

    // Assert
    expect(allows).toHaveLength(2);
    expect(arrayBufferToString(allows[0].id as ArrayBuffer)).toBe("M");
    expect(arrayBufferToString(allows[1].id as ArrayBuffer)).toBe("Ma");
    expect(allows[0].type).toBe("public-key");
  });

  it("allowCredentials が省略された discoverable login options を透過的に扱うこと", () => {
    // Arrange: discoverable login では allowCredentials は空
    const raw = {
      publicKey: {
        challenge: "TWFu",
        rpId: "example.com",
        userVerification: "preferred",
        timeout: 60000,
      },
    };

    // Act
    const result = decodeRequestOptions(raw);
    const pk = result.publicKey!;

    // Assert
    expect(pk.allowCredentials).toBeUndefined();
    expect(pk.rpId).toBe("example.com");
    expect(pk.userVerification).toBe("preferred");
    expect(pk.timeout).toBe(60000);
  });

  it("publicKey が欠落しているとき TypeError を throw すること", () => {
    // Assert
    expect(() => decodeRequestOptions({})).toThrow(TypeError);
    expect(() => decodeRequestOptions(null)).toThrow(TypeError);
  });

  it("publicKey.challenge が文字列でないとき TypeError を throw すること", () => {
    // Arrange
    const raw = { publicKey: { challenge: null } };

    // Assert
    expect(() => decodeRequestOptions(raw)).toThrow(TypeError);
  });
});

describe("encodeAttestationResponse", () => {
  it("id / type / rawId / response.clientDataJSON / response.attestationObject を base64url JSON にエンコードすること", () => {
    // Arrange: 最小限のテスト用 PublicKeyCredential（duck-typed）。
    // 実際の PublicKeyCredential は jsdom で構築できないため、encode 関数が
    // 参照する field だけを持つオブジェクトを組み立てて cast する。
    const cred = {
      id: "TWFu",
      type: "public-key",
      rawId: stringToArrayBuffer("Man"),
      response: {
        clientDataJSON: stringToArrayBuffer("M"), // → "TQ"
        attestationObject: stringToArrayBuffer("Ma"), // → "TWE"
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAttestationResponse(cred) as Record<string, unknown>;

    // Assert
    expect(result.id).toBe("TWFu");
    expect(result.type).toBe("public-key");
    expect(result.rawId).toBe("TWFu");
    const response = result.response as Record<string, unknown>;
    expect(response.clientDataJSON).toBe("TQ");
    expect(response.attestationObject).toBe("TWE");
  });

  it("サーバ go-webauthn の必須フィールド（top-level `id` / `type`）を出力に必ず含めること", () => {
    // Arrange: go-webauthn の CredentialCreationResponse.Parse() は
    // ccr.ID == "" と ccr.Type != "public-key" を明示的に reject するため、
    // 本 encode 関数の出力はこの 2 フィールドを必ず含める必要がある。
    const cred = {
      id: "some-credential-id",
      type: "public-key",
      rawId: new ArrayBuffer(1),
      response: {
        clientDataJSON: new ArrayBuffer(1),
        attestationObject: new ArrayBuffer(1),
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAttestationResponse(cred) as Record<string, unknown>;

    // Assert
    expect(Object.keys(result)).toEqual(expect.arrayContaining(["id", "type", "rawId", "response"]));
    expect(result.id).toBe("some-credential-id");
    expect(result.type).toBe("public-key");
  });

  it("出力全体が JSON.stringify 可能であること（送信 payload としての安全性）", () => {
    // Arrange
    const cred = {
      id: "id",
      type: "public-key",
      rawId: new Uint8Array([1, 2, 3]).buffer,
      response: {
        clientDataJSON: new Uint8Array([4, 5, 6]).buffer,
        attestationObject: new Uint8Array([7, 8, 9]).buffer,
      },
    } as unknown as PublicKeyCredential;

    // Act
    const encoded = encodeAttestationResponse(cred);
    const json = JSON.stringify(encoded);

    // Assert: 循環参照なし・ArrayBuffer が漏れていないこと
    expect(json).not.toContain("[object ArrayBuffer]");
    const parsed = JSON.parse(json);
    expect(typeof parsed.rawId).toBe("string");
    expect(typeof parsed.response.clientDataJSON).toBe("string");
  });
});

describe("encodeAssertionResponse", () => {
  it("id / type / rawId / response の全 4 base64url フィールドをエンコードすること", () => {
    // Arrange
    const cred = {
      id: "TWFu",
      type: "public-key",
      rawId: stringToArrayBuffer("Man"),
      response: {
        clientDataJSON: stringToArrayBuffer("M"),
        authenticatorData: stringToArrayBuffer("Ma"),
        signature: stringToArrayBuffer("Man"),
        userHandle: stringToArrayBuffer("user_id"),
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAssertionResponse(cred) as Record<string, unknown>;

    // Assert
    expect(result.id).toBe("TWFu");
    expect(result.type).toBe("public-key");
    expect(result.rawId).toBe("TWFu");
    const response = result.response as Record<string, unknown>;
    expect(response.clientDataJSON).toBe("TQ");
    expect(response.authenticatorData).toBe("TWE");
    expect(response.signature).toBe("TWFu");
    expect(response.userHandle).toBe("dXNlcl9pZA");
  });

  it("userHandle が null のとき response フィールドから省略すること（go-webauthn omitempty 相当）", () => {
    // Arrange: discoverable login では userHandle が null / 空になり得る
    const cred = {
      id: "id",
      type: "public-key",
      rawId: new ArrayBuffer(1),
      response: {
        clientDataJSON: new ArrayBuffer(1),
        authenticatorData: new ArrayBuffer(1),
        signature: new ArrayBuffer(1),
        userHandle: null,
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAssertionResponse(cred) as Record<string, unknown>;
    const response = result.response as Record<string, unknown>;

    // Assert
    expect(response.userHandle).toBeUndefined();
    expect(Object.keys(response)).not.toContain("userHandle");
  });

  it("userHandle が undefined のときも response フィールドから省略すること", () => {
    // Arrange
    const cred = {
      id: "id",
      type: "public-key",
      rawId: new ArrayBuffer(1),
      response: {
        clientDataJSON: new ArrayBuffer(1),
        authenticatorData: new ArrayBuffer(1),
        signature: new ArrayBuffer(1),
        // userHandle 未定義
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAssertionResponse(cred) as Record<string, unknown>;
    const response = result.response as Record<string, unknown>;

    // Assert
    expect(Object.keys(response)).not.toContain("userHandle");
  });

  it("サーバ go-webauthn の必須フィールド（top-level `id` / `type`）を出力に必ず含めること", () => {
    // Arrange: 認証応答側の CredentialAssertionResponse.Parse() も同様に
    // top-level id / type を検証する（PublicKeyCredential 継承）。
    const cred = {
      id: "assertion-id",
      type: "public-key",
      rawId: new ArrayBuffer(1),
      response: {
        clientDataJSON: new ArrayBuffer(1),
        authenticatorData: new ArrayBuffer(1),
        signature: new ArrayBuffer(1),
      },
    } as unknown as PublicKeyCredential;

    // Act
    const result = encodeAssertionResponse(cred) as Record<string, unknown>;

    // Assert
    expect(result.id).toBe("assertion-id");
    expect(result.type).toBe("public-key");
    expect(Object.keys(result)).toEqual(expect.arrayContaining(["id", "type", "rawId", "response"]));
  });
});
