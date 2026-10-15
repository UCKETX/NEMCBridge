package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nemcbridge/internal/config"
)

func testConsoleAuth(t *testing.T) *consoleAuth {
	t.Helper()
	a, _, err := loadConsoleAuth(t.TempDir(), "test-admin-password", false)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func testConsoleRequest(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:18321"+path, strings.NewReader(body))
	r.Header.Set("Origin", "http://127.0.0.1:18321")
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestAdminCredentialPersistenceAndReset(t *testing.T) {
	dir := t.TempDir()
	a, password, err := loadConsoleAuth(dir, "", false)
	if err != nil || len(password) != 24 {
		t.Fatalf("initialization: generated=%t err=%v", password != "", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "admin.json"))
	if err != nil || bytes.Contains(raw, []byte(password)) {
		t.Fatal("credential not stored as a hash")
	}
	if bcrypt.CompareHashAndPassword(a.hash, []byte(password)) != nil {
		t.Fatal("generated password does not match")
	}
	reloaded, displayed, err := loadConsoleAuth(dir, "", false)
	if err != nil || displayed != "" || !bytes.Equal(reloaded.hash, a.hash) {
		t.Fatalf("reload failed: %v", err)
	}
	reset, newPassword, err := loadConsoleAuth(dir, "", true)
	if err != nil || newPassword == password || bcrypt.CompareHashAndPassword(reset.hash, []byte(password)) == nil {
		t.Fatalf("reset failed: %v", err)
	}
	override, echoed, err := loadConsoleAuth(dir, "environment-password", false)
	if err != nil || echoed != "" || bcrypt.CompareHashAndPassword(override.hash, []byte("environment-password")) != nil {
		t.Fatalf("environment override failed: %v", err)
	}
	if _, _, err := loadConsoleAuth(dir, "short", false); err == nil {
		t.Fatal("short password accepted")
	}
}

func TestConsoleAuthenticationLifecycle(t *testing.T) {
	m := newManager(t.TempDir(), config.Default(), "", log.New(io.Discard, "", 0))
	a := testConsoleAuth(t)
	handler := makeHandler(m, a)
	for _, path := range []string{"/api/status", "/api/start", "/api/stop", "/api/config", "/api/exit", "/api/logout"} {
		method := http.MethodPost
		if path == "/api/status" {
			method = http.MethodGet
		}
		if w := testConsoleRequest(handler, method, path, "{}", nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", path, w.Code)
		}
	}
	if w := testConsoleRequest(handler, http.MethodGet, "/", "", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatal("anonymous page did not redirect to login")
	}
	if w := testConsoleRequest(handler, http.MethodGet, "/login", "", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "login-form") {
		t.Fatal("login page unavailable")
	}
	wrong := testConsoleRequest(handler, http.MethodPost, "/api/login", `{"username":"admin","password":"wrong-password"}`, nil)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatal("wrong password accepted")
	}
	login := testConsoleRequest(handler, http.MethodPost, "/api/login", `{"username":"admin","password":"test-admin-password"}`, nil)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatal("login failed")
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != int(adminSessionTTL.Seconds()) {
		t.Fatal("session cookie attributes invalid")
	}
	if w := testConsoleRequest(handler, http.MethodGet, "/api/status", "", cookie); w.Code != http.StatusOK {
		t.Fatal("authenticated status denied")
	}
	if w := testConsoleRequest(handler, http.MethodPost, "/api/logout", "{}", cookie); w.Code != http.StatusOK {
		t.Fatal("logout failed")
	}
	if w := testConsoleRequest(handler, http.MethodGet, "/api/status", "", cookie); w.Code != http.StatusUnauthorized {
		t.Fatal("logged out session still valid")
	}
	// Replayed random or expired tokens cannot access any protected API.
	a.sessions[sha256.Sum256([]byte(cookie.Value))] = time.Now().Add(-time.Second)
	if w := testConsoleRequest(handler, http.MethodGet, "/api/status", "", cookie); w.Code != http.StatusUnauthorized {
		t.Fatal("expired session accepted")
	}
	forged := &http.Cookie{Name: adminCookieName, Value: strings.Repeat("a", 43)}
	if w := testConsoleRequest(handler, http.MethodGet, "/api/status", "", forged); w.Code != http.StatusUnauthorized {
		t.Fatal("forged session accepted")
	}
}

func TestConsoleLoginOriginAndRateLimit(t *testing.T) {
	m := newManager(t.TempDir(), config.Default(), "", log.New(io.Discard, "", 0))
	a := testConsoleAuth(t)
	handler := makeHandler(m, a)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "test-admin-password"})
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18321/api/login", bytes.NewReader(body))
	r.Header.Set("Origin", "http://attacker.example")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("cross-origin login accepted")
	}
	for attempt := 0; attempt < 6; attempt++ {
		w = testConsoleRequest(handler, http.MethodPost, "/api/login", `{"username":"admin","password":"wrong-password"}`, nil)
		want := http.StatusUnauthorized
		if attempt == 5 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("attempt %d: got %d, want %d", attempt, w.Code, want)
		}
	}
}

func TestStartupBannerPlainAndColor(t *testing.T) {
	for _, color := range []bool{false, true} {
		var out bytes.Buffer
		printStartupBanner(&out, color)
		if !strings.Contains(out.String(), "NEMCBridge") || strings.Contains(out.String(), "\x1b[") != color {
			t.Fatal("banner color mode mismatch")
		}
	}
}
