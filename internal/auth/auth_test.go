package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"

	"nemcbridge/internal/config"
	"nemcbridge/internal/secret"
)

func TestAuthContractAndSensitiveErrors(t *testing.T) {
	token := "private-test-token"
	good := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/phoenix/tan_lobby_create" || r.Method != "POST" {
			t.Error("incorrect auth endpoint")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["login_token"] != token {
			t.Error("token missing")
		}
		if !good {
			json.NewEncoder(w).Encode(map[string]any{"success": false, "error_info": "invalid login_token " + token})
			return
		}
		json.NewEncoder(w).Encode(CreateInfo{ProtocolID: 42, TanLobbyCreateResponse: bunker.TanLobbyCreateResponse{Success: true, UserUniqueID: 42, UserPlayerName: "test", RaknetServerAddress: "127.0.0.1:1234", SignalingServerAddress: "127.0.0.1:1235", RaknetRand: make([]byte, 16), RaknetAESRand: make([]byte, 16), SignalingSeed: make([]byte, 16), SignalingTicket: make([]byte, 16), EncryptKeyBytes: make([]byte, 32), DecryptKeyBytes: make([]byte, 32)}})
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.AuthServer = server.URL
	info, err := FetchCreate(context.Background(), cfg, token)
	if err != nil || info.ProtocolID != 42 {
		t.Fatalf("valid contract rejected: %v", err)
	}
	good = false
	_, err = FetchCreate(context.Background(), cfg, token)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatal("credential leak or success:false accepted")
	}
}
func TestAuthRejectsCredentialRedirect(t *testing.T) {
	cfg := config.Default()
	request, _ := http.NewRequest("POST", "https://attacker.example/auth", nil)
	if newAuthHTTP(cfg.AuthServer).CheckRedirect(request, []*http.Request{{}}) == nil {
		t.Fatal("credentials may follow arbitrary redirect")
	}
	request, _ = http.NewRequest("POST", "https://fgauth.ipv4.ucket.top:9380/api/phoenix/tan_lobby_create", nil)
	if err := newAuthHTTP(cfg.AuthServer).CheckRedirect(request, []*http.Request{{}}); err != nil {
		t.Fatal("known FGAuth redirect rejected")
	}
}
func TestTokenStorageRoundTrip(t *testing.T) {
	path := t.TempDir() + "/secrets.dat"
	token := "test-private-value"
	if err := secret.Save(path, token); err != nil {
		t.Fatal(err)
	}
	actual, err := secret.Load(path)
	if err != nil || actual != token {
		t.Fatal("stored token cannot be recovered")
	}
}
