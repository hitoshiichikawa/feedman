# Requirements Document

## Introduction

SSRF 対策の事前検証 `ValidateURL`（`internal/security/ssrf_guard.go`）は、URL のスキーム・ホスト・
IP アドレスを静的に検証し、危険な宛先へのリクエスト送信前にエラーを返す多層防御の一層である。
しかしホスト名ベースのブロック判定で用いるブロック対象ホスト名リストが `localhost` 1 件のみで、
ループバック別名やクラウドメタデータエンドポイントのホスト名表記を取りこぼす網羅性の低さがある。
本要件では、事前検証のブロック対象ホスト名リストを拡充し、既存の正当な URL の通過挙動・大文字
小文字非依存判定・完全一致判定（部分一致での過剰ブロック回避）を維持することを定義する。

## Requirements

### Requirement 1: ブロック対象ホスト名の拡充

**Objective:** As a Feedman の運用者, I want ループバック別名やメタデータエンドポイントのホスト名表記を事前検証で確実に拒否してほしい, so that フィード登録経路における内部宛先への意図しないリクエスト送信のリスクを多層防御として下げられる

#### Acceptance Criteria

1. If `localhost` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す
2. If `localhost.localdomain` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す
3. If `ip6-localhost` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す
4. If `ip6-loopback` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す
5. If `metadata.google.internal` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す
6. If `metadata` をホスト名とする URL が事前検証に渡されたとき, the SSRF Guard shall ブロック対象ホストとしてエラーを返す

### Requirement 2: 正当なホスト名の通過維持

**Objective:** As a フィードを登録する利用者, I want 正当な外部フィードのホスト名が従来どおり検証を通過してほしい, so that ブロックリスト拡充によって正常な購読登録が妨げられない

#### Acceptance Criteria

1. When 通常の外部ホスト名（例: `example.com`）の URL が事前検証に渡されたとき, the SSRF Guard shall エラーを返さず検証を通過させる
2. When ブロック対象ホスト名を部分文字列として含むだけの正当なホスト名（例: `localhost.example.com`）の URL が事前検証に渡されたとき, the SSRF Guard shall エラーを返さず検証を通過させる
3. When ブロック対象ホスト名を部分文字列として含むだけの正当なホスト名（例: `metadata.example.com`）の URL が事前検証に渡されたとき, the SSRF Guard shall エラーを返さず検証を通過させる

### Requirement 3: 大文字小文字非依存のブロック判定

**Objective:** As a Feedman の運用者, I want ホスト名の大文字小文字表記に関わらずブロック対象が一貫して拒否されてほしい, so that 表記揺れによる事前検証のすり抜けを防げる

#### Acceptance Criteria

1. If 大文字小文字混在のブロック対象ホスト名（例: `LocalHost`）の URL が事前検証に渡されたとき, the SSRF Guard shall 小文字化した上でブロック対象ホストとしてエラーを返す
2. If 全大文字のブロック対象ホスト名（例: `LOCALHOST`）の URL が事前検証に渡されたとき, the SSRF Guard shall 小文字化した上でブロック対象ホストとしてエラーを返す

### Requirement 4: ブロック判定の完全一致維持

**Objective:** As a 本コードベースの保守担当者, I want ホスト名のブロック判定が完全一致で行われることを維持したい, so that 部分一致導入による正当なホスト名の過剰ブロックを防げる

#### Acceptance Criteria

1. The SSRF Guard shall ブロック対象ホスト名の判定をホスト名全体の完全一致で行う
2. When ブロック対象ホスト名を接尾辞として含む正当なホスト名（例: `notlocalhost`）の URL が事前検証に渡されたとき, the SSRF Guard shall エラーを返さず検証を通過させる

### Requirement 5: 事前検証とフェッチ層の責務分担の明示

**Objective:** As a 本コードベースの保守担当者, I want 事前検証のブロックリストとフェッチ層の検証（IP レンジ・DNS 解決後検証）の責務分担が明示されていてほしい, so that 事前検証の網羅性に過度に依存せず多層防御の前提を誤解しない

#### Acceptance Criteria

1. The SSRF Guard shall 事前検証がホスト名・スキーム・静的 IP の検証を担い、DNS 解決後の宛先 IP 検証はフェッチ層の責務であることを参照可能な形で明示する

## Non-Functional Requirements

### NFR 1: 後方互換性

1. When 本変更導入前に検証を通過していた正当な外部 URL が事前検証に渡されたとき, the SSRF Guard shall 本変更導入前と同一に検証を通過させる
2. When 本変更導入前に拒否されていた URL（`localhost`・プライベート IP・ループバック IP・メタデータ IP 等）が事前検証に渡されたとき, the SSRF Guard shall 本変更導入前と同一に拒否する

### NFR 2: 検証可能性

1. The SSRF Guard shall 拡充後の各ブロック対象ホスト名・大文字小文字混在表記・部分一致回避ケースに対する受理／拒否を単体テストで検証可能な形で提供する

## Out of Scope

- フェッチ層 `safeurl` Dialer フック実装の変更
- IP レンジ（`blockedNetworks` / IP アドレスのブロック判定）の拡充
- DNS リバインディング全般への対策強化
- 事前検証での DNS 解決の導入有無（実装詳細として design 領分に委ねる）
- ブロック対象ホスト名リストの外部設定化・動的更新

## Open Questions

- なし（Issue 本文・既存コメント・既存実装で受入基準を確定できた。Triage 自動コメント以外に人間の追加決定事項なし。Issue 本文「仮案・判断を委ねたい点」の事前 DNS 解決導入有無 / safeurl 委譲方針の docstring 明記は実装者・レビュアー判断とし、requirements では責務分担の明示を Requirement 5 で user/operator-observable な粒度に正規化した）
