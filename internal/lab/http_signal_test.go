package lab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutSignalUsesBrokerTokenFromEnvironment(t *testing.T) {
	t.Setenv("LAB_BROKER_TOKEN", "environment-token")
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := PutSignal(context.Background(), server.URL, "test", "offer", Signal{Type: "offer", SDP: "v=0"})
	if err != nil {
		t.Fatalf("put signal: %v", err)
	}
	if authorization != "Bearer environment-token" {
		t.Fatalf("authorization = %q", authorization)
	}
}

func TestPutSignalExplicitTokenOverridesEnvironment(t *testing.T) {
	t.Setenv("LAB_BROKER_TOKEN", "environment-token")
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := PutSignal(context.Background(), server.URL, "test", "offer", Signal{Type: "offer", SDP: "v=0"}, "explicit-token")
	if err != nil {
		t.Fatalf("put signal: %v", err)
	}
	if authorization != "Bearer explicit-token" {
		t.Fatalf("authorization = %q", authorization)
	}
}
