package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func TestBrokerRequiresConfiguredBearerToken(t *testing.T) {
	b := &broker{sessions: make(map[string]*sessionState), sharedSecret: "lab-secret", now: time.Now}

	request := httptest.NewRequest(http.MethodGet, "/sessions/test/offer", nil)
	response := httptest.NewRecorder()
	b.handleSessionSignal(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodGet, "/sessions/test/offer", nil)
	request.Header.Set("Authorization", "Bearer lab-secret")
	response = httptest.NewRecorder()
	b.handleSessionSignal(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("authorized status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestBrokerPrunesExpiredSessions(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	b := &broker{
		sessions: map[string]*sessionState{
			"expired": {UpdatedAt: now.Add(-11 * time.Minute)},
			"active":  {UpdatedAt: now.Add(-9 * time.Minute)},
		},
		sessionTTL: 10 * time.Minute,
		now:        func() time.Time { return now },
	}

	if removed := b.pruneExpired(now); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, ok := b.sessions["expired"]; ok {
		t.Fatal("expired session remains")
	}
	if _, ok := b.sessions["active"]; !ok {
		t.Fatal("active session was removed")
	}
}

func TestBrokerLazilyExpiresSignal(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	offer := lab.Signal{Type: "offer", SDP: "v=0"}
	b := &broker{
		sessions: map[string]*sessionState{
			"stale": {Offer: &offer, UpdatedAt: now.Add(-time.Hour)},
		},
		sessionTTL: 10 * time.Minute,
		now:        func() time.Time { return now },
	}

	response := httptest.NewRecorder()
	b.getSignal(response, "stale", "offer")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if _, ok := b.sessions["stale"]; ok {
		t.Fatal("lazily expired session remains")
	}
}

func TestCORSAllowsAuthorizationHeader(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodOptions, "/sessions/test/offer", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allowed headers = %q", got)
	}
}
