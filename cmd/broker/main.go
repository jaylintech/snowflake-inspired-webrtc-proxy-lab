package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"snowflakeprotocolpoc/internal/lab"
)

type broker struct {
	mu       sync.RWMutex
	sessions map[string]*sessionState
}

type sessionState struct {
	Offer     *lab.Signal `json:"offer,omitempty"`
	Answer    *lab.Signal `json:"answer,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address for the signaling broker")
	flag.Parse()

	b := &broker{sessions: make(map[string]*sessionState)}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/sessions/", b.handleSessionSignal)

	log.Printf("signaling broker listening on %s", *listen)
	if err := http.ListenAndServe(*listen, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (b *broker) handleSessionSignal(w http.ResponseWriter, r *http.Request) {
	sessionID, kind, ok := parseSignalPath(r.URL.Path)
	if !ok {
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

func (b *broker) getSignal(w http.ResponseWriter, sessionID, kind string) {
	b.mu.RLock()
	state := b.sessions[sessionID]
	b.mu.RUnlock()

	if state == nil {
		http.NotFound(w, nil)
		return
	}

	var payload *lab.Signal
	switch kind {
	case "offer":
		payload = state.Offer
	case "answer":
		payload = state.Answer
	}

	if payload == nil {
		http.NotFound(w, nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *broker) postSignal(w http.ResponseWriter, r *http.Request, sessionID, kind string) {
	defer r.Body.Close()

	var payload lab.Signal
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON signal", http.StatusBadRequest)
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

	now := time.Now().UTC()
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

func parseSignalPath(path string) (sessionID, kind string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "sessions" {
		return "", "", false
	}
	if parts[2] != "offer" && parts[2] != "answer" {
		return "", "", false
	}

	decodedSession, err := url.PathUnescape(parts[1])
	if err != nil || strings.TrimSpace(decodedSession) == "" {
		return "", "", false
	}

	return decodedSession, parts[2], true
}
