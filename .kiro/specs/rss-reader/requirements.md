# Requirements Document

## Project Description (Input)
RSSリーダー Webサービス 要件定義（最終版）

⸻

1. 概要

本サービスは、Web上で利用できるRSS/Atomリーダーである。
ユーザーはRSS/Atomフィードを登録し、2ペインUIで記事一覧と記事内容を閲覧できる。
各記事には外部リンク遷移機能を持ち、はてなブックマーク数を表示する。

⸻

2. 認証・ユーザー登録・セッション

2.1 ユーザー登録
    •    Google OAuthによる登録を採用する
    •    OAuth認証が成功した時点で、未登録ユーザーであればDBにユーザーレコードを自動作成する
    •    ログインと登録を同一フローで処理する（初回OAuth認証＝登録、2回目以降＝ログイン）

2.2 認証
    •    初期はGoogle OAuthでログインする
    •    将来的に他IdP（Microsoft/GitHub等）追加を可能とする
    •    外部アカウント紐付け用テーブルを持つ設計とする

2.2 データ分離
    •    データはユーザー単位で分離する
    •    ユーザー間共有機能は提供しない

2.3 セッション方式
    •    BFFはHTTP Only Cookieセッションを採用
    •    CSRF対策を実装（SameSite=Lax Cookie + CORSポリシー）
    •    Next.jsフロントエンドとGo APIサーバーはクロスオリジン構成。API通信先は`NEXT_PUBLIC_API_URL`環境変数（ビルド時バンドル）で指定し、`credentials: 'include'`でCookie送信

⸻

3. UI/UX

3.1 レイアウト

2ペイン構成

左ペイン
    •    フィード一覧
    •    フィードタイトル表示
    •    favicon表示（存在する場合）

右ペイン
    •    記事一覧

⸻

3.2 記事表示仕様
    •    記事クリックで同ペイン内に内容展開
    •    展開時に既読化
    •    展開は1件のみ（排他的）
    •    元記事URLへの遷移ボタンを表示

⸻

3.3 一覧仕様
    •    並び順：published_at DESC
    •    published_atが無い場合はfetched_atを代用（推定フラグ付与）
    •    無限スクロール
    •    1回取得件数：50件

⸻

3.4 フィルタ

以下3種
    •    全て
    •    未読のみ
    •    スターのみ

⸻

3.5 テーマ
    •    Tailwind + shadcn
    •    ライト/ダーク切替
    •    デフォルトはライト

⸻

4. フィード登録

4.1 入力仕様
    •    入力欄は1つ
    •    URLを入力

4.2 自動判定
    •    RSS/Atom URLなら直接登録
    •    HTML URLなら <head> を解析しRSS/Atom検出
    •    見つからなければエラー

4.3 複数候補時の選択

優先順位
    1.    同一ホスト優先
    2.    Atom優先
    3.    RSS
    4.    同条件なら先頭

登録後はフィードURLをUIに表示し、ユーザーが変更可能

⸻

4.4 favicon
    •    登録時に取得
    •    保存
    •    更新しない
    •    取得失敗時は未表示

⸻

5. 記事データモデル

5.1 同一性判定

優先順位
    1.    (feed_id, guid_or_id)
    2.    (feed_id, link)
    3.    hash(title + published + summary)

⸻

5.2 更新
    •    再取得時は上書き
    •    履歴は保持しない

⸻

5.3 ユーザー状態

各記事に対して
    •    既読（is_read）
    •    スター（is_starred）

APIはトグルではなく明示更新（冪等）

⸻

6. フェッチ処理

6.1 フェッチ間隔

ユーザー設定
    •    最短30分
    •    30分刻み
    •    最大12時間

⸻

6.2 ワーカー
    •    5分ごとに実行
    •    next_fetch_at方式
    •    next_fetch_at <= now の購読を取得
    •    FOR UPDATE SKIP LOCKEDで排他
    •    最大10並列フェッチ

⸻

6.3 HTTPキャッシュ
    •    ETag / Last-Modified を保存
    •    条件付きGET使用

⸻

6.4 フェッチ制限
    •    タイムアウト：10秒
    •    最大レスポンス：5MB

⸻

6.5 リトライと停止

バックオフ戦略採用

停止条件
    •    404 / 410 → 停止
    •    401 / 403 → 停止
    •    パース失敗10回連続 → 停止
    •    429 / 5xx → バックオフ継続

⸻

6.6 手動復帰

ユーザーがUIから「再開」ボタンで復帰

⸻

7. はてなブックマーク
    •    item単位で保持
    •    hatebu_count
    •    hatebu_fetched_at

再取得TTL：24時間

取得失敗時
    •    前回値維持
    •    UIはエラー表示しない

⸻

8. データ制限・ライフサイクル

8.1 購読上限
    •    ユーザーあたり最大100フィード

⸻

8.2 記事保持
    •    180日保持
    •    超過は削除対象

⸻

8.3 削除仕様

購読解除
    •    subscription削除
    •    user item_state削除

退会
    •    user / identities / subscriptions / item_states / settings削除
    •    feeds / itemsは残す（共有キャッシュ）
    •    孤児は将来GC対象

⸻

9. セキュリティ

9.1 SSRF対策
    •    egress制限
    •    プライベートIP拒否
    •    リンクローカル拒否
    •    メタデータIP拒否
    •    登録時とフェッチ時の二重チェック

⸻

9.2 コンテンツサニタイズ

許可タグ
    •    p br a ul ol li blockquote pre code strong em img

制限
    •    imgはhttpsのみ
    •    script iframe style禁止
    •    on*イベント禁止

aタグ
    •    target="_blank"
    •    rel="noopener noreferrer"

⸻

10. レート制限
    •    API：120 req/min/user
    •    登録：10 req/min/user

⸻

11. ログとエラー

11.1 ログ

JSON構造化ログ

出力内容
    •    user_id
    •    feed_id
    •    url
    •    HTTPステータス
    •    処理時間
    •    エラー種別
    •    取り込み件数

保持期間
    •    14日

⸻

11.2 エラー表示

UI表示
    •    原因カテゴリ
    •    対処方法

詳細はログのみ

⸻

12. 技術構成

12.1 アーキテクチャ
    •    フロントエンド + BFF(API)

12.2 言語
    •    API：Go
    •    Worker：Go

12.3 DB
    •    PostgreSQL

12.4 実行環境
    •    Docker Compose 4コンテナ構成（web: Next.js standalone、api: Go APIサーバー、worker: Goワーカー、db: PostgreSQL）

12.5 設定

環境変数で制御
    •    DB接続
    •    OAuth設定
    •    フェッチ制限
    •    並列数
    •    レート制限
    •    はてブTTL

⸻

12.6 DBマイグレーション
    •    マイグレーション管理ツール使用
    •    pg_dump等でバックアップ可能な構成

⸻

12.7 依存ライブラリ方針
    •    Goモジュールは最新安定
    •    breaking changeは検証後反映

⸻

13. 観測性（管理画面なし運用）

最低限のメトリクス
    •    fetch_success_count
    •    fetch_fail_count
    •    parse_fail_count
    •    http_status_count
    •    fetch_latency
    •    items_upserted_count

⸻

14. 非機能要件まとめ
    •    同時フェッチ最大10並列
    •    スケール可能なDBロック方式
    •    冪等API設計
    •    XSS/SSRF対策実装
    •    Cookieセッション + SameSite=Lax + CORS対策
    •    ログ/メトリクスによる運用可視化

## Requirements

### Requirement 1: ユーザー登録・認証とセッション管理
**Objective:** ユーザーとして、Googleアカウントで簡単に登録・ログインしてサービスを利用したい。自分のデータが他のユーザーから保護されることを保証するため。

#### Acceptance Criteria
1. When ユーザーがログイン/登録ボタンをクリックした時, the RSSリーダー shall Google OAuthフローを開始する
2. When OAuth認証が成功し、該当ユーザーがDBに未登録の場合, the RSSリーダー shall ユーザーレコードと外部アカウント紐付け（identities）レコードを自動作成する
3. When OAuth認証が成功し、該当ユーザーがDBに登録済みの場合, the RSSリーダー shall 既存ユーザーとしてログインする
4. When OAuth認証が成功した時, the RSSリーダー shall HTTP Only Cookieベースのセッションを発行する
5. When セッションが発行される時, the RSSリーダー shall SameSite=Lax CookieとCORSポリシーによるCSRF対策を適用する
6. The RSSリーダー shall ユーザーデータをユーザー単位で分離し、他ユーザーのデータにアクセスできないようにする
7. The RSSリーダー shall 外部アカウント紐付け用テーブル（identities）を持ち、将来的に他IdP（Microsoft/GitHub等）を追加可能な構造とする
8. The RSSリーダー shall クロスオリジン構成（Next.js :3000 + API :8080）で動作し、`NEXT_PUBLIC_API_URL`でAPI通信先を指定、`credentials: 'include'`でCookieセッションを送信する

### Requirement 2: フィード登録
**Objective:** ユーザーとして、URLを入力するだけで簡単にRSS/Atomフィードを登録したい。手動でフィードURLを探す手間を省くため。

#### Acceptance Criteria
1. When ユーザーがRSS/AtomフィードのURLを入力した時, the RSSリーダー shall そのフィードを直接登録する
2. When ユーザーがHTML URLを入力した時, the RSSリーダー shall `<head>`タグを解析してRSS/Atomフィードリンクを検出し登録する
3. When 複数のフィード候補が検出された時, the RSSリーダー shall 同一ホスト優先、Atom優先、RSS、同条件なら先頭の優先順位で自動選択する
4. When フィード登録が完了した時, the RSSリーダー shall 選択されたフィードURLをUIに表示し、ユーザーが変更可能とする
5. When フィード登録が完了した時, the RSSリーダー shall フィードのfaviconを取得して保存する（以後更新しない）
6. If HTML URLからRSS/Atomフィードが検出できなかった場合, then the RSSリーダー shall エラーメッセージを表示する
7. If favicon取得に失敗した場合, then the RSSリーダー shall faviconを非表示とする
8. While ユーザーの購読数が100件に達している時, the RSSリーダー shall 新規フィード登録を拒否し、上限に達している旨を通知する

### Requirement 3: 2ペインUI・レイアウト
**Objective:** ユーザーとして、フィード一覧と記事一覧を同時に確認したい。効率的にフィードを閲覧するため。

#### Acceptance Criteria
1. The RSSリーダー shall 左ペインにフィード一覧、右ペインに記事一覧を表示する2ペインレイアウトを提供する
2. The RSSリーダー shall 左ペインにフィードタイトルとfavicon（存在する場合）を表示する
3. When ユーザーが左ペインのフィードを選択した時, the RSSリーダー shall 右ペインに該当フィードの記事一覧を表示する
4. The RSSリーダー shall Tailwind CSSとshadcnコンポーネントを使用してUIを構築する
5. The RSSリーダー shall ライト/ダークテーマの切替機能を提供し、デフォルトはライトとする

### Requirement 4: 記事一覧と表示
**Objective:** ユーザーとして、最新の記事を効率よく閲覧し、元記事にアクセスしたい。情報収集を素早く行うため。

#### Acceptance Criteria
1. The RSSリーダー shall 記事一覧をpublished_atの降順で表示する
2. When 記事にpublished_atが存在しない時, the RSSリーダー shall fetched_atを代用し、推定フラグを付与する
3. The RSSリーダー shall 無限スクロールによる記事一覧の読み込みを提供し、1回の取得件数を50件とする
4. When ユーザーが記事をクリックした時, the RSSリーダー shall 同ペイン内に記事内容を展開表示する
5. When 記事が展開表示された時, the RSSリーダー shall その記事を既読状態にする
6. When 新たな記事が展開された時, the RSSリーダー shall 以前展開されていた記事を閉じる（排他的展開）
7. The RSSリーダー shall 展開された記事に元記事URLへの遷移ボタンを表示する

### Requirement 5: 記事フィルタリング
**Objective:** ユーザーとして、未読記事やスター付き記事を絞り込みたい。重要な記事を効率的に管理するため。

#### Acceptance Criteria
1. The RSSリーダー shall 「全て」「未読のみ」「スターのみ」の3種類のフィルタを提供する
2. When ユーザーが「未読のみ」フィルタを選択した時, the RSSリーダー shall 未読記事のみを表示する
3. When ユーザーが「スターのみ」フィルタを選択した時, the RSSリーダー shall スター付き記事のみを表示する

### Requirement 6: 記事の既読・スター管理
**Objective:** ユーザーとして、記事の既読状態とスターを管理したい。読了状況の把握と重要記事の保存のため。

#### Acceptance Criteria
1. The RSSリーダー shall 各記事に対して既読（is_read）とスター（is_starred）の状態を保持する
2. When ユーザーが既読/未読の状態変更をリクエストした時, the RSSリーダー shall 冪等な明示的更新（トグルではない）で状態を変更する
3. When ユーザーがスターの状態変更をリクエストした時, the RSSリーダー shall 冪等な明示的更新（トグルではない）で状態を変更する

### Requirement 7: 記事の同一性判定と更新
**Objective:** システムとして、フィードの再取得時に記事を正しく識別し、重複登録を防ぎたい。データの整合性を保つため。

#### Acceptance Criteria
1. The RSSリーダー shall 記事の同一性を (feed_id, guid_or_id) を最優先、次に (feed_id, link)、最後に hash(title + published + summary) の優先順位で判定する
2. When フィード再取得時に既存記事と同一と判定された場合, the RSSリーダー shall 既存記事を上書き更新する（履歴は保持しない）

### Requirement 8: フィードフェッチ処理
**Objective:** システムとして、登録フィードの記事を定期的に自動取得したい。ユーザーが常に最新の記事を閲覧できるようにするため。

#### Acceptance Criteria
1. The RSSリーダー shall ワーカーを5分ごとに実行し、next_fetch_at <= 現在時刻の購読を取得する
2. The RSSリーダー shall FOR UPDATE SKIP LOCKEDで排他制御し、最大10並列でフィードをフェッチする
3. The RSSリーダー shall ETag/Last-Modifiedを保存し、条件付きGETを使用する
4. The RSSリーダー shall フェッチのタイムアウトを10秒、最大レスポンスサイズを5MBとする
5. When ユーザーがフェッチ間隔を設定する時, the RSSリーダー shall 最短30分、30分刻み、最大12時間の範囲で設定を受け付ける

### Requirement 9: フェッチのリトライと停止
**Objective:** システムとして、フェッチ失敗時に適切にリトライまたは停止したい。不要なリソース消費を避けつつ、一時的な障害から回復するため。

#### Acceptance Criteria
1. If フェッチが404または410を返した場合, then the RSSリーダー shall そのフィードのフェッチを停止する
2. If フェッチが401または403を返した場合, then the RSSリーダー shall そのフィードのフェッチを停止する
3. If パース失敗が10回連続した場合, then the RSSリーダー shall そのフィードのフェッチを停止する
4. If フェッチが429または5xxを返した場合, then the RSSリーダー shall バックオフ戦略でリトライを継続する
5. When ユーザーが停止中フィードの「再開」ボタンをクリックした時, the RSSリーダー shall フェッチを再開する

### Requirement 10: はてなブックマーク連携
**Objective:** ユーザーとして、各記事のはてなブックマーク数を確認したい。記事の注目度を把握するため。

#### Acceptance Criteria
1. The RSSリーダー shall 各記事にはてなブックマーク数（hatebu_count）とその取得日時（hatebu_fetched_at）を保持する
2. When はてなブックマーク数の取得から24時間が経過した時, the RSSリーダー shall 再取得する
3. If はてなブックマーク数の取得に失敗した場合, then the RSSリーダー shall 前回取得値を維持し、UIにエラー表示しない

### Requirement 11: コンテンツサニタイズ
**Objective:** ユーザーとして、安全に記事コンテンツを閲覧したい。XSS等のセキュリティリスクから保護されるため。

#### Acceptance Criteria
1. The RSSリーダー shall 記事コンテンツに対して許可タグ（p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img）のみを許可するサニタイズ処理を行う
2. The RSSリーダー shall script, iframe, styleタグおよびon*イベント属性を除去する
3. The RSSリーダー shall imgタグのsrcをhttpsスキームのみに制限する
4. The RSSリーダー shall aタグにtarget="_blank"とrel="noopener noreferrer"を付与する

### Requirement 12: SSRF対策
**Objective:** システムとして、サーバーサイドリクエストフォージェリを防止したい。内部ネットワークへの不正アクセスを防ぐため。

#### Acceptance Criteria
1. The RSSリーダー shall フィード登録時とフェッチ時の両方でSSRF対策チェックを実行する
2. The RSSリーダー shall プライベートIPアドレス、リンクローカルアドレス、メタデータIPアドレスへのリクエストを拒否する
3. The RSSリーダー shall egress制限を適用する

### Requirement 13: レート制限
**Objective:** システムとして、過剰なリクエストからサービスを保護したい。安定したサービス運用を維持するため。

#### Acceptance Criteria
1. The RSSリーダー shall API全般に対して120リクエスト/分/ユーザーのレート制限を適用する
2. The RSSリーダー shall フィード登録APIに対して10リクエスト/分/ユーザーのレート制限を適用する

### Requirement 14: データライフサイクル管理
**Objective:** システムとして、データの保持期間と削除を適切に管理したい。ストレージの効率的な利用とユーザーの権利を保護するため。

#### Acceptance Criteria
1. The RSSリーダー shall 記事データを180日間保持し、超過分を削除対象とする
2. When ユーザーが購読を解除した時, the RSSリーダー shall subscriptionとuser item_stateを削除する
3. When ユーザーが退会した時, the RSSリーダー shall user、identities、subscriptions、item_states、settingsを削除する
4. When ユーザーが退会した時, the RSSリーダー shall feeds、itemsデータは共有キャッシュとして残す（孤児データは将来GC対象）

### Requirement 15: ログとメトリクス
**Objective:** 運用者として、サービスの状態を可視化し問題を迅速に検知したい。管理画面なしでも安定した運用を実現するため。

#### Acceptance Criteria
1. The RSSリーダー shall JSON構造化ログを出力し、user_id、feed_id、url、HTTPステータス、処理時間、エラー種別、取り込み件数を含める
2. The RSSリーダー shall ログを14日間保持する
3. The RSSリーダー shall fetch_success_count、fetch_fail_count、parse_fail_count、http_status_count、fetch_latency、items_upserted_countのメトリクスを提供する
4. When UIにエラーを表示する時, the RSSリーダー shall 原因カテゴリと対処方法を提示し、詳細はログのみに記録する

### Requirement 16: 環境設定と構成管理
**Objective:** 運用者として、環境変数でサービスの動作を制御したい。柔軟なデプロイと運用管理のため。

#### Acceptance Criteria
1. The RSSリーダー shall DB接続、OAuth設定、フェッチ制限、並列数、レート制限、はてブTTLを環境変数で制御可能とする
2. The RSSリーダー shall Docker Compose 4コンテナ構成（web、api、worker、db）で動作する
3. The RSSリーダー shall DBマイグレーション管理ツールを使用し、pg_dump等でバックアップ可能な構成とする
