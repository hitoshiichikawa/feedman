package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/repository"

	_ "github.com/lib/pq"
)

// 本ファイルは Issue #172 の E2E DB-backed 契約テスト。
// real auth.Service / real auth.TokenService / real PostgresAuthCodeRepo /
// real PostgresRefreshTokenRepo / real JWTIssuer / real JWTVerifier を wire し、
// native OAuth callback → token 交換 → refresh rotation → revoke → Bearer 付き
// 既存 API 到達の動線を 1 経路で通す（design.md「E2E DB-backed Test」）。
//
// 環境変数 TEST_DATABASE_URL の PostgreSQL に接続できない環境では t.Skipf で
// スキップされる（NFR 1.1。CI では postgres service container 上で実行される）。
// OAuthProvider だけは外部ネットワークを避けるため fake を注入する。

// e2eJWTSecret は E2E 用の固定署名鍵（NFR 1.3。production secret ではない。
// router_test.go の固定鍵パターンと同型）。
var e2eJWTSecret = []byte("e2e-contract-test-secret-32bytes!")

// e2eCodeVerifier は RFC 7636 §4.1 形式（unreserved 43〜128 文字）の固定 verifier。
const e2eCodeVerifier = "e2e-contract-code-verifier-0123456789abcdefghijklmn"

// e2eWrongVerifier は形式は正しいが challenge と一致しない verifier（Req 3.2 用）。
const e2eWrongVerifier = "e2e-contract-wrong-verifier-0123456789abcdefghijklm"

// e2eCodeChallenge は e2eCodeVerifier から導出した S256 challenge。
func e2eCodeChallenge() string {
	sum := sha256.Sum256([]byte(e2eCodeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// e2eTestDatabaseURL はテスト用のデータベース URL を返す
// （repository パッケージの withdrawTestDatabaseURL と同方針）。
func e2eTestDatabaseURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupNativeAuthE2EDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skipf でテストをスキップする
// （repository.setupWithdrawTestDB と同型のセットアップ）。
func setupNativeAuthE2EDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := e2eTestDatabaseURL()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	cleanupSQL := `
		DROP TABLE IF EXISTS passkey_challenges CASCADE;
		DROP TABLE IF EXISTS passkey_credentials CASCADE;
		DROP TABLE IF EXISTS refresh_tokens CASCADE;
		DROP TABLE IF EXISTS refresh_token_families CASCADE;
		DROP TABLE IF EXISTS auth_codes CASCADE;
		DROP TABLE IF EXISTS user_cross_feed_views CASCADE;
		DROP TABLE IF EXISTS sessions CASCADE;
		DROP TABLE IF EXISTS user_settings CASCADE;
		DROP TABLE IF EXISTS item_states CASCADE;
		DROP TABLE IF EXISTS subscriptions CASCADE;
		DROP TABLE IF EXISTS items CASCADE;
		DROP TABLE IF EXISTS feeds CASCADE;
		DROP TABLE IF EXISTS identities CASCADE;
		DROP TABLE IF EXISTS users CASCADE;
		DROP TABLE IF EXISTS schema_migrations CASCADE;
	`
	if _, err := db.Exec(cleanupSQL); err != nil {
		db.Close()
		t.Fatalf("クリーンアップに失敗: %v", err)
	}
	if err := database.RunMigrations(dbURL); err != nil {
		db.Close()
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}
	return db
}

// fakeOAuthProviderForE2E は外部ネットワークを避けるための最小 OAuthProvider fake。
// ExchangeCode は固定の OAuthUserInfo を返す（NFR 1.1）。
type fakeOAuthProviderForE2E struct{}

func (f *fakeOAuthProviderForE2E) GetLoginURL(state string) string {
	return "https://accounts.google.com/o/oauth2/auth?state=" + state
}

func (f *fakeOAuthProviderForE2E) ExchangeCode(ctx context.Context, code string) (*auth.OAuthUserInfo, error) {
	return &auth.OAuthUserInfo{
		ProviderUserID: "google-sub-e2e",
		Email:          "e2e@example.com",
		Name:           "E2E Contract User",
		Provider:       "google",
	}, nil
}

// newNativeAuthE2ERouter は real service / repo / JWT を wire した router を構築する。
func newNativeAuthE2ERouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	userRepo := repository.NewPostgresUserRepo(db)
	identRepo := repository.NewPostgresIdentityRepo(db)
	sessionRepo := repository.NewPostgresSessionRepo(db)
	authCodeRepo := repository.NewPostgresAuthCodeRepo(db)
	refreshTokenRepo := repository.NewPostgresRefreshTokenRepo(db)

	issuer := auth.NewJWTIssuer(e2eJWTSecret, "v1-e2e")
	verifier := auth.NewJWTVerifier(e2eJWTSecret)
	tokenService := auth.NewTokenService(authCodeRepo, refreshTokenRepo, issuer)
	authService := auth.NewService(
		&fakeOAuthProviderForE2E{}, userRepo, identRepo, sessionRepo, authCodeRepo,
		auth.ServiceConfig{SessionMaxAge: 86400},
	)

	deps := &RouterDeps{
		SessionFinder:     sessionRepo,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService:       authService,
		AuthConfig:        AuthHandlerConfig{BaseURL: "http://localhost:3000"},
		NativeAuthHandler: NewNativeAuthHandler(tokenService),
		JWTVerifier:       verifier,
		SubscriptionService: &mockSubscriptionService{
			// userID 連動 fixture: Bearer 経由で context に注入された userID が
			// fake OAuthProvider から解決された user.ID と一致することを応答で確認する。
			listSubscriptionsFn: func(ctx context.Context, uid string) ([]subscriptionResponse, error) {
				return []subscriptionResponse{{ID: "sub-of-" + uid, FeedID: "feed-e2e"}}, nil
			},
		},
	}
	return NewRouter(deps)
}

// e2eRunNativeLoginAndCallback は native login → callback を実 service 経路で実行し、
// アプリスキーム Location から払い出された平文 auth_code を返す。
func e2eRunNativeLoginAndCallback(t *testing.T, router http.Handler) string {
	t.Helper()

	// native login: state / challenge Cookie を取得
	loginURL := "/auth/google/login?flow=native&code_challenge=" + e2eCodeChallenge() + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodGet, loginURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	var oauthStateC, nativeChallengeC *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "oauth_state":
			oauthStateC = c
		case "oauth_native_challenge":
			nativeChallengeC = c
		}
	}
	if oauthStateC == nil || nativeChallengeC == nil {
		t.Fatal("login: expected both oauth_state and oauth_native_challenge cookies")
	}

	// callback: real auth.Service.HandleNativeCallback が auth_codes 行を作成し、
	// アプリスキームへ平文 auth_code をリダイレクトする（Req 1.1）
	callbackURL := "/auth/google/callback?code=e2e-google-code&state=" + oauthStateC.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateC)
	req.AddCookie(nativeChallengeC)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp = w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("callback Location parse error: %v", err)
	}
	if u.Scheme != "feedman" {
		t.Fatalf("callback Location scheme = %q, want feedman", u.Scheme)
	}
	authCode := u.Query().Get("auth_code")
	if authCode == "" {
		t.Fatal("callback Location: auth_code is empty")
	}
	return authCode
}

// e2ePostJSON は JSON POST を送り Recorder を返す。
func e2ePostJSON(router http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// e2eDecodeTokenPair は token 系応答の 4 フィールド契約を検証して値を返す。
func e2eDecodeTokenPair(t *testing.T, w *httptest.ResponseRecorder, label string) (accessToken, refreshToken string) {
	t.Helper()
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d, want %d (body=%s)", label, w.Result().StatusCode, http.StatusOK, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("%s decode body: %v", label, err)
	}
	if len(body) != 4 {
		t.Errorf("%s response has %d keys, want exactly 4", label, len(body))
	}
	if body["token_type"] != "Bearer" {
		t.Errorf("%s token_type = %v, want Bearer", label, body["token_type"])
	}
	if v, _ := body["expires_in"].(float64); v != 900 {
		t.Errorf("%s expires_in = %v, want 900", label, body["expires_in"])
	}
	accessToken, _ = body["access_token"].(string)
	refreshToken, _ = body["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("%s access_token / refresh_token must be non-empty", label)
	}
	return accessToken, refreshToken
}

// e2eAssertRejection は拒否応答の status / code と、message に原因区別語が含まれない
// ことを検証する（Req 3.6 / 既存 TestIntegration_* と同方針）。
func e2eAssertRejection(t *testing.T, w *httptest.ResponseRecorder, label string, wantStatus int, wantCode string) {
	t.Helper()
	if w.Result().StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d (body=%s)", label, w.Result().StatusCode, wantStatus, w.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(w.Result().Body).Decode(&body)
	if body["code"] != wantCode {
		t.Errorf("%s code = %v, want %q", label, body["code"], wantCode)
	}
	msg, _ := body["message"].(string)
	for _, word := range []string{"used", "expired", "rotated", "revoked", "signature", "unknown", "consumed", "verifier", "mismatch"} {
		if strings.Contains(strings.ToLower(msg), word) {
			t.Errorf("%s message %q must not contain cause-distinguishing word %q (Req 3.6)", label, msg, word)
		}
	}
}

// e2eQueryInt は単一整数値クエリを実行する検証ヘルパー。
func e2eQueryInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return n
}

// TestE2E_NativeAuthFullFlow_DBBacked は native auth の全動線を実物経路で通す
// （Req 1.1〜1.6, 3.1〜3.4 / design.md「State transitions checked」step1〜6）。
func TestE2E_NativeAuthFullFlow_DBBacked(t *testing.T) {
	db := setupNativeAuthE2EDB(t)
	defer db.Close()
	router := newNativeAuthE2ERouter(t, db)

	// --- step1〜2: native login → callback（auth_code 発行 / Req 1.1） ---
	authCode1 := e2eRunNativeLoginAndCallback(t, router)
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM auth_codes WHERE used = false`); got != 1 {
		t.Fatalf("auth_codes (unused) = %d, want 1 (callback で 1 行作成)", got)
	}

	// --- step3: token 交換（PKCE 検証 + 本トークン交換 / Req 1.2） ---
	w := e2ePostJSON(router, "/api/auth/token",
		fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authCode1, e2eCodeVerifier))
	access1, refresh1 := e2eDecodeTokenPair(t, w, "exchange")

	// DB: auth_code は単回消費済み、refresh family + token が永続化されている
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM auth_codes WHERE used = true`); got != 1 {
		t.Errorf("auth_codes (used) = %d, want 1 (単回消費確定)", got)
	}
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM refresh_token_families`); got != 1 {
		t.Errorf("refresh_token_families = %d, want 1", got)
	}
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM refresh_tokens`); got != 1 {
		t.Errorf("refresh_tokens = %d, want 1", got)
	}

	// --- step3-reject: 不正 / 使用済み auth_code・verifier 不一致（Req 3.1, 3.2） ---
	w = e2ePostJSON(router, "/api/auth/token",
		fmt.Sprintf(`{"auth_code":"never-issued-code","code_verifier":%q}`, e2eCodeVerifier))
	e2eAssertRejection(t, w, "exchange unknown code", http.StatusBadRequest, "INVALID_GRANT")

	w = e2ePostJSON(router, "/api/auth/token",
		fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authCode1, e2eCodeVerifier))
	e2eAssertRejection(t, w, "exchange consumed code", http.StatusBadRequest, "INVALID_GRANT")

	// 新しい auth_code を払い出し、不一致 verifier で拒否されることを確認
	authCode2 := e2eRunNativeLoginAndCallback(t, router)
	w = e2ePostJSON(router, "/api/auth/token",
		fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authCode2, e2eWrongVerifier))
	e2eAssertRejection(t, w, "exchange wrong verifier", http.StatusBadRequest, "INVALID_GRANT")
	// verifier 不一致は auth_code を消費しない（MarkUsed 前に拒否）ため、正しい verifier で
	// 交換が成立する（family2 は後続の revoke シナリオで使用する）
	w = e2ePostJSON(router, "/api/auth/token",
		fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authCode2, e2eCodeVerifier))
	_, refresh3 := e2eDecodeTokenPair(t, w, "exchange after wrong verifier")

	// --- step4: refresh rotation（Req 1.3） ---
	w = e2ePostJSON(router, "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh1))
	_, refresh2 := e2eDecodeTokenPair(t, w, "refresh")
	if refresh2 == refresh1 {
		t.Error("refresh must rotate to a new token")
	}
	// DB: 旧 token は rotation 確定、同一 family に 2 世代
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE rotated_at IS NOT NULL`); got != 1 {
		t.Errorf("rotated refresh_tokens = %d, want 1", got)
	}
	// family は exchange 2 回で 2 つ存在する（family1: refresh1/2 系列、family2: refresh3）
	if got := e2eQueryInt(t, db, `SELECT COUNT(DISTINCT family_id) FROM refresh_tokens`); got != 2 {
		t.Errorf("distinct families = %d, want 2", got)
	}

	// --- step4-reject: rotation 済み token の再提示 → family 全滅（Req 3.3, 3.4） ---
	w = e2ePostJSON(router, "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh1))
	e2eAssertRejection(t, w, "refresh replay rotated", http.StatusUnauthorized, "INVALID_REFRESH_TOKEN")
	// DB: family1 が失効している
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM refresh_token_families WHERE revoked_at IS NOT NULL`); got != 1 {
		t.Errorf("revoked families = %d, want 1 (再利用検知で family 失効)", got)
	}
	// family 全滅: 現役だったはずの refresh2 も拒否される（Req 3.4）
	w = e2ePostJSON(router, "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh2))
	e2eAssertRejection(t, w, "refresh after family revoke", http.StatusUnauthorized, "INVALID_REFRESH_TOKEN")
	// 不明 refresh token も同一の拒否（Req 3.3）
	w = e2ePostJSON(router, "/api/auth/refresh", `{"refresh_token":"never-issued-refresh-token"}`)
	e2eAssertRejection(t, w, "refresh unknown token", http.StatusUnauthorized, "INVALID_REFRESH_TOKEN")

	// --- step5: revoke（Req 1.4）。family2（refresh3）を失効する ---
	w = e2ePostJSON(router, "/api/auth/revoke", fmt.Sprintf(`{"refresh_token":%q}`, refresh3))
	if w.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want %d", w.Result().StatusCode, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("revoke body = %q, want empty", w.Body.String())
	}
	// revoke 後の refresh は拒否（family2 も失効済み）
	w = e2ePostJSON(router, "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh3))
	e2eAssertRejection(t, w, "refresh after revoke", http.StatusUnauthorized, "INVALID_REFRESH_TOKEN")
	// 再 revoke / 不明 token revoke も同一の 204（冪等・列挙オラクルなし）
	w = e2ePostJSON(router, "/api/auth/revoke", fmt.Sprintf(`{"refresh_token":%q}`, refresh3))
	if w.Result().StatusCode != http.StatusNoContent {
		t.Errorf("re-revoke status = %d, want %d (冪等)", w.Result().StatusCode, http.StatusNoContent)
	}
	w = e2ePostJSON(router, "/api/auth/revoke", `{"refresh_token":"never-issued-refresh-token"}`)
	if w.Result().StatusCode != http.StatusNoContent {
		t.Errorf("revoke unknown status = %d, want %d", w.Result().StatusCode, http.StatusNoContent)
	}
	// DB: 両 family とも失効済み
	if got := e2eQueryInt(t, db, `SELECT COUNT(*) FROM refresh_token_families WHERE revoked_at IS NOT NULL`); got != 2 {
		t.Errorf("revoked families = %d, want 2", got)
	}

	// --- step6: real JWT の Bearer 提示で既存 API に到達（Req 1.5） ---
	// fake OAuthProvider の identity から解決された user.ID を DB から取得する
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE email = 'e2e@example.com'`).Scan(&userID); err != nil {
		t.Fatalf("e2e user lookup failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer "+access1)
	wRec := httptest.NewRecorder()
	router.ServeHTTP(wRec, req)
	if wRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("Bearer API status = %d, want %d (body=%s)", wRec.Result().StatusCode, http.StatusOK, wRec.Body.String())
	}
	// 応答の userID 連動 fixture が JWT sub（= DB の user.ID）から解決されている
	if !strings.Contains(wRec.Body.String(), "sub-of-"+userID) {
		t.Errorf("Bearer API body %q does not contain %q (JWT sub = users.id の解決)",
			wRec.Body.String(), "sub-of-"+userID)
	}
}
