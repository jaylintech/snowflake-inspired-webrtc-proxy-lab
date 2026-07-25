package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func newTestHandler() http.Handler {
	return NewHandlerWithBroker(&broker{
		sessions:    make(map[string]*sessionState),
		maxBodySize: 1 << 16,
	})
}

func TestHealthzReturns204(t *testing.T) {
	mux := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("healthz status = %d, want 204", w.Code)
	}
}

func TestPostAndGetOffer(t *testing.T) {
	mux := newTestHandler()

	signal := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}

	body, _ := json.Marshal(signal)
	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("POST offer status = %d, want 204", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/sessions/test-sess/offer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET offer status = %d, want 200", w.Code)
	}

	var got lab.Signal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Type != "offer" {
		t.Fatalf("signal type = %q, want offer", got.Type)
	}
	if got.SDP != signal.SDP {
		t.Fatalf("signal SDP = %q, want %q", got.SDP, signal.SDP)
	}
}

func TestPostAndGetAnswer(t *testing.T) {
	mux := newTestHandler()

	offer := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ := json.Marshal(offer)
	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	answer := lab.Signal{Type: "answer", SDP: "v=0\no=- 789 012 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ = json.Marshal(answer)
	req = httptest.NewRequest(http.MethodPost, "/sessions/test-sess/answer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("POST answer status = %d, want 204", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/sessions/test-sess/answer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET answer status = %d, want 200", w.Code)
	}

	var got lab.Signal
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Type != "answer" {
		t.Fatalf("signal type = %q, want answer", got.Type)
	}
}

func TestPostOfferClearsPreviousAnswer(t *testing.T) {
	mux := newTestHandler()

	offer := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	answer := lab.Signal{Type: "answer", SDP: "v=0\no=- 789 012 IN IP4 127.0.0.1\ns=-\nt=0 0"}

	body, _ := json.Marshal(offer)
	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body, _ = json.Marshal(answer)
	req = httptest.NewRequest(http.MethodPost, "/sessions/test-sess/answer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodGet, "/sessions/test-sess/answer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected answer to exist after POST")
	}

	newOffer := lab.Signal{Type: "offer", SDP: "v=0\no=- 999 888 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ = json.Marshal(newOffer)
	req = httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodGet, "/sessions/test-sess/answer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected answer to be cleared after new offer, got %d", w.Code)
	}
}

func TestGetOfferBeforePostReturns404(t *testing.T) {
	mux := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/sessions/no-such-sess/offer", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestPostSignalRejectsSDPMismatch(t *testing.T) {
	mux := newTestHandler()

	answer := lab.Signal{Type: "answer", SDP: "v=0\no=- 789 012 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ := json.Marshal(answer)
	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for SDP type mismatch", w.Code)
	}
}

func TestPostSignalRejectsInvalidJSON(t *testing.T) {
	mux := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid JSON", w.Code)
	}
}

func TestDeleteSession(t *testing.T) {
	mux := newTestHandler()

	offer := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ := json.Marshal(offer)
	req := httptest.NewRequest(http.MethodPost, "/sessions/del-test/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodDelete, "/sessions/del-test", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/sessions/del-test/offer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	mux := newTestHandler()

	req := httptest.NewRequest(http.MethodOptions, "/sessions/test/offer", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS allow-origin header")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	mux := newTestHandler()

	req := httptest.NewRequest(http.MethodPut, "/sessions/test/offer", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status = %d, want 405", w.Code)
	}
}

func TestParseSignalPathInvalid(t *testing.T) {
	cases := []string{
		"/sessions",
		"/sessions/",
		"/sessions/foo/offer/extra",
		"/sessions//offer",
		"/not-sessions/foo/offer",
		"/sessions/foo/invalid-kind",
	}
	for _, path := range cases {
		sessionID, kind, ok := parseSignalPath(path)
		if ok {
			t.Errorf("parseSignalPath(%q) unexpectedly succeeded: id=%q kind=%q", path, sessionID, kind)
		}
	}
}

func TestParseSignalPathValid(t *testing.T) {
	sessionID, kind, ok := parseSignalPath("/sessions/my-session/offer")
	if !ok || sessionID != "my-session" || kind != "offer" {
		t.Fatalf("got id=%q kind=%q ok=%v, want id=my-session kind=offer ok=true", sessionID, kind, ok)
	}

	sessionID, kind, ok = parseSignalPath("/sessions/a/b/answer")
	if !ok || sessionID != "a/b" || kind != "answer" {
		t.Fatalf("got id=%q kind=%q, want id=a/b kind=answer", sessionID, kind)
	}
}

func TestCORSMiddlewareAddsHeaders(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS allow-origin header")
	}
}

func TestPostSignalRejectsOversizedBody(t *testing.T) {
	b := &broker{sessions: make(map[string]*sessionState), maxBodySize: 32}
	mux := NewHandlerWithBroker(b)

	largeBody := strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodPost, "/sessions/test-sess/offer", bytes.NewReader([]byte(largeBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d, want 400", w.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	b := &broker{
		sessions:    make(map[string]*sessionState),
		sessionTTL:  50 * time.Millisecond,
		maxBodySize: 1 << 16,
	}
	mux := NewHandlerWithBroker(b)

	signal := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ := json.Marshal(signal)
	req := httptest.NewRequest(http.MethodPost, "/sessions/expire-test/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	time.Sleep(60 * time.Millisecond)

	cleaned := b.cleanExpiredSessions(50 * time.Millisecond)
	if cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", cleaned)
	}

	req = httptest.NewRequest(http.MethodGet, "/sessions/expire-test/offer", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after expiry, got %d", w.Code)
	}
}

func TestSessionExpiryDoesNotRemoveFresh(t *testing.T) {
	b := &broker{sessions: make(map[string]*sessionState), maxBodySize: 1 << 16}
	mux := NewHandlerWithBroker(b)

	signal := lab.Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	body, _ := json.Marshal(signal)
	req := httptest.NewRequest(http.MethodPost, "/sessions/fresh-test/offer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	cleaned := b.cleanExpiredSessions(time.Hour)
	if cleaned != 0 {
		t.Fatalf("cleaned = %d, want 0 for fresh session", cleaned)
	}
}

func TestRejectsLongSessionID(t *testing.T) {
	mux := newTestHandler()

	longID := strings.Repeat("a", maxSessionIDLength+1)
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+longID+"/offer", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("long session ID status = %d, want 404", w.Code)
	}
}
