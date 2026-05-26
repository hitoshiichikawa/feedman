# Requirements Document

## Introduction

web コンポーネント（Next.js standalone server）は、起動時に `process.env.HOSTNAME` が指す
アドレスへ bind する。Docker は既定でコンテナ ID を `HOSTNAME` に自動設定するため、現状の
web コンテナはコンテナ ID が解決する IP に bind し、公開ポート（NAT 先 IP）と bind 先が
食い違って接続拒否となる。web サービスは `internal` / `external` の 2 ネットワークに所属し
複数 IP を持つため、この食い違いが顕在化している。本要件は、web イメージが全 interface に
bind し、公開ポート経由でブラウザ／HTTP から到達できる状態を、image 自己完結的に担保する
ことを目的とする。

## Requirements

### Requirement 1: 全 interface への bind による公開ポート到達性

**Objective:** As a Feedman を構築・運用する運用者, I want web コンテナが公開ポート経由で
HTTP 到達可能であること, so that ブラウザから Feedman の UI にアクセスできる

#### Acceptance Criteria

1. When web コンテナがデフォルト構成で起動されたとき, the Web Container shall すべての
   ネットワーク interface（全 IP）からの受付が可能な状態で待ち受ける
2. When web コンテナが起動を完了したとき, the Web Container shall 起動ログの Network 行に
   全 interface への待ち受けを示すアドレス（`http://0.0.0.0:3000`）を出力する
3. When 公開ポート経由で web コンテナのルートパス（`/`）に HTTP GET を送信したとき, the
   Web Container shall HTTP ステータス `200` を返す
4. When 公開ポート経由で web コンテナのルートパス（`/`）に HTTP GET を送信したとき, the
   Web Container shall 応答 HTML に `<title>Feedman - RSS リーダー</title>` を含める
5. The Web Container shall 公開ポートのホスト側ポート番号が既定値（`3000`）から変更された
   場合でも、変更後のホスト側ポート経由で到達可能である

### Requirement 2: image 自己完結性と上書き可能性

**Objective:** As a Feedman を構築・運用する運用者, I want bind アドレスの設定が image 内に
組み込まれていること, so that 追加の environment 指定なしでもコンテナが到達可能になる

#### Acceptance Criteria

1. When web イメージから生成したコンテナを、外部から bind アドレスを指示する environment を
   一切付与せず起動したとき, the Web Container shall 全 interface へ bind した状態になる
2. Where 外部からコンテナ起動時に bind アドレスを指示する environment が付与されているとき,
   the Web Container shall 外部から指定された bind アドレスを優先して適用する
3. The Web Container shall 待ち受けポートが既定値から変更可能であり、変更後のポートで
   待ち受ける

### Requirement 3: 異常系・既存挙動の保持

**Objective:** As a Feedman を構築・運用する運用者, I want 本変更が既存の起動時検証や
セキュリティ前提を壊さないこと, so that 誤設定の検出やネットワーク分離が従来どおり機能する

#### Acceptance Criteria

1. If web コンテナ起動時に内部 API 接続先設定（`API_INTERNAL_URL`）が未設定または空である
   とき, the Web Container shall bind アドレス設定の有無に関わらず非ゼロ終了で fail-fast する
2. While 内部 API 接続先設定が有効であるとき, the Web Container shall 起動を継続し全
   interface へ bind したうえで待ち受ける
3. If 公開ポートにマッピングされていないネットワーク（外部から到達不可な内部ネットワーク）
   からのみアクセスされたとき, the Web Container shall 当該経路の到達可否を従来の
   ネットワーク分離方針どおりに維持する

## Non-Functional Requirements

### NFR 1: 互換性

1. The Web Container shall 本変更の前後で、公開ポート経由のルートパス `/` への HTTP GET に
   対して同一の HTTP ステータス（`200`）応答内容（`<title>Feedman - RSS リーダー</title>`
   を含む HTML）を返し、UI の表示挙動を変えない
2. The Web Container shall web 以外のコンポーネント（api / worker / db）の起動・通信挙動を
   変更しない

### NFR 2: 可観測性

1. When web コンテナが起動を完了したとき, the Web Container shall 待ち受けアドレスを起動
   ログに出力し、運用者が bind 先を 1 行のログで確認できるようにする

## Out of Scope

- api / worker / db コンテナの bind 設定や起動挙動の変更
- web の待ち受けポート番号そのものの既定値変更（既定 `3000` を維持する）
- リバースプロキシ・TLS 終端・ロードバランサ等、コンテナ外のネットワーク構成の導入
- ネットワーク設計（`internal` / `external` の 2 ネットワーク所属）の見直し
- Next.js のアップグレードや standalone 出力方式そのものの変更

## Open Questions

- 恒久対応として bind アドレス設定を image 側（Dockerfile の `ENV`）に組み込むか、compose 側
  の environment で与えるかは、Issue 本文で「image の自己完結性のため Dockerfile への追加が
  望ましい（compose 側 environment でも可）」と方針が示されている。本要件は image 自己完結性
  を Requirement 2.1 として採用するが、最終的な設定箇所（Dockerfile / compose の双方に置くか
  片方のみか）の確定は design.md の領分とする。
