package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

const (
	defaultSessionTTL      = 30 * time.Minute
	defaultCleanupInterval = 5 * time.Minute
	defaultMaxBodyBytes    = 1 << 16
	maxSessionIDLength     = 256
)

type broker struct {
	mu           sync.RWMutex
	sessions     map[string]*sessionState
	sessionTTL   time.Duration
	maxBodySize  int64
	sharedSecret string
	now          func() time.Time
}

type sessionState struct {
	Offer     *lab.Signal `json:"offer,omitempty"`
	Answer    *lab.Signal `json:"answer,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address for the signaling broker")
	sessionTTL := flag.Duration("session-ttl", defaultSessionTTL, "session time-to-live; 0 disables expiry")
	cleanupInterval := flag.Duration("cleanup-interval", defaultCleanupInterval, "interval for expired-session cleanup")
	maxBody := flag.Int64("max-body", defaultMaxBodyBytes, "maximum request body size in bytes")
	sharedSecret := flag.String("shared-secret", os.Getenv("LAB_BROKER_TOKEN"), "optional bearer token for signaling endpoints; prefer LAB_BROKER_TOKEN")
	flag.Parse()

	if *sessionTTL < 0 {
		log.Fatal("session TTL must be 0 or greater")
	}
	if *sessionTTL > 0 && *cleanupInterval <= 0 {
		log.Fatal("cleanup interval must be greater than 0 when session expiry is enabled")
	}

	b := &broker{
		sessions:     make(map[string]*sessionState),
		sessionTTL:   *sessionTTL,
		maxBodySize:  *maxBody,
		sharedSecret: strings.TrimSpace(*sharedSecret),
		now:          time.Now,
	}

	if *sessionTTL > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go b.cleanupLoop(ctx, *cleanupInterval)
	}

	authMode := "disabled"
	if b.sharedSecret != "" {
		authMode = "enabled"
	}
	log.Printf("signaling broker listening on %s; session-ttl=%s; max-body=%d; bearer auth=%s", *listen, b.sessionTTL, b.maxBodySize, authMode)

	handler := NewHandlerWithBroker(b)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		log.Fatal(err)
	}
}

func NewBrokerHandler(sessionTTL time.Duration, maxBodySize int64) http.Handler {
	return NewHandlerWithBroker(&broker{
		sessions:    make(map[string]*sessionState),
		sessionTTL:  sessionTTL,
		maxBodySize: maxBodySize,
	})
}

func NewHandlerWithBroker(b *broker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/sessions/", b.handleSessionSignal)
	return withCORS(mux)
}

func NewHandler() http.Handler {
	return NewBrokerHandler(0, defaultMaxBodyBytes)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (b *broker) handleSessionSignal(w http.ResponseWriter, r *http.Request) {
	if !b.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID, kind, ok := parseSignalPath(r.URL.Path)
	if !ok || len(sessionID) > maxSessionIDLength {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		b.getSignal(w, sessionID, kind)
	case http.MethodPost:
		b.postSignal(w, r, sessionID, kind)
	case http.MethodDelete:
		b.deleteSession(w, sessionID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *broker) authorized(r *http.Request) bool {
	if b.sharedSecret == "" {
		return true
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return subtle.ConstantTimeCompare([]byte(provided), []byte(b.sharedSecret)) == 1
}

func (b *broker) getSignal(w http.ResponseWriter, sessionID, kind string) {
	b.mu.Lock()
	state := b.sessions[sessionID]
	if b.expired(state, b.now().UTC()) {
		delete(b.sessions, sessionID)
		state = nil
	}

	var payload *lab.Signal
	if state != nil {
		switch kind {
		case "offer":
			if state.Offer != nil {
				copy := *state.Offer
				payload = &copy
			}
		case "answer":
			if state.Answer != nil {
				copy := *state.Answer
				payload = &copy
			}
		}
	}
	b.mu.Unlock()

	if payload == nil {
		http.NotFound(w, nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *broker) postSignal(w http.ResponseWriter, r *http.Request, sessionID, kind string) {
	defer r.Body.Close()

	if b.maxBodySize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, b.maxBodySize)
	}

	var payload lab.Signal
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid or oversized JSON signal", http.StatusBadRequest)
		return
	}
	desc, err := payload.Description()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if kind != desc.Type.String() {
		http.Error(w, "SDP type does not match signaling endpoint", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	state := b.sessions[sessionID]
	if state == nil {
		state = &sessionState{}
		b.sessions[sessionID] = state
	}

	now := b.now().UTC()
	state.UpdatedAt = now
	switch kind {
	case "offer":
		state.Offer = &payload
		state.Answer = nil
	case "answer":
		state.Answer = &payload
	}
	b.mu.Unlock()

	log.Printf("stored %s for session %q", kind, sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func (b *broker) deleteSession(w http.ResponseWriter, sessionID string) {
	b.mu.Lock()
	delete(b.sessions, sessionID)
	b.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (b *broker) expired(state *sessionState, now time.Time) bool {
	return state != nil && b.sessionTTL > 0 && !state.UpdatedAt.Add(b.sessionTTL).After(now)
}

func (b *broker) pruneExpired(now time.Time) int {
	if b.sessionTTL <= 0 {
		return 0
	}
	removed := 0
	b.mu.Lock()
	for sessionID, state := range b.sessions {
		if b.expired(state, now) {
			delete(b.sessions, sessionID)
			removed++
		}
	}
	b.mu.Unlock()
	return removed
}

func (b *broker) cleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if removed := b.pruneExpired(now.UTC()); removed > 0 {
				log.Printf("expired %d stale signaling session(s)", removed)
			}
		}
	}
}

func (b *broker) cleanExpiredSessions(ttl time.Duration) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().UTC().Add(-ttl)
	var cleaned int
	for id, state := range b.sessions {
		if state.UpdatedAt.Before(cutoff) {
			delete(b.sessions, id)
			cleaned++
		}
	}
	return cleaned
}

func parseSignalPath(path string) (sessionID, kind string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "sessions" {
		return "", "", false
	}
	if len(parts) == 2 {
		decodedSession, err := url.PathUnescape(parts[1])
		if err != nil || strings.TrimSpace(decodedSession) == "" {
			return "", "", false
		}
		return decodedSession, "", true
	}
	kind = parts[len(parts)-1]
	if kind != "offer" && kind != "answer" {
		return "", "", false
	}
	rawSession := strings.Join(parts[1:len(parts)-1], "/")
	decodedSession, err := url.PathUnescape(rawSession)
	if err != nil || strings.TrimSpace(decodedSession) == "" {
		return "", "", false
	}
	return decodedSession, kind, true
}
