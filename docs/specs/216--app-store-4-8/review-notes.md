# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-24T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-216-impl--app-store-4-8
- HEAD commit: 2616e5adf89f69b9002df4ad3b5ba55770578ceb
- Compared to: develop..HEAD（全 8 task の HEAD 全体レビュー）
- 検証: `go build ./...` OK / `go vet ./...` clean / `go test ./...`（DB 非依存の全 package pass。DB 結合・E2E テストは TEST_DATABASE_URL 未設定でローカル skip 経路。impl-notes 記載どおり）
- Feature Flag Protocol: CLAUDE.md 採否 = opt-out のため flag 観点は適用せず、通常 3 カテゴリ判定のみ実施

## Verified Requirements

- 1.1 — `RegistrationService.BeginRegistrationNew`（registration_service.go）+ passkey_handler_test の begin 応答
- 1.2 — `FinishRegistrationNew` の user + credential 作成、registration_service_test 成功系
- 1.3 — Finish が userID 返却 → handler 200、以降 authentication で解決可能（e2e で担保）
- 1.4 — `FindByNormalizedUsername` pre-check + `ErrUsernameTaken`、registration_service_test / postgres_user_repo_test
- 1.5 — `ValidateAndNormalize`（username.go）+ username_test.go 境界値網羅（空/2/3/32/33/Unicode/制御/記号）
- 1.6 — `FinishRegistrationNew` が `Email:""` で作成、test「email 未指定でも user 行作成」
- 1.7 — 拒否を `ErrRegistrationFailed` に uniform 化、registration_service_test 異常系
- 2.1 — `BeginAuthentication`（authentication_service.go）+ authentication_service_test
- 2.2 — `FinishAuthentication` の assertion 検証 + user 解決 + auth_code 発行、成功系テスト
- 2.3 — `auth.NativeAuthCodeTTL` / `HashNativeSecret` / `AuthCodeCreator.Create`、AuthCode 生成テスト
- 2.4 — `TestE2E_PasskeyFullFlow_DBBacked`（登録→認証→既存 `/api/auth/token` 交換→Bearer 到達）
- 2.5 — `ErrAuthenticationFailed` uniform 化、authentication_service_test 異常系（challenge/credential/counter/PKCE）
- 2.6 — 拒否本文一律テスト（uniform rejection）
- 3.1 — `BeginAddCredential` + router 認証必須グループ配置
- 3.2 — `FinishAddCredential` が current user へ紐付け、registration_service_test 追加登録成功系
- 3.3 — identities 非参照（credential 追加のみ、コード上 identities に触れない）
- 3.4 — 同一 user 複数 credential 許可テスト
- 3.5 — 認証必須グループ + handler 401 二段防衛、passkey_handler_test / router_test 未認証系
- 3.6 — `FindByCredentialID` pre-check + UNIQUE 衝突最終防衛、別 user credential 提示拒否テスト
- 3.7 — 追加登録拒否も `ErrRegistrationFailed` uniform 化
- 4.1 / 4.2 — `ChallengeStore.Issue`（TTL + hash 保存）、challenge_store_test
- 4.3 — `MarkConsumed` atomic UPDATE、二重消費テスト
- 4.4 — `Consume` の期限切れ/consumed/kind 不一致拒否テスト
- 4.5 — challenge entropy は adopt した `go-webauthn`（32byte=256bit random）が供給（design 採択、webauthn_adapter round-trip テストで ceremony 動作を担保）
- 5.1 — `AASAHandler.Serve` 200、aasa_handler_test.go
- 5.2 — Content-Type / Cache-Control 付与テスト
- 5.3 — user secret 非含有 + トップレベルキー限定テスト
- 5.4 — router で `unauthIPMW` の外側 + 認証グループ外に配置、router_test で担保
- 6.1〜6.5 — router_unauth_ratelimit_test.go の passkey 4 endpoint 429 / 独立カウント / 既存同形式テスト
- 7.1 — `withdrawTx` の passkey_credentials 削除段 + `TestWithdrawIntegration_PasskeyCredentialCleanup`
- 7.2 — 同一 tx 内削除（sessions 直後・auth_codes 直前）+ service_test / withdraw_wiring_test 順序検証
- 7.3 — `TestService_Withdraw_Tx_RollsBackOnPasskeyCredentialDeleteError`
- 7.4 — `DeleteByUserID` の `WHERE user_id` + 他 user 非影響 integration テスト
- 7.5 — `TestWithdrawIntegration_UsernameReusableAfterWithdraw`（UNIQUE 制約解放）
- 8.1〜8.5 — fail-closed nil 縮退（router.go）+ 既存 native contract テスト無変更 green（NFR 2.3）、router_test 独立 fail-close
- NFR 1.1 — model.PasskeyCredential は検証情報のみ（migration / schema）
- NFR 1.2 — `shortID` / hash 先頭 8 文字ログ、平文非出力（各 service）
- NFR 1.3 — 固定 APIError（model/errors.go）、入力反射なし
- NFR 1.4 — counter 後退（CloneWarning）→ `ErrAuthenticationFailed`、authentication_service_test / webauthn_adapter_test
- NFR 2.1〜2.3 — env 未設定時の既存挙動維持、`GenerateAuthCode` は純 rename で挙動不変、contract-notes 無変更
- NFR 3.1〜3.2 — `logRejection`（slog.Warn）+ 平文非含有
- NFR 4.1 — 外部ネットワーク非依存テスト（stub / virtualwebauthn synthetic authenticator）
- NFR 4.2 — 退会後 credential 0 件を DB 直接 SELECT で検証

## Findings

なし

## Summary

全 numeric AC（Req 1〜8 + NFR 1〜4）に対応する実装と AC 紐付けテストを確認。build / vet / test はいずれも green。boundary 逸脱なし（`auth/native.go` の `generateAuthCode`→`GenerateAuthCode` rename は task 5 が明示認可・挙動不変、テスト側変更は新 interface 充足の no-op stub / cleanup SQL 追加のみで assert 弱体化なし、tasks.md 変更は checkbox マークのみ）。missing test も検出されず。

RESULT: approve
