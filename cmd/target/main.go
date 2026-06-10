package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"
)

type targetResponse struct {
	Message   string `json:"message"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	RequestID string `json:"request_id,omitempty"`
	Time      string `json:"time"`
}

func main() {
	listen := flag.String("listen", ":9090", "HTTP listen address for the controlled target server")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", handleRequest)

	log.Printf("controlled target server listening on %s", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	requestID := r.Header.Get("X-Proxy-Request-ID")
	if requestID == "" {
		requestID = r.Header.Get("X-Relay-Request-ID")
	}
	log.Printf("target hit: method=%s path=%s request_id=%s remote=%s", r.Method, r.URL.RequestURI(), requestID, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targetResponse{
		Message:   "controlled target reached through WebRTC proxy lab",
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		RequestID: requestID,
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
}
