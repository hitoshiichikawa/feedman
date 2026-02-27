# Research & Design Decisions

---
**Purpose**: rss-reader機能のディスカバリーフェーズで得られた調査結果、アーキテクチャ検討、設計判断の根拠を記録する。

**Usage**:
- ディスカバリーフェーズでの調査活動と結果を記録
- design.mdに記載するには詳細すぎるトレードオフを文書化
- 将来の監査や再利用のための参照と根拠を提供
---

## Summary
- **Feature**: `rss-reader`
- **Discovery Scope**: New Feature（グリーンフィールド）
- **Key Findings**:
  - Go標準ライブラリ`log/slog`がJSON構造化ログの標準選択肢として成熟している
  - `gofeed`ライブラリがRSS/Atom両対応の統一パーサーとして最も実績がある
  - はてなブックマークAPIは認証不要で、最大50URLの一括取得に対応している

## Research Log

### RSSフィードパーサーライブラリ選定
- **Context**: RSS/Atomの両フォーマットに対応するパーサーが必要
- **Sources Consulted**:
  - [mmcdole/gofeed - GitHub](https://github.com/mmcdole/gofeed)
  - [gofeed - Go Packages](https://pkg.go.dev/github.com/mmcdole/gofeed)
- **Findings**:
  - `gofeed`は3段階（検出→パース→変換）で動作し、RSS/Atom/JSONフィードを統一モデルに変換する
  - 壊れたXMLフィードに対してもベストエフォートでパースを試みる
  - Dublin Core、iTunes等の拡張に対応
  - カスタムTranslatorによる変換ロジックのカスタマイズが可能
- **Implications**: `gofeed.Parser`を統一パーサーとして採用し、`gofeed.Feed`モデルからドメインモデルへの変換レイヤーを設計する

### Goウェブフレームワーク選定
- **Context**: BFF（API）サーバーのフレームワーク選定
- **Sources Consulted**:
  - [Go Web Frameworks Comparison 2026](https://www.techedubyte.com/go-web-frameworks-2026-gin-fiber-echo-chi-beego-comparison/)
  - [Best Go Backend Frameworks 2026 - Encore](https://encore.dev/articles/best-go-backend-frameworks)
- **Findings**:
  - **Chi**: 標準`net/http`互換のルーターで、ミドルウェアチェーンが柔軟。マイクロサービスで広く使用される
  - **Echo**: 型安全性が高く、エンタープライズ向け。`context.Context`を使用し、パニックではなくエラーを返す設計
  - **Gin**: 最も人気（Go開発者の48%が使用）だが、独自コンテキストの使用が標準ライブラリとの乖離を生む
- **Implications**: Chiはnet/http標準との互換性が高く、BFFパターンに適している。ミドルウェアの組み合わせが柔軟で、将来の拡張にも対応しやすい

### はてなブックマークAPI仕様
- **Context**: 記事ごとのはてなブックマーク数を取得する機能の実装
- **Sources Consulted**:
  - [はてなブックマーク件数取得API](https://developer.hatena.ne.jp/ja/documents/bookmark/apis/getcount)
  - [はてなブックマークAPI プログラミング解説](https://so-zou.jp/web-app/tech/web-api/hatena/entry/)
- **Findings**:
  - 単一URL: `GET https://bookmark.hatenaapis.com/count/entry?url={encoded_url}` → 数値を返却
  - 複数URL: `GET https://bookmark.hatenaapis.com/count/entries?url={url1}&url={url2}` → JSONオブジェクトを返却
  - 最大50URLまで一括取得可能（超過時は414エラー）
  - ブックマーク数0の場合は空データが返る（数値0ではない）
  - 認証不要、明確なレート制限はないが数秒の間隔を推奨
- **Implications**: バッチ取得（最大50URL）を活用してAPI呼び出し回数を削減する。空レスポンスのハンドリングが必要

### HTMLサニタイザーライブラリ
- **Context**: XSS対策としてフィードコンテンツのサニタイズが必要
- **Sources Consulted**:
  - [microcosm-cc/bluemonday - GitHub](https://github.com/microcosm-cc/bluemonday)
  - [bluemonday - Go Packages](https://pkg.go.dev/github.com/microcosm-cc/bluemonday)
- **Findings**:
  - OWASPガイドラインに準拠したHTMLサニタイザー
  - 許可リストベースのポリシー設定が可能
  - script/style要素はデフォルトで除去
  - 高速なトークンベースパーサー（`net/html`ベース）
  - 本番環境での実績あり
- **Implications**: 要件で指定された許可タグ（p, br, a等）のカスタムポリシーを定義して使用する

### SSRF対策ライブラリ
- **Context**: フィード登録時・フェッチ時のSSRF防止
- **Sources Consulted**:
  - [doyensec/safeurl - GitHub](https://github.com/doyensec/safeurl)
  - [daenney/ssrf - GitHub](https://github.com/daenney/ssrf)
  - [OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- **Findings**:
  - `safeurl`: net/httpクライアントの直接置き換えが可能。RFC1918準拠のプライベートIP拒否がデフォルト
  - `daenney/ssrf`: IANA Special Purpose Registriesと自動同期。`net.Dialer`にフックして使用
  - DNS再バインディング攻撃への対策も必要
- **Implications**: `safeurl`はHTTPクライアントの直接置き換えとして使えるため、フィードフェッチャーに組み込みやすい

### レート制限の実装方式
- **Context**: ユーザーごとのAPIレート制限の実装
- **Sources Consulted**:
  - [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)
  - [Rate limiting your Go application - LogRocket](https://blog.logrocket.com/rate-limiting-go-application/)
- **Findings**:
  - `golang.org/x/time/rate`: 標準ライブラリのトークンバケットアルゴリズム実装。スレッドセーフ
  - ユーザーごとのリミッターマップを管理し、定期的に期限切れエントリをクリーンアップする方式が一般的
  - ミドルウェアパターンで実装し、HTTPハンドラの前段に配置
- **Implications**: インメモリのトークンバケット方式で十分。将来的にRedisベースへの移行も可能な設計とする

### セッション管理
- **Context**: BFFのCookieベースセッション管理
- **Sources Consulted**:
  - [gorilla/sessions - GitHub](https://github.com/gorilla/sessions)
  - [gorilla/sessions - Go Packages](https://pkg.go.dev/github.com/gorilla/sessions)
- **Findings**:
  - 署名付き・暗号化Cookieのセッション管理
  - ファイルシステム、データベース等のカスタムバックエンドに対応
  - キーローテーション機能あり
  - SameSite属性の設定が可能
- **Implications**: gorilla/sessionsをセッションストアとして使用。セッションデータはDB保存も検討可能

### データベースマイグレーション
- **Context**: PostgreSQLのスキーマ管理ツール選定
- **Sources Consulted**:
  - [golang-migrate/migrate - GitHub](https://github.com/golang-migrate/migrate)
  - [Database migrations in Go with golang-migrate - Better Stack](https://betterstack.com/community/guides/scaling-go/golang-migrate/)
- **Findings**:
  - CLIとGoライブラリの両方で利用可能
  - up/downペアのSQLファイルでマイグレーション管理
  - PostgreSQLのDDLトランザクションをサポート
  - タイムスタンプベースのバージョニングが推奨
- **Implications**: `golang-migrate`を採用し、タイムスタンプベースのマイグレーションファイルで管理する

### 構造化ログ
- **Context**: JSON構造化ログの実装方式
- **Sources Consulted**:
  - [Logging in Go with Slog - Better Stack](https://betterstack.com/community/guides/logging/logging-in-go/)
  - [Go公式ブログ - Structured Logging with slog](https://go.dev/blog/slog)
- **Findings**:
  - Go 1.21で標準ライブラリに`log/slog`が追加
  - JSONHandlerでJSON形式のログ出力が可能
  - キー・バリューペアによる構造化ログ
  - ハンドラーの差し替えが容易（テスト時のモック等）
  - zerologは最高性能だが、slogは外部依存なし
- **Implications**: 標準ライブラリの`log/slog`を採用。外部依存を最小化し、将来的なハンドラー差し替えも容易

### フロントエンド技術スタック
- **Context**: React + Tailwind + shadcnによるフロントエンド構築
- **Sources Consulted**:
  - [shadcn/ui 公式サイト](https://ui.shadcn.com/)
  - [Next.js BFF Architecture](https://nextjs.org/docs/app/guides/backend-for-frontend)
- **Findings**:
  - shadcn/uiはコンポーネントをコピー&ペーストして使用する方式（npmパッケージではない）
  - Radix UIプリミティブとTailwind CSSベース
  - Next.js App Routerと組み合わせてBFFパターンを実現可能
  - ダークモードはTailwindの`dark:`クラスで対応
- **Implications**: Next.js App RouterをBFFレイヤーとして使用するか、Go側でBFFを実装するかの選択が必要。要件ではAPI/WorkerがGoと指定されているため、Go側でBFF APIを提供し、フロントエンドはNext.jsのSPAとして構築する

### メトリクス収集
- **Context**: Prometheusメトリクスの実装
- **Sources Consulted**:
  - [prometheus/client_golang](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus/promhttp)
  - [go-chi/metrics](https://github.com/go-chi/metrics)
- **Findings**:
  - Counter、Gauge、Histogram、Summaryの4種のメトリクスタイプ
  - ラベルのカーディナリティ管理が重要（user_idはラベルにしない）
  - ミドルウェアパターンでHTTPメトリクスを自動収集可能
  - `/metrics`エンドポイントでPrometheusスクレイプに対応
- **Implications**: 要件のメトリクス（fetch_success_count等）はカスタムメトリクスとして定義。HTTPメトリクスはミドルウェアで自動収集

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| クリーンアーキテクチャ | ドメイン中心の層構造。外部依存を内側に向けない | テスト容易性、ドメインの独立性、技術交換の柔軟性 | 初期のボイラープレートが多い、小規模では過剰 | Go + BFFの構成に適合 |
| レイヤードアーキテクチャ | Handler→Service→Repository の3層構造 | シンプルで理解しやすい、Goプロジェクトで広く採用 | ドメインロジックがサービス層に集中しやすい | 本プロジェクトの規模に適切 |
| ヘキサゴナルアーキテクチャ | ポートとアダプタによる依存の反転 | 高いテスト容易性、外部サービスの差し替えが容易 | 学習コストが高い、アダプタ層の増加 | クリーンアーキテクチャと類似 |

## Design Decisions

### Decision: Webフレームワークとして Chi を採用
- **Context**: BFF APIサーバーのルーティング・ミドルウェア基盤が必要
- **Alternatives Considered**:
  1. Gin — 最も人気だが独自コンテキストがnet/http標準と乖離
  2. Echo — 高機能だが学習コストが高い
  3. Chi — net/http標準互換のルーター
- **Selected Approach**: Chiをルーターとして採用
- **Rationale**: net/http標準との完全互換性があり、ミドルウェアチェーンが柔軟。BFFパターンでのカスタムミドルウェア（認証、CSRF、レート制限）の組み合わせに最適
- **Trade-offs**: Gin/Echoほどの組み込み機能はないが、標準ライブラリとの一貫性を優先
- **Follow-up**: ミドルウェアスタックの順序設計を実装時に検証

### Decision: レイヤードアーキテクチャの採用
- **Context**: アプリケーション全体のアーキテクチャパターンの選定
- **Alternatives Considered**:
  1. クリーンアーキテクチャ — ドメイン中心だが初期コストが高い
  2. ヘキサゴナルアーキテクチャ — アダプタ層が増える
  3. レイヤードアーキテクチャ — シンプルな3層構造
- **Selected Approach**: Handler → Service → Repository の3層構造を採用
- **Rationale**: プロジェクトの規模に適切で、Goコミュニティで広く採用されているパターン。サービス層にドメインロジックを集約し、リポジトリ層でデータアクセスを抽象化する
- **Trade-offs**: クリーンアーキテクチャほどのドメイン独立性はないが、開発速度と理解しやすさを優先
- **Follow-up**: サービス層が肥大化した場合はドメインサービスの分離を検討

### Decision: フロントエンドにNext.js（SPA）を採用
- **Context**: フロントエンドフレームワークの選定。要件でTailwind + shadcnが指定されている
- **Alternatives Considered**:
  1. Next.js（App Router + BFF） — SSR/SSG対応、APIルートでBFF実装可能
  2. React SPA + Vite — シンプルだがBFFレイヤーが別途必要
  3. Next.js（Pages Router） — 旧式だが安定
- **Selected Approach**: Next.js App Routerを採用。Go側がBFF APIを提供し、Next.jsはフロントエンドとしてGo APIを呼び出す
- **Rationale**: 要件でAPI/WorkerがGoと指定されているため、GoがBFFの役割を担う。Next.jsはReactのレンダリングフレームワークとして、shadcn/uiとの統合に最適
- **Trade-offs**: BFFロジックがGo側に集中するため、フロントエンドは純粋なSPAに近い動作となる
- **Follow-up**: SSRの必要性は初期フェーズでは不要と判断。SEOが必要な場合は再検討

### Decision: 構造化ログにlog/slogを採用
- **Context**: JSON構造化ログの実装ライブラリ選定
- **Alternatives Considered**:
  1. zerolog — 最高性能だが外部依存
  2. zap — 高性能だが設定が複雑
  3. log/slog — 標準ライブラリ、Go 1.21+
- **Selected Approach**: 標準ライブラリの`log/slog`を採用
- **Rationale**: 外部依存なし、ハンドラーの差し替えが容易、Go標準として長期サポートが保証される
- **Trade-offs**: zerologほどの極限的性能はないが、本プロジェクトの規模では十分
- **Follow-up**: 性能がボトルネックになった場合はslogハンドラーとしてzerologを使用可能

### Decision: SSRF対策にsafeurlを採用
- **Context**: フィード登録・フェッチ時のSSRF防止
- **Alternatives Considered**:
  1. safeurl — HTTPクライアントの直接置き換え
  2. daenney/ssrf — net.Dialerへのフック
  3. 自前実装 — IP検証ロジックを自作
- **Selected Approach**: `doyensec/safeurl`を採用
- **Rationale**: net/httpクライアントのドロップイン置き換えとして使用でき、RFC1918準拠のプライベートIP拒否がデフォルト。DNS再バインディング攻撃への対策も内蔵
- **Trade-offs**: 外部依存が増えるが、セキュリティ面での自前実装リスクを回避
- **Follow-up**: safeurl設定のカスタマイズ（許可ポート、スキーム等）を実装時に検証

## Risks & Mitigations
- **フィードパース失敗の多発** — gofeedのベストエフォートパースに加え、カスタムエラーハンドリングで連続失敗を検出し停止する仕組みを実装
- **はてなブックマークAPIの可用性** — レート制限が明確でないため、呼び出し間隔を保守的に設定。失敗時は前回値を維持するフォールバック設計
- **セッションハイジャック** — HTTP Only Cookie + SameSite属性 + CSRFトークンの三重防御。Secure属性も本番環境で有効化
- **ワーカーの排他制御** — FOR UPDATE SKIP LOCKEDによるPostgreSQLレベルの排他制御。デッドロック回避のためタイムアウトを設定
- **メモリリーク（レートリミッター）** — ユーザーごとのインメモリリミッターは定期クリーンアップで管理。将来的にはRedisベースへ移行可能

## References
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed) — RSS/Atom/JSONフィードパーサー
- [go-chi/chi](https://github.com/go-chi/chi) — 軽量HTTPルーター
- [gorilla/sessions](https://github.com/gorilla/sessions) — Cookieセッション管理
- [microcosm-cc/bluemonday](https://github.com/microcosm-cc/bluemonday) — HTMLサニタイザー
- [doyensec/safeurl](https://github.com/doyensec/safeurl) — SSRF防止HTTPクライアント
- [golang-migrate/migrate](https://github.com/golang-migrate/migrate) — DBマイグレーション
- [はてなブックマーク件数取得API](https://developer.hatena.ne.jp/ja/documents/bookmark/apis/getcount) — ブックマーク数取得
- [prometheus/client_golang](https://github.com/prometheus/client_golang) — Prometheusメトリクス
- [shadcn/ui](https://ui.shadcn.com/) — UIコンポーネント
