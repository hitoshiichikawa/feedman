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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/passkey"
	"github.com/hitoshi/feedman/internal/repository"
)

// 本ファイルは Issue #216 の E2E DB-backed 契約テスト（design.md §Testing Strategy
// ケース E2E/Contract 1 / task 8）。
//
// 動線: passkey 新規登録（begin → finish）→ passkey 認証（begin → finish）→
// 既存 `POST /api/auth/token` に auth_code + code_verifier を渡して token 交換 →
// Bearer access token で保護 API（/api/subscriptions）到達までを、real service /
// real repo / real JWT で 1 経路で通す。
//
// synthetic authenticator（github.com/descope/virtualwebauthn v1.0.5、Task 3 で採用済み
// の test-only 依存）を使うため、外部ネットワーク非依存（NFR 4.1）。
//
// 環境変数 TEST_DATABASE_URL の PostgreSQL に接続できない環境では t.Skipf で
// スキップされる（NFR 4.1）。

// e2ePasskeyJWTSecret は E2E 用の固定署名鍵（既存 native_auth_e2e_db_test の
// e2eJWTSecret と別値で衝突を回避）。
var e2ePasskeyJWTSecret = []byte("e2e-passkey-test-secret-32bytes!!")

// e2ePasskeyRP は synthetic authenticator の Relying Party 設定。
// origin / RPID は本テスト内の WebAuthn adapter 設定と一致させる必要がある。
var e2ePasskeyRP = virtualwebauthn.RelyingParty{
	Name:   "Feedman E2E",
	ID:     "example.com",
	Origin: "https://example.com",
}

// e2ePasskeyOrigins は adapter に渡す許容 origin 一覧（RP.Origin と一致）。
var e2ePasskeyOrigins = []string{"https://example.com"}

// e2ePasskeyCodeVerifier は RFC 7636 §4.1 形式（unreserved 43〜128 文字）の固定 verifier。
const e2ePasskeyCodeVerifier = "e2e-passkey-code-verifier-01234567890abcdefghijklmn"

// e2ePasskeyCodeChallenge は e2ePasskeyCodeVerifier から導出した S256 challenge。
func e2ePasskeyCodeChallenge() string {
	sum := sha256.Sum256([]byte(e2ePasskeyCodeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// e2ePasskeyTestDatabaseURL はテスト用のデータベース URL を返す
// （native_auth_e2e_db_test.go の e2eTestDatabaseURL と同方針）。
func e2ePasskeyTestDatabaseURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupPasskeyE2EDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skipf でスキップする。
func setupPasskeyE2EDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := e2ePasskeyTestDatabaseURL()
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

// newPasskeyE2ERouter は real passkey / native auth service を wire した router を構築する。
// PasskeyHandler + NativeAuthHandler を同 router に載せて、passkey 認証成功 →
// 既存 token 交換 endpoint への合流までを 1 経路で検証できるようにする。
func newPasskeyE2ERouter(t *testing.T, db *sql.DB) (http.Handler, string) {
	t.Helper()

	// 全 repo を real 実装で組む
	userRepo := repository.NewPostgresUserRepo(db)
	sessionRepo := repository.NewPostgresSessionRepo(db)
	authCodeRepo := repository.NewPostgresAuthCodeRepo(db)
	refreshTokenRepo := repository.NewPostgresRefreshTokenRepo(db)
	passkeyCredRepo := repository.NewPostgresPasskeyCredentialRepo(db)
	passkeyChallengeRepo := repository.NewPostgresPasskeyChallengeRepo(db)

	// Native auth（token 交換 endpoint）を real で組む
	issuer := auth.NewJWTIssuer(e2ePasskeyJWTSecret, "v1-passkey-e2e")
	verifier := auth.NewJWTVerifier(e2ePasskeyJWTSecret)
	tokenService := auth.NewTokenService(authCodeRepo, refreshTokenRepo, issuer)

	// Passkey service を real で組む（synthetic authenticator と一致する RP 設定を使う）
	webAuthnAdapter, err := passkey.NewGoWebAuthnAdapter(e2ePasskeyRP.ID, e2ePasskeyRP.Name, e2ePasskeyOrigins)
	if err != nil {
		t.Fatalf("NewGoWebAuthnAdapter: %v", err)
	}
	challengeStore := passkey.NewChallengeStore(passkeyChallengeRepo, 5*time.Minute)
	registrationSvc := passkey.NewRegistrationService(
		webAuthnAdapter, challengeStore, userRepo, passkeyCredRepo, nil,
	)
	authenticationSvc := passkey.NewAuthenticationService(
		webAuthnAdapter, challengeStore, passkeyCredRepo, userRepo, authCodeRepo, nil,
	)

	deps := &RouterDeps{
		SessionFinder:     sessionRepo,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		NativeAuthHandler: NewNativeAuthHandler(tokenService),
		JWTVerifier:       verifier,
		PasskeyHandler:    NewPasskeyHandler(registrationSvc, authenticationSvc),
		// 保護 API 到達を確認するための最小 stub。userID 連動 fixture で JWT sub の
		// 解決を可視化する（native_auth_e2e_db_test.go と同方針）。
		SubscriptionService: &mockSubscriptionService{
			listSubscriptionsFn: func(ctx context.Context, uid string) ([]subscriptionResponse, error) {
				return []subscriptionResponse{{ID: "sub-of-" + uid, FeedID: "feed-passkey-e2e"}}, nil
			},
		},
	}
	return NewRouter(deps), "https://example.com"
}

// e2ePasskeyPostJSON は JSON POST を送り Recorder を返す共通ヘルパ。
func e2ePasskeyPostJSON(router http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestE2E_PasskeyFullFlow_DBBacked は passkey 全動線（登録 → 認証 → 既存 token 交換 →
// Bearer で保護 API 到達）を real service / real repo / synthetic authenticator で通す
// E2E 契約テスト（Issue #216 / Req 1.1〜1.3, 2.1〜2.4, NFR 2.3, NFR 4.1）。
//
// 検証の柱:
//  1. passkey 登録 begin → finish で users 行と passkey_credentials 行が新規作成される
//  2. passkey 認証 begin → finish で auth_code が既存 native auth 契約と同形式で発行される
//  3. 発行された auth_code が既存 POST /api/auth/token で受理され access/refresh token が返る
//     （NFR 2.3: 既存 native auth 契約は無変更のまま合流）
//  4. Bearer access token で保護 API（/api/subscriptions）に到達し、JWT sub = users.id の
//     解決が動作する
func TestE2E_PasskeyFullFlow_DBBacked(t *testing.T) {
	db := setupPasskeyE2EDB(t)
	defer db.Close()
	router, _ := newPasskeyE2ERouter(t, db)

	ctx := context.Background()

	// --- synthetic authenticator を用意 ---
	authenticator := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// --- step 1: passkey 新規登録 begin ---
	regBeginBody := fmt.Sprintf(`{"username":"e2e-alice","code_challenge":%q}`, e2ePasskeyCodeChallenge())
	w := e2ePasskeyPostJSON(router, "/api/passkey/registration/begin", regBeginBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("registration/begin status = %d, want 200 (body=%s)", w.Result().StatusCode, w.Body.String())
	}
	var regBeginResp passkeyBeginResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&regBeginResp); err != nil {
		t.Fatalf("registration/begin decode: %v", err)
	}
	if regBeginResp.ChallengeID == "" || len(regBeginResp.Options) == 0 {
		t.Fatalf("registration/begin missing challenge_id or options: %+v", regBeginResp)
	}

	// --- step 2: synthetic authenticator で attestation response を生成 ---
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(regBeginResp.Options))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(e2ePasskeyRP, authenticator, cred, *attOpts)

	// --- step 3: passkey 新規登録 finish ---
	regFinishBody := fmt.Sprintf(`{"challenge_id":%q,"credential":%s}`, regBeginResp.ChallengeID, attestationResp)
	w = e2ePasskeyPostJSON(router, "/api/passkey/registration/finish", regFinishBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("registration/finish status = %d, want 200 (body=%s)", w.Result().StatusCode, w.Body.String())
	}
	var regFinishResp registrationFinishNewResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&regFinishResp); err != nil {
		t.Fatalf("registration/finish decode: %v", err)
	}
	if regFinishResp.UserID == "" {
		t.Fatal("registration/finish returned empty user_id")
	}

	// DB sanity: users 行と passkey_credentials 行が作成されている
	var userCount, credCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, regFinishResp.UserID).Scan(&userCount); err != nil {
		t.Fatalf("users COUNT: %v", err)
	}
	if userCount != 1 {
		t.Errorf("users after registration: got %d, want 1 (user_id=%q)", userCount, regFinishResp.UserID)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM passkey_credentials WHERE user_id = $1`, regFinishResp.UserID).Scan(&credCount); err != nil {
		t.Fatalf("passkey_credentials COUNT: %v", err)
	}
	if credCount != 1 {
		t.Errorf("passkey_credentials after registration: got %d, want 1", credCount)
	}

	// --- step 4: passkey 認証 begin ---
	// authenticator を認証用にセットアップ（user handle を設定し credential を登録）
	authenticator.Options.UserHandle = []byte(regFinishResp.UserID)
	cred.Counter++ // library の CloneWarning 判定を避けるため counter を進める
	authenticator.AddCredential(cred)

	authBeginBody := fmt.Sprintf(`{"code_challenge":%q}`, e2ePasskeyCodeChallenge())
	w = e2ePasskeyPostJSON(router, "/api/passkey/authentication/begin", authBeginBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("authentication/begin status = %d, want 200 (body=%s)", w.Result().StatusCode, w.Body.String())
	}
	var authBeginResp passkeyBeginResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&authBeginResp); err != nil {
		t.Fatalf("authentication/begin decode: %v", err)
	}
	if authBeginResp.ChallengeID == "" || len(authBeginResp.Options) == 0 {
		t.Fatalf("authentication/begin missing challenge_id or options: %+v", authBeginResp)
	}

	// --- step 5: synthetic authenticator で assertion response を生成 ---
	assOpts, err := virtualwebauthn.ParseAssertionOptions(string(authBeginResp.Options))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(e2ePasskeyRP, authenticator, cred, *assOpts)

	// --- step 6: passkey 認証 finish で auth_code を得る（Req 2.2 / 2.3） ---
	authFinishBody := fmt.Sprintf(`{"challenge_id":%q,"credential":%s}`, authBeginResp.ChallengeID, assertionResp)
	w = e2ePasskeyPostJSON(router, "/api/passkey/authentication/finish", authFinishBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("authentication/finish status = %d, want 200 (body=%s)", w.Result().StatusCode, w.Body.String())
	}
	var authFinishResp authenticationFinishResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&authFinishResp); err != nil {
		t.Fatalf("authentication/finish decode: %v", err)
	}
	if authFinishResp.AuthCode == "" {
		t.Fatal("authentication/finish returned empty auth_code")
	}

	// DB sanity: auth_codes 行が単回消費前の状態で作成されている（Req 2.3）
	var unusedAuthCodes int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1 AND used = false`, regFinishResp.UserID,
	).Scan(&unusedAuthCodes); err != nil {
		t.Fatalf("auth_codes unused COUNT: %v", err)
	}
	if unusedAuthCodes != 1 {
		t.Errorf("unused auth_codes after passkey auth: got %d, want 1 (Req 2.3)", unusedAuthCodes)
	}

	// --- step 7: 既存 POST /api/auth/token で auth_code を交換（Req 2.4 / NFR 2.3） ---
	// passkey 由来 auth_code が既存 native auth 契約と同一形式で受理されることを検証
	tokenBody := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authFinishResp.AuthCode, e2ePasskeyCodeVerifier)
	w = e2ePasskeyPostJSON(router, "/api/auth/token", tokenBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("token 交換 status = %d, want 200 (body=%s / Req 2.4 の合流失敗)",
			w.Result().StatusCode, w.Body.String())
	}
	var tokenResp map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&tokenResp); err != nil {
		t.Fatalf("token 交換 decode: %v", err)
	}
	if tokenResp["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer (NFR 2.3 契約破り)", tokenResp["token_type"])
	}
	if v, _ := tokenResp["expires_in"].(float64); v != 900 {
		t.Errorf("expires_in = %v, want 900", tokenResp["expires_in"])
	}
	accessToken, _ := tokenResp["access_token"].(string)
	refreshToken, _ := tokenResp["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("access_token / refresh_token must be non-empty (access=%q, refresh 非空)", accessToken)
	}

	// DB sanity: auth_code が単回消費され、refresh family + token が永続化されている
	var usedAuthCodes, familyCount, refreshCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1 AND used = true`, regFinishResp.UserID,
	).Scan(&usedAuthCodes); err != nil {
		t.Fatalf("auth_codes used COUNT: %v", err)
	}
	if usedAuthCodes != 1 {
		t.Errorf("used auth_codes after exchange: got %d, want 1 (単回消費)", usedAuthCodes)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, regFinishResp.UserID,
	).Scan(&familyCount); err != nil {
		t.Fatalf("refresh_token_families COUNT: %v", err)
	}
	if familyCount != 1 {
		t.Errorf("refresh_token_families after exchange: got %d, want 1", familyCount)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, regFinishResp.UserID,
	).Scan(&refreshCount); err != nil {
		t.Fatalf("refresh_tokens COUNT: %v", err)
	}
	if refreshCount != 1 {
		t.Errorf("refresh_tokens after exchange: got %d, want 1", refreshCount)
	}

	// --- step 8: Bearer access token で保護 API に到達 ---
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	wRec := httptest.NewRecorder()
	router.ServeHTTP(wRec, req)
	if wRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("Bearer API status = %d, want 200 (body=%s)", wRec.Result().StatusCode, wRec.Body.String())
	}
	// userID 連動 fixture が JWT sub（= users.id）から解決されている
	if !strings.Contains(wRec.Body.String(), "sub-of-"+regFinishResp.UserID) {
		t.Errorf("Bearer API body %q does not contain %q (passkey 由来 JWT sub 解決失敗)",
			wRec.Body.String(), "sub-of-"+regFinishResp.UserID)
	}
}
