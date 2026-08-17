# Requirements Document

## Introduction

Feedman は Google OAuth / パスキー / native ユーザー名認証を提供している（Issue #216 で
パスキー認証を追加）が、ユーザーがパスキーだけで登録し、かつ全てのパスキー credential を
喪失した場合、現状ではアカウントに再アクセスする手段が存在しない。#242 で 2 つ目以降の
パスキー追加登録の Web 導線は提供されるものの、複数登録前に唯一の端末を喪失するケースや
同期先を含めた全端末を同時喪失するケースの救済策は未提供である。App Store Review
Guideline 4.8 の姿勢（メール等の追加個人情報の収集を強制しない）を維持したまま、
GitHub 2FA リカバリコードと同型のリカバリコード方式で復旧手段を追加する必要がある。

本 spec は (1) 認証済みユーザーが任意にリカバリコード一式を発行する導線、(2) 発行時のみ
生値を 1 度だけ表示する提示規則、(3) 未発行ユーザーへの UI 上のリマインド、(4) 全パスキー
喪失時にリカバリコードで復旧セッションを確立する導線、(5) 復旧成功時に既存 credential・
セッションを全失効させ新パスキー登録を強制する乗っ取り耐性、(6) 再生成での旧一式全無効化、
(7) 復旧エンドポイントへの IP レート制限、(8) 生値をサーバ・ログ・レスポンスに残さない
セキュリティ規約、をユーザー・運用者から観察可能な形で要件化する。iOS 側 UI、パスキー
credential 管理 UI（一覧・削除・リネーム）、メール送信基盤・メールによるリカバリは本 spec の
スコープ外とする（Out of Scope 参照）。

人間運用者が Issue #243 コメントで確定した 2 つの決定事項（発行タイミング / 復旧成功後の
credential 処理方針）は本 spec の前提として組み込み済みで、Open Questions からは除外する。

## Requirements

### Requirement 1: リカバリコード発行と一度限りの全文表示

**Objective:** As a 認証済み Feedman ユーザー, I want アカウント設定からリカバリコード一式を
任意発行して安全な場所に控えたい, so that パスキーを全て失った場合の復旧手段を自分で用意
できる

**決定事項の反映:** 発行タイミングは Issue #243 の人間確定事項に従い「アカウント設定からの
任意発行（Option B）」とし、登録完了直後の自動発行は行わない。

#### Acceptance Criteria

1. When 認証済みユーザーが Account Settings UI からリカバリコード発行を要求したとき, the
   Recovery Code Service shall 相互に異なる複数個のリカバリコードを 1 回の操作で発行する
2. When リカバリコードが発行されたとき, the Account Settings UI shall 発行されたコード一式の
   生値を発行直後に **1 度だけ** ユーザーが視認およびコピーできる形で全文表示する
3. While 認証済みユーザーが Account Settings UI を再訪したとき, the Account Settings UI
   shall 既発行のコード一式の生値を再表示せず、発行済みである事実のみを提示する
4. When リカバリコードが発行されたとき, the Recovery Code Service shall 生成されたコードの
   生値を永続ストレージに保存せず、照合可能な変換値のみを保存する
5. If 未認証クライアントがリカバリコード発行エンドポイントを呼び出したとき, the Recovery
   Code Service shall 発行を試行せず未認証として要求を拒否する
6. When リカバリコード発行が完了したとき, the Feedman システム shall 発行操作を、
   アカウント作成やその他既存機能の利用可否を変化させずに追加的な機能として提供する

### Requirement 2: 未発行ユーザーへの UI リマインド

**Objective:** As a 認証済みユーザー, I want リカバリコードを未発行のまま放置している場合に
Web 画面上で気づけるようにしたい, so that パスキー喪失前に自発的に発行操作を行える

#### Acceptance Criteria

1. While 認証済みユーザーが Web を利用中で、当該ユーザーがまだリカバリコード一式を発行
   していないとき, the Web Application shall リカバリコード発行が推奨される旨のリマインドを
   ユーザーが視認できる形で提示する
2. While 認証済みユーザーが既にリカバリコード一式を発行済みのとき, the Web Application shall
   Requirement 2.1 のリマインドを提示しない
3. When 認証済みユーザーがリマインドから発行導線を辿ったとき, the Web Application shall
   Requirement 1.1 と同一のリカバリコード発行フローを開始する
4. The Web Application shall Requirement 2.1 のリマインド本文にリカバリコードの生値・
   照合可能変換値・その他ユーザー個別の機密情報を含めない

### Requirement 3: リカバリコードによる復旧セッションの確立

**Objective:** As a 全てのパスキーを喪失したユーザー, I want 保管していたリカバリコードで
Feedman に再度アクセスしたい, so that パスキー再登録に必要な認証済み状態に到達できる

#### Acceptance Criteria

1. When 未認証クライアントがユーザー識別子とリカバリコードを提示して復旧開始を要求した
   とき, the Recovery Code Service shall 提示されたコードが当該ユーザーの有効かつ未使用の
   コードのいずれかと一致することを検証する
2. When 提示されたリカバリコードが検証に成立したとき, the Recovery Code Service shall
   当該ユーザーとしての復旧セッションを確立し、Requirement 4 で規定する新パスキー登録
   フローに合流可能な認証済み状態を提示する
3. When 復旧セッションが確立したとき, the Recovery Code Service shall 使用された当該
   リカバリコード 1 個を以降の復旧試行で再利用不能な状態にする
4. If 提示されたリカバリコードが存在しない・既に使用済み・当該ユーザーに紐付かないの
   いずれの理由で拒否されたとき, the Recovery Code Service shall 復旧セッションを確立せず、
   拒否理由の内部区別をユーザー側に反射しない uniform なエラーで要求を拒否する
5. If 提示されたユーザー識別子に対応するアカウントが存在しないとき, the Recovery Code
   Service shall Requirement 3.4 と同一形式のエラーで要求を拒否し、ユーザーの存在有無を
   識別可能な情報を返さない
6. If 提示されたリカバリコードが空・許容外形式・許容外長さのいずれかであるとき, the
   Recovery Code Service shall 復旧セッションを確立せず、Requirement 3.4 と同一形式の
   エラーで要求を拒否する

### Requirement 4: 復旧成功時の既存 credential 全失効と新パスキー登録の強制

**Objective:** As a リカバリコードで復旧したユーザー, I want 復旧直後は必ず新しいパスキーの
登録を求められ、以前の credential とセッションが全て無効化されている状態にしたい, so that
リカバリコードの漏洩・盗難時の乗っ取りを最小限に抑えられる

**決定事項の反映:** 復旧成功後の credential 処理方針は Issue #243 の人間確定事項に従い
「既存 credential・セッションを全失効させ、新パスキー登録を強制する（Option A）」とし、
乗っ取り耐性を優先する（GitHub 2FA リカバリコードと同型）。

#### Acceptance Criteria

1. When リカバリコードによる復旧セッションが確立したとき, the Recovery Code Service shall
   当該ユーザーに紐付く既存の全パスキー credential を以降のパスキー認証で利用不能な状態に
   する
2. When リカバリコードによる復旧セッションが確立したとき, the Recovery Code Service shall
   当該ユーザーの既存の全認証済みセッションおよび既発行の全認証トークンを、以降の API
   呼び出しで利用不能な状態にする
3. While ユーザーが復旧セッション状態にあるとき, the Web Application shall 通常の 2 ペイン
   UI の利用に進む前に新パスキーの登録を必須ステップとして提示する
4. When ユーザーが復旧セッション上で新パスキーの登録を完了したとき, the Web Application
   shall 当該ユーザーを通常の認証済み状態へ遷移させ、以降の API 呼び出しを新規に確立された
   セッションで実行可能な状態にする
5. While 復旧セッション状態にあり新パスキー登録が完了していないとき, the Web Application
   shall 新パスキー登録以外の Feedman 機能（フィード購読・記事閲覧・退会・リカバリコード
   再発行等）を実行不能にする
6. If ユーザーが復旧セッションを新パスキー登録の完了前に破棄したとき, the Recovery Code
   Service shall 当該ユーザーの既存 credential・既発行トークンを Requirement 4.1 / 4.2 で
   無効化した状態のまま維持し、失効を巻き戻さない

### Requirement 5: 単回利用と再生成による旧一式の無効化

**Objective:** As a 認証済みユーザー, I want リカバリコードを再発行したときに以前のコード
一式を無効化したい, so that 過去に共有・紛失した可能性のあるコードを継続利用されない

#### Acceptance Criteria

1. The Recovery Code Service shall 発行済みリカバリコードの各 1 個を、Requirement 3 の
   復旧で成功裏に消費された時点で以降の復旧試行に再利用不能な状態にする
2. When 認証済みユーザーがリカバリコードの再発行を要求したとき, the Recovery Code Service
   shall 当該ユーザーの既発行のコード一式（未使用・使用済みを問わず）全てを以降の復旧試行
   で無効な状態にしてから、新しい一式を発行する
3. If Requirement 5.2 の再発行後に旧一式のいずれかのコードが復旧要求として提示されたとき,
   the Recovery Code Service shall 復旧セッションを確立せず、Requirement 3.4 と同一形式の
   エラーで要求を拒否する
4. When 再発行が完了したとき, the Account Settings UI shall 新一式の生値を Requirement 1.2
   と同一の規則（発行直後に 1 度だけ全文表示）で提示する

### Requirement 6: 復旧エンドポイントへの IP レート制限

**Objective:** As a Feedman 運用者, I want 未認証で公開される復旧エンドポイントを同一 IP 単位
でレート制限したい, so that リカバリコードの総当たり試行・フラッディングからサービスを
保護できる

#### Acceptance Criteria

1. When 同一クライアント IP から復旧開始または復旧応答検証への単位時間あたりリクエスト数
   が閾値を超過したとき, the IP レート制限 shall 復旧処理を試行せずに HTTP 429 Too Many
   Requests を返す
2. While 同一クライアント IP からのリクエスト数が閾値以内であるとき, the IP レート制限
   shall 復旧要求を後続処理へ通常どおり通過させる
3. When 異なるクライアント IP から復旧エンドポイントへリクエストが到達したとき, the IP
   レート制限 shall 各 IP のリクエスト数を独立にカウントする
4. When 閾値超過により拒否したとき, the IP レート制限 shall 既存の未認証エンドポイント
   （Google ログイン入口・native auth token / refresh / revoke・パスキー登録／認証）の
   閾値超過応答と同一形式で応答する

### Requirement 7: 既存認証フローとの共存と後方互換

**Objective:** As a Feedman Web ユーザー・iOS ユーザー・運用者, I want 本機能の追加によって
既存の Google OAuth / パスキー登録・認証 / native auth の各挙動が変化しないでほしい,
so that 既存ユーザーが再ログインや再設定を強いられずに従来通り利用できる

#### Acceptance Criteria

1. The Google OAuth Web Login Flow and Native Auth Flow shall 本機能導入前と同一の
   request / response 契約で動作する
2. The Passkey Registration Service and Passkey Authentication Service shall 本機能導入前と
   同一の request / response 契約で動作する
3. The Web Application shall リカバリコードを未発行のユーザーにおいても、既存の 2 ペイン UI
   および既存の認証・退会・フィード購読の各導線を、本機能導入前と同一に利用可能な状態に
   維持する
4. While ユーザーが Requirement 4 の復旧フロー外で既存のパスキー認証・Google OAuth・
   native auth のいずれかによりログインしたとき, the Feedman システム shall 当該ユーザーの
   既発行リカバリコード一式を消費・無効化しない

## Non-Functional Requirements

### NFR 1: 機密情報の非漏出

1. The Recovery Code Service shall リカバリコードの生値を永続ストレージに保存せず、
   照合可能な変換値のみを保存する
2. The Recovery Code Service shall リカバリコードの生値を運用ログ・エラー応答・API
   レスポンス本文・レスポンスヘッダに含めない
3. The Web Application shall リカバリコードの生値を Requirement 1.2 / 5.4 の 1 度限り表示
   以外の経路（ブラウザのローカルストレージ・sessionStorage・URL クエリ文字列・ブラウザ
   コンソール出力・エラー表示テキスト等）に残さない
4. The Recovery Code Service shall 拒否応答（Requirement 3.4 / 3.5 / 3.6 / 5.3）に
   クライアント入力値・内部詳細（スタックトレース・内部エラー原因等）を反射しない

### NFR 2: 既存挙動および App Store 4.8 姿勢の維持

1. The Feedman システム shall リカバリコード機能の導入に際して、アカウント作成や利用の
   前提としてメールアドレス等の追加個人情報の収集を必須化しない
2. The Feedman システム shall 本機能追加を、既存 API 契約の破壊的変更なしに導入する
3. The Existing Auth Contract Test Suite shall 本機能導入後も無変更で green 状態を維持する

### NFR 3: 可観測性

1. When リカバリコードの発行・再発行・復旧成功・復旧拒否・レート制限拒否のいずれかの
   事象が発生したとき, the Recovery Code Service shall 当該事象を運用ログとして記録する
2. The Recovery Code Service shall 運用ログにリカバリコードの生値・照合可能変換値・
   ユーザー識別子以外の個人情報を含めない

### NFR 4: テスト容易性

1. The Recovery Code Test Suite shall 発行成功・再発行による旧一式の全無効化・単回利用の
   enforce・存在しないコードでの拒否・既使用コードでの拒否・別ユーザーコードでの拒否・
   ユーザー未存在での拒否・IP レート超過での拒否・復旧成功後の新パスキー登録強制・
   復旧成功後の既存 credential 全失効・未発行ユーザーへのリマインド表示・発行済みユーザーで
   のリマインド非表示 の各ケースを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- メール送信基盤の追加およびメールによるリカバリフロー（App Store 4.8 の姿勢維持のため
  メール収集を強制しない）
- パスキー credential のセルフサービス管理 UI（一覧・削除・リネーム・命名）
- iOS クライアント側 UI 実装（本 spec はサーバ API と Web UI を先行し、iOS は別 Issue で
  扱う）
- 管理者・カスタマーサポート経由の手動アカウント復旧
- リカバリコードの印刷・PDF 出力・QR コード生成等の付加的な保存手段
- 復旧成功後のユーザーに対するメール通知（メール基盤不要の方針を維持）
- 復旧フローで新パスキー登録を完了できなかったユーザーに対する追加復旧手段（本 spec は
  復旧セッションで必ず新パスキー登録に至ることを前提とする）
- 既発行リカバリコードの部分無効化・個別失効 UI（無効化操作は再発行による一括無効化のみ）
- リカバリコード発行後の期限切れ（TTL）による自動失効
- 複数アカウントに対する一括発行・エクスポート機能

## Open Questions

- **リカバリコードの本数・桁数・文字種**: Issue #243 のオーケストレーター指示で「PM 推奨値を
  採用してよい」とされたが、AC としては「相互に異なる複数個を 1 回の操作で発行する」までを
  observable な粒度で要件化した。具体的な本数（例: 10 個）・桁数・文字集合・区切りの有無は
  design で確定する
- **IP レート制限の閾値**: 既存の未認証エンドポイント（Google ログイン入口・native auth・
  パスキー登録／認証）と同一の閾値・制限方式を流用する前提で Requirement 6 を要件化した。
  具体的な閾値秒数・許容リクエスト数・制限方式は既存規約を踏襲する形で design が確定する
- **リマインド提示手段**: Requirement 2.1 の「リマインド提示」の具体形（アカウント設定内の
  警告表示・グローバルバナー・ダイアログ・トースト等）は design / UI 判断とする
- **発行時 UI 補助**: Requirement 1.2 / 5.4 の「視認およびコピーできる形で全文表示」の具体的
  UI 補助（コピーボタン・テキストファイルダウンロード・確認チェックボックス等）は design で
  確定する
- **復旧開始時のユーザー識別方式**: Requirement 3.1 の「ユーザー識別子とリカバリコード」の
  具体的な入力導線（ユーザー名の直接入力・discoverable な認証入口の再利用等）は design で
  確定する
- **復旧セッションの表現**: Requirement 4 の「復旧セッション」を既存の Cookie セッション枠の
  縮退モードで表現するか、新規の状態タグ・claim で表現するかは design 判断とする（本 spec
  は user-observable な「新パスキー登録以外を実行不能とする状態」までを要件化）

## 関連

- Parent: #216
- Depends on: #216
- Related: #242 #223 #231
