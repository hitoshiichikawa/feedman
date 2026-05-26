# Requirements Document

## Introduction

Google OAuth プロバイダーの外部 HTTP 通信（トークン交換・ユーザー情報取得）が、クライアントレベルの
タイムアウトを持たないため、上流が応答しない場合にリクエストが無期限にハングし得る。これにより
処理リソースが解放されず滞留し、リソース枯渇につながるリスクがある。本機能では、OAuth エンドポイントへの
外部リクエストに明示的なタイムアウトを設け、無応答・応答遅延時に一定時間でエラーとして処理を打ち切る。
OAuth フロー自体のロジックや正常系の挙動は変更しない。

## Requirements

### Requirement 1: OAuth 外部通信のタイムアウト適用

**Objective:** As a 運用者, I want OAuth エンドポイントへの全外部リクエストにタイムアウトが適用されること, so that 上流の無応答によるリクエストの無期限ハングとリソース滞留を防げる

#### Acceptance Criteria

1. When トークン交換リクエストが送出されたとき, the OAuth Provider shall 明示的なタイムアウトを持つ HTTP クライアント経由で送信する
2. When ユーザー情報取得リクエストが送出されたとき, the OAuth Provider shall 明示的なタイムアウトを持つ HTTP クライアント経由で送信する
3. The OAuth Provider shall 外部リクエストのタイムアウトを 10 秒に設定する

### Requirement 2: 無応答・応答遅延時のタイムアウト動作

**Objective:** As a 運用者, I want 上流が応答しない場合にリクエストが一定時間で打ち切られること, so that 処理が滞留し続けずリソースが解放される

#### Acceptance Criteria

1. If 上流がタイムアウト時間を超えて応答しない場合, the OAuth Provider shall リクエストを打ち切りエラーを返す
2. If トークン交換でタイムアウトが発生した場合, the OAuth Provider shall 呼び出し側へエラーを伝播し silent fail にしない
3. If ユーザー情報取得でタイムアウトが発生した場合, the OAuth Provider shall 呼び出し側へエラーを伝播し silent fail にしない
4. While 上流応答がタイムアウト時間内に返らない状態が続くとき, the OAuth Provider shall リクエストを無期限に待機させない

### Requirement 3: 正常系の後方互換性

**Objective:** As a エンドユーザー, I want タイムアウト導入後も既存の OAuth ログインが従来どおり完了すること, so that 認証フローの挙動が変わらず利用を継続できる

#### Acceptance Criteria

1. When 上流がタイムアウト時間内に正常応答したとき, the OAuth Provider shall 従来どおりアクセストークンを取得する
2. When 上流がタイムアウト時間内に正常応答したとき, the OAuth Provider shall 従来どおりユーザー情報（ProviderUserID / Email / Name / Provider）を取得する
3. If 上流が HTTP エラーステータスをタイムアウト時間内に返した場合, the OAuth Provider shall 従来どおりステータスに基づくエラーを返す

## Non-Functional Requirements

### NFR 1: 可用性・リソース保護

1. The OAuth Provider shall 外部リクエストごとに上限 10 秒でリクエストを終了し、それを超える滞留を発生させない
2. If リクエストがタイムアウトで打ち切られた場合, the OAuth Provider shall 当該リクエストに紐づく接続・処理リソースを解放する

### NFR 2: 後方互換

1. The OAuth Provider shall 正常系の OAuth フロー（ログイン URL 生成・トークン交換・ユーザー情報取得）の観測可能な挙動を本機能導入前と同一に保つ
2. The OAuth Provider shall 既存のテスト用エンドポイント上書き（TokenURL / UserInfoURL）を引き続きサポートする

## Out of Scope

- OAuth フロー自体のロジック変更（スコープ・グラントタイプ・パラメータ等）
- リトライ・サーキットブレーカー等の高度な耐障害機構の追加
- SSRF 対策（OAuth エンドポイントは固定の Google URL であり SSRF 対象外）
- トークン交換以外の外部通信（フィード取得・はてなブックマーク連携等）のタイムアウト見直し

## Open Questions

- タイムアウト値は Issue の想定どおり 10 秒で確定とした。環境差（高レイテンシ環境）で調整が必要になる可能性があるが、現時点では固定値で問題ないと判断。可変化（設定値化）が必要なら別 Issue とする。
- タイムアウト付き HTTP クライアントを「関数内生成」か「provider のフィールド注入」かは実装方針（design.md / Architect の領分）のため本要件では規定しない。要件としては「明示的タイムアウトを持つクライアント経由であること」（Requirement 1）のみを課す。
