# 実装ノート: Issue #172 native auth API の contract / integration tests

## 実装サマリ

Issue #172 の「native auth API の contract / integration テスト整備、および iOS 仕様
（SERVER.md §1）との同期確認」を、design.md / tasks.md の指針に厳密に従って実装した。
production code への変更は **ゼロ**（テスト追加 + 文書追加のみ）。

成果物は design.md の 3 系統そのまま:

1. **契約テスト 6 本**（`internal/handler/integration_test.go` 末尾追加 / 外部依存なし）:
   JSON 厳密 shape（余剰キー検出付き）× token / refresh、revoke 204 + 空ボディ、
   native callback Location の URL 構造契約、Bearer↔Cookie の応答等価、
   Bearer 拒否 4 区分の uniform 検証（全区分で status / body / Content-Type 完全同一）
2. **E2E DB-backed テスト 1 本**（`internal/handler/native_auth_e2e_db_test.go` 新規）:
   real `auth.Service`（fake OAuthProvider のみ注入）+ real `TokenService` + real
   `PostgresAuthCodeRepo` / `PostgresRefreshTokenRepo` + real `JWTIssuer` / `JWTVerifier` を
   wire し、native login → callback → token 交換 → refresh rotation → 再利用 family 全滅 →
   revoke 冪等 → Bearer 付き既存 API 到達を 1 経路で通す。各 step で DB の永続化状態
   （auth_codes.used / rotated_at / families.revoked_at / 行数）も検証。
   `TEST_DATABASE_URL` 未到達時は `t.Skipf`（既存 repository DB テストと同一規約）
3. **契約同期文書**（`docs/specs/172-native-auth-contract-tests/contract-notes.md` 新規）:
   SERVER.md §1.2〜§1.8 と実装 #164〜#171 の対照表 + §1.8 受け入れ基準 ↔ テスト ID 対応 +
   承認済み逸脱 2 件の明文化 + 未承認差分エスカレーション方針

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | JSON 応答契約テスト（token / refresh / revoke） | 65ea261 `test(handler)` | （直後の docs(tasks)） |
| 2 | native callback Location 契約テスト | `test(handler)`（task 2） | 同上 |
| 3 | Bearer↔Cookie 等価応答の契約テスト | `test(handler)`（task 3） | 同上 |
| 4 | Bearer 拒否 4 区分 uniform 契約テスト | `test(handler)`（task 4） | 同上 |
| 5 | E2E DB-backed full-flow テスト | `test(handler)`（task 5） | 同上 |
| 6 | contract-notes.md（SERVER.md 同期文書） | `docs(specs)`（task 6） | 同上 |
| 7 | 既存テスト非干渉の最終確認 | （diff なし。確認のみ） | `docs(tasks): mark 7 as done` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約は自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: Native OAuth フロー全体の契約検証

| AC ID | 担保テスト |
|---|---|
| 1.1 (callback の auth_code 発行 + アプリスキーム redirect) | `TestE2E_NativeAuthFullFlow_DBBacked` step1〜2（real service 経路 / DB 行検証付き）/ `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` |
| 1.2 (token 交換 + PKCE 検証) | `TestE2E_NativeAuthFullFlow_DBBacked` step3（real PKCE 検証・wrong verifier 拒否含む） |
| 1.3 (refresh rotation) | 同 step4（DB の rotated_at / 同一 family 2 世代を検証） |
| 1.4 (revoke 失効) | 同 step5（204 → refresh 401 → 再 revoke 204 冪等 / families.revoked_at 検証） |
| 1.5 (Bearer で Cookie と同一ユーザー識別) | 同 step6（real JWT の sub = DB users.id を応答 fixture で確認）/ `TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie`（応答 body 等価） |
| 1.6 (外部接続なしで実行可能) | 全テスト httptest 駆動。外部接続は `TEST_DATABASE_URL` の PostgreSQL のみ（OAuthProvider は fake） |

### Requirement 2: JSON 応答契約の検証

| AC ID | 担保テスト |
|---|---|
| 2.1 / 2.3 / 2.4 (token 応答 4 フィールド / Bearer / 900) | `TestContract_TokenResponse_ExactJSONShape`（総キー数 4 の余剰キー検出付き） |
| 2.2 (refresh 応答 4 フィールド) | `TestContract_RefreshResponse_ExactJSONShape` |
| 2.5 (revoke 204 + ボディなし) | `TestContract_RevokeResponse_204AndEmptyBody` |
| 2.6 (callback Location 形式) | `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode`（scheme / host / path / クエリ名 + auth_code 非空 + クエリ 1 個のみ + session Cookie 非発行） |

### Requirement 3: エラー契約の検証

| AC ID | 担保テスト |
|---|---|
| 3.1 (不正・期限切れ・使用済み auth_code 拒否) | `TestE2E_NativeAuthFullFlow_DBBacked` step3-reject（unknown / consumed。期限切れの時刻依存は #166 unit が正本 — NFR 3.1 の非重複方針） |
| 3.2 (PKCE verifier 不一致拒否) | 同 step3-reject（wrong verifier → 400 / 非消費の確認として直後の正規 verifier 交換 200） |
| 3.3 (不明・失効・rotation 済み refresh token 拒否) | 同 step4-reject（unknown / rotated replay / family 失効後の現役 token） |
| 3.4 (再利用検知で family 全滅) | 同 step4-reject（replay → families.revoked_at set → 現役 refresh2 も 401）+ 既存 `TestIntegration_ReuseDetection_FamilyRevoked` |
| 3.5 (Bearer 4 区分拒否 / fallback なし) | `TestContract_BearerToken_RejectionUniformity_AllRejectionShapes`（署名不正 / 期限切れ / token_use 不一致 / 形式不正 + Cookie 併送 fallback 禁止） |
| 3.6 (拒否応答が原因を区別不能) | 同上（全区分の status / body / Content-Type 完全同一比較）+ `e2eAssertRejection`（message の原因区別語 9 種の否定検証） |

### Requirement 4: iOS 仕様との同期確認

| AC ID | 成果物 |
|---|---|
| 4.1 (整合確認結果の記録) | `contract-notes.md`（§1.2〜§1.8 の対照表。結論: 承認済み逸脱 2 件を除き整合） |
| 4.2 (差分の承認済み逸脱明示) | 同「承認済み逸脱」節（根拠 Issue / spec 参照付き） |
| 4.3 (revoke 未認証化の明文化) | 同 逸脱 1（#168 Open Questions / RFC 7009 §2.1 / iOS への影響を記載） |
| 4.4 (DB スキーマ差分の明文化) | 同 逸脱 2（families 分離 / used boolean / device_label 未実装を含む） |
| 4.5 (未承認差分のエスカレーション方針) | 同 末尾節（本確認では未承認差分の発見なし） |

### Non-Functional Requirements

| NFR | 担保 |
|---|---|
| NFR 1.1 (外部接続なし) | OAuthProvider は fake。DB は `TEST_DATABASE_URL` のみ（未到達 skip） |
| NFR 1.2 (時刻依存の決定論的再現) | 本 suite 内の時刻依存検証（Bearer 期限切れ拒否）は **過去時刻 exp を直接固定した crafted token** で決定論化（task 4）。auth_code 60 秒 / refresh 30 日の境界検証は固定 now 注入済みの各 spec unit テストが正本（NFR 3.1 の非重複方針）。詳細は後述「実装上の判断」 |
| NFR 1.3 (固定署名鍵の注入) | `e2eJWTSecret` / 契約テスト各所の固定 secret（production secret ではない） |
| NFR 2.1 (既存 Cookie 動線の挙動不変) | 既存 Cookie 系テスト無変更 green + `TestContract_BearerAccessToken_...` の Cookie 側 200 |
| NFR 2.2 (既存テスト結果不変) | 既存テスト・既存ヘルパーへの変更ゼロ（diff は追加のみ。task 7 で機械確認） |
| NFR 3.1 (既存 spec との非重複) | 各 spec 固有の unit 範囲は再検証せず、横断契約のみ追加（design.md Non-Goals 準拠） |

## 検証結果

- `go build ./...`: 成功 / `go vet ./...`: 警告なし
- `TEST_DATABASE_URL` を設定した実 PostgreSQL 16 で `go test -p 1 ./...` 全 pass
  （`TestE2E_NativeAuthFullFlow_DBBacked` 実行を含む）
- DB 不在環境で `go test ./internal/handler/`: `TestE2E_*` のみ SKIP、他はすべて green
- 既存テスト・既存ヘルパーの diff: 削除・変更行ゼロ（追加のみ）
- `gofmt -l <変更ファイル群>`: 出力なし

## design からの逸脱

1 点のみ（下記）。それ以外の File Structure Plan / コンポーネント構成 / Testing Strategy
1〜7 / contract-notes.md の Structure は design.md どおり。

### E2E テストでの `now` 固定注入は行っていない（NFR 1.2 の充足方法）

design.md は E2E テストで「`now` field を package-private override で固定」と記述しているが、
`auth.JWTIssuer` / `JWTVerifier` / `TokenService` の `now` は **auth パッケージ非公開 field**
であり、handler パッケージのテストからは注入できない（production code に setter を追加する
ことは本 spec の Non-Goals「production code 変更なし」と矛盾する）。

実装では以下の構成で NFR 1.2（時刻依存検証の決定論的再現）を充足した:

- E2E は **時刻境界を検証対象に含めない**（unknown / consumed / wrong-verifier / rotated
  replay はいずれも時刻非依存で決定論的。auth_code 60 秒 TTL 内に全 step が完了する）
- 本 suite 内で唯一時刻依存の「Bearer 期限切れ拒否」（Req 3.5）は、**exp が過去の絶対時刻を
  直接埋め込んだ crafted token** で検証（結果は実行時刻によらず常に拒否 = 決定論的）
- 60 秒 / 30 日 / 900 秒の**境界値**検証は、固定 now 注入を備えた各 spec の unit テスト
  （`jwt_issuer_test.go` / `jwt_verifier_test.go` / `token_service_test.go`）が正本であり、
  本 spec はそれを再検証しない（NFR 3.1）

## 実装上の判断

### wrong verifier 拒否後の正規交換を E2E に含めた

PKCE 検証は `MarkUsed`（単回消費）より前に行われるため、verifier 不一致は auth_code を
消費しない。E2E では wrong verifier 拒否（Req 3.2）の直後に同じ auth_code を正しい
verifier で交換して 200 を確認し、「拒否が状態を変更しない」ことまで通しで固定した
（この交換で生まれる family2 を revoke シナリオに使い、テストを 1 関数に収めた）。

### 契約テストの共有ヘルパー

token / refresh の 4 フィールド厳密検証は `assertTokenPairExactShape` に集約し、
E2E 側にも同等の `e2eDecodeTokenPair` を置いた（mock 経路と real 経路でファイルが分かれる
ため。検証内容は同一）。

## 追加した依存

無し（`lib/pq` の blank import は既存依存。go.mod / go.sum 変更なし）。

## 確認事項（PR 本文転載用）

- production code への変更はゼロ（テスト + 文書のみ）。
- design.md の「E2E での now 固定注入」は auth パッケージ非公開 field のため実施せず、
  時刻依存検証は crafted token（過去時刻 exp の直接固定）と各 spec unit テストへの委譲で
  NFR 1.2 を充足した（上記「design からの逸脱」参照。production code に setter を追加する
  選択肢は Non-Goals と矛盾するため不採用）。
- SERVER.md §1 との整合確認の結果、**承認済み逸脱 2 件以外の未承認差分は発見されなかった**
  （contract-notes.md に確認日付きで記録。Req 4.5 のエスカレーションは不要だった）。

STATUS: complete
