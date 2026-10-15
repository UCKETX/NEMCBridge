package friends

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDynamicTokenReferenceVectors(t *testing.T) {
	// Independent fixtures for the social gateway and game-core variants.
	for _, tc := range []struct{ token, path, body, want, core string }{
		{"fixture-token", "/user-addme-friends/", "{}", "NClpuGGtdPsp666u1", "CKjURyMQiwSo1yoQ1"},
		{"other-fixture", "/user-reply-friend/", `{"fid":123456789,"accept":true,"accept_ui":"联系人-好友申请"}`, "u3hsKb7rLzY1uijv1", "h4evVwMoE0iIByjT1"},
	} {
		key := md5.Sum([]byte(tc.token))
		if got := dynamicToken(key[:], tc.path, []byte(tc.body)); got != tc.want {
			t.Fatalf("signature = %s, want %s", got, tc.want)
		}
		if got := signToken(key[:], tc.path, []byte(tc.body), true); got != tc.core {
			t.Fatalf("core signature = %s, want %s", got, tc.core)
		}
	}
}

func TestPollAcceptsOnlyPendingDeduplicatesAndRetries(t *testing.T) {
	key := md5.Sum([]byte("fixture-token"))
	list := `[{"uid":"9007199254740993","nickname":"Alice\n","apply_time":10},{"uid":42,"nickname":"self"},{"uid":0},{"uid":"bad"},{"uid":9007199254740993,"apply_time":10}]`
	var attempts []uint64
	var events []string
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.Header.Get("user-id") != "42" || r.Header.Get("user-token") != dynamicToken(key[:], r.URL.Path, body) {
			t.Error("missing or incorrect authenticated request")
		}
		switch r.URL.Path {
		case "/user-addme-friends/":
			if string(body) != "{}" {
				t.Error("incorrect list request body")
			}
			io.WriteString(w, `{"code":0,"entities":`+list+`}`)
		case "/user-reply-friend/":
			var reply struct {
				FID      uint64 `json:"fid"`
				Accept   bool   `json:"accept"`
				AcceptUI string `json:"accept_ui"`
			}
			if json.Unmarshal(body, &reply) != nil || !reply.Accept || reply.AcceptUI != "联系人-好友申请" {
				t.Error("incorrect acceptance payload")
			}
			attempts = append(attempts, reply.FID)
			if fail {
				io.WriteString(w, `{"code":123,"message":"private-test-secret"}`)
			} else {
				io.WriteString(w, `{"code":0}`)
			}
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	c := &client{http: server.Client(), gateway: server.URL, key: key[:], uid: "42"}
	w := &worker{client: c, accepted: make(map[uint64]string), event: func(s string) { events = append(events, s) }}
	ctx := context.Background()
	if err := w.poll(ctx); err == nil || strings.Contains(err.Error(), "secret") || len(events) != 0 {
		t.Fatal("business failure accepted or leaked response")
	}
	fail = false
	for range 2 {
		if err := w.poll(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(attempts, []uint64{9007199254740993, 9007199254740993}) || len(events) != 1 || strings.Contains(events[0], "\n") {
		t.Fatalf("incorrect retries/deduplication/events: %v %v", attempts, events)
	}
	// Same applicant can send another request after a removal, even if the API
	// omits apply_time. A changed apply_time also identifies a new request.
	list = `[]`
	if err := w.poll(ctx); err != nil || len(w.accepted) != 0 {
		t.Fatal("cache not pruned")
	}
	list = `[{"uid":9007199254740993,"apply_time":11}]`
	if err := w.poll(ctx); err != nil {
		t.Fatal(err)
	}
	list = `[{"uid":9007199254740993,"apply_time":12}]`
	if err := w.poll(ctx); err != nil || len(attempts) != 4 {
		t.Fatal("new application suppressed")
	}
}

func TestPollBoundedAndCancellation(t *testing.T) {
	var applicants []map[string]any
	for i := range 25 {
		applicants = append(applicants, map[string]any{"uid": i + 100})
	}
	accepted := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user-addme-friends/" {
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "entities": applicants})
		} else {
			accepted++
			io.WriteString(w, `{"code":0}`)
		}
	}))
	defer server.Close()
	w := &worker{client: &client{http: server.Client(), gateway: server.URL, uid: "42", key: make([]byte, 16)}, accepted: make(map[uint64]string), event: func(string) {}}
	if err := w.poll(context.Background()); err != nil || accepted != 20 {
		t.Fatalf("batch limit failed: %d %v", accepted, err)
	}
	if err := w.poll(context.Background()); err != nil || accepted != 25 {
		t.Fatal("remaining requests not handled")
	}
	w.accepted = make(map[uint64]string)
	w.pace = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	w.event = func(string) { cancel() }
	if err := w.poll(ctx); err != context.Canceled || accepted != 26 {
		t.Fatalf("cancellation failed: %d %v", accepted, err)
	}
	if wait(ctx, time.Hour) {
		t.Fatal("cancelled wait completed normally")
	}
}

func TestHTTPFailureAndCancellation(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"code":1,"message":"private-test-secret"}`, `{"code":0,"entities":[`, strings.Repeat("x", (1<<20)+1)} {
		t.Run("bad response", func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer s.Close()
			c := &client{http: s.Client(), gateway: s.URL, uid: "42", key: make([]byte, 16)}
			if _, err := c.pending(context.Background()); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("bad response accepted or leaked")
			}
		})
	}
	started := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer s.Close()
	c := &client{http: s.Client(), gateway: s.URL, uid: "42", key: make([]byte, 16)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.pending(ctx); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled HTTP succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP cancellation blocked")
	}
}

func TestDiscoverAndNoCredentialRedirect(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("user-token") != "" || r.Header.Get("user-id") != "" {
			t.Error("metadata request contained credentials")
		}
		io.WriteString(w, `{"ApiGatewayUrl":"https://gateway.example/"}`)
	}))
	defer s.Close()
	c := &client{http: s.Client(), metadata: s.URL}
	if err := c.discover(context.Background()); err != nil || c.gateway != "https://gateway.example" {
		t.Fatalf("discovery failed: %v", err)
	}
	// Use the production HTTP client to check that redirects don't forward its
	// signed account headers to an unrelated endpoint.
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c.http, c.gateway, c.uid, c.key = newHTTPClient(), redirect.URL, "42", make([]byte, 16)
	if _, err := c.pending(context.Background()); err == nil || reached {
		t.Fatal("followed credential redirect")
	}
}

func TestAccountID(t *testing.T) {
	for _, raw := range []string{`null`, `true`, `0`, `-1`, `1.5`, `1e3`, `"bad"`, `18446744073709551616`} {
		if _, ok := accountID(json.RawMessage(raw)); ok {
			t.Fatalf("invalid account accepted: %s", raw)
		}
	}
}

func TestSigningNegotiatedOnReadOnlyRequest(t *testing.T) {
	for _, mode := range []string{"gateway", "core", "reject"} {
		t.Run(mode, func(t *testing.T) {
			reads, writes := 0, 0
			wantList := "NClpuGGtdPsp666u1"
			wantReply := "NP0hKf897v6umq671"
			if mode == "core" {
				wantList = "CKjURyMQiwSo1yoQ1"
				wantReply = "CECgVwCArHoQhC941"
			}
			key := md5.Sum([]byte("fixture-token"))
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user-addme-friends/" {
					reads++
					if mode == "reject" || r.Header.Get("user-token") != wantList {
						io.WriteString(w, `{"code":22}`)
					} else {
						io.WriteString(w, `{"code":0,"entities":[{"uid":123456789}]}`)
					}
				} else {
					writes++
					body, _ := io.ReadAll(r.Body)
					if string(body) != `{"fid":123456789,"accept":true,"accept_ui":"联系人-好友申请"}` || r.Header.Get("user-token") != wantReply {
						t.Error("acceptance did not preserve negotiated format")
					}
					// A write rejection must not cause a second submission.
					io.WriteString(w, `{"code":22}`)
				}
			}))
			defer s.Close()
			c := &client{http: s.Client(), gateway: s.URL, uid: "42", key: key[:]}
			applications, err := c.pending(context.Background())
			if mode == "reject" {
				if err == nil || reads != 2 {
					t.Fatal("expected bounded negotiation failure")
				}
				c.pending(context.Background())
				if reads != 3 || writes != 0 {
					t.Fatal("repeated negotiation or sent write after failed read")
				}
				return
			}
			wantReads := 1
			if mode == "core" {
				wantReads = 2
			}
			if err != nil || len(applications) != 1 || reads != wantReads {
				t.Fatalf("negotiation failed: reads=%d err=%v", reads, err)
			}
			if err := c.accept(context.Background(), 123456789); err == nil || writes != 1 {
				t.Fatal("write was retried or rejection ignored")
			}
			if _, err := c.pending(context.Background()); err != nil || reads != wantReads+1 {
				t.Fatal("negotiated format not reused")
			}
		})
	}
}
