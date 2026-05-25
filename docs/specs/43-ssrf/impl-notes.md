# 実装メモ: Issue #43 SSRF ブロック対象ホスト名リストの拡充

## 実装サマリ

SSRF 事前検証 `ValidateURL`（`internal/security/ssrf_guard.go`）で使用する
ブロック対象ホスト名リスト `blockedHostnames` を `localhost` 1 件から 6 件に拡充した。

### 変更ファイル

| ファイル | 変更内容 |
|---|---|
| `internal/security/ssrf_guard.go` | `blockedHostnames` を 6 ホスト名（`localhost` / `localhost.localdomain` / `ip6-localhost` / `ip6-loopback` / `metadata.google.internal` / `metadata`）へ拡充。各ホスト名の意図（ループバック別名・メタデータ別名）をコメントで明示。`blockedHostnames` および `isBlockedHostname` の docstring に「ホスト名の静的な完全一致ブロックのみを担い、DNS 解決後の宛先 IP 検証はフェッチ層（`NewSafeClient` の safeurl Dialer フック）の責務である」旨を明記（Requirement 5 対応）。完全一致判定ロジック自体（`lower == blocked`）は不変。 |
| `internal/security/ssrf_guard_test.go` | 拡充ホスト名・部分一致回避・大文字小文字非依存・接尾辞回避の各観点を table-driven / `t.Run` でテスト追加。 |

## 設計判断

### 事前 DNS 解決を導入せず docstring で責務分担を明記した理由

Issue 本文「判断を委ねたい点」の 2 番（事前 DNS 解決 `net.LookupHost` を入れるか／
safeurl 委譲方針を docstring に明記するに留めるか）について、**事前 DNS 解決は導入せず、
責務分担を docstring に明記する方針**を採った。理由は以下:

- **(a)** requirements.md の Out of Scope で「事前検証での DNS 解決の導入有無」「DNS
  リバインディング全般への対策強化」をスコープ外と明示している。
- **(b)** DNS 解決後の宛先 IP 検証はフェッチ層 `NewSafeClient` の safeurl Dialer フック
  （`net.Dialer` の Control フック）が既に担保しており（多層防御の役割分担）、事前検証へ
  二重に DNS 解決を持ち込む必要がない。
- **(c)** `ValidateURL` は docstring（`ssrf_guard.go` 95-99 行）で「DNS 解決を伴わない静的な
  検証」と明記している。事前検証へ DNS 解決を入れると静的検証でなくなり責務が混濁する。

→ Requirement 5（責務分担の明示）に対応するため、`blockedHostnames` / `isBlockedHostname`
の docstring に役割分担を参照可能な形で記述した。

### 完全一致判定の維持

Requirement 4 の要請に従い `isBlockedHostname` の完全一致判定（`lower == blocked`）を
維持した。部分一致・接尾辞一致には変更せず、`localhost.example.com` /
`metadata.example.com` / `notlocalhost` が誤ブロックされないことをテストで担保した。

## AC ↔ テストケース対応表

| Requirement / AC | 検証テスト | 観点 |
|---|---|---|
| Req 1.1 (`localhost`) | `TestValidateURL_BlockedHostnames/http://localhost/feed`（既存 `TestValidateURL_LoopbackAddress` でもカバー） | 異常系 |
| Req 1.2 (`localhost.localdomain`) | `TestValidateURL_BlockedHostnames/http://localhost.localdomain/feed` | 異常系 |
| Req 1.3 (`ip6-localhost`) | `TestValidateURL_BlockedHostnames/http://ip6-localhost/feed` | 異常系 |
| Req 1.4 (`ip6-loopback`) | `TestValidateURL_BlockedHostnames/http://ip6-loopback/feed` | 異常系 |
| Req 1.5 (`metadata.google.internal`) | `TestValidateURL_BlockedHostnames/http://metadata.google.internal/feed` | 異常系 |
| Req 1.6 (`metadata`) | `TestValidateURL_BlockedHostnames/http://metadata/feed` | 異常系 |
| Req 2.1 (`example.com` 通過) | 既存 `TestValidateURL_PublicURL` | 正常系 |
| Req 2.2 (`localhost.example.com` 通過) | `TestValidateURL_BlockedHostnameSubstring/http://localhost.example.com/feed` | 正常系（部分一致回避） |
| Req 2.3 (`metadata.example.com` 通過) | `TestValidateURL_BlockedHostnameSubstring/http://metadata.example.com/feed` | 正常系（部分一致回避） |
| Req 3.1 (`LocalHost` ブロック) | `TestValidateURL_BlockedHostnameCaseInsensitive/http://LocalHost/feed` | 境界値（大文字小文字混在） |
| Req 3.2 (`LOCALHOST` ブロック) | `TestValidateURL_BlockedHostnameCaseInsensitive/http://LOCALHOST/feed` | 境界値（全大文字） |
| Req 4.1 (完全一致判定) | `TestValidateURL_BlockedHostnameSubstring` / `TestValidateURL_BlockedHostnameSuffix` で間接的に担保 | 実装維持 |
| Req 4.2 (`notlocalhost` 通過) | `TestValidateURL_BlockedHostnameSuffix` | 境界値（接尾辞回避） |
| Req 5.1 (責務分担の明示) | `blockedHostnames` / `isBlockedHostname` の docstring に記述（コードレビューで確認可能） | docstring |
| NFR 1.1 (後方互換・正当 URL 通過) | 既存 `TestValidateURL_PublicURL` | 後方互換 |
| NFR 1.2 (後方互換・拒否維持) | 既存 `TestValidateURL_PrivateIP` / `_LoopbackAddress` / `_LinkLocalAddress` / `_MetadataIP` / `_IPv6Loopback` / `_ZeroAddress` | 後方互換 |
| NFR 2.1 (単体テストで検証可能) | 上記追加テスト群全体 | 検証可能性 |

> 補足: Req 4.1（完全一致判定の維持）は実装側の不変条件であり、部分一致・接尾辞ケース
> （Req 2.2 / 2.3 / 4.2）の正常系テストが通過することで「完全一致以外でブロックしない」
> ことを間接的に担保している。Req 5.1 は user/operator-observable な挙動ではなく docstring
> による明示であるため、実行テストではなくコードレビューで確認する。

## 検証コマンドの実行結果

- `gofmt -l internal/security/ssrf_guard.go internal/security/ssrf_guard_test.go`
  → 出力なし（差分なし・整形済み）
- `go vet ./internal/security/...`
  → 出力なし（pass）
- `go test ./internal/security/...`
  → `ok  github.com/hitoshi/feedman/internal/security`（全テスト pass。追加した
    `TestValidateURL_Blocked*` 4 関数および既存テストすべて green）

Red→Green 確認: 実装前に拡充 5 ホスト名（`localhost.localdomain` / `ip6-localhost` /
`ip6-loopback` / `metadata.google.internal` / `metadata`）のブロックテストが FAIL する
ことを観測し、実装後に PASS することを確認した（`localhost` は既存実装で既にブロック済み）。

## 確認事項

なし。requirements.md と Issue 本文の委譲点（事前 DNS 解決の扱い）について矛盾はなく、
Out of Scope の記述に沿って docstring 明記方針で実装した。

STATUS: complete
