# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T09:50:00Z -->

## Reviewed Scope

- Branch: claude/issue-172-impl-native-auth-contract-tests
- HEAD commit: 708593bb8081bbf9eb93a0f1a7d4b8ff8007f746
- Compared to: develop..HEAD

## Verified Requirements

### Requirement 1: Native OAuth フロー全体の契約検証

- 1.1 — `internal/handler/native_auth_e2e_db_test.go` `TestE2E_NativeAuthFullFlow_DBBacked` step1〜2 で native login → callback → アプリスキーム redirect + auth_codes 行 1 件作成を実物経路で検証。`integration_test.go` の `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` でも mock 経路から callback Location を契約検証
- 1.2 — `TestE2E_NativeAuthFullFlow_DBBacked` step3 で `POST /api/auth/token` の PKCE 検証 + 実 token 交換（real TokenService + real DB 永続化）を検証
- 1.3 — 同 step4 で refresh rotation を実物経路で検証（新旧 token 相違 + DB `rotated_at` + 同一 family 2 世代）
- 1.4 — 同 step5 で revoke 204 + 後続 refresh 拒否 + families.revoked_at + 冪等性を検証
- 1.5 — 同 step6 で real JWT を Bearer 提示し `/api/subscriptions` 200 + fake fixture が DB の users.id を反映することを確認。加えて `TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie` で Bearer と Cookie 両経路の応答 body 完全同一を契約として固定
- 1.6 — 全契約テストは httptest 駆動、E2E のみ `TEST_DATABASE_URL` の PostgreSQL に接続（fake OAuthProvider 注入で実 Google OAuth・実 FCM・実外部ネットワーク不要）。DB 未到達時は `t.Skipf` で silent skip（実機で確認: DB 不在で SKIP、`localhost:15432` で PASS）

### Requirement 2: JSON 応答契約

- 2.1 — `TestContract_TokenResponse_ExactJSONShape` で 4 フィールド + `len(got) == 4` 余剰キー検出
- 2.2 — `TestContract_RefreshResponse_ExactJSONShape` で同一の厳密集合検証
- 2.3 — 上記 2 件と E2E の `e2eDecodeTokenPair` で `token_type == "Bearer"` をアサート
- 2.4 — 同上で `expires_in == 900` をアサート
- 2.5 — `TestContract_RevokeResponse_204AndEmptyBody` で 204 + `w.Body.Len() == 0` を直接固定
- 2.6 — `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` で scheme=`feedman` / host=`auth` / path=`/callback` / クエリ名=`auth_code` 非空 / クエリ数 1 / `session_id` Cookie 非発行を検証

### Requirement 3: エラー契約

- 3.1 — E2E `TestE2E_NativeAuthFullFlow_DBBacked` step3-reject で unknown / consumed auth_code を `INVALID_GRANT` で拒否（時刻依存の expired は #166 unit が正本。NFR 3.1 の非重複方針）
- 3.2 — 同 step3-reject で wrong verifier 拒否 + 拒否で auth_code を消費しないことを直後の正規 verifier 交換 200 で確認
- 3.3 — 同 step4-reject で unknown / rotated / family 失効後の現役 refresh token を `INVALID_REFRESH_TOKEN` 拒否
- 3.4 — 同 step4-reject で rotated 再提示 → family 失効 → 現役だった refresh2 も拒否を実物経路で検証 + 既存 `TestIntegration_ReuseDetection_FamilyRevoked` 再利用
- 3.5 — `TestContract_BearerToken_RejectionUniformity_AllRejectionShapes` で署名不正・期限切れ・token_use 不一致・形式不正の 4 区分 + Cookie 併送 fallback 禁止を table-driven で 401 検証
- 3.6 — 同テストで全区分の status / body / Content-Type が完全同一であることをアサート。E2E の `e2eAssertRejection` で `used` / `expired` / `rotated` / `revoked` / `signature` / `unknown` / `consumed` / `verifier` / `mismatch` 9 種の原因区別語が message に含まれないことを否定アサート

### Requirement 4: iOS 仕様（SERVER.md §1）との同期確認

- 4.1 — `contract-notes.md` で §1.2 / §1.3 / §1.4 / §1.5 / §1.6 / §1.7 / §1.8 の対照表を作成、確認日 2026-06-12 / 確認対象範囲を明記
- 4.2 — 「承認済み逸脱」節で根拠 Issue（#168 / #164 / RFC 7009 §2.1）と iOS への影響を明示
- 4.3 — 逸脱 1 として「revoke 未認証 + token 所持 = 権限」を #168 Open Questions・RFC 7009 §2.1・#171 IP rate limit と共に明文化
- 4.4 — 逸脱 2 として `refresh_token_families` 分離・`auth_codes` の `used` boolean・`device_label`/`last_used_at` 未実装を明文化
- 4.5 — 「未承認差分エスカレーション方針」節で Issue コメント経由の人間判断手順 3 ステップを明記。今回の確認では未承認差分の発見なしを宣言

### Non-Functional Requirements

- NFR 1.1 — 全契約テスト httptest 駆動、E2E のみ PostgreSQL に接続（fake OAuthProvider で外部接続なし）。実機で DB 不在時 SKIP / DB 到達時 PASS を確認
- NFR 1.2 — Bearer 期限切れ拒否は **過去時刻 exp を直接埋め込んだ crafted token**（`TestContract_BearerToken_RejectionUniformity_AllRejectionShapes` の `expiredClaims`）で決定論的に検証。900s / 30d / 60s の境界値検証は固定 now 注入を備えた各 spec の unit テスト（jwt_issuer_test.go / jwt_verifier_test.go / token_service_test.go）が正本（NFR 3.1 の非重複方針）。下記 Findings の「design 逸脱の妥当性確認」参照
- NFR 1.3 — E2E で固定 secret `[]byte("e2e-contract-test-secret-32bytes!")`、契約テストで `[]byte("contract-bearer-secret-32bytes-xx")` を注入
- NFR 2.1 — 既存 Cookie 系統合テスト群を変更せず、`TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie` の Cookie 側 200 で fallback 経路の不変を確認
- NFR 2.2 — `git diff develop..HEAD` で確認: 既存テスト・既存ヘルパーへの変更 0（追加のみ）。production code への変更も 0
- NFR 3.1 — design.md Non-Goals および impl-notes.md「実装上の判断」で各 spec 固有の unit 範囲を再検証しない方針を明示し、本 spec 追加テストは横断契約に限定

## Findings

なし（承認対象観点での問題なし）。

### 補足: design 逸脱の妥当性確認（独立判断）

impl-notes.md「design からの逸脱」に明示された 1 件（E2E での `now` 固定注入を実施せず、Bearer 期限切れは crafted token、auth_code/refresh の境界値検証は各 spec unit に委譲）について独立に妥当性を評価:

- design.md の「`now` field を package-private override で固定」は `auth.JWTIssuer` / `JWTVerifier` / `TokenService` の `now` を auth パッケージ非公開 field 経由で行う想定だったが、handler パッケージのテストから直接アクセスできない事実は技術的に正当（go の可視性ルール）
- production code に setter を追加する選択肢は本 spec の **Non-Goals「production code 変更なし」と直接矛盾**（design.md Non-Goals / requirements.md「テスト・文書のみ」と一致）
- NFR 1.2 の要請は「**決定論的に再現可能**」であり、本実装の Bearer expired 検証は過去時刻 exp の絶対値を埋め込むため実行時刻によらず常に拒否（= 決定論的）。900s / 30d / 60s の境界値そのものは #166 / #167 等の unit テストで固定 now 注入下に検証済み
- NFR 3.1 は「各 spec 固有の内部詳細を再検証しない」と明記しており、境界値の重複検証は spec 規約に反する
- impl-notes.md に逸脱として明示・根拠記載済み、`contract-notes.md` でも整合確認結果に未承認差分なしと宣言済み

以上より、AC 未カバーには該当しない（NFR 1.2 / NFR 3.1 / Non-Goals を整合的に充足する選択）。

### Boundary 検証

- 変更ファイル一覧: `internal/handler/integration_test.go`（追加のみ）/ `internal/handler/native_auth_e2e_db_test.go`（新規）/ `docs/specs/172-.../contract-notes.md`（新規）/ `docs/specs/172-.../impl-notes.md`（新規）/ `docs/specs/172-.../tasks.md`（`- [ ]` → `- [x]` のみ）
- design.md File Structure Plan の「Modified Files」「新規ファイル」と完全一致
- 既存テスト・既存ヘルパー・production code への変更 0（`git diff` で確認）
- 他 spec の `requirements.md` / `design.md` / `tasks.md` への書き換えなし

### 実行確認

- `go vet ./internal/handler/`: 警告なし
- `go test ./internal/handler/ -run 'TestContract_|TestE2E_NativeAuthFullFlow_DBBacked' -v`: 全契約テスト PASS、E2E は DB 不在で SKIP
- `TEST_DATABASE_URL='postgres://feedman:feedman@localhost:15432/feedman_test?sslmode=disable' go test ./internal/handler/ -run TestE2E -v`: `TestE2E_NativeAuthFullFlow_DBBacked` PASS（0.27s）

## Summary

要件定義の全 numeric ID（Req 1.1〜4.5, NFR 1.1〜3.1）が design.md File Structure Plan と完全に整合する 5 ファイル追加（既存テスト・production code 変更ゼロ）で担保されている。impl-notes.md に明示された design 逸脱 1 件（E2E での now 固定注入未実施）は本 spec の Non-Goals「production code 変更なし」を守るための妥当な選択であり、NFR 1.2 / NFR 3.1 を整合的に充足する。実機で契約テストおよび E2E DB-backed テストの PASS を確認した。

RESULT: approve
