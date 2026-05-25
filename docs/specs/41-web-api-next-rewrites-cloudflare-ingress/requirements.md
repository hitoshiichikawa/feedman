# Requirements Document

## Introduction

現状の Feedman は、ブラウザが `web`（:3000）と `api`（:8080）の 2 つのオリジンへ直接アクセスする構成で、
ブラウザから見た API の URL を `NEXT_PUBLIC_API_URL` としてフロントエンドのビルド時に静的バンドルへ
焼き込んでいる。このため同一イメージを環境横断で再利用できず（build-once が成立しない）、
本番に `localhost` などの誤った既定値が混入したまま気付けないリスクがある。また 2 オリジン構成は
クロスオリジン Cookie と CORS の取り回しを恒常的に必要とする。

本機能は、ブラウザから見たアクセス先を単一オリジンに統合し、フロントエンドが API を同一オリジンの
相対パス経由で呼べるようにすることで、ビルド時 URL 焼き込みの廃止・build-once の成立・first-party
Cookie 化・CORS 依存の縮小を実現する。実行時に解決される内部 API 接続先設定が未指定なら起動時に
fail-fast させ、誤設定の本番投入を防ぐ。

## Requirements

### Requirement 1: ビルド時 API URL 焼き込みの廃止と build-once 成立

**Objective:** As a 運用者, I want フロントエンドのビルド成果物に API の URL を焼き込まないようにしたい, so that 同一イメージを環境を問わず再利用でき、本番への誤った URL 混入を防げる

#### Acceptance Criteria

1. When フロントエンドのビルドを `NEXT_PUBLIC_API_URL` 未指定で実行したとき, the Web ビルドプロセス shall ビルドを正常に完了する
2. The フロントエンドのビルド成果物 shall API オリジンの絶対 URL を含まない
3. When 同一のフロントエンドイメージを異なる環境にデプロイしたとき, the Web アプリケーション shall 環境ごとの再ビルドなしに各環境の API へ到達する
4. If ビルド時または実行時に `NEXT_PUBLIC_API_URL` が値として与えられたとき, the Web アプリケーション shall ブラウザから見た API アクセス先を当該値に切り替えず、同一オリジンの相対パス経由のアクセスを維持する

### Requirement 2: 同一オリジン経由での API アクセス

**Objective:** As a エンドユーザー, I want ブラウザからの API 呼び出しが単一オリジン経由で成功すること, so that クロスオリジンに起因する失敗なくフィードリーダーを利用できる

#### Acceptance Criteria

1. When ブラウザがログイン後に `/api/*` のエンドポイントへリクエストしたとき, the Web アプリケーション shall 同一オリジン経由でバックエンドへ転送し、バックエンドの応答をブラウザへ返す
2. When ブラウザが認証フロー用の `/auth/*` エンドポイントへリクエストしたとき, the Web アプリケーション shall 同一オリジン経由でバックエンドへ転送し、バックエンドの応答をブラウザへ返す
3. The Web アプリケーション shall ブラウザからの API リクエストにクロスオリジンの CORS プリフライトを要求しない
4. While 単一オリジン構成で稼働している間, when バックエンドが 4xx / 5xx のエラー応答を返したとき, the Web アプリケーション shall 当該ステータスコードと応答本文をブラウザへそのまま伝達する
5. When バックエンドの応答が `Set-Cookie` ヘッダを含むとき, the Web アプリケーション shall 当該 `Set-Cookie` ヘッダをブラウザへ伝達する

### Requirement 3: first-party Cookie による Google OAuth フローの完結

**Objective:** As a エンドユーザー, I want 単一オリジン構成のまま Google OAuth ログインが完結すること, so that third-party Cookie ブロックの影響を受けずにログイン状態を維持できる

#### Acceptance Criteria

1. When ユーザーが Google OAuth ログインを開始し認証を完了したとき, the 認証フロー shall ブラウザのアクセス先と同一オリジンに対して発行されたセッション Cookie を確立する
2. While セッション Cookie が確立されている間, when ブラウザが認証必須の `/api/*` エンドポイントへリクエストしたとき, the Web アプリケーション shall 当該セッション Cookie を付与した状態でバックエンドへ転送する
3. When OAuth コールバック処理が完了したとき, the 認証フロー shall ブラウザを単一オリジンのフロントエンドへリダイレクトする
4. The セッション Cookie shall first-party Cookie として扱われ、`SameSite=None` を要求しない
5. If OAuth の `state` 検証に失敗したとき, the 認証フロー shall セッション Cookie を確立せずエラー応答を返す

### Requirement 4: 内部 API 接続先設定の fail-fast

**Objective:** As a 運用者, I want 内部 API 接続先の設定が欠落しているときに起動時へ即座に失敗してほしい, so that 誤設定のまま本番が稼働してアクセス不能になる事態を防げる

#### Acceptance Criteria

1. If 実行時に内部 API 接続先設定が未指定または空文字のまま Web アプリケーションを起動したとき, the Web アプリケーション shall 起動を中断し失敗として終了する
2. If 内部 API 接続先設定が未指定で起動が失敗したとき, the Web アプリケーション shall 不足している設定項目を識別できるエラーメッセージを出力する
3. When 内部 API 接続先設定が有効な値で与えられたとき, the Web アプリケーション shall 起動を完了し API 転送を受け付ける

### Requirement 5: 環境間でのルーティング一貫性

**Objective:** As a 運用者, I want ローカル開発・本番のいずれでも同一の API ルーティング規約で動作させたい, so that 環境差異に起因する不具合や設定ミスを減らせる

#### Acceptance Criteria

1. The ローカル開発環境および本番環境 shall ブラウザから API へのアクセスに同一の `/api` ルーティング規約を用いる
2. When ローカル開発環境で API へアクセスしたとき, the Web アプリケーション shall 本番環境と同一のオリジン相対パス規約でバックエンドへ転送する
3. The 構成定義 shall ブラウザから見た API オリジンを環境ごとに切り替えるためのビルド時引数を要求しない

### Requirement 6: 既存機能の回帰防止

**Objective:** As a 開発者, I want 単一オリジン化の後も既存のフロントエンド・バックエンド機能が壊れないこと, so that 移行によるデグレードなく利用を継続できる

#### Acceptance Criteria

1. When 既存のフロントエンドテストスイートを実行したとき, the テストスイート shall すべて成功する
2. When 既存のバックエンドテストスイートを実行したとき, the テストスイート shall すべて成功する
3. The フロントエンドの API クライアント shall フィード・記事・購読・ユーザーの各既存エンドポイントへ同一オリジン相対パスで到達する

## Non-Functional Requirements

### NFR 1: 互換性・移行性

1. While 単一オリジン構成へ移行した後も, the フロントエンドの API クライアント shall 既存の各エンドポイントパス（`/api/feeds` 等）を変更せず利用できる
2. The 移行手順 shall 環境変数 `NEXT_PUBLIC_API_URL` の設定なしで本番デプロイを完了できる

### NFR 2: 可観測性

1. If 内部 API 接続先設定の欠落により起動が失敗したとき, the Web アプリケーション shall 失敗理由をログとして 1 件以上出力する

### NFR 3: セキュリティ

1. The セッション Cookie shall `HttpOnly` 属性を保持する
2. While ブラウザのアクセス先が HTTPS のとき, the セッション Cookie shall `Secure` 属性を保持する
3. The Web アプリケーション shall API 転送に伴ってブラウザへ機密情報（セッション値・OAuth クライアントシークレット等）を露出しない

## Out of Scope

- バックエンドの API エンドポイントのパス設計やビジネスロジックの変更（本機能はアクセス経路の統合のみを対象とする）
- 単一 ingress を提供する具体的なインフラ製品の選定・構築手順（design / 運用ドキュメントの領分）
- CORS ミドルウェアや関連オリジン設定の撤去・整理（#23 の領分。本機能では同一オリジン化に伴う前提変更のみを扱う）
- フロントエンド API クライアントの構造的リファクタリング（#25 の領分）
- CSP ヘッダの追加・強化（#53 の領分）
- 認証方式そのものの変更（Cookie セッションから他方式への移行等）
- 既存セッションデータやユーザーデータのマイグレーション

## 確認事項

> 以下は Issue 本文「Open Questions for Design Review」に対応し、要件として確定できない判断事項。
> design レビューで PoC 検証のうえ決定し、必要なら本書を更新する。Issue にコメントは存在せず人間の回答は未取得。

- **Set-Cookie 伝播の検証方法**: バックエンドが返す `Set-Cookie`（セッション Cookie・OAuth `state` Cookie）が
  単一オリジンのプロキシ経由でブラウザへ確実に伝播することを、どの手段（結合テスト / E2E / 手動 PoC）で
  保証するか。Requirement 2.5 / 3.1 の検証戦略として design で確定する必要がある。
- **セッション Cookie の SameSite 設定**: 現状の `SameSite=Lax` を維持するか、単一オリジン化に合わせて
  変更するか。first-party 化により `SameSite=None` は不要となる前提だが、`Lax` / `Strict` のいずれを
  採用するかは未確定（Requirement 3.4 は `SameSite=None` を要求しないことのみを規定）。
- **バックエンドのマウントポイントと `/api` プレフィックスの扱い**: 転送時に `/api` プレフィックスを strip して
  バックエンド（ルートに `/api/*` と `/auth/*` を持つ）へ渡すか、バックエンドを `/api` 配下へ再マウントするか。
  あわせて `/auth/*` 経路の転送規約（プレフィックス有無）も design で確定する必要がある。
- **OAuth リダイレクト先設定の更新範囲**: 単一オリジン化に伴い `GOOGLE_REDIRECT_URL` / `BASE_URL`
  （コールバック後のリダイレクト先・Cookie Secure 自動判定に利用）をブラウザ可視オリジンへ合わせる必要が
  あるか。Requirement 3.3 の挙動に影響するため、設定項目の更新範囲を design / 運用手順で明確化する。

## 関連

- Related: #23 #25 #53
