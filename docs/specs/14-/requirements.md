# Requirements Document

## Introduction

Feedman のロガーは出力レベルが `slog.LevelInfo` にハードコードされており、環境変数で切り替えられない。
このため開発時に DEBUG ログを有効化したり、本番で WARN 以上に絞ったりするには再ビルドが必要になっている。
本要件は、環境変数（仮称 `LOG_LEVEL`）により起動時にログレベルを選択可能とし、運用環境ごとにログ量を
調整できるようにすることを目的とする。なお、ロガーは設定読み込み（`config.Load`）より前に初期化される制約
（「設定読み込み前にログを使えるようにする」というアプリ初期化順序）があるため、ログレベルの決定は config 構造体を
経由できない前提となる。後方互換（未設定時 INFO）と不正値時のサイレント失敗回避を非機能要件として維持する。

## Requirements

### Requirement 1: 環境変数によるログレベル選択

**Objective:** As a 運用者, I want 環境変数でログレベルを選択して起動できること, so that 再ビルドなしで開発時は詳細ログ・本番は重要ログのみに切り替えられる

#### Acceptance Criteria

1. When `LOG_LEVEL=DEBUG` を指定して起動したとき, the Logger shall DEBUG 以上のレベル（DEBUG / INFO / WARN / ERROR）のログを出力する。
2. When `LOG_LEVEL=INFO` を指定して起動したとき, the Logger shall INFO 以上のレベル（INFO / WARN / ERROR）のログを出力し DEBUG を抑制する。
3. When `LOG_LEVEL=WARN` を指定して起動したとき, the Logger shall WARN 以上のレベル（WARN / ERROR）のログを出力し DEBUG / INFO を抑制する。
4. When `LOG_LEVEL=ERROR` を指定して起動したとき, the Logger shall ERROR レベルのログのみを出力し DEBUG / INFO / WARN を抑制する。
5. The Logger shall 起動時に決定したログレベルを当該プロセスの実行中は固定して適用する（起動時の 1 回のみ反映する）。

### Requirement 2: 未設定時の後方互換

**Objective:** As a 既存運用者, I want 環境変数を設定しないとき従来どおりの挙動になること, so that 本変更を取り込んでも既存の起動構成を変更せずに済む

#### Acceptance Criteria

1. When 環境変数 `LOG_LEVEL` が未設定のまま起動したとき, the Logger shall INFO 以上のレベル（INFO / WARN / ERROR）のログを出力する。
2. If 環境変数 `LOG_LEVEL` が空文字で指定されたとき, the Logger shall INFO 以上のレベルを適用し未設定時と同一の挙動とする。

### Requirement 3: 不正値指定時のフォールバック

**Objective:** As a 運用者, I want 不正なレベル文字列を指定してもサイレントに失敗せず一貫した挙動になること, so that 設定ミスに気づきつつアプリの起動継続を妨げない

#### Acceptance Criteria

1. If `LOG_LEVEL` に許容値（DEBUG / INFO / WARN / ERROR）以外の文字列（例 `VERBOSE`）が指定されたとき, the Logger shall デフォルトの INFO レベルにフォールバックして起動を継続する。
2. If `LOG_LEVEL` に不正値が指定されてフォールバックが発生したとき, the Logger shall 指定キー・指定値・採用したデフォルトレベルを含む警告ログを出力する。
3. If `LOG_LEVEL` に不正値が指定されたとき, the Logger shall プロセスを異常終了させずに起動を継続する。

### Requirement 4: 大文字小文字非依存の解釈

**Objective:** As a 運用者, I want レベル文字列の大文字小文字を区別せず指定できること, so that 表記ゆれを意識せずに環境変数を設定できる

#### Acceptance Criteria

1. When `LOG_LEVEL` にレベル名を小文字（例 `debug`）で指定したとき, the Logger shall 対応する大文字表記（`DEBUG`）と同一のログレベルを適用する。
2. When `LOG_LEVEL` にレベル名を大文字小文字混在（例 `Warn`）で指定したとき, the Logger shall 大文字小文字を区別せず対応するログレベルを適用する。

## Non-Functional Requirements

### NFR 1: 後方互換性

1. While 環境変数 `LOG_LEVEL` を設定していない状態で起動している間, the Logger shall 本変更導入前と同一の出力レベル（INFO 以上）を維持する。

### NFR 2: 障害耐性（サイレント失敗回避）

1. While 不正な `LOG_LEVEL` 値が指定された状態で起動した間, the Logger shall 警告ログ出力とデフォルトレベル採用を毎回同じ手順で一貫して行い、サイレントに失敗しない。
2. The Logger shall 環境変数の不正値に対する挙動を、本プロジェクトの他環境変数で採用済みの「警告ログ出力 + デフォルト採用」方針と運用者から見て一貫した形にする。

## Out of Scope

- ログフォーマットの変更（JSON 構造化ログ以外の形式への切り替え）。
- ログ出力先の切り替え（標準出力以外への出力先指定）。
- ランタイム中の動的なログレベル変更（起動後にレベルを変更する機構）。
- ログレベル以外の環境変数（出力先・サンプリング・ローテーション等）の追加。

## Open Questions

- なし（環境変数名 `LOG_LEVEL`・不正値時は INFO フォールバック + 警告・大文字小文字非依存、を本要件で確定。実装手段（環境変数の読み取り箇所が logger 側か config 経由か）は design / 実装の領分）。
