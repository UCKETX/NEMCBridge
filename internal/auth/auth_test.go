package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAuthIPv4IPv6RedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, base, target string
		allowed            bool
	}{
		{"primary to IPv4", "https://fgauth.ucket.top", "https://fgauth.ipv4.ucket.top:9380/api/phoenix/tan_lobby_create", true},
		{"primary to IPv6", "https://fgauth.ucket.top", "https://fgauth.ipv6.ucket.top:8888/api/phoenix/tan_lobby_create", true},
		{"IPv4 to IPv6", "https://fgauth.ipv4.ucket.top:9380", "https://fgauth.ipv6.ucket.top:8888/auth", true},
		{"IPv6 to primary", "https://fgauth.ipv6.ucket.top:8888", "https://fgauth.ucket.top/auth", true},
		{"IPv6 to IPv4", "https://fgauth.ipv6.ucket.top:8888", "https://fgauth.ipv4.ucket.top:9380/auth", true},
		{"default HTTPS port and case", "https://FGAUTH.UCKET.TOP:443", "https://fgauth.ipv6.ucket.top:8888/auth", true},
		{"custom same origin", "https://auth.example:9443", "https://AUTH.EXAMPLE:9443/auth", true},
		{"loopback same origin", "http://127.0.0.1:8080", "http://127.0.0.1:8080/auth", true},
		{"arbitrary domain", "https://fgauth.ucket.top", "https://attacker.example/auth", false},
		{"suffix spoof", "https://fgauth.ucket.top", "https://fgauth.ipv6.ucket.top.attacker.example:8888/auth", false},
		{"unexpected port", "https://fgauth.ucket.top", "https://fgauth.ipv6.ucket.top:9999/auth", false},
		{"unexpected subdomain", "https://fgauth.ucket.top", "https://other.ucket.top/auth", false},
		{"foreign root", "https://auth.example", "https://fgauth.ipv6.ucket.top:8888/auth", false},
		{"foreign root port", "https://fgauth.ucket.top:9443", "https://fgauth.ipv6.ucket.top:8888/auth", false},
		{"downgrade", "https://fgauth.ucket.top", "http://fgauth.ucket.top/auth", false},
		{"IPv6 downgrade", "https://fgauth.ucket.top", "http://fgauth.ipv6.ucket.top:8888/auth", false},
		{"userinfo", "https://fgauth.ucket.top", "https://user:password@fgauth.ipv6.ucket.top:8888/auth", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = newAuthHTTP(tc.base).CheckRedirect(request, []*http.Request{{}})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v", tc.allowed, err)
			}
		})
	}
	request, _ := http.NewRequest(http.MethodPost, "https://fgauth.ipv6.ucket.top:8888/auth", nil)
	if newAuthHTTP("https://fgauth.ucket.top").CheckRedirect(request, make([]*http.Request, 4)) == nil {
		t.Fatal("redirect limit no longer enforced")
	}
}

type authRoundTripFunc func(*http.Request) (*http.Response, error)

func (f authRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAuthRedirectPreservesPostWithoutLeakingToUntrustedHost(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(map[bool]string{true: "known IPv6", false: "untrusted host"}[allowed], func(t *testing.T) {
			calls := 0
			client := newAuthHTTP("https://fgauth.ucket.top")
			target := "https://fgauth.ipv6.ucket.top:8888/api/phoenix/tan_lobby_create"
			if !allowed {
				target = "https://attacker.example/auth"
			}
			client.Transport = authRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil || r.Method != http.MethodPost || string(body) != `{"login_token":"test-only-token"}` {
					t.Fatal("redirect changed POST body or method")
				}
				_ = r.Body.Close()
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				}
				if !allowed || r.URL.Host != "fgauth.ipv6.ucket.top:8888" {
					t.Fatal("token sent to unexpected host")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: r}, nil
			})
			request, _ := http.NewRequest(http.MethodPost, "https://fgauth.ucket.top/api/phoenix/tan_lobby_create", strings.NewReader(`{"login_token":"test-only-token"}`))
			response, err := client.Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
			if allowed && (err != nil || calls != 2) || !allowed && (err == nil || calls != 1) {
				t.Fatalf("allowed=%v, requests=%d, error=%v", allowed, calls, err)
			}
		})
	}
	root, _ := url.Parse("https://fgauth.ucket.top")
	if trustedAuthRedirect(root, nil) || trustedAuthRedirect(nil, root) {
		t.Fatal("invalid endpoint accepted")
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
