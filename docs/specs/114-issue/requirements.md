# Requirements Document

## Introduction

Feedman の Web UI には「フィード追加」ダイアログがあり、ユーザーは URL を入力してフィード
を登録できる。現在の実装ではフィード登録が成功すると、ダイアログがそのまま「登録完了画面」
（登録されたフィード URL を表示し、ユーザーが手動で「閉じる」ボタンを押すまで残り続ける
中間ステップ）へ遷移する。このワンクッションは追加導線の体験を遅くしており、登録後すぐに
左ペインの一覧に戻りたいというユーザー要望と乖離している。

本要件は、フィード登録が成功した瞬間にダイアログを自動で閉じ、関連する「登録完了画面」の
UI を整理することで、追加導線を 1 ステップ短縮する変更を定義する。失敗系のフィードバック
（フィード未検出、購読上限到達、想定外エラー）および登録成功時の左ペイン再取得・登録完了
通知は現状の挙動を維持する。

## Requirements

### Requirement 1: 登録成功時のダイアログ自動クローズ

**Objective:** As a Feedman の利用者, I want フィード登録が成功した時点でダイアログが自動的に閉じる, so that 余計なクリック操作なしに左ペインの一覧へ即座に戻れる

#### Acceptance Criteria

1. When フィード登録要求が成功応答を返したとき, the Feed Register Dialog shall ダイアログを閉じた状態に遷移させ画面から非表示にする
2. When フィード登録要求が成功応答を返したとき, the Feed Register Dialog shall 左ペインのフィード一覧が新しく追加されたフィードを含む状態に再取得・反映されるよう促す
3. When フィード登録要求が成功応答を返したとき, the Feed Register Dialog shall 呼び出し元コンポーネントへの登録完了通知を従来どおり 1 回だけ伝搬する
4. When ダイアログが登録成功により自動で閉じたとき, the Feed Register Dialog shall 次回ダイアログを開いた際に URL 入力欄が空の入力フェーズで表示されるよう状態をリセットする

### Requirement 2: 「登録完了画面」UI の削除

**Objective:** As a Feedman のメンテナ, I want 不要になった「登録完了画面」の UI と関連コードが削除されている, so that 残存コードによる挙動誤解と保守コストを排除できる

#### Acceptance Criteria

1. The Feed Register Dialog shall フィード登録成功後に「登録完了」というダイアログタイトル・「フィードが正常に登録されました」という説明文・登録済みフィード URL を表示する入力欄・「閉じる」ボタンのいずれも表示しない
2. The Feed Register Dialog shall 登録成功直後にユーザーが登録済みフィード URL を編集・確認できる UI 要素を提供しない
3. The Feed Register Dialog shall 登録成功状態を表現するためだけに存在する画面・ボタン・ラベルなどの UI コードを含まない

### Requirement 3: 失敗系・操作中のダイアログ保持

**Objective:** As a Feedman の利用者, I want 登録が失敗したり処理中であるあいだはダイアログが閉じない, so that エラー内容を確認し再入力・再試行できる

#### Acceptance Criteria

1. If フィード登録要求がフィード未検出を示すエラーを返したとき, the Feed Register Dialog shall ダイアログを開いたままエラーメッセージと対処方法を表示する
2. If フィード登録要求が購読上限到達を示すエラーを返したとき, the Feed Register Dialog shall ダイアログを開いたままエラーメッセージと対処方法を表示する
3. If フィード登録要求が想定外のエラーを返したとき, the Feed Register Dialog shall ダイアログを開いたまま汎用エラーメッセージと再試行を促す対処方法を表示する
4. While フィード登録要求の応答待ちが継続しているあいだ, the Feed Register Dialog shall ダイアログを閉じず登録ボタンを操作不可状態にして処理中であることを示す
5. When エラー表示状態でユーザーが URL を修正し再度登録を実行したとき, the Feed Register Dialog shall エラー表示を解消したうえで再度登録処理を呼び出す

### Requirement 4: ユーザー操作によるダイアログクローズ

**Objective:** As a Feedman の利用者, I want 登録を実行する前でも自分の操作でダイアログを閉じられる, so that 入力途中で気が変わった場合にキャンセルできる

#### Acceptance Criteria

1. When ユーザーが背景クリック・Esc キー・ダイアログ標準のクローズ操作のいずれかを実行したとき, the Feed Register Dialog shall ダイアログを閉じる
2. When ダイアログがユーザー操作によって閉じられた直後に再度開かれたとき, the Feed Register Dialog shall URL 入力欄を空にした入力フェーズで開く

## Out of Scope

- 登録された URL を成功後にユーザーが編集・修正するワークフロー（本変更で UI ごと削除）
- 登録成功時のトースト通知やバナー通知の追加（要望には含まれていない）
- 登録ボタン押下時のローディング表現の刷新（既存のボタン無効化＋ラベル変更を維持）
- 左ペインのフィード一覧表示・並び替え・スクロール挙動の変更（一覧の再取得は既存のキャッシュ無効化に委譲）
- 失敗時のエラーメッセージ文言の刷新（既存のフィード未検出・購読上限到達・想定外エラーの 3 カテゴリ表示を維持）
- フィード登録 API（バックエンド）の挙動変更

## Open Questions

- なし
