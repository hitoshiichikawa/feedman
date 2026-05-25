# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-43-impl-ssrf
- HEAD commit: 1cf614baeeb63738d58f40e8bf235cd81a9bec0c（実装+テストは 2fd2074、docs は 1cf614b）
- Compared to: develop..HEAD
- 変更ファイル: `internal/security/ssrf_guard.go` / `internal/security/ssrf_guard_test.go` / `docs/specs/43-ssrf/`

## Verified Requirements

- 1.1 — `blockedHostnames` に `localhost`（ssrf_guard.go:165）／`TestValidateURL_BlockedHostnames/http://localhost/feed`
- 1.2 — `localhost.localdomain`（ssrf_guard.go:166）／同 `TestValidateURL_BlockedHostnames` ケース
- 1.3 — `ip6-localhost`（ssrf_guard.go:167）／同ケース
- 1.4 — `ip6-loopback`（ssrf_guard.go:168）／同ケース
- 1.5 — `metadata.google.internal`（ssrf_guard.go:170）／同ケース
- 1.6 — `metadata`（ssrf_guard.go:171）／同ケース
- 2.1 — `ValidateURL` の静的検証で外部ホストは通過、既存 `TestValidateURL_PublicURL`（`example.com` 系）
- 2.2 — 完全一致のため `localhost.example.com` は通過、`TestValidateURL_BlockedHostnameSubstring/http://localhost.example.com/feed`
- 2.3 — `metadata.example.com` は通過、`TestValidateURL_BlockedHostnameSubstring/http://metadata.example.com/feed`
- 3.1 — `isBlockedHostname` が `strings.ToLower` で小文字化後に比較（ssrf_guard.go:181）／`TestValidateURL_BlockedHostnameCaseInsensitive/http://LocalHost/feed`
- 3.2 — 同上ロジック／`.../http://LOCALHOST/feed`
- 4.1 — 完全一致判定 `lower == blocked`（ssrf_guard.go:183）を不変維持
- 4.2 — 接尾辞 `notlocalhost` は通過、`TestValidateURL_BlockedHostnameSuffix`
- 5.1 — `blockedHostnames`（ssrf_guard.go:159-162）／`isBlockedHostname`（ssrf_guard.go:174-179）の docstring に「静的な完全一致ブロックのみを担い、DNS 解決後の宛先 IP 検証はフェッチ層（safeurl Dialer フック）の責務」と参照可能な形で明示。事前 DNS 解決を導入せず docstring 明記とする判断は Issue で実装者・レビュアーに委ねられた点であり Out of Scope と整合
- NFR 1.1 — 完全一致ロジック不変・既存 `TestValidateURL_PublicURL` 維持で後方互換（正当 URL 通過）
- NFR 1.2 — `blockedNetworks` 不変・既存 `_PrivateIP` / `_LoopbackAddress` / `_LinkLocalAddress` / `_MetadataIP` / `_IPv6Loopback` / `_ZeroAddress` 維持で後方互換（拒否維持）
- NFR 2.1 — 追加した `TestValidateURL_Blocked*` 4 関数で受理／拒否を単体テスト検証可能

## Findings

なし

## Summary

Feature Flag Protocol は opt-out のため flag 観点は適用せず通常 3 カテゴリで判定。Req 1〜5・NFR 1〜2 のすべてに対応する実装・テストが diff または既存コードに存在し、`go test ./internal/security/...` は green。変更は `internal/security/ssrf_guard.go` とその近傍テストに閉じ、Out of Scope（safeurl Dialer・IP レンジ・DNS リバインディング）への逸脱なし。完全一致判定（`lower == blocked`）も維持されている。

RESULT: approve
