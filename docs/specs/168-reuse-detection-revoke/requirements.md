# Requirements Document

## Introduction

refresh token rotation（Issue #167）では、rotation 済みの旧 token が再び提示された場合、
それは応答消失後のクライアント再送か token 漏えいのいずれかであり、安全側に倒して
当該 rotation 系列（family）全体を失効させる必要がある（SERVER.md §1.4 の盗難対策）。
また iOS アプリのログアウト時には、クライアントが refresh token を明示失効できる
`POST /api/auth/revoke` が必要である（SERVER.md §1.3）。

本 spec は (1) rotation 済み token 再利用検知時の **family 全体失効への昇格**（#167 の
単純拒否分岐の拡張）、(2) **`POST /api/auth/revoke`** の追加、を対象とする。
Bearer middleware（#169）、IP レート制限（#171）、contract tests 全体整備（#172）は
スコープ外。

## Requirements

### Requirement 1: Rotation 済み token 再利用の検知と family 失効

**Objective:** As a Feedman 運用者, I want 旧 refresh token の再利用を漏えいの兆候として
扱いたい, so that 盗まれた token 系列によるアクセス継続を遮断できる

#### Acceptance Criteria

1. When rotation 済みの refresh token が refresh エンドポイントに提示されたとき, the Refresh Endpoint shall 当該 token が属する family 全体を失効させ、要求を拒否する
2. When 並行 rotation の競合に敗れた要求（atomic な rotation 確定で先行された提示）が検知されたとき, the Refresh Endpoint shall 同様に family 全体を失効させ、要求を拒否する
3. When family が失効されたとき, the Refresh Endpoint shall 当該 family に属するすべての refresh token による以後の再発行要求を拒否する
4. The Refresh Endpoint shall 再利用検知時の拒否応答を、通常の無効 token 拒否（Issue #167 Req 2.6）と区別できない同一応答とする
5. If family 失効の永続化が失敗したとき, the Refresh Endpoint shall 新 token を発行せず要求を拒否する

### Requirement 2: Revoke エンドポイント

**Objective:** As a Feedman iOS アプリ, I want ログアウト時に refresh token を明示失効したい,
so that 端末から削除した token がサーバー側でも使用不能になる

#### Acceptance Criteria

1. When revoke エンドポイントが既知の refresh token を受け取ったとき, the Revoke Endpoint shall 当該 token が属する family 全体を失効させ、ボディなしの成功応答（204）を返す
2. When 不明・失効済み・rotation 済み・期限切れの refresh token を受け取ったとき, the Revoke Endpoint shall token の存在有無を区別できない同一の成功応答（204）を返す（冪等）
3. When family が revoke エンドポイント経由で失効されたとき, the Refresh Endpoint shall 当該 family の全 token による再発行要求を拒否する
4. The Revoke Endpoint shall Cookie セッション・Bearer 認証のいずれも無しで呼び出し可能とする（refresh token の所持自体を失効権限とみなす）
5. If リクエストボディが不正な JSON、または必須フィールド（`refresh_token`）が欠落しているとき, the Revoke Endpoint shall 入力不正として要求を拒否する
6. The Revoke Endpoint shall 既存 Web ログアウト（Cookie セッション破棄）の挙動を変更しない

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The Refresh Endpoint and Revoke Endpoint shall refresh token の平文を永続化領域・ログ・エラーメッセージのいずれにも残さない
2. The Revoke Endpoint shall 応答内容・ステータスから token の存在有無・状態を推測できないようにする
3. The Refresh Endpoint and Revoke Endpoint shall エラー応答にクライアント入力値・内部詳細を反射しない

### NFR 2: 後方互換性

1. The API Server shall 既存のすべてのルート（Web ログイン / ログアウト・既存 API・token 交換・refresh の正常系）の挙動を変更しない
2. The API Server shall 署名鍵未設定の環境では revoke エンドポイントも公開しない（token 交換・refresh と同一の fail-closed 縮退）

### NFR 3: テスト容易性

1. The Native Auth Module shall 再利用検知・family 失効・revoke 成功・revoke 冪等性・revoke 後の refresh 拒否の各ケースを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- `POST /api/auth/token` の初回交換（#166）と refresh の基本 rotation（#167）
- Bearer middleware による既存 API の Bearer 対応（#169）
- 未認証 IP レート制限（#171）と contract tests 全体整備（#172）
- iOS 側のログアウト UI / token 破棄処理
- access token（JWT）の即時失効（jti ブラックリスト等）— 短命（900 秒）で吸収する設計（SERVER.md §1.4）
- 再利用検知イベントの通知・アラート連携（ログ出力までを本 spec で行う）

## Open Questions

- 再利用検知の strict 方針（応答消失後の正規クライアント再送も family 失効になる）は
  SERVER.md §1.4「失効済みトークンの提示時は当該ファミリ全失効」に従う。誤検知時も
  ユーザーは再ログインで回復できる（データ損失なし）
- `POST /api/auth/revoke` は SERVER.md §1.3 に「Bearer 認証下」と注記があるが、Bearer
  middleware は後続 Issue #169 の領分であること、および RFC 7009（OAuth 2.0 Token
  Revocation）が public client の revocation を token 所持ベースで許す慣行であることから、
  本 spec では**未認証 + token 所持 = 権限**の境界を採用する（Issue 本文「仮案・判断を
  委ねたい点」への回答。失効は破壊のみで奪取に使えず、冪等 204 で列挙オラクルも生じない）

## 関連

- Parent: #163
- Depends on: #167
- Sibling: #164 #165 #166 #169 #170 #171 #172
