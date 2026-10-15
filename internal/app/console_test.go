package app

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"nemcbridge/internal/config"
)

func TestConsoleURLsAndHosts(t *testing.T) {
	ips := []net.IP{net.ParseIP("192.168.1.20"), net.ParseIP("10.1.2.3")}
	var out bytes.Buffer
	printConsoleURLs(&out, 18321, ips)
	for _, want := range []string{"0.0.0.0:18321", "http://127.0.0.1:18321/", "http://192.168.1.20:18321/", "http://10.1.2.3:18321/"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in terminal output: %s", want, out.String())
		}
	}
	hosts := consoleHosts(18321, ips)
	if !hosts["192.168.1.20:18321"] || !hosts["localhost:18321"] || hosts["attacker.example:18321"] || hosts["192.168.1.20:18322"] {
		t.Fatal("console host allowlist mismatch")
	}
}

func TestConsoleLANAccessAndOrigin(t *testing.T) {
	ips := consoleIPv4Addresses()
	if len(ips) == 0 {
		t.Skip("no active LAN IPv4 interface")
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.WebPort = listener.Addr().(*net.TCPAddr).Port
	m := newManager(t.TempDir(), cfg, nil, log.New(io.Discard, "", 0))
	handler := makeHandler(m, testConsoleAuth(t))
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	address := "http://" + net.JoinHostPort(ips[0].String(), strconv.Itoa(cfg.WebPort))
	login, _ := http.NewRequest(http.MethodPost, address+"/api/login", strings.NewReader(`{"username":"admin","password":"test-admin-password"}`))
	login.Header.Set("Origin", address)
	login.Header.Set("Content-Type", "application/json")
	loginResponse, err := client.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK || len(loginResponse.Cookies()) != 1 {
		t.Fatal("LAN login failed")
	}
	cookie := loginResponse.Cookies()[0]
	for _, tc := range []struct {
		method, path, origin string
		status               int
	}{
		{http.MethodGet, "/", "", http.StatusOK},
		{http.MethodGet, "/api/status", "", http.StatusOK},
		{http.MethodPost, "/api/stop", address, http.StatusOK},
		{http.MethodPost, "/api/stop", "http://attacker.example", http.StatusForbidden},
	} {
		req, err := http.NewRequest(tc.method, address+tc.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%s %s origin=%s: got %d, want %d", tc.method, tc.path, tc.origin, resp.StatusCode, tc.status)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example/api/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatal("unexpected Host accepted")
	}
}
