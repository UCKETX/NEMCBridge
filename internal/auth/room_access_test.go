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
)

func TestNativeRoomAuthPreservesPOSTAcrossRedirectAndRedactsErrors(t *testing.T) {
	const token = "private-native-test-only"
	fail := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body["login_token"] != token || body["room_id"] != "123456" {
			t.Error("room login request changed across redirect")
		}
		if fail {
			json.NewEncoder(w).Encode(map[string]any{"success": false, "error_info": "invalid " + token})
			return
		}
		json.NewEncoder(w).Encode(bunker.TanLobbyLoginResponse{Success: true, UserUniqueID: 42, RoomOwnerID: 43, RaknetServerAddress: "127.0.0.1:1234", SignalingServerAddress: "127.0.0.1:1235", RaknetRand: make([]byte, 16), RaknetAESRand: make([]byte, 32), EncryptKeyBytes: make([]byte, 32), DecryptKeyBytes: make([]byte, 32), SignalingSeed: make([]byte, 16), SignalingTicket: make([]byte, 32), RoomModDisplayName: []string{"4690087423136283144"}})
	}))
	defer destination.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer start.Close()
	cfg := config.Default()
	cfg.AuthServer = start.URL
	a := NewRoomAccess(context.Background(), cfg, token)
	info, err := a.GetAccess("123456")
	if err != nil || len(info.RaknetAESRand) != 16 || len(info.SignalingTicket) != 16 || len(info.RoomModDisplayName) != 1 {
		t.Fatal("valid redirected room auth failed", err)
	}
	fail = true
	_, err = a.GetAccess("123456")
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatal("failed auth accepted or token exposed")
	}
}
