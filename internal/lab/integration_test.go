package lab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
)

func newTestBroker() *httptest.Server {
	mux := http.NewServeMux()
	b := &testBroker{sessions: make(map[string]*testSession)}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/sessions/", b.handleSession)
	mux.HandleFunc("/sessions", b.handleSession)

	return httptest.NewServer(withTestCORS(mux))
}

type testSession struct {
	Offer  *Signal `json:"offer,omitempty"`
	Answer *Signal `json:"answer,omitempty"`
}

type testBroker struct {
	mu       sync.RWMutex
	sessions map[string]*testSession
}

func (b *testBroker) handleSession(w http.ResponseWriter, r *http.Request) {
	sessionID, kind, ok := parseTestPath(r.URL.Path)
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

func (b *testBroker) getSignal(w http.ResponseWriter, sessionID, kind string) {
	b.mu.RLock()
	s := b.sessions[sessionID]
	b.mu.RUnlock()

	if s == nil {
		http.NotFound(w, nil)
		return
	}
	var payload *Signal
	switch kind {
	case "offer":
		payload = s.Offer
	case "answer":
		payload = s.Answer
	}
	if payload == nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *testBroker) postSignal(w http.ResponseWriter, r *http.Request, sessionID, kind string) {
	defer r.Body.Close()
	var payload Signal
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	desc, err := payload.Description()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if kind != desc.Type.String() {
		http.Error(w, "type mismatch", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	s := b.sessions[sessionID]
	if s == nil {
		s = &testSession{}
		b.sessions[sessionID] = s
	}
	switch kind {
	case "offer":
		s.Offer = &payload
		s.Answer = nil
	case "answer":
		s.Answer = &payload
	}
	b.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (b *testBroker) deleteSession(w http.ResponseWriter, sessionID string) {
	b.mu.Lock()
	delete(b.sessions, sessionID)
	b.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func parseTestPath(path string) (sessionID, kind string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "sessions" {
		return "", "", false
	}
	if parts[2] != "offer" && parts[2] != "answer" {
		return "", "", false
	}
	sessionID = parts[1]
	if sessionID == "" {
		return "", "", false
	}
	return sessionID, parts[2], true
}

func withTestCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestSignalRoundTripThroughBroker(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	ctx := context.Background()
	signal := Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}

	if err := PutSignal(ctx, srv.URL, "test-sess", "offer", signal); err != nil {
		t.Fatalf("put offer: %v", err)
	}

	got, ok, err := GetSignal(ctx, srv.URL, "test-sess", "offer")
	if err != nil {
		t.Fatalf("get offer: %v", err)
	}
	if !ok {
		t.Fatal("offer not found")
	}
	if got.Type != "offer" || got.SDP != signal.SDP {
		t.Fatalf("got %+v, want %+v", got, signal)
	}
}

func TestPollSignalReturnsSignal(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	answer := Signal{Type: "answer", SDP: "v=0\no=- 789 012 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	if err := PutSignal(ctx, srv.URL, "poll-test", "answer", answer); err != nil {
		t.Fatalf("put answer: %v", err)
	}

	got, err := PollSignal(ctx, srv.URL, "poll-test", "answer", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("poll answer: %v", err)
	}
	if got.Type != "answer" {
		t.Fatalf("type = %q, want answer", got.Type)
	}
}

func TestSignalNotFound(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	_, ok, err := GetSignal(context.Background(), srv.URL, "no-such-sess", "offer")
	if err != nil {
		t.Fatalf("get nonexistent: %v", err)
	}
	if ok {
		t.Fatal("expected not found")
	}
}

func TestDeleteThenNotFound(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	ctx := context.Background()
	signal := Signal{Type: "offer", SDP: "v=0\no=- 123 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	if err := PutSignal(ctx, srv.URL, "del-test", "offer", signal); err != nil {
		t.Fatalf("put offer: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/sessions/del-test/offer", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}

	_, ok, err := GetSignal(ctx, srv.URL, "del-test", "offer")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatal("expected not found after delete")
	}
}

func TestSignalTypeMismatch(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	answer := Signal{Type: "answer", SDP: "v=0\no=- 789 012 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	err := PutSignal(context.Background(), srv.URL, "test", "offer", answer)
	if err == nil {
		t.Fatal("expected error for SDP type mismatch")
	}
}

func TestWebRTCEndToEndThroughBrokerSignaling(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pcA, err := NewPeerConnection("", 0, 0, "")
	if err != nil {
		t.Fatalf("create peer A: %v", err)
	}
	defer pcA.Close()

	pcB, err := NewPeerConnection("", 0, 0, "")
	if err != nil {
		t.Fatalf("create peer B: %v", err)
	}
	defer pcB.Close()

	var msgWG sync.WaitGroup
	msgWG.Add(1)

	pcB.OnDataChannel(func(d *webrtc.DataChannel) {
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			if string(msg.Data) == "ping" {
				msgWG.Done()
			}
		})
	})

	dcA, err := pcA.CreateDataChannel("test-e2e", nil)
	if err != nil {
		t.Fatalf("create data channel: %v", err)
	}

	offer, err := pcA.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	gatherA := webrtc.GatheringCompletePromise(pcA)
	if err := pcA.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local offer A: %v", err)
	}
	<-gatherA

	localOffer := pcA.LocalDescription()
	if localOffer == nil {
		t.Fatal("missing local offer after gathering")
	}

	if err := PutSignal(ctx, srv.URL, "e2e", "offer", FromDescription(*localOffer)); err != nil {
		t.Fatalf("post offer: %v", err)
	}

	offerSignal, err := PollSignal(ctx, srv.URL, "e2e", "offer", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("poll offer: %v", err)
	}
	offerDesc, err := offerSignal.Description()
	if err != nil {
		t.Fatalf("parse offer: %v", err)
	}
	if err := pcB.SetRemoteDescription(offerDesc); err != nil {
		t.Fatalf("set remote offer B: %v", err)
	}

	answer, err := pcB.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("create answer B: %v", err)
	}
	gatherB := webrtc.GatheringCompletePromise(pcB)
	if err := pcB.SetLocalDescription(answer); err != nil {
		t.Fatalf("set local answer B: %v", err)
	}
	<-gatherB

	localAnswerB := pcB.LocalDescription()
	if localAnswerB == nil {
		t.Fatal("missing local answer after gathering")
	}

	if err := PutSignal(ctx, srv.URL, "e2e", "answer", FromDescription(*localAnswerB)); err != nil {
		t.Fatalf("post answer: %v", err)
	}

	answerSignal, err := PollSignal(ctx, srv.URL, "e2e", "answer", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("poll answer: %v", err)
	}
	answerDesc, err := answerSignal.Description()
	if err != nil {
		t.Fatalf("parse answer: %v", err)
	}
	if err := pcA.SetRemoteDescription(answerDesc); err != nil {
		t.Fatalf("set remote answer A: %v", err)
	}

	dcA.OnOpen(func() {
		if err := dcA.SendText("ping"); err != nil {
			t.Logf("send ping: %v", err)
		}
	})

	done := make(chan struct{})
	go func() {
		msgWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for WebRTC data channel message through broker signaling")
	}
}

func TestNewOfferClearsAnswer(t *testing.T) {
	srv := newTestBroker()
	defer srv.Close()

	ctx := context.Background()
	offer := Signal{Type: "offer", SDP: "v=0\no=- 123 IN IP4 127.0.0.1\ns=-\nt=0 0"}
	answer := Signal{Type: "answer", SDP: "v=0\no=- 456 IN IP4 127.0.0.1\ns=-\nt=0 0"}

	_ = PutSignal(ctx, srv.URL, "clear-test", "offer", offer)
	_ = PutSignal(ctx, srv.URL, "clear-test", "answer", answer)

	_, ok, _ := GetSignal(ctx, srv.URL, "clear-test", "answer")
	if !ok {
		t.Fatal("expected answer to exist before new offer")
	}

	_ = PutSignal(ctx, srv.URL, "clear-test", "offer", offer)

	_, ok, _ = GetSignal(ctx, srv.URL, "clear-test", "answer")
	if ok {
		t.Fatal("expected answer to be cleared after new offer")
	}
}
