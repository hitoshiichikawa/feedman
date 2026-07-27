# 実装ノート（#223）

Web にパスキー導線（新規作成 / ログイン）を追加し、いずれの導線でも auth_code → Cookie
session の合流経路を通じて既存の 2 ペイン UI 認証状態に到達させる spec の実装ノートを
task 単位で記録する。前方伝播（先行 task の learning を後続 task で温存）を規律とする。

## Implementation Notes

### Task 1

- **採用方針**: `SessionExchangeService` は `TokenService.ExchangeAuthCode` の前半 3 段
  （`HashNativeSecret` → `AuthCodeConsumer.FindByHash` → `VerifyPKCES256Verifier` →
  `AuthCodeConsumer.MarkUsed`）を再利用し、以降のみ session 発行（`generateSessionID`
  → `SessionCreator.Create`）に差し替える。
- **重要な判断**:
  - `AuthCodeConsumer` は既存 `internal/auth/token_service.go` の interface をそのまま
    流用（新規 interface を作らない / CLAUDE.md §4 コピペ禁止）。`SessionCreator` のみ
    新規に `Create` 1 メソッドの狭い interface として追加した（interface segregation /
    CLAUDE.md §5）。`repository.SessionRepository` が構造的に充足する。
  - session ID 生成は `internal/auth/service.go` の既存 `generateSessionID`（32 バイト
    crypto random / hex）を直接呼び、Google OAuth callback と同じ 32 バイト crypto random
    /hex 方針を維持する。ロジックの複製は行わない。
  - now は `func() time.Time` フィールドで抽象化し `NewSessionExchangeService` で
    `time.Now` を注入。テストは同 package から `svc.now` を上書きして時刻を固定する
    （既存 `TokenService` と同じ idiom）。
  - 拒否は `ErrInvalidGrant` に uniform 化、infra error は `fmt.Errorf("session
    exchange: ...: %w", err)` で wrap して handler が 500 に振り分けられる形にする。
    平文 auth_code / code_verifier / session_id はログ・エラーに出さず、追跡ログは
    `code_hash[:8]` / `session_id_hash[:8]` のみ出力する（既存 `TokenService.ExchangeAuthCode`
    と同方針 / NFR 1.1）。
- **残存課題**: task 2 で `NewSessionExchangeService(authCodeRepo, sessionRepo,
  sessionTTL)` を `internal/app/app.go` で wiring し、`NativeAuthHandler` に注入する
  必要がある。`sessionTTL` は既存 `SessionMaxAge`（秒 int）を `time.Duration` 化して
  渡す（`time.Duration(cfg.SessionMaxAge) * time.Second`）。handler 層で
  `errors.Is(err, auth.ErrInvalidGrant)` により 400 INVALID_GRANT へ、その他 error は
  500 INTERNAL_ERROR へ振り分ける（design.md §Error Handling / task 2 詳細）。

## AC トレース

Task 1 で担保した AC は以下:

- **3.1（新規作成後のセッション合流）** / **4.2（ログイン成功後のセッション合流）**:
  `ExchangeAuthCodeForSession` の 5 段（find → verify → markUsed → generate → create）
  および正常系テスト `TestExchangeAuthCodeForSession_Success` で担保。拒否時の
  session 非永続化は `TestExchangeAuthCodeForSession_AuthCodeNotFound` /
  `_VerifierMismatch` / `_MarkUsedNotUsable` で担保。
- **NFR 1.1（機密情報の非漏出）**:
  `TestExchangeAuthCodeForSession_DoesNotLeakPlainSecretsInError` が ErrInvalidGrant /
  infra wrap の両経路で error メッセージに平文 authCode / codeVerifier が含まれない
  ことを検証。実装側は追跡ログを `code_hash[:8]` / `session_id_hash[:8]` に留めており、
  平文値を slog にも出さない（既存 `TokenService` と同方針）。

## 確認事項

（現時点でなし。design.md § SessionExchangeService の Contracts と実装は一致している）
