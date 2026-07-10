# Requirements Document

## Introduction

ユーザーがフィード URL を登録しようとした際、対象サイト側がボット保護や一時的なサーバ
エラーで非 2xx レスポンスを返したケースで、現状の Feed Detector はステータスコードを
確認せずにレスポンスボディを HTML としてパースしている。その結果、エラー画面の HTML
に `<link rel="alternate" type="application/rss+xml">` が含まれないため「指定された URL
から RSS/Atom フィードを検出できませんでした」というメッセージが返り、ユーザーには
「フィードが存在しない」と誤認させる UX 不良が発生している（例: Issue #153 の
`https://www.wtwco.com/ja-jp/search/getsearchrssfeed?type=Insight` は Vercel の
ボット保護で HTTP 429 を返すが、現状は「フィード未検出」として失敗する）。

本仕様はボット保護の回避を目的とせず、検出処理が HTTP レスポンスのステータスコードを
正しく解釈し、HTTP エラー時には「フィードが存在しない」とは別の原因カテゴリで失敗を
返して、ユーザーが次の手（別 URL を試す・時間をおいて再試行する・サイト側の制約を
理解する）を打てるようにすることをスコープとする。

## Requirements

### Requirement 1: HTTP エラーレスポンスの正しい識別

**Objective:** As a フィードを登録しようとするユーザー, I want 検出処理が HTTP エラーレスポンスを「フィード未検出」と区別して扱うこと, so that 失敗の原因が「サイトがアクセスを拒否した」のか「サイトはアクセスできたがフィードが存在しない」のかを判別できる

#### Acceptance Criteria

1. When the Feed Detector が検出対象 URL に対する HTTP レスポンスを受信した時, the Feed Detector shall レスポンスのステータスコードを判定対象として扱う
2. When the Feed Detector が 2xx 系のステータスコードを受信した時, the Feed Detector shall 既存の Content-Type / ボディ解析フローを継続する
3. If the Feed Detector が 4xx 系または 5xx 系のステータスコードを受信した場合, the Feed Detector shall レスポンスボディを HTML としてパースせず、HTTP エラー由来の固有エラーとして失敗を返す
4. If the Feed Detector が 3xx 系のステータスコードを最終応答として受信した場合（HTTP クライアントのリダイレクト追跡後にも 3xx が残る場合）, the Feed Detector shall HTTP エラー由来の固有エラーとして失敗を返す
5. The Feed Detector shall HTTP エラー由来の失敗と「2xx だが HTML 内にフィードリンクが無い失敗」を別エラーコードで区別する

### Requirement 2: ユーザーが次に取るべきアクションを示すエラーメッセージ

**Objective:** As a フィードを登録しようとするユーザー, I want 失敗時のエラーメッセージから次の手が読み取れること, so that 不要な再試行や問い合わせを避けて自力で解決できる

#### Acceptance Criteria

1. When 検出処理が HTTP エラー由来の失敗を返す時, the Feed Registration UI shall 「サイトがアクセスをブロックしている可能性がある」「URL が間違っている可能性がある」「一時的にサーバが応答しない可能性がある」のいずれか該当する原因示唆と、ユーザーが取れる対処（別 URL を試す / 時間を置いて再試行する 等）を含むメッセージを表示する
2. When 検出処理が「2xx 応答だが HTML 内にフィードリンクが見つからなかった」失敗を返す時, the Feed Registration UI shall 既存の「フィード未検出」相当のメッセージ（フィードが存在しない可能性を示唆する内容）を継続して表示する
3. The エラーメッセージ shall 内部スタックトレースや HTTP レスポンスボディの抜粋などユーザーが対処に使えない情報を露出しない
4. Where 失敗原因に紐付く受信ステータスコードが利用可能な場合, the エラーメッセージ shall ステータスコード（例: 429, 404, 503）をユーザー向けに付記する

### Requirement 3: 既存の正常系・既存エラー分類との互換性維持

**Objective:** As a 既に Feedman を運用している運用者, I want 本変更によって今まで成功していた登録フローが壊れないこと, so that 既存ユーザーの登録済みフィードや今後の新規登録が回帰しない

#### Acceptance Criteria

1. When 検出対象 URL が 200 で RSS/Atom 固有 Content-Type を返す時, the Feed Detector shall 本変更前と同じ結果（当該 URL をフィード URL として返す）を返す
2. When 検出対象 URL が 200 で HTML を返し、HTML 内に有効な `rel="alternate"` フィードリンクを含む時, the Feed Detector shall 本変更前と同じ優先順位ロジックで最適なフィード URL を返す
3. If 検出処理中に HTTP クライアント自体がネットワーク到達不能・タイムアウト・SSRF 拒否などでレスポンスを取得できなかった場合, the Feed Detector shall HTTP エラー由来の固有エラーではなく、既存のネットワーク失敗系エラー（ステータスコード受信前の失敗）として扱う
4. The Feed Detector shall 本変更によって新たな外部依存（ヘッドレスブラウザ・ボット保護回避ライブラリ等）を追加しない

### Requirement 4: 境界・異常系の確定的な扱い

**Objective:** As a 本機能を保守する開発者, I want ステータスコード境界・空ボディ・Content-Type 欠落などの異常系挙動が一意に決まること, so that 将来の変更で挙動が暗黙に崩れない

#### Acceptance Criteria

1. When 検出対象 URL が 200 だがレスポンスボディが空の時, the Feed Detector shall 「2xx 応答だが HTML 内にフィードリンクが見つからなかった」失敗として扱う
2. When 検出対象 URL が 200 で Content-Type ヘッダが欠落しているがボディが RSS/Atom XML として有効な時, the Feed Detector shall 当該 URL をフィード URL として返す（既存の XML 判定ロジックを継続適用する）
3. If 検出対象 URL が 200 で Content-Type も HTML でなく XML でもなく、ボディからもフィードと判定できない場合, the Feed Detector shall 「2xx 応答だが HTML 内にフィードリンクが見つからなかった」失敗として扱う
4. When 検出対象 URL が 4xx でレスポンスボディに HTML 形式のフィードリンクが含まれている時, the Feed Detector shall HTML パースを実行せず HTTP エラー由来の固有エラーとして失敗を返す（4xx 応答内のリンクは信頼しない）

## Non-Functional Requirements

### NFR 1: 互換性・既存挙動の温存

1. The Feed Detector shall 2xx 応答に対する既存の検出結果（成功 URL / 失敗エラーコード）を本変更前と同一に保つ
2. The Feed Detector shall 本変更後も 1 リクエストあたりの読み込みサイズ上限（既存の 5 MB 制限）を超えないこと
3. The Feed Detector shall 本変更後も 1 リクエストあたりのタイムアウト（既存の 10 秒）を延長しないこと

### NFR 2: 観測性

1. When 検出処理が HTTP エラー由来の失敗を返す時, the Feed Detector shall 受信したステータスコードと対象 URL（PII を含まない範囲）をログ出力し、運用者が頻発するブロッキング先を特定できるようにする

### NFR 3: セキュリティ

1. The Feed Detector shall 本変更を理由にユーザーエージェント偽装・JS チャレンジ解決・Cookie 永続化などボット保護を回避する挙動を追加しないこと

## Out of Scope

- Vercel / Cloudflare 等のボット保護を回避してフィードを取得すること（技術的・倫理的に対象外）
- HTML のメタタグや `<link>` 以外のヒューリスティクス（サイトマップ走査・推測 URL `/feed`・`/rss` の試行など）による検出強化
- フィード取得（Worker 側の定期フェッチ処理）に対する同様の HTTP エラー扱い改善（本 Issue は登録時の検出フローのみが対象。Worker 側の変更は別 Issue で扱う想定）
- 5xx 系の一過性エラーに対する自動リトライ（必要なら別 Issue で検討）
- Issue #153 で報告された特定 URL（`wtwco.com`）を登録可能にすること自体

## Open Questions

- HTTP エラー応答の細分化粒度: 4xx と 5xx で別エラーコード（さらに 403 / 404 / 429 を個別カテゴリ化するか）を持つか、まとめて 1 つの「HTTP エラー応答」カテゴリとしてステータスコードを付帯情報で表現するか。ユーザー向けメッセージの出し分け要件（Req 2.1）を実現できる粒度であれば実装は自由としてよいか
- 既存の「ネットワーク到達不能（レスポンス取得前の失敗）」と新カテゴリ「HTTP エラー応答（レスポンス取得後の非 2xx）」の境界をユーザー向けメッセージ上でも区別する必要があるか、内部分類のみで十分か
- 3xx 最終応答が実運用上どの程度発生するか（HTTP クライアントが既定でリダイレクトを追跡する場合、最終応答に 3xx が残るのは Location ヘッダ欠落など限定ケース）。Req 1.4 を独立 AC として扱うか、HTTP エラー応答に統合するかの判断材料
