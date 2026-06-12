# 実装ノート: Issue #169 Bearer-or-Session 複合認証

## 実装サマリ

Issue #169 の「既存認証必須 API への Bearer 認証の追加」を、design.md / tasks.md の指針に
厳密に従って実装した。`Authorization: Bearer` を検出した要求は #166 の JWTIssuer と同一規約
（同一署名鍵 / HS256 / claims）で access token を検証し、成立すれば既存 Cookie セッション
認証と同一の user context を注入する。Bearer 不在の要求は既存 `SessionMiddleware` へ完全
委譲する。

設計の主要ポイントは原文どおり踏襲している:

- **判定フロー 0〜3 をそのまま実装**: verifier nil なら `NewSessionMiddleware(sessionFinder)`
  をそのまま返す縮退（Req 4.2 / 4.3。構成が導入前と文字どおり同一）/ Authorization 無し・
  Bearer 以外の scheme は委譲（Req 3.1 / 3.2）/ Bearer 検出後は成功 or 401 の二択で Cookie へ
  fallback しない（Req 1.4 / 2.4）
- **JWTVerifier の検証規則**: `jwt.WithValidMethods(["HS256"])`（alg confusion 対策）/
  `jwt.WithExpirationRequired` + leeway 0 / `token_use == "access"` 厳密一致 / `sub` 非空必須。
  jti / kid は参照しない（失効リスト・複数鍵は Out of Scope）
- **401 応答の同形性**: `http.Error(w, "unauthorized", http.StatusUnauthorized)` で既存
  SessionMiddleware の未認証応答と status / body / Content-Type が完全一致（Req 2.6。
  テストで両者の応答を直接比較）
- **router は 1 行差し替え**: 認証必須グループの `NewSessionMiddleware(deps.SessionFinder)` を
  `NewBearerOrSessionMiddleware(deps.JWTVerifier, deps.SessionFinder)` に差し替えのみ。
  RateLimit / MaxBodyBytes / Logging の順序・位置・適用範囲は不変（NFR 2.1）
- **既存 session.go は無変更・削除なし**。下流 handler も一切変更なし（既存
  `ContextWithUserID` / `UserIDFromContext` を共用し context key の同一性を構造的に保証）
- **wiring**: `cfg.NativeAuthJWTSecret != ""` 分岐内で `auth.NewJWTVerifier` を生成し
  interface 型のローカル変数経由で注入（typed-nil を作らない）。未設定時の warn は
  「Bearer auth も無効」を含む文言に更新（warn は 1 回のまま・起動成功）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | auth: JWTVerifier を追加 | fbd21d7 `feat(auth)` | b4a6f74 `docs(tasks): mark 1` |
| 2 | middleware: BearerOrSession 複合認証を追加 | 059af31 `feat(middleware)` | 67264f1 `docs(tasks): mark 2` |
| 3 | router / app: 認証必須グループの差し替えと wiring | 43faf4e `feat(handler)` | 7b5bcfb `docs(tasks): mark 3` |
| 4 | 統合確認: 発行 ↔ 検証の通しと既存回帰 | c51b84a `test(handler)` | b6b422d `docs(tasks): mark 4` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約（2 → 1、3 → 2、4 → 3）は
自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: Bearer token による認証の成立

| AC ID | 担保テスト |
|---|---|
| 1.1 (有効 Bearer を Cookie と同一のユーザー識別へ解決) | `middleware.TestBearerOrSession_ValidBearer_InjectsUserID` / `handler.TestNewRouter_BearerAuth_IssuerVerifierRoundTrip`（実物ペア通し） |
| 1.2 (Cookie 認証時と同一の応答内容・形式) | `handler.TestNewRouter_BearerAuth_ReachesAPIWithoutCookie`（既存ルート・既存 handler を変更ゼロで透過動作）。下流は `UserIDFromContext` のみ参照 |
| 1.3 (Cookie 併送を要求しない) | `middleware.TestBearerOrSession_ValidBearer_InjectsUserID`（Cookie 無し 200 / sessionFinder 0 回） |
| 1.4 (両方提示時は token 側のユーザーで処理) | Bearer 検出後は Cookie 評価に到達しない実装構造 + `middleware.TestBearerOrSession_InvalidBearerWithValidCookie_Returns401`（検出後は成功 or 401 の二択の裏面検証） |

### Requirement 2: 無効な Bearer token の拒否

| AC ID | 担保テスト |
|---|---|
| 2.1 (署名検証不能を拒否) | `auth.TestVerifyAccessToken_WrongSecret` |
| 2.2 (期限切れを拒否) | `auth.TestVerifyAccessToken_Expired`（超過 / 丁度の境界値）/ `handler.TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401` |
| 2.3 (用途種別不一致を拒否) | `auth.TestVerifyAccessToken_TokenUseMismatch`（refresh / 欠落 / 空文字） |
| 2.4 (無効 Bearer + 有効 Cookie で fallback しない) | `middleware.TestBearerOrSession_InvalidBearerWithValidCookie_Returns401`（sessionFinder 0 回）/ `handler.TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401` |
| 2.5 (token 部空・形式不正を拒否) | `middleware.TestBearerOrSession_SchemeAndTokenEdgeCases`（`Bearer` 単独 / `Bearer ` 空白のみ）/ `auth.TestVerifyAccessToken_MalformedTokens`（非 JWT 文字列 / 空文字） |
| 2.6 (未認証応答が既存と同一形式) | `middleware.TestBearerOrSession_401SameShapeAsSessionMiddleware`（status / body / Content-Type を SessionMiddleware の実応答と直接比較） |
| 2.7 (拒否理由を区別できる情報を返さない) | 全拒否が同一の `http.Error` 固定応答（2.6 のテストで形状固定を担保）。理由は slog.Warn のみ |

### Requirement 3: Bearer 不在時の委譲（既存挙動の維持）

| AC ID | 担保テスト |
|---|---|
| 3.1 (Bearer 不在は Cookie 認証で処理) | `middleware.TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession`（有効 Cookie 200 / verifier 0 回） |
| 3.2 (Bearer 以外の方式は提示なし扱い) | `middleware.TestBearerOrSession_SchemeAndTokenEdgeCases`（Basic → 委譲） |
| 3.3 (Cookie 有効時は導入前と同一応答) | 委譲先が既存 `NewSessionMiddleware` の戻り値そのもの + `handler.TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior`（Cookie 200） |
| 3.4 (両方不在時は導入前と同一の未認証応答) | `middleware.TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession`（Cookie 無し 401） |

### Requirement 4: 検証の構成と安全な縮退

| AC ID | 担保テスト |
|---|---|
| 4.1 (発行側と同一の鍵設定で受理) | `auth.TestVerifyAccessToken_IssuerVerifierRoundTrip`（同一 package の #166 JWTIssuer と固定 secret / 固定 now 共有）/ `handler.TestNewRouter_BearerAuth_IssuerVerifierRoundTrip`（router 通しの実物ペア） |
| 4.2 (鍵未設定で Bearer 無効化・Cookie のみ提供) | `middleware.TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware` / `handler.TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior` |
| 4.3 (鍵未設定時は token を評価しない) | `middleware.TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware`（不正 Bearer 付きでも Cookie で 200） |
| 4.4 (鍵未設定でも起動成功) | `app.go` の分岐は verifier を生成しないだけで起動継続（#166 の既存分岐に同乗。既存 `config.TestLoad_NativeAuthJWT` が未設定起動を担保） |
| 4.5 (固定鍵・固定時刻で決定論的検証) | `auth.newFixedVerifier`（非公開 now の注入。#166 issuer と同パターン）を全 unit test で使用 |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (token 値をログ・エラー応答に残さない) | `auth.TestVerifyAccessToken_ErrorDoesNotContainToken`。middleware の slog.Warn は error 文字列のみ。401 応答は固定文言 |
| NFR 1.2 (セッション情報・外部照会なしで検証完結) | `JWTVerifier` は secret と now のみで検証（stateless）。Bearer パスで sessionFinder 0 回（`TestBearerOrSession_ValidBearer_InjectsUserID`） |
| NFR 2.1 (既存 Cookie 認証の API 挙動を変更しない) | `session.go` 無変更・差し替え 1 箇所・middleware 順序不変。既存 Cookie 系テスト無変更 green（実 DB 接続のフルスイートで確認） |
| NFR 2.2 (鍵未設定の既存デプロイで導入前と同一動作) | `handler.TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior`（nil 構成 = 従来構成） |
| NFR 3.1 (全ケースを外部ネットワーク依存なしで検証) | auth 8 テスト / middleware 6 テスト / router 4 テストすべて in-process（DB / net 不要） |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ pass
- `TEST_DATABASE_URL` を設定した実 PostgreSQL 16 でも `go test -p 1 ./...` 全 pass（CI と同条件）
- `gofmt -l <変更ファイル群>`: 出力なし

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- 判定フロー 0〜3（nil 縮退 / 委譲 / Bearer 二択）
- 検証規則表（HS256 限定 / exp 必須 leeway 0 / token_use 厳密一致 / sub 非空 / jti・kid 不参照）
- File Structure Plan の 6 ファイル（新規 2 + テスト 2 + 変更 2。新規 migration / 新規 config /
  新規依存 / `.env.sample` 変更なし）
- 401 応答の同形性（`http.Error` 固定）と `WWW-Authenticate` 不付与
- Requirements Traceability / Testing Strategy の全項目

## 実装上の判断

### scheme / token 分離に `strings.Cut` を使用

design.md は `strings.SplitN(v, " ", 2)` を例示しているが、等価でより読みやすい
`strings.Cut(authz, " ")` を採用した（Go 1.18+ の標準 idiom。分離結果・挙動は同一で、
scheme 比較は design どおり `strings.EqualFold`）。

### 期限切れ token の組み立て（router 通しテスト）

`auth.JWTIssuer` / `auth.JWTVerifier` の `now` は非公開 field のため handler パッケージから
注入できない。router 通しの期限切れケースは同一 secret で exp が過去の JWT をテスト内で
直接組み立てて検証した（検証側は実時刻で評価されるため決定論的に 401 になる）。

## 追加した依存

無し（`golang-jwt/jwt/v5` は #166 で追加済みの依存を共用。go.mod / go.sum 変更なし）。

## 後続 Issue への引き継ぎ事項

- **Issue #171 (IP レート制限)**: 本 spec の変更とは独立（認証必須グループの user 単位
  RateLimit は従来位置のまま不変）。#171 は認証不要グループの native auth 3 ルートに
  `unauthIPMW` を重ねる。
- **Issue #172 (contract tests)**: Bearer 認証の契約（401 同形 / fallback なし / nil 縮退）は
  本 spec の middleware / router テストでカバー済み。
- **鍵ローテーション（将来 Issue）**: 検証は単一鍵 v1。JWT ヘッダ kid は発行側で出力済みの
  ため、複数鍵受理は verifier 側の拡張のみで対応可能。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- `strings.SplitN` の代わりに等価な `strings.Cut` を使用（上記「実装上の判断」参照）。
- 未設定時の warn 文言を「POST /api/auth/token and Bearer auth are disabled」に更新
  （design.md の指示どおり。warn 回数は 1 回のまま）。

STATUS: complete
