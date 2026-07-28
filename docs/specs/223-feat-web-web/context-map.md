# Context Map for #223 after #231 normative delta

`docs/specs/231-design-web-auth/design.md` Delta 1〜6 が、#223 の旧 registration
sequence・fail-closed 表・CSRF 説明・エラー状態を supersede する。

## 新規作成フロー

```text
Web
  registration/begin
    → navigator.credentials.create
    → registration/finish (exact Origin + JSON)
    → [user + credential + session] を同一 DB transaction で INSERT
    → commit
    → session_id Set-Cookie
    → ["auth", "me"] invalidate
    → AuthGuard
    → 2 ペイン UI

iOS
  registration/begin
    → registration/finish (Origin なし)
    → [user + credential] を同一 DB transaction で INSERT
    → commit
    → {user_id}（session / Cookie なし）
```

Web registration では、登録後の `authentication/begin`、二度目の
`navigator.credentials.get`、`authentication/finish`、登録用 `/api/auth/session` を行わない。

## fail-closed 5 列

W = `WEBAUTHN_RP_ID` + `WEBAUTHN_ORIGINS`、N = `NATIVE_AUTH_JWT_SECRET`、
C = valid な明示 `CORS_ALLOWED_ORIGIN`。

| W | N | C | Web capability | Web login exchange | Web registration direct session | iOS registration | iOS authentication |
|---|---|---|---|---|---|---|---|
| ✓ | ✓ | ✓ | 200 | 204 | 200 + Cookie | 200 | 200 |
| ✓ | ✗ | ✓ | 404 | 404 | 200 + Cookie（公式 UI からは未到達） | 200 | 200 |
| ✓ | ✓ | ✗ | 404 | 403 | 403 | 200 | 200 |
| ✗ | ✓ | ✓ | 404 | 204 | 404 | 404 | 404 |
| ✗ | ✗ | * | 404 | 404 | 404 | 404 | 404 |

capability は `SessionReady()`、valid exact Origin、`webRegistrationReady()` の全成立を
要求する。Web registration の readiness は tx beginner、session writer、session factory、
正の TTL / Cookie MaxAge を含む。iOS 2 列の gate は W のみで、N/C に依存しない。

## CSRF / PKCE 境界

- Web registration の主防御は authenticator authorization gesture、exact Origin、
  JSON Content-Type + CORS preflight、SameSite=Lax。PKCE は直接登録 session を防御しない。
- Web login exchange は exact Origin、JSON/CORS、SameSite=Lax に加え、単回・短 TTL の
  auth_code と PKCE を使う。Origin 不在・不一致・許可 Origin 未設定は 403。
- 残余リスクは同一オリジン XSS と、攻撃者自身の credential を使う login CSRF。
  前者は CSP/DOMPurify を主対策とし、後者の純粋な cross-site 経路は exact Origin/CORS で拒否する。

## 登録完了不明状態

```text
finish dispatch 前の preparation failure → server_error（未登録確定）
finish の任意の 4xx                 → server_rejected（拒否確定）
finish の fetch reject / Abort / 5xx /
  2xx body parse failure              → registration_uncertain

registration_uncertain
  → 「ログインで確認する」
  → discoverable login 成功
      → Cookie session → AuthGuard → 2 ペイン UI
  → authentication/finish の 400 AUTHENTICATION_FAILED
      → この時点でのみ「再度作成する」
      → registration/authentication の両 mutation を reset
```

begin の拒否、cancel、network、5xx、session exchange 失敗では「再度作成する」を提示しない。
