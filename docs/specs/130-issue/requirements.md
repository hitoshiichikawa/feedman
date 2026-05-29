# Requirements Document

## Introduction

Feedman の Web UI では、フィード購読の解除に必要なバックエンド API（`DELETE /api/subscriptions/:id`）
と購読設定 UI コンポーネント（`SubscriptionSettings`）が既に実装されているが、これらがメイン画面
（2 ペインレイアウト）から到達できる動線として統合されていない。本仕様は、左ペインのフィード
一覧から個別フィードの設定パネルを開き、購読解除を含む既存の購読管理操作をユーザーが画面上で
完結できるようにすることを目的とする。新規ロジック（API・サービス層）の追加ではなく、既存
コンポーネントの統合と動線整備が中心となる。

## Requirements

### Requirement 1: フィード項目からの設定起動動線

**Objective:** As a Feedman ユーザー, I want 左ペインのフィード一覧から個別フィードの設定画面を開きたい, so that フィードを選択することなく購読設定や購読解除にアクセスできる

#### Acceptance Criteria

1. While 左ペインのフィード項目にポインタがホバーしていない状態, the Feed List UI shall 当該項目に設定起動コントロール（ギアアイコン）を表示しない
2. While 左ペインのフィード項目にポインタがホバーしている状態, the Feed List UI shall 当該項目に設定起動コントロール（ギアアイコン）を表示する
3. When ユーザーがフィード項目の設定起動コントロールをクリックしたとき, the Feed List UI shall 当該フィードに紐づく購読設定パネルを開く
4. When ユーザーがフィード項目の設定起動コントロールをクリックしたとき, the Feed List UI shall フィード選択イベント（右ペインの記事一覧切替）を発火しない
5. Where キーボード操作が利用されている場合, the Feed List UI shall 設定起動コントロールにフォーカス可能な手段（Tab 到達・Enter / Space 起動）を提供する

### Requirement 2: 購読設定パネルの統合表示

**Objective:** As a Feedman ユーザー, I want フィードごとに購読設定パネル（更新間隔変更・フェッチ再開・購読解除）を画面上で利用したい, so that フィード単位の運用操作を 1 つの UI で完結できる

#### Acceptance Criteria

1. When 購読設定パネルが開かれたとき, the Subscription Settings UI shall 対象フィードの現在の更新間隔・フェッチ状態を反映した状態で表示される
2. When ユーザーが購読設定パネルで更新間隔を変更したとき, the Subscription Settings UI shall 変更内容を購読データに反映し、フィード一覧の最新状態に同期する
3. While 対象フィードのフェッチが停止状態またはエラー状態である間, the Subscription Settings UI shall フェッチ再開操作を提供する
4. When ユーザーが購読設定パネルからフェッチ再開を実行したとき, the Subscription Settings UI shall 再開操作の結果をフィード一覧の状態表示に反映する
5. When ユーザーが購読設定パネルを閉じる操作を行ったとき, the Subscription Settings UI shall パネルを閉じ、左ペインのフィード一覧表示状態に戻す

### Requirement 3: 購読解除の確認と実行

**Objective:** As a Feedman ユーザー, I want 購読解除を誤操作で発火させない確認動線を踏みたい, so that 不可逆操作を意図的にのみ実行できる

#### Acceptance Criteria

1. When ユーザーが購読設定パネルで購読解除ボタンを押下したとき, the Subscription Settings UI shall 確認用ダイアログを表示する
2. The 確認用ダイアログ shall 対象フィードのタイトルと、購読解除に伴って記事の既読・スター状態も削除される旨を明示する
3. When ユーザーが確認用ダイアログでキャンセルを選択したとき, the Subscription Settings UI shall 購読解除を実行せずダイアログを閉じる
4. When ユーザーが確認用ダイアログで購読解除を確定したとき, the Subscription Settings UI shall バックエンドの購読解除エンドポイント（`DELETE /api/subscriptions/:id`）を呼び出す
5. While 購読解除リクエストが処理中である間, the Subscription Settings UI shall 確定操作を多重実行できないよう確定ボタンを非活性化または進行表示にする

### Requirement 4: 購読解除完了後の UI 整合

**Objective:** As a Feedman ユーザー, I want 購読解除を実行した直後にフィード一覧と右ペインが整合した状態に戻ること, so that 解除済みフィードの残骸表示を見せられず迷わない

#### Acceptance Criteria

1. When 購読解除リクエストが成功したとき, the Feed List UI shall 解除されたフィードを一覧から除外した状態に更新する
2. When 購読解除されたフィードが当該操作の時点で右ペインに選択されていたとき, the Application Shell shall 右ペインを初期状態（フィード未選択時の表示）にクリアする
3. When 購読解除されたフィードが当該操作の時点で右ペインに選択されていなかったとき, the Application Shell shall 右ペインの現在の選択状態と表示を維持する
4. When 購読解除リクエストが成功したとき, the Subscription Settings UI shall 確認用ダイアログと購読設定パネルを閉じる
5. When 購読解除に伴う関連状態削除がバックエンドで実行されたとき, the Feed List UI shall 当該フィードの未読数バッジ・ステータスアイコンを以後表示しない

### Requirement 5: 購読解除の異常系

**Objective:** As a Feedman ユーザー, I want 購読解除が失敗したときに状態が破壊されず、再試行可能であること, so that 一時的な失敗で UI が不整合に陥らない

#### Acceptance Criteria

1. If 購読解除リクエストがネットワークエラーまたはサーバエラーで失敗したとき, the Subscription Settings UI shall ユーザーが認識可能なエラー表示を行い、フィード一覧から該当フィードを除外しない
2. If 購読解除リクエストが失敗したとき, the Subscription Settings UI shall 確認用ダイアログを保持または再表示し、再試行を可能にする
3. If 購読解除リクエストが失敗したとき, the Application Shell shall 右ペインの選択状態を変更しない

## Non-Functional Requirements

### NFR 1: 既存挙動との互換性

1. The Feed List UI shall 本仕様導入前から存在する既存挙動（フィード選択・未読数バッジ・ステータスアイコン表示・favicon フォールバック）を変更しない
2. The Subscription Settings UI shall 本仕様導入前から存在する更新間隔変更・フェッチ再開の挙動を変更しない

### NFR 2: アクセシビリティ

1. The 設定起動コントロール shall キーボードフォーカス可能であり、用途が判別できるアクセシブルネーム（例: 「<フィード名> の設定」）を持つ
2. The 確認用ダイアログ shall フォーカスをダイアログ内に閉じ込め、Esc キーまたはキャンセル操作で閉じられる

### NFR 3: フィードバック応答性

1. While 購読解除リクエストが処理中である間, the Subscription Settings UI shall 1 秒以内に進行中であることをユーザーに視覚提示する（ボタン非活性化または進行ラベル表示）

## Out of Scope

- フィードそのもの（マスタデータ）の物理削除。本仕様は購読（subscription）レイヤの解除に限定する
- 複数フィードを同時に解除する一括解除機能
- 解除済みフィードの復元（undo）動線
- 購読解除時の関連状態削除ロジック自体の挙動変更（バックエンドの既存挙動に準拠する）
- 購読設定パネル内の新規設定項目追加（更新間隔・フェッチ再開・購読解除以外）

## Open Questions

- なし（Issue 本文に期待挙動と受入基準候補が明示されており、既存実装（`SubscriptionSettings` / `useUnsubscribe` / バックエンド `Service.Unsubscribe`）と整合する形で要件化済み）
