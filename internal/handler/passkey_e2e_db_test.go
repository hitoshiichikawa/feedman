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
	"github.com/hitoshi/feedman/internal/model"
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
	return newPasskeyE2ERouterWithFactory(t, db, auth.NewSessionFactory(time.Hour))
}

func newPasskeyE2ERouterWithFactory(
	t *testing.T,
	db *sql.DB,
	sessionFactory auth.SessionFactoryFunc,
) (http.Handler, string) {
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
	// Issue #230: 新規登録 finish の 1 tx 化に必要な RegistrationTxBeginner を real DB で組む。
	// *repository.SQLTx は passkey.RegistrationTx を構造的に充足するため、
	// e2eRealPasskeyRegTxBeginner で戻り値型を interface に一致させる（本ファイル末尾に定義）。
	regTxBeginner := &e2ePasskeyRegTxBeginner{beginner: repository.NewSQLTxBeginner(db)}
	registrationSvc := passkey.NewRegistrationService(
		webAuthnAdapter, challengeStore, userRepo, passkeyCredRepo,
		regTxBeginner, sessionRepo, sessionFactory, nil,
	)
	authenticationSvc := passkey.NewAuthenticationService(
		webAuthnAdapter, challengeStore, passkeyCredRepo, userRepo, authCodeRepo, nil,
	)

	deps := &RouterDeps{
		SessionFinder:           sessionRepo,
		CORSAllowedOrigin:       "http://localhost:3000",
		WebPasskeyAllowedOrigin: e2ePasskeyRP.Origin,
		RateLimiter:             middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		NativeAuthHandler:       NewNativeAuthHandler(tokenService),
		JWTVerifier:             verifier,
		PasskeyHandler: NewPasskeyHandler(
			registrationSvc,
			authenticationSvc,
			WithWebRegistrationSession(e2ePasskeyRP.Origin, "", true, int(time.Hour.Seconds())),
		),
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

type fixedE2ESessionFactory struct {
	id  string
	now time.Time
	ttl time.Duration
}

func (f *fixedE2ESessionFactory) NewSession(userID string) (*model.Session, error) {
	return &model.Session{
		ID:        f.id,
		UserID:    userID,
		CreatedAt: f.now,
		ExpiresAt: f.now.Add(f.ttl),
	}, nil
}

// e2ePasskeyPostJSON は JSON POST を送り Recorder を返す共通ヘルパ。
func e2ePasskeyPostJSON(router http.Handler, path, body string) *httptest.ResponseRecorder {
	return e2ePasskeyPostJSONWithOrigin(router, path, body, "")
}

func e2ePasskeyPostJSONWithOrigin(
	router http.Handler,
	path string,
	body string,
	origin string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func executeE2EPasskeyRegistration(
	t *testing.T,
	router http.Handler,
	username string,
	origin string,
) *httptest.ResponseRecorder {
	t.Helper()
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	beginBody := fmt.Sprintf(
		`{"username":%q,"code_challenge":%q}`,
		username,
		e2ePasskeyCodeChallenge(),
	)
	begin := e2ePasskeyPostJSON(router, "/api/passkey/registration/begin", beginBody)
	if begin.Code != http.StatusOK {
		t.Fatalf("registration/begin status = %d, want 200 (body=%s)", begin.Code, begin.Body.String())
	}
	var response passkeyBeginResponse
	if err := json.NewDecoder(begin.Result().Body).Decode(&response); err != nil {
		t.Fatalf("registration/begin decode: %v", err)
	}
	options, err := virtualwebauthn.ParseAttestationOptions(string(response.Options))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(
		e2ePasskeyRP,
		authenticator,
		credential,
		*options,
	)
	finishBody := fmt.Sprintf(
		`{"challenge_id":%q,"credential":%s}`,
		response.ChallengeID,
		attestation,
	)
	return e2ePasskeyPostJSONWithOrigin(
		router,
		"/api/passkey/registration/finish",
		finishBody,
		origin,
	)
}

func TestE2E_PasskeyWebRegistrationCreatesAtomicSession(t *testing.T) {
	db := setupPasskeyE2EDB(t)
	defer db.Close()
	router, origin := newPasskeyE2ERouter(t, db)

	// Act
	w := executeE2EPasskeyRegistration(t, router, "web-alice", origin)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("registration/finish status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var response registrationFinishNewResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&response); err != nil {
		t.Fatalf("registration/finish decode: %v", err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v, want one session cookie", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != sessionCookieName || cookie.Value == "" || cookie.MaxAge != int(time.Hour.Seconds()) {
		t.Errorf("session cookie = %+v, want non-empty %s with MaxAge=%d",
			cookie, sessionCookieName, int(time.Hour.Seconds()))
	}

	var userCount, credentialCount, sessionCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE id = $1`,
		response.UserID,
	).Scan(&userCount); err != nil {
		t.Fatalf("users count: %v", err)
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM passkey_credentials WHERE user_id = $1`,
		response.UserID,
	).Scan(&credentialCount); err != nil {
		t.Fatalf("credentials count: %v", err)
	}
	var createdAt, expiresAt time.Time
	if err := db.QueryRow(
		`SELECT COUNT(*), MIN(created_at), MIN(expires_at)
		 FROM sessions WHERE user_id = $1`,
		response.UserID,
	).Scan(&sessionCount, &createdAt, &expiresAt); err != nil {
		t.Fatalf("sessions count/timestamps: %v", err)
	}
	if userCount != 1 || credentialCount != 1 || sessionCount != 1 {
		t.Errorf("user/credential/session counts = %d/%d/%d, want 1/1/1",
			userCount, credentialCount, sessionCount)
	}
	if expiresAt.Sub(createdAt) != time.Hour {
		t.Errorf("session lifetime = %v, want %v", expiresAt.Sub(createdAt), time.Hour)
	}
}

func TestE2E_PasskeyWebRegistrationSessionFailureRollsBackAllRows(t *testing.T) {
	db := setupPasskeyE2EDB(t)
	defer db.Close()
	ctx := context.Background()

	// Arrange: 別 user の既存 session と同じ ID を factory に固定し、registration tx の
	// session INSERT だけを UNIQUE 違反にする。
	existingUser := &model.User{
		ID:                 "11111111-1111-4111-8111-111111111111",
		Email:              "",
		Name:               "",
		Username:           "existing-user",
		UsernameNormalized: "existing-user",
	}
	userRepo := repository.NewPostgresUserRepo(db)
	if err := userRepo.CreateUserOnly(ctx, existingUser); err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	const duplicateSessionID = "duplicate-session-id"
	sessionRepo := repository.NewPostgresSessionRepo(db)
	fixedNow := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if err := sessionRepo.Create(ctx, &model.Session{
		ID:        duplicateSessionID,
		UserID:    existingUser.ID,
		CreatedAt: fixedNow,
		ExpiresAt: fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create existing session: %v", err)
	}
	factory := &fixedE2ESessionFactory{
		id:  duplicateSessionID,
		now: fixedNow,
		ttl: time.Hour,
	}
	router, origin := newPasskeyE2ERouterWithFactory(t, db, factory)

	// Act
	w := executeE2EPasskeyRegistration(t, router, "rollback-alice", origin)

	// Assert
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("registration/finish status = %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Errorf("cookies = %v, want none on session INSERT failure", w.Result().Cookies())
	}
	var rolledBackUsers, rolledBackCredentials, existingSessions int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE username_normalized = 'rollback-alice'`,
	).Scan(&rolledBackUsers); err != nil {
		t.Fatalf("rolled-back users count: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM passkey_credentials`).Scan(&rolledBackCredentials); err != nil {
		t.Fatalf("rolled-back credentials count: %v", err)
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE id = $1`,
		duplicateSessionID,
	).Scan(&existingSessions); err != nil {
		t.Fatalf("existing sessions count: %v", err)
	}
	if rolledBackUsers != 0 || rolledBackCredentials != 0 || existingSessions != 1 {
		t.Errorf("rolledBack users/credentials and existing session = %d/%d/%d, want 0/0/1",
			rolledBackUsers, rolledBackCredentials, existingSessions)
	}
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

	// DB sanity: Origin 不在の iOS/native mode は users / passkey_credentials の 2 行だけを
	// 作成し、session / Cookie を発行しない（Issue #231 Delta 1 / #216 回帰）。
	var userCount, credCount, sessionCount int
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
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sessions WHERE user_id = $1`,
		regFinishResp.UserID,
	).Scan(&sessionCount); err != nil {
		t.Fatalf("sessions COUNT: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("sessions after Origin-less registration: got %d, want 0", sessionCount)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Errorf("Origin-less registration cookies = %v, want none", w.Result().Cookies())
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

// TestE2E_PasskeyFullFlow_DBBacked_BackupEligible は Issue #234 の中核 E2E regression。
//
// virtualwebauthn v1.0.5 の `AuthenticatorOptions.BackupEligible` / `BackupState` を true に
// 設定した BE=1/BS=1 の synthetic authenticator を用いて、実ブラウザで登録された同期パスキー
// と同等の attestation / assertion を生成し、passkey 全動線（登録 begin → finish → 認証 begin →
// finish → auth_code 発行 → token 交換 → Bearer で保護 API 到達）が green で通ることを検証する。
//
// 検証の柱:
//  1. 登録直後に `passkey_credentials.backup_eligible` / `backup_state` に true が保存されている
//     （Req 1.1 / 1.3。task 4 の registration service 実装が BE/BS を素通しで永続化する経路の
//     DB sanity）
//  2. 認証 ceremony が成功し auth_code が発行される（Req 3.1）— library の
//     `Backup Eligible flag inconsistency detected`（go-webauthn login.go:371）を、task 5 の
//     lookup 反映（stored BE/BS を `webauthn.Credential.Flags` に載せる修正）が回避できて
//     いることの regression 保険
//  3. 認証 finish 後の DB 状態: `backup_eligible` は true のまま **不変**（Req 4.3。task 5 の
//     `UpdateAuthenticationState` が SET 句に含めない SQL 設計で担保）、`backup_state` は
//     assertion 由来の最新観測値（BE=1/BS=1 authenticator では true のまま維持）に更新される
//     （Req 4.2）
//  4. 既存 `TestE2E_PasskeyFullFlow_DBBacked`（BE=0 baseline）を無変更で維持することで、
//     BE=1 経路と BE=0 経路が **同一 authentication_service.go の lookup 経路** を共有した
//     状態で同時に green を保つ（Req 5.1 / 5.2 / 5.4）
//
// Req 3.1 / 3.2 について: 実ブラウザ経路（Web）と iOS/native 経路は同じ
// `PasskeyAuthenticationService.FinishLogin` → adapter → `lookup` closure を共有するため、
// 本 E2E で Web 経路の BE=1 動線が通れば、native 経路も同じ lookup 反映で成立する。本 E2E は
// 経路の内部差を作らず、共有経路の 1 サンプルとして Web 経路を通す。
func TestE2E_PasskeyFullFlow_DBBacked_BackupEligible(t *testing.T) {
	db := setupPasskeyE2EDB(t)
	defer db.Close()
	router, _ := newPasskeyE2ERouter(t, db)

	ctx := context.Background()

	// --- synthetic authenticator を用意 ---
	// BE=1/BS=1 の同期パスキー相当（Apple/Google Password Manager 等）を再現するため、
	// `virtualwebauthn.NewAuthenticator()` 直後に BackupEligible / BackupState を true に設定する。
	// これにより attestation / assertion の両方の authenticatorData.flags に BE=1/BS=1 が
	// 反映される（virtualwebauthn v1.0.5 `AuthenticatorOptions` の contract）。
	authenticator := virtualwebauthn.NewAuthenticator()
	authenticator.Options.BackupEligible = true
	authenticator.Options.BackupState = true
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// --- step 1: passkey 新規登録 begin ---
	regBeginBody := fmt.Sprintf(`{"username":"e2e-be1-alice","code_challenge":%q}`, e2ePasskeyCodeChallenge())
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

	// DB sanity: 登録直後の users / passkey_credentials 件数（既存 BE=0 テストの回帰基盤）
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

	// DB sanity（本 test の中核 / Req 1.1, 1.3）:
	// task 4 の registration service が `parsed.BackupEligible` / `parsed.BackupState` を
	// `PasskeyCredential.BackupEligible` / `.BackupState` にセットして CreateExec 経由で
	// 保存した BE=1/BS=1 が実 DB へ到達していることを SELECT で直接確認する。ここで false が
	// 返る状態は「BE/BS が保存されていない」regression であり、そのまま authentication ceremony
	// でも lookup 反映が false になり library の BE 一致判定を通過できない。
	var beAfterReg, bsAfterReg bool
	if err := db.QueryRowContext(
		ctx,
		`SELECT backup_eligible, backup_state FROM passkey_credentials WHERE user_id = $1`,
		regFinishResp.UserID,
	).Scan(&beAfterReg, &bsAfterReg); err != nil {
		t.Fatalf("SELECT backup_eligible/backup_state after registration: %v", err)
	}
	if !beAfterReg {
		t.Errorf("backup_eligible after registration = false, want true (Req 1.1: BE=1 authenticator 由来の値が永続化されていない)")
	}
	if !bsAfterReg {
		t.Errorf("backup_state after registration = false, want true (Req 1.3: BS=1 authenticator 由来の値が永続化されていない)")
	}

	// --- step 4: passkey 認証 begin ---
	// authenticator を認証用にセットアップ（user handle を設定し credential を登録）
	// BE=1/BS=1 の `Options` は `NewAuthenticator()` 後に設定済みなので、以降の
	// CreateAssertionResponse でも BE=1/BS=1 の authenticatorData.flags が伝搬される。
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

	// --- step 5: synthetic authenticator で assertion response を生成（BE=1/BS=1） ---
	assOpts, err := virtualwebauthn.ParseAssertionOptions(string(authBeginResp.Options))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(e2ePasskeyRP, authenticator, cred, *assOpts)

	// --- step 6: passkey 認証 finish で auth_code を得る（Req 3.1） ---
	// task 5 の authentication service が `lookup` closure で stored BE/BS を
	// `webauthn.Credential.Flags` に反映しているため、library の login.go:371 の BE 一致判定
	// （`credential.Flags.BackupEligible != assertion の BE`）を通過して assertion が受理される。
	// この修正が入っていない環境ではここで 400 が返り、test が fail する（本 test の直接の regression 観測点）。
	authFinishBody := fmt.Sprintf(`{"challenge_id":%q,"credential":%s}`, authBeginResp.ChallengeID, assertionResp)
	w = e2ePasskeyPostJSON(router, "/api/passkey/authentication/finish", authFinishBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("authentication/finish status = %d, want 200 (body=%s / Req 3.1: BE=1 assertion が受理されていない)",
			w.Result().StatusCode, w.Body.String())
	}
	var authFinishResp authenticationFinishResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&authFinishResp); err != nil {
		t.Fatalf("authentication/finish decode: %v", err)
	}
	if authFinishResp.AuthCode == "" {
		t.Fatal("authentication/finish returned empty auth_code")
	}

	// DB sanity（本 test の中核 / Req 4.2, 4.3）:
	// 認証 finish 直後の `backup_eligible` は不変（true のまま）、`backup_state` は assertion 由来の
	// 最新観測値。BE=1/BS=1 authenticator では BS=1 のまま観測されるため true が期待値。
	// もし task 5 の SQL SET 句に `backup_eligible` を含めた regression が発生していれば、この assert が
	// 検出する（`backup_eligible` を「認証時の観測値」で上書きしない Req 4.3 を SQL レベル + テストで
	// 二重に担保）。
	var beAfterAuth, bsAfterAuth bool
	if err := db.QueryRowContext(
		ctx,
		`SELECT backup_eligible, backup_state FROM passkey_credentials WHERE user_id = $1`,
		regFinishResp.UserID,
	).Scan(&beAfterAuth, &bsAfterAuth); err != nil {
		t.Fatalf("SELECT backup_eligible/backup_state after authentication: %v", err)
	}
	if !beAfterAuth {
		t.Errorf("backup_eligible after authentication = false, want true (Req 4.3: 認証成功時に BE を上書き更新してはいけない)")
	}
	if !bsAfterAuth {
		t.Errorf("backup_state after authentication = false, want true (Req 4.2: BE=1/BS=1 authenticator の assertion 由来 BS が反映されていない)")
	}

	// DB sanity: auth_codes 行が単回消費前の状態で作成されている
	var unusedAuthCodes int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1 AND used = false`, regFinishResp.UserID,
	).Scan(&unusedAuthCodes); err != nil {
		t.Fatalf("auth_codes unused COUNT: %v", err)
	}
	if unusedAuthCodes != 1 {
		t.Errorf("unused auth_codes after passkey auth: got %d, want 1", unusedAuthCodes)
	}

	// --- step 7: 既存 POST /api/auth/token で auth_code を交換 ---
	// passkey 由来 auth_code が既存 native auth 契約と同一形式で受理されることを確認する
	// （BE=1 経路でも token 交換の合流点が壊れないことの回帰保険）。
	tokenBody := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, authFinishResp.AuthCode, e2ePasskeyCodeVerifier)
	w = e2ePasskeyPostJSON(router, "/api/auth/token", tokenBody)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("token 交換 status = %d, want 200 (body=%s)", w.Result().StatusCode, w.Body.String())
	}
	var tokenResp map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&tokenResp); err != nil {
		t.Fatalf("token 交換 decode: %v", err)
	}
	if tokenResp["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", tokenResp["token_type"])
	}
	if v, _ := tokenResp["expires_in"].(float64); v != 900 {
		t.Errorf("expires_in = %v, want 900", tokenResp["expires_in"])
	}
	accessToken, _ := tokenResp["access_token"].(string)
	refreshToken, _ := tokenResp["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("access_token / refresh_token must be non-empty (access=%q, refresh 非空)", accessToken)
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
		t.Errorf("Bearer API body %q does not contain %q (BE=1 経路の JWT sub 解決失敗)",
			wRec.Body.String(), "sub-of-"+regFinishResp.UserID)
	}
}

// e2ePasskeyRegTxBeginner は E2E DB テスト向けに *repository.SQLTxBeginner を
// passkey.RegistrationTxBeginner に適合させる薄いアダプタ（Issue #230 / Req 1.1〜1.6）。
// 本番 wiring は internal/app/withdraw_wiring.go の passkeyRegistrationTxBeginnerAdapter
// を使うが、handler パッケージから app パッケージを import できない（循環回避）ため、
// 本ファイル内に同構造の最小アダプタを置く。
type e2ePasskeyRegTxBeginner struct {
	beginner *repository.SQLTxBeginner
}

func (a *e2ePasskeyRegTxBeginner) BeginTx(ctx context.Context) (passkey.RegistrationTx, error) {
	tx, err := a.beginner.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
