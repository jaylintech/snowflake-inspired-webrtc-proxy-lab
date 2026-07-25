package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return u
}

func TestParseTargetValid(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"http://example.com", "http://example.com"},
		{"https://example.com/path", "https://example.com/path"},
		{"https://example.com/lab/", "https://example.com/lab/"},
	}
	for _, c := range cases {
		got, err := ParseTarget(c.raw)
		if err != nil {
			t.Errorf("ParseTarget(%q): %v", c.raw, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("ParseTarget(%q) = %q, want %q", c.raw, got.String(), c.want)
		}
	}
}

func TestParseTargetRejectsInvalid(t *testing.T) {
	cases := []string{
		"ftp://example.com",
		"",
		"  ",
		"no-scheme",
		"https://example.com/../admin",
	}
	for _, raw := range cases {
		_, err := ParseTarget(raw)
		if err == nil {
			t.Errorf("ParseTarget(%q) should have failed", raw)
		}
	}
}

func TestBoundedTargetURL(t *testing.T) {
	target, _ := ParseTarget("https://example.com/lab")

	got, err := BoundedTargetURL(target, "/robots.txt")
	if err != nil {
		t.Fatalf("BoundedTargetURL: %v", err)
	}
	want := "https://example.com/lab/robots.txt"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBoundedTargetURLWithoutBasePath(t *testing.T) {
	target, _ := ParseTarget("https://example.com")

	got, err := BoundedTargetURL(target, "/page")
	if err != nil {
		t.Fatalf("BoundedTargetURL: %v", err)
	}
	want := "https://example.com/page"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBoundedTargetURLRejectsAbsolute(t *testing.T) {
	target, _ := ParseTarget("https://example.com")
	_, err := BoundedTargetURL(target, "https://evil.com/")
	if err == nil {
		t.Fatal("expected absolute URL rejection")
	}
}

func TestBoundedTargetURLRejectsParentDir(t *testing.T) {
	target, _ := ParseTarget("https://example.com/lab")
	_, err := BoundedTargetURL(target, "/../admin")
	if err == nil {
		t.Fatal("expected parent dir rejection")
	}
}

func TestRelayResponseBodyText(t *testing.T) {
	body, format := RelayResponseBody([]byte("hello"), "text/plain")
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if body != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

func TestRelayResponseBodyBinary(t *testing.T) {
	body, format := RelayResponseBody([]byte{0x00, 0x01, 0xff}, "image/png")
	if format != "base64" {
		t.Fatalf("format = %q, want base64", format)
	}
	if body != "AAH/" {
		t.Fatalf("body = %q, want AAH/", body)
	}
}

func TestRelayResponseBodyNoContentType(t *testing.T) {
	body, format := RelayResponseBody([]byte("plain text"), "")
	if format != "text" {
		t.Fatalf("format = %q, want text for valid utf-8 without content-type", format)
	}
	if body != "plain text" {
		t.Fatalf("body = %q", body)
	}
}

func TestRelayResponseBodyJSON(t *testing.T) {
	_, format := RelayResponseBody([]byte(`{"key":"value"}`), "application/json")
	if format != "text" {
		t.Fatalf("format = %q, want text for json", format)
	}
}

func TestSelectedResponseHeaders(t *testing.T) {
	h := make(http.Header)
	h.Set("Content-Type", "text/html")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Custom", "ignored")

	got := SelectedResponseHeaders(h)
	if got["content-type"] != "text/html" {
		t.Fatalf("content-type = %q", got["content-type"])
	}
	if got["cache-control"] != "no-cache" {
		t.Fatalf("cache-control = %q", got["cache-control"])
	}
	if _, exists := got["x-custom"]; exists {
		t.Fatal("x-custom should not be in selected headers")
	}
}

func TestAllowedRequestHeader(t *testing.T) {
	headers := map[string]string{"Accept": "text/plain"}
	got := AllowedRequestHeader(headers, "Accept", "text/html")
	if got != "text/plain" {
		t.Fatalf("got %q, want text/plain", got)
	}

	got = AllowedRequestHeader(nil, "Accept", "fallback")
	if got != "fallback" {
		t.Fatalf("got %q, want fallback for nil headers", got)
	}
}

func TestApplyRelayRequestHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	relayReq := lab.RelayRequest{
		ID:   "test-001",
		Path: "/test",
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}
	ApplyRelayRequestHeaders(req, relayReq)

	if req.Header.Get("X-Proxy-Request-ID") != "test-001" {
		t.Fatalf("X-Proxy-Request-ID = %q", req.Header.Get("X-Proxy-Request-ID"))
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Fatalf("Accept = %q", req.Header.Get("Accept"))
	}
}

func TestResponsePreviewText(t *testing.T) {
	got := ResponsePreview([]byte("short"), false, "text")
	if got != "short" {
		t.Fatalf("got %q", got)
	}
}

func TestResponsePreviewBinary(t *testing.T) {
	got := ResponsePreview([]byte{0x00, 0x01}, false, "base64")
	want := "[binary response body: 2 bytes]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResponsePreviewTruncated(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 600)
	got := ResponsePreview(data, true, "text")
	if len(got) > 600 {
		t.Fatal("preview should be shorter than full body")
	}
}

func TestHasParentPathSegment(t *testing.T) {
	if !HasParentPathSegment("/../admin") {
		t.Fatal("expected ../ to be detected")
	}
	if !HasParentPathSegment("/foo/../bar") {
		t.Fatal("expected ../ in middle to be detected")
	}
	if HasParentPathSegment("/foo/bar") {
		t.Fatal("expected clean path to pass")
	}
}

func TestJoinTargetPath(t *testing.T) {
	cases := []struct {
		base, request, want string
	}{
		{"", "/foo", "/foo"},
		{"/lab", "/foo", "/lab/foo"},
		{"/lab", "foo", "/lab/foo"},
	}
	for _, c := range cases {
		got := JoinTargetPath(c.base, c.request)
		if got != c.want {
			t.Errorf("JoinTargetPath(%q, %q) = %q, want %q", c.base, c.request, got, c.want)
		}
	}
}

func TestRedirectAllowed(t *testing.T) {
	target, _ := ParseTarget("https://example.com/lab")

	if !RedirectAllowed(target, mustURL(t, "https://example.com/lab/next")) {
		t.Fatal("same-origin same-base redirect should be allowed")
	}
	if RedirectAllowed(target, mustURL(t, "https://example.com/other")) {
		t.Fatal("same-origin different-base redirect should be blocked")
	}
	if RedirectAllowed(target, mustURL(t, "https://evil.com/lab/next")) {
		t.Fatal("cross-origin redirect should be blocked")
	}
	if RedirectAllowed(target, nil) {
		t.Fatal("nil redirect should be blocked")
	}
}

func TestBuildRelayRequestValid(t *testing.T) {
	target, _ := ParseTarget("https://example.com")
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-001",
		Method: "GET",
		Path:   "/robots.txt",
	}

	req, err := BuildRelayRequest(context.Background(), relayReq, target)
	if err != nil {
		t.Fatalf("BuildRelayRequest: %v", err)
	}
	if req.Method != "GET" {
		t.Fatalf("method = %q", req.Method)
	}
	if req.URL.String() != "https://example.com/robots.txt" {
		t.Fatalf("url = %q", req.URL.String())
	}
}

func TestBuildRelayRequestRejectsPOSTOutsideProxy(t *testing.T) {
	target, _ := ParseTarget("https://example.com")
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-002",
		Method: "PUT",
		Path:   "/update",
	}
	_, err := BuildRelayRequest(context.Background(), relayReq, target)
	if err == nil {
		t.Fatal("expected PUT to be rejected")
	}
}

func TestBuildRelayRequestPOST(t *testing.T) {
	target, _ := ParseTarget("https://example.com")
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-003",
		Method: "POST",
		Path:   "/submit",
		Body:   "data",
	}

	req, err := BuildRelayRequest(context.Background(), relayReq, target)
	if err != nil {
		t.Fatalf("BuildRelayRequest POST: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("method = %q", req.Method)
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != "data" {
		t.Fatalf("body = %q", string(body))
	}
}

func TestBuildRelayResponse(t *testing.T) {
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-004",
		Method: "GET",
		Path:   "/",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	response := BuildRelayResponse(relayReq, resp, 4096)
	if response.Status != 200 {
		t.Fatalf("status = %d", response.Status)
	}
	if response.Bytes != 5 {
		t.Fatalf("bytes = %d", response.Bytes)
	}
	if response.Body != "hello" {
		t.Fatalf("body = %q", response.Body)
	}
}

func TestBuildRelayResponseTruncates(t *testing.T) {
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-005",
		Method: "GET",
		Path:   "/",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	response := BuildRelayResponse(relayReq, resp, 10)
	if !response.Truncated {
		t.Fatal("expected truncated response")
	}
	if response.Bytes != 10 {
		t.Fatalf("bytes = %d, want 10", response.Bytes)
	}
}

func TestErrorResponse(t *testing.T) {
	resp := ErrorResponse("test-id", "something went wrong")
	if resp.Type != lab.RelayResponseType {
		t.Fatalf("type = %q", resp.Type)
	}
	if resp.ID != "test-id" {
		t.Fatalf("id = %q", resp.ID)
	}
	if resp.Error != "something went wrong" {
		t.Fatalf("error = %q", resp.Error)
	}
}

type mockDC struct {
	lastPayload []byte
}

func (m *mockDC) Send(payload []byte) error {
	m.lastPayload = payload
	return nil
}

func TestSendRelayResponseSmall(t *testing.T) {
	dc := &mockDC{}
	response := lab.RelayResponse{
		Type:   lab.RelayResponseType,
		ID:     "test-001",
		Status: 200,
		Body:   "small",
		Time:   "2026-01-01T00:00:00Z",
	}
	SendRelayResponse(dc, response)

	if dc.lastPayload == nil {
		t.Fatal("expected data to be sent")
	}
}

func TestSendRelayResponseChunked(t *testing.T) {
	dc := &mockDC{}
	body := bytes.Repeat([]byte("A"), ResponseChunkBytes*2+1)
	response := lab.RelayResponse{
		Type:   lab.RelayResponseType,
		ID:     "chunk-test",
		Status: 200,
		Body:   string(body),
	}
	SendRelayResponse(dc, response)

	if dc.lastPayload == nil {
		t.Fatal("expected chunked data to be sent")
	}

	var result lab.RelayResponse
	if err := json.Unmarshal(dc.lastPayload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Type != lab.RelayResponseChunkType {
		t.Fatalf("type = %q, want chunk type", result.Type)
	}
	if result.ChunkIndex != 2 {
		t.Fatalf("chunk_index = %d, want 2", result.ChunkIndex)
	}
	if result.ChunkTotal != 3 {
		t.Fatalf("chunk_total = %d, want 3", result.ChunkTotal)
	}
}

func TestShouldTreatBodyAsText(t *testing.T) {
	cases := []struct {
		body []byte
		ct   string
		want bool
	}{
		{[]byte("hello"), "text/plain", true},
		{[]byte("hello"), "text/html", true},
		{[]byte("{}"), "application/json", true},
		{[]byte("<svg>"), "image/svg+xml", true},
		{[]byte{0x00, 0x01}, "image/png", false},
		{[]byte("hello"), "", true},
		{[]byte{0xff, 0xfe}, "", false},
	}
	for _, c := range cases {
		got := ShouldTreatBodyAsText(c.body, c.ct)
		if got != c.want {
			t.Errorf("ShouldTreatBodyAsText(%q, %q) = %v, want %v", string(c.body), c.ct, got, c.want)
		}
	}
}

func TestBuildRelayRequestRejectsPathOutsideBase(t *testing.T) {
	target, _ := ParseTarget("https://example.com/lab")
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "test-006",
		Method: "GET",
		Path:   "/../etc/passwd",
	}
	_, err := BuildRelayRequest(context.Background(), relayReq, target)
	if err == nil {
		t.Fatal("expected parent-dir path outside target base to be rejected")
	}
}

func TestSendRelayResponseChunksLogsError(t *testing.T) {
	errDC := &errDataChannel{}
	body := bytes.Repeat([]byte("B"), ResponseChunkBytes+1)
	response := lab.RelayResponse{
		Type:   lab.RelayResponseType,
		ID:     "err-test",
		Status: 200,
		Body:   string(body),
	}
	SendRelayResponse(errDC, response)
}

type errDataChannel struct{}

func (e *errDataChannel) Send(payload []byte) error {
	return fmt.Errorf("send error")
}

func TestBuildRelayResponseFollowsFinalURL(t *testing.T) {
	relayReq := lab.RelayRequest{
		Type:   lab.RelayRequestType,
		ID:     "redir-test",
		Method: "GET",
		Path:   "/redirect",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("final"))
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/redirect")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	response := BuildRelayResponse(relayReq, resp, 4096)
	if response.Target != ts.URL+"/final" {
		t.Fatalf("target = %q, want %s/final", response.Target, ts.URL)
	}
	if response.Status != 200 {
		t.Fatalf("status = %d, want 200", response.Status)
	}
}
