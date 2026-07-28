# Requirements Document

## Introduction

本 spec は Issue #223（Web ログイン画面のパスキー導線追加）に対する **差分設計（normative delta）**
の prerequisite である。#223 の実装 PR #229 のレビュー過程で、#223 spec（requirements /
design / tasks）と #229 の実挙動・実変更ファイルの間に 5 件の不整合が発見された。それらは
いずれも #223 spec を上書きまたは補正しないと Reviewer 判定・後続保守が破綻するため、
差分要件・差分設計・差分タスクを本 spec（`docs/specs/231-design-web-auth/`）に隔離して確定し、
design PR merge 後に PR #229 の needs-iteration 1 回で製品コードと impl-notes / context-map
へ反映する運用とする。

本 spec は **#223 の requirements.md / design.md / tasks.md を書き換えない**（#223 spec の
物理ファイルには一切手を入れない）。かわりに本 spec の各 Requirement 内で、どの
#223 requirement / design 節を supersede（上書き・補正）するかを明記する。#223 spec の
その他の要件・設計判断は本 spec の変更対象外であり、そのまま有効である。

本 spec の対象範囲は「観察可能な契約・境界の補正」に限定する。#223 spec と同じく、実装詳細
（内部関数名・具体 SQL・ライブラリ内部呼び出し）は書かない。ただし「追加の WebAuthn 認証操作を
要求しない」「iOS パスキー API 契約を破壊しない」「特定 config ファイルを PR #229 の変更対象に
含める」といった、ユーザー／運用者が観察できる契約・境界は本 spec で要件化する。

人間運用者が Issue #231 コメントで確定した 3 つの決定事項（後述 Requirement 1 / 4 / 6 に反映）
は本 spec の前提として組み込み済みであり、本 spec の Open Questions からは除外する。

## Requirements

### Requirement 1: 新規作成 finish 直後の追加 WebAuthn 操作の禁止

**Objective:** As a パスキーで新規作成を完了しようとする Web 訪問者, I want 登録の authorization
gesture（パスキー作成の WebAuthn ceremony。現設定は user verification を必須化しないため、必ずしも
生体認証プロンプトではない）を一度だけ行い、以降は追加のブラウザ WebAuthn プロンプト（追加の
authorization gesture）なしで Web の 2 ペイン UI に到達したい,
so that 新規作成体験が「登録操作 → 完了」の 1 ステップとして完結する

**Supersedes:** 本要件は #223 Requirement 2.3 および #223 Requirement 3.1 の「追加操作なしで
Cookie セッション合流」の意味を、#223 design.md 「新規作成フロー（Sequence / 抜粋）」節が
含む二度目の `navigator.credentials.get()` 呼び出しを **禁止する** 方向に補正する。#223 spec の
2.3 / 3.1 の文言自体は維持され、本要件はその挙動具現化における「追加操作」の解釈を厳密化する。

#### Acceptance Criteria

1. When 新規作成の登録 finish がサーバから成功応答を受け取ったとき, the Web Passkey Registration
   Flow shall 追加のブラウザ WebAuthn プロンプト（追加の authorization gesture / WebAuthn 認証
   セレモニー）を起動せずに、Cookie セッションベース認証状態への合流処理を進行させる
2. If ブラウザまたはユーザーが登録セレモニーとは別の第 2 の認証セレモニー実行を要求される場合,
   the Web Passkey Registration Flow shall そのフローを新規作成の正常系と扱わず、実装として
   採用しない
3. When Web（許可オリジンからのリクエスト）の登録 finish が検証に成功したとき, the Feedman
   API shall user・credential・Web session の 3 行を **単一 DB トランザクション**で作成し、
   commit 後に同一 finish 応答（JSON body は `{user_id}` のまま不変）へ session Cookie を
   `Set-Cookie` で付与し、追加の request・追加 ceremony・auth_code 交換を一切要求しない
   （native/iOS からの同一 endpoint は user・credential の **2 行のみ**を作成し、session 行も
   Cookie も発行しない）
4. When 新規作成が完了し Cookie セッションに合流したとき, the Web App shall Google OAuth
   経由でログインしたときと同一の 2 ペイン UI を初期表示する
5. The Web Passkey Registration Flow shall #223 Requirement 4（既存パスキーによるログイン）で
   ユーザーが明示的にログイン導線を選んだときの追加認証セレモニーの正当性には影響を与えない
   （ログイン導線での認証プロンプト提示は依然として正常挙動）

### Requirement 2: PR #229 の実変更対象ファイルの明示化

**Objective:** As a #223 spec を参照して PR #229 のスコープを判定する Reviewer / Architect,
I want File Structure Plan と tasks が PR #229 の実変更対象ファイルを漏れなく列挙している
状態を得たい, so that 変更対象が spec の File Structure Plan と食い違う理由で Reviewer 判定が
破綻しない

**Supersedes:** 本要件は #223 design.md 「File Structure Plan」節の以下を補正する。
`web/src/lib/api.ts` を「無変更」と記述していた箇所を「変更対象」に、`web/src/lib/api.test.ts`
・`.env.sample`・`docker-compose.yml` を File Structure Plan に列挙されていなかった状態から
「変更対象」に、それぞれ明示する。#223 requirements.md 側の要件見出しは変更しない。

#### Acceptance Criteria

1. The #231 Design shall PR #229 が変更する Web ファイル（`web/src/lib/api.ts` および
   `web/src/lib/api.test.ts` を含む）を File Structure Plan 差分の「変更対象」区分に明示する
2. The #231 Design shall PR #229 が変更する運用 config ファイル（`.env.sample` および
   `docker-compose.yml`）を File Structure Plan 差分の「変更対象」区分に明示する
3. The #231 Tasks shall 上記変更対象ファイルへの差分反映作業を、PR #229 の needs-iteration
   1 回で完結できる粒度のタスクとして列挙する
4. The #231 Tasks shall #223 spec 側の requirements.md / design.md / tasks.md を書き換える
   タスクを **含まない**
5. When Reviewer が PR #229 の変更ファイル一覧を #231 design の File Structure Plan と
   突き合わせたとき, the Reviewer shall PR #229 の全変更ファイルが #231 design で
   「変更対象」または「新規追加」として説明されている状態を確認できる

### Requirement 3: fail-closed 挙動と iOS #216 パスキー API 契約の両立

**Objective:** As a Feedman 運用者（env 組合せの異なる複数環境をデプロイし iOS クライアントも
サポートする）, I want fail-closed 表が実際の Web 側縮退挙動と iOS 側 API 提供状態を正確に
表現している状態を得たい, so that iOS 用パスキー API が停止していないのに「停止している」と
運用が誤認する事故を避けられる

**Supersedes:** 本要件は #223 design.md 「fail-closed 挙動」節の表を補正する。特に、Web 用
セッション交換 endpoint の未提供と、iOS #216 が既に依存しているパスキー registration /
authentication endpoint 群の提供状態を、env 組合せごとに **別列** として正確に表現する
（現行 #223 design 表は Web 用 env 未設定時に全パスキー endpoint が 404 になると誤って表現
している）。#216 requirements / design はそのまま維持し、本要件からは変更しない。

#### Acceptance Criteria

1. The #231 Design shall Web 用セッション交換 endpoint の登録・未登録を、パスキー
   registration / authentication endpoint 群（iOS #216 が依存）の登録・未登録と **独立した
   列** として fail-closed 表に表現する
2. The #231 Design shall Web 用パスキー capability probe endpoint の登録・未登録を、
   パスキー registration / authentication endpoint 群の登録・未登録と **独立した列** として
   fail-closed 表に表現する
3. When Web 用セッション交換に必要な env（native 認証系）のみが未設定で、パスキー
   registration / authentication endpoint 群に必要な env（WebAuthn 系）が設定されている
   環境で運用したとき, the Feedman API shall iOS #216 の request / response 契約を
   維持したままパスキー registration / authentication endpoint 群を提供し続け、
   Web 側は Google 単体表示に縮退する
4. When パスキー registration / authentication endpoint 群に必要な env（WebAuthn 系）が
   未設定の環境で運用したとき, the Feedman API shall Web 用 capability probe を 404 とし、
   Web 側は Google 単体表示に縮退する（iOS 側からも当該 env 依存の endpoint は利用不能に
   なるが、これは #216 で既に確定済みの挙動）
5. The #231 Design shall iOS #216 の API 契約
   （`/api/passkey/registration/*` / `/api/passkey/authentication/*` の request / response
   の各フィールド構造）を破壊的に変更しないことを、fail-closed 表の記述および
   File Structure Plan の変更対象から明示的に排除する形で示す
6. If Web 直接登録 session の発行に必要な readiness（許可 exact Origin・session 書込・
   登録トランザクション・session factory・正の TTL / Cookie MaxAge のいずれか）が未充足の
   環境で運用したとき, the Feedman API shall Web 用 capability probe を 404 とし
   （公式 UI からパスキー導線を出さない fail-closed）、登録 endpoint が 200 を返すのに
   capability だけ 200 になって signup が 500 で破綻する不整合を発生させない

### Requirement 4: CSRF / PKCE 説明の正確化と残余リスクの明示

**Objective:** As a セキュリティレビューを行う運用者 / Reviewer, I want ログインフローの
CSRF 対策の記述が実際に有効な防御要素と限界を正確に説明している状態を得たい, so that
「攻撃者は auth_code + code_verifier ペアを取得できない」という不正確な前提に基づいて
残余リスクを過小評価する事故を避けられる

**Supersedes:** 本要件は #223 design.md 「Security Considerations」節の CSRF 記述および
`NativeAuthHandler.Session` 節の「CSRF 対策」記述を補正する。特に、旧記述の「攻撃者が事前に
(auth_code, code_verifier) ペアを入手する経路が存在しない」の主張を撤回し、正しい防御要素と
残余リスクを記述する。#223 requirements.md の NFR 1.4（同一オリジンのみ実行）はそのまま維持
される。

人間運用者による決定事項として、追加の browser-bound state（transaction cookie / CSRF token /
Origin-bound challenge 拡張）は本 spec では要求せず、下記に列挙する主防御を「受容」する構成を
採用する。

#### Acceptance Criteria

1. The #231 Design shall Web セッション交換 endpoint に対する CSRF 主防御として、
   同一オリジン Origin 検証・JSON Content-Type 要求・CORS preflight・SameSite Cookie 属性・
   PKCE 束縛の各要素の役割を要素ごとに明記する
2. The #231 Design shall 旧 #223 design の「攻撃者は (auth_code, code_verifier) ペアを取得
   できない」旨の記述が不正確である旨と、正しい前提（攻撃者は自身のブラウザで自身の
   auth_code + code_verifier を取得できる）を明記する
3. The #231 Design shall 上記主防御を受容した上での残余リスクを列挙する（少なくとも、
   同一オリジン内 XSS 経由 / 攻撃者自身の credential による攻撃者アカウントへの誘引の
   2 種を含む）
4. The #231 Design shall 追加の browser-bound state（transaction cookie / 追加 CSRF token /
   Origin-bound challenge 拡張）を本 spec では要求しないことを、決定事項として明記する
5. The #231 Design shall セッション交換 endpoint の Cookie 属性（HttpOnly / SameSite /
   Secure / Path / Max-Age）を既存 Google OAuth Callback と一致させる規約を維持する
6. If 将来 XSS 由来の CSRF 突破や credential 差し替え型攻撃を封じる必要が発生したとき,
   the #231 Design shall 残余リスクとして列挙した箇所を出発点に再検討する導線を残す
   （追加防御を導入する Issue を将来切り出す前提を明示する）
7. The #231 Design shall PKCE（`code_challenge` / `code_verifier`）が Web 直接登録 session の
   発行経路を防御しないこと（登録 begin の `code_challenge` は #216 契約維持のための形式検証
   のみで、永続化・束縛せず、session 発行に用いない）を明記し、PKCE の役割を login auth_code
   交換経路（`POST /api/auth/session`）に限定して記述する

### Requirement 5: 登録完了不明状態（第 3 状態）の提示と discoverable ログイン復旧導線

**Objective:** As a パスキー新規作成中にネットワーク断・サーバ 5xx を経験した Web 訪問者,
I want 「登録が確定した／されていない」を単純な二値で誤って断定されず、次に取れる行動を
案内される状態を得たい, so that 実際は登録済みなのに再登録を試みてユーザー名重複エラーで
詰まる事故を避けられる

**Supersedes:** 本要件は #223 spec が扱っていない新規要件を追加する。#223 Requirement 2.8 /
3.4 は「サーバエラー時の汎用エラー表示」および「合流失敗時のログイン画面復帰」を規定するが、
「サーバ側の登録 commit は成立したがクライアント側でレスポンスが失われた」ケースを
第 3 の状態として明示していない。本要件はこのギャップを埋める形で追加される。

#### Acceptance Criteria

1. When 登録 finish の `fetch()` 呼出し後に確定的な pre-commit の 4xx 応答を得られなかった
   （実送達可否を判定できないネットワーク断・timeout・fetch 中 Abort・5xx 応答・
   commit 済みを示唆する 2xx だが
   応答 body が欠損／途中切断／parse 不能）とき, the Web Passkey Registration Flow shall
   そのケースを「登録成功」「登録失敗」のいずれとも断定せず、「登録が確定したかどうか判別
   できない」旨の第 3 の完了不明状態としてユーザーに提示する
2. When 完了不明状態を提示するとき, the Web Passkey Registration Flow shall ユーザーが
   次に取るべき行動として「ログイン導線から discoverable なパスキーログイン（ユーザー名の
   入力を伴わない）を試すことで、登録の成否を確認する」旨の案内を提示し、再作成導線は
   同列に並置せず、この確認ログインが失敗した後にのみ提示する
3. If 完了不明状態のユーザーが discoverable なパスキーログインを試行して成功したとき,
   the Web App shall 通常の Cookie セッション認証状態に到達し 2 ペイン UI を提示する
4. If 完了不明状態のユーザーが discoverable なパスキーログインを試行し、authentication finish が
   HTTP 400 `AUTHENTICATION_FAILED` で拒否されたとき, the Web Passkey Registration Flow shall
   内部理由（credential 未解決等）を区別して提示せず、その一律失敗の後にはじめてユーザーに
   再度新規作成を試みる導線を提示する
5. The Web Passkey Registration Flow shall 完了不明状態の表示テキストにサーバの内部詳細
   （スタックトレース・内部エラー原因・SQL / DB 名等）を反射しない
6. The Web Passkey Registration Flow shall 完了不明状態を、ユーザー名形式不正・
   ユーザー名重複・キャンセル・サーバ拒否のいずれとも区別可能な文言で提示する
   （既存の #223 Requirement 2.5 / 2.6 / 2.7 / 2.8 の表示と混同されないこと）

### Requirement 6: 運用 config 変更を PR #229 に統合する運用境界

**Objective:** As a PR #229 をレビューする Reviewer / #231 spec を参照する Architect,
I want `.env.sample` と `docker-compose.yml` の変更を PR #229 に正式な変更対象として
統合された状態を得たい, so that config 変更を別 prerequisite PR に分離することで PR 分岐が
複雑化することを避けられる

**Supersedes:** 本要件は #223 design.md 「Configuration」節の「Web 側: 追加 env 無し」および
「サーバ側: 追加 env 無し」の記述を補正する。#223 spec 全体を「env 追加なし」で通す既定は
維持するが、`.env.sample` および `docker-compose.yml` の記述差分（既存 env の記述整備・
デプロイ表面の記述整合）は PR #229 のスコープに含まれることを本要件で明示する。

人間運用者の決定事項として、この config 変更を別 prerequisite PR として分離することは
採用しない。

#### Acceptance Criteria

1. The #231 Design shall `.env.sample` の記述差分を PR #229 の変更対象として File Structure
   Plan 差分に明示する
2. The #231 Design shall `docker-compose.yml` の記述差分を PR #229 の変更対象として File
   Structure Plan 差分に明示する
3. The #231 Tasks shall 上記 config 変更の反映作業を、PR #229 の needs-iteration 1 回で
   完結できる粒度のタスクとして列挙する
4. The #231 Design shall 上記 config 変更を別 prerequisite PR として分離しないことを、
   決定事項として明記する
5. The #231 Design shall 上記 config 変更が #223 requirements.md の Scope（既存 env 定義の
   流用に留め、新規 env は追加しない）を破らないことを、変更差分の内容と併記する形で示す

## Non-Functional Requirements

### NFR 1: 差分適用の運用境界

1. The #231 Spec shall #223 spec の requirements.md / design.md / tasks.md の物理ファイルを
   一切書き換えず、本 spec 内（`docs/specs/231-design-web-auth/`）でのみ差分を宣言する
2. The #231 Spec shall 独立した実装 PR を作らず、design PR merge 後に PR #229 の
   needs-iteration 1 回で製品コード・テスト・`impl-notes.md` / `context-map.md` へ差分を反映
   させる運用境界を維持する
3. The #231 Tasks shall #216（iOS パスキー API）の requirements.md / design.md / tasks.md
   の物理ファイル、および `/api/passkey/registration/*` / `/api/passkey/authentication/*` の
   request / response 契約を変更するタスクを **含まない**

### NFR 2: 機密情報の非漏出（#223 NFR 1 の踏襲）

1. The Web Passkey Registration Flow shall Requirement 5 の完了不明状態の判定・表示・
   復旧導線に関わるすべての経路で、attestation 生バイト列・チャレンジ識別子・auth_code
   平文・code_verifier 平文をブラウザのコンソールログ・localStorage / sessionStorage・
   URL クエリ文字列・エラー表示テキストに残さない
2. The #231 Design shall CSRF 記述および fail-closed 表の記述で、内部の秘密値（session_id
   生値・auth_code 平文・RP ID の運用固有値など）を例示に用いない

### NFR 3: 既存挙動の後方互換

1. The Web App shall #231 の差分適用後も、#223 Requirement 6（既存 Google OAuth ログイン
   挙動の後方互換）に規定される既存 UI・既存機能・既存テストのいずれをも本差分導入前と
   同一に動作させる
2. The Feedman API shall #231 の差分適用後も、#216（iOS パスキー API）の request / response
   契約を破壊的に変更しない

## Out of Scope

- Issue #230 が扱う範囲は「user レコードと credential レコードの作成」までであり、Web 直接
  session（Requirement 1）で必要となる **session 行を含む単一 DB トランザクションへの拡張
  （3 行 atomic）は本 spec / PR #229 の責務**である（#230 に委ねない）。本 spec は
  Requirement 1 の合流方式（追加 ceremony なしで finish 応答が Cookie session を確定する）を
  要件として宣言し、その 3 行トランザクション化の設計形態は #231 design.md で確定する
- 本 spec 内で PR #229 の製品コード（`internal/**` / `web/src/**` の実装ファイル）を直接
  変更すること（実反映は design PR merge 後の PR #229 needs-iteration 1 回で行う）
- #216 で確定済みの iOS 向けパスキー API 契約
  （`/api/passkey/registration/*` / `/api/passkey/authentication/*` の request / response
  フィールド構造）の破壊的置換
- 追加の browser-bound state（transaction cookie / 追加 CSRF token / Origin-bound challenge
  拡張）の導入（Requirement 4 の決定事項として本 spec では要求しない）
- 別 prerequisite PR として `.env.sample` / `docker-compose.yml` 変更を切り出すこと
  （Requirement 6 の決定事項として本 spec では採用しない）
- パスキー credential のセルフサービス管理 UI・アカウント統合・Sign in with Apple の Web
  導入など、#223 spec が既に Out of Scope としている事項

## 確定済み決定事項（人間運用者による #231 決定）

以下は Issue #231 の人間運用者コメントで確定済みであり、本 spec の前提として組み込む
（Open Questions からは除外する）:

- **合流方式**: Requirement 1 は **`POST /api/passkey/registration/finish` の検証成功から
  同一 DB トランザクションで user・credential・Web session の 3 行を作成し、commit 後に同じ
  finish 応答で `Set-Cookie` する直接 session 方式**で満たす。登録用 auth_code / 二度目の
  `navigator.credentials.get()` / 登録専用の `/api/auth/session` 呼び出し / exchange artifact は
  導入しない。`/api/auth/session` は既存パスキー **ログイン** 用（auth_code 交換）として残す
- **#216 JSON 契約**: `registration/begin` / `registration/finish` の request / response JSON
  フィールドは変更しない（finish 応答は Web / iOS とも `{user_id}` のまま。Web の差分は
  `Set-Cookie` ヘッダのみ）。iOS には session 行も Cookie も作らない
- **PKCE の役割分離**: 登録 begin の `code_challenge` は #216 契約維持のためフィールドを残すが
  **形式検証のみ**で永続化・束縛しない。PKCE は直接登録経路を防御せず、ログイン auth_code
  交換にのみ役割を持つ
- **fail-closed 表の列**: Web capability / Web login exchange / Web registration direct session /
  iOS registration/* / iOS authentication/* を **別列** として表現する（env 軸は
  `WEBAUTHN_RP_ID`+`WEBAUTHN_ORIGINS` / `NATIVE_AUTH_JWT_SECRET` / `CORS_ALLOWED_ORIGIN`）
- **完了不明状態の UI 実現方式（旧 Open Question / design で確定）**: Requirement 5.1 / 5.2 の
  「第 3 状態」は **既存 `passkey-signup-dialog.tsx` 内の状態分岐**（専用完了画面を新設しない）で
  提示する。提示順序は「discoverable ログインで確認 → authentication finish の HTTP 400
  `AUTHENTICATION_FAILED` の後にのみ再作成導線」に固定する
  （design.md §Delta 5 §状態遷移図で確定）。未決事項として残さない
- **完了不明状態と Requirement 3.4 の関係（旧 Open Question / design で確定）**: 直接 session 化に
  より registration 経路の「session 交換段の失敗」は消滅するため、#223 Requirement 3.4（合流
  失敗時のログイン画面復帰）と本 spec Requirement 5 は **finish 応答が確定したか否か**で分岐する
  （parse 可能な期待形の 2xx=成功 / 確定 4xx=拒否 / それ以外=uncertain）。両者の状態遷移は
  design.md §Delta 5 §状態
  遷移図に統合済みで、表示重複は生じない。未決事項として残さない

## 関連

- Parent: #223
- Split from: #223
- Related: #224 #229 #230
