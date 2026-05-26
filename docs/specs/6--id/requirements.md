# Requirements Document

## Introduction

認証サービスのログアウト処理は、処理完了時に「user logged out」ログを出力する際、セッション ID を平文のまま
記録している。セッション ID は認証トークン相当の機密情報であり、平文でログに残るとログ閲覧権限を持つ者による
セッション乗っ取りのリスクを生む。本機能では、ログアウト時のログにセッション ID が平文で残らないようにしつつ、
障害調査・追跡に必要な最低限の識別情報を復元不能な短縮値として残すことで、機密性と運用上の追跡可能性を両立する。
Issue コメントの人間判断により、セッション ID を完全削除せず、ハッシュ化した先頭 8 文字を残す方式（Option B）を
採用する。

## Requirements

### Requirement 1: ログアウト時のセッション ID 機密化

**Objective:** As a 運用者, I want ログアウト時のログにセッション ID が平文で残らないこと, so that ログ閲覧経由でのセッション乗っ取りリスクを排除できる

#### Acceptance Criteria

1. When ログアウト処理がログを出力するとき, the Auth Service shall ログ出力に生のセッション ID を含めない
2. When ログアウト処理がセッション ID をログに残すとき, the Auth Service shall 元のセッション ID を復元できない短縮値に変換して記録する
3. The Auth Service shall ログに残す短縮値をハッシュ値の先頭 8 文字に限定する

### Requirement 2: 追跡可能性の維持

**Objective:** As a 運用者, I want 同一セッションのログアウトが一貫した短縮値で記録されること, so that 機密性を保ったままログ上でセッション単位の追跡ができる

#### Acceptance Criteria

1. When 同一のセッション ID から短縮値を生成したとき, the Auth Service shall 常に同一の短縮値を返す
2. When 異なるセッション ID から短縮値を生成したとき, the Auth Service shall それぞれ区別可能な短縮値を返す

### Requirement 3: 短縮値生成の堅牢性

**Objective:** As a 運用者, I want 短縮値生成が空入力でも安全に動作すること, so that 入力値に依存してログ処理が異常終了することを防げる

#### Acceptance Criteria

1. If 空文字のセッション ID が短縮値生成に渡された場合, the Auth Service shall パニックを起こさず短縮値を返す
2. The Auth Service shall 任意の入力文字列に対して短縮値生成を副作用なく完了する

### Requirement 4: ログアウト挙動の後方互換性

**Objective:** As a エンドユーザー, I want 機密化対応後もログアウトが従来どおり完了すること, so that 認証フローの挙動が変わらず利用を継続できる

#### Acceptance Criteria

1. When 有効なセッション ID でログアウトが要求されたとき, the Auth Service shall 従来どおりセッションを破棄し正常完了する
2. If 空文字のセッション ID でログアウトが要求された場合, the Auth Service shall 従来どおりログ出力に到達する前にエラーを返す
3. The Auth Service shall ログアウト成功時に従来どおりログレベル Info で完了ログを出力する

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The Auth Service shall ログ出力に機密トークン相当のセッション ID 原文を一切含めない
2. The Auth Service shall ログに記録する短縮値から元のセッション ID 全体を復元できない一方向変換を用いる

### NFR 2: 後方互換

1. The Auth Service shall ログアウトの観測可能な挙動（セッション破棄・エラー条件・ログ出力の有無とレベル）を本機能導入前と同一に保つ

## Out of Scope

- ログアウト以外でセッション ID をログ出力している箇所の機密化（本 Issue ではログアウト処理のみを対象とする）
- 既に出力済みの既存ログに含まれるセッション ID の遡及的マスキング・削除
- セッション ID 以外の機密値（OAuth トークン・メールアドレス等）のログ機密化方針の見直し
- ログフォーマット全体の刷新やログ基盤の変更

## Open Questions

- なし（主要論点である「完全削除（Option A）か短縮値保持（Option B）か」は Issue コメントで Option B 採用に確定済み。短縮値の長さも「先頭 8 文字」で確定済み）。
