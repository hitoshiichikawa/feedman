# Requirements Document

## Introduction

`internal/config` は環境変数からアプリケーション設定を起動時に 1 回読み込む。数値・期間系の
オプション設定を読む補助関数（`getEnvInt` / `getEnvInt64` / `getEnvDuration`）は、環境変数に
不正な値が設定されていてもサイレントにデフォルト値へフォールバックするため、運用者は設定
ミス（typo・不正フォーマット）に気づけず、「設定したはずの値が反映されない」原因の切り分けが
困難になっている。本要件は、パース失敗時に構造化された警告ログを出力して運用者が異常を検知
できるようにする。既存のフォールバック挙動・補助関数のシグネチャは維持し、起動を中断しない。

## Requirements

### Requirement 1: 整数パース失敗時の警告ログ出力

**Objective:** As a 運用者, I want 整数型の設定環境変数が不正値だったときに警告ログを得る, so that 設定ミスに気づいてデフォルト値が採用されたことを検知できる

#### Acceptance Criteria

1. If 整数型の設定環境変数に整数として解釈できない値が設定されている, the Config Loader shall 当該設定でデフォルト値を採用する
2. If 整数型の設定環境変数に整数として解釈できない値が設定されている, the Config Loader shall 警告レベルのログを 1 件出力する
3. When 整数パース失敗の警告ログを出力する, the Config Loader shall ログに対象の環境変数キー名・設定されていた不正値・採用したデフォルト値を構造化フィールドとして含める

### Requirement 2: 64bit 整数パース失敗時の警告ログ出力

**Objective:** As a 運用者, I want 64bit 整数型の設定環境変数が不正値だったときに警告ログを得る, so that サイズ系設定の入力ミスを検知できる

#### Acceptance Criteria

1. If 64bit 整数型の設定環境変数に整数として解釈できない値が設定されている, the Config Loader shall 当該設定でデフォルト値を採用する
2. If 64bit 整数型の設定環境変数に整数として解釈できない値が設定されている, the Config Loader shall 警告レベルのログを 1 件出力する
3. When 64bit 整数パース失敗の警告ログを出力する, the Config Loader shall ログに対象の環境変数キー名・設定されていた不正値・採用したデフォルト値を構造化フィールドとして含める

### Requirement 3: 期間（Duration）パース失敗時の警告ログ出力

**Objective:** As a 運用者, I want 期間型の設定環境変数が不正値だったときに警告ログを得る, so that タイムアウト・間隔系設定の入力ミスを検知できる

#### Acceptance Criteria

1. If 期間型の設定環境変数を期間値として解釈できない値が設定されている, the Config Loader shall 当該設定でデフォルト値を採用する
2. If 期間型の設定環境変数を期間値として解釈できない値が設定されている, the Config Loader shall 警告レベルのログを 1 件出力する
3. When 期間パース失敗の警告ログを出力する, the Config Loader shall ログに対象の環境変数キー名・設定されていた不正値・採用したデフォルト値を構造化フィールドとして含める

### Requirement 4: 正常系・未設定時のログ抑制と既存挙動維持

**Objective:** As a 運用者, I want 正常な設定や未設定の場合に余計な警告ログが出ない, so that 警告ログを設定ミスの信号として信頼できる

#### Acceptance Criteria

1. When 整数・64bit 整数・期間いずれかの設定環境変数に正しく解釈できる値が設定されている, the Config Loader shall 当該の値を採用する
2. When 設定環境変数に正しく解釈できる値が設定されている, the Config Loader shall パース失敗の警告ログを出力しない
3. While 設定環境変数が未設定（空文字）の状態, the Config Loader shall デフォルト値を採用する
4. While 設定環境変数が未設定（空文字）の状態, the Config Loader shall パース失敗の警告ログを出力しない

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The Config Loader shall 各補助関数の入出力（環境変数キー・デフォルト値を受け取り、採用値を返す）と、パース失敗時にデフォルト値へフォールバックする挙動を本機能導入前と同一に保つ
2. The Config Loader shall 設定項目の追加・削除およびデフォルト値の変更を行わない

### NFR 2: 起動順序への非依存

1. While ログ初期化前後のいずれの状態, the Config Loader shall パース失敗ログ出力処理によって panic せず設定読み込みを完了する

### NFR 3: 可観測性

1. When パース失敗の警告ログを出力する, the Config Loader shall 機械的に検索・集計可能な構造化フィールド形式（キー名・不正値・デフォルト値を独立したフィールドとして）で出力する

## Out of Scope

- パース失敗を fatal error として起動を中断する挙動への変更（現状のデフォルト値フォールバックを維持する）
- 設定項目の追加・削除、デフォルト値の見直し
- 必須環境変数（未設定時にエラーを返すフィールド）に関する挙動変更
- 文字列型・bool 型など、パース失敗の概念を持たない設定の扱いの変更
- ログ出力先・フォーマット・ログレベル閾値そのものの再設計（既存のログ基盤に準拠する）

## Open Questions

- なし（Issue 本文の受入基準・制約・スコープ除外で要件は確定でき、コメントには人間による追加の決定事項は存在しない）
