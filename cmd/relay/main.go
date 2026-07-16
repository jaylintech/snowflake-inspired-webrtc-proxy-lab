package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/pion/webrtc/v3"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

const responseChunkBytes = 24 * 1024

func main() {
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "proxy-session", "shared signaling session id")
	stunServers := flag.String("stun", lab.DefaultSTUN, "comma-separated STUN URLs; empty disables external STUN")
	icePortMin := flag.Uint("ice-port-min", 0, "minimum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	icePortMax := flag.Uint("ice-port-max", 0, "maximum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	advertiseIP := flag.String("advertise-ip", "", "public IP to advertise for ICE host candidates when using a router port forward")
	target := flag.String("target", "http://127.0.0.1:9090", "single controlled target base URL for the bounded proxy server")
	maxBody := flag.Int64("max-body", 262144, "maximum response body bytes returned to the client")
	requestTimeout := flag.Duration("request-timeout", 10*time.Second, "target request timeout")
	pollInterval := flag.Duration("poll", time.Second, "broker polling interval")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time to wait for signaling")
	flag.Parse()

	targetURL, err := parseTarget(*target)
	if err != nil {
		log.Fatalf("invalid target: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signalCtx, cancelSignal := context.WithTimeout(ctx, *timeout)
	defer cancelSignal()

	pc, err := lab.NewPeerConnectionWithOptions(lab.PeerConnectionOptionsFromEnvironment(*stunServers, *icePortMin, *icePortMax, *advertiseIP))
	if err != nil {
		log.Fatalf("create peer connection: %v", err)
	}
	defer func() {
		if err := pc.Close(); err != nil {
			log.Printf("close peer connection: %v", err)
		}
	}()

	httpClient, err := newBoundedHTTPClient(targetURL, *requestTimeout)
	if err != nil {
		log.Fatalf("create target HTTP client: %v", err)
	}

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE connection state: %s", state.String())
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			if pair, err := lab.SelectedCandidatePair(pc); err != nil {
				log.Printf("selected ICE candidate pair unavailable: %v", err)
			} else {
				log.Printf("selected ICE candidate pair: %s", pair)
			}
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("peer connection state: %s", state.String())
	})
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("local ICE candidate: %s", candidate.String())
		}
	})

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		log.Printf("proxy data channel %q created by client; target=%s", d.Label(), targetURL.String())

		d.OnOpen(func() {
			log.Printf("proxy data channel %q open", d.Label())
		})

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			go handleRelayMessage(ctx, httpClient, d, targetURL, msg.Data, *maxBody)
		})
	})

	log.Printf("proxy server waiting for SDP offer at %s session %q", *brokerURL, *sessionID)
	offerSignal, err := lab.PollSignal(signalCtx, *brokerURL, *sessionID, "offer", *pollInterval)
	if err != nil {
		log.Fatalf("poll offer: %v", err)
	}

	offer, err := offerSignal.Description()
	if err != nil {
		log.Fatalf("parse offer: %v", err)
	}
	if offer.Type != webrtc.SDPTypeOffer {
		log.Fatalf("expected offer, got %s", offer.Type.String())
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Fatalf("set remote offer: %v", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Fatalf("create answer: %v", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Fatalf("set local answer: %v", err)
	}
	<-gatherComplete

	localAnswer := pc.LocalDescription()
	if localAnswer == nil {
		log.Fatal("missing local answer after ICE gathering")
	}

	if err := lab.PutSignal(signalCtx, *brokerURL, *sessionID, "answer", lab.FromDescription(*localAnswer)); err != nil {
		log.Fatalf("post answer: %v", err)
	}

	log.Printf("answer posted; proxy server is ready for bounded WebRTC target requests")
	<-ctx.Done()
}

func newBoundedHTTPClient(target *url.URL, timeout time.Duration) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: timeout,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if !redirectAllowed(target, req.URL) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}, nil
}

func redirectAllowed(target, redirectURL *url.URL) bool {
	if redirectURL == nil {
		return false
	}
	return sameTargetOrigin(target, redirectURL) && withinTargetBasePath(target, redirectURL.Path)
}
func parseTarget(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("target scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("target host is required")
	}
	if hasParentPathSegment(parsed.Path) {
		return nil, fmt.Errorf("target path must not contain parent-directory segments")
	}
	parsed.Fragment = ""
	return parsed, nil
}

func handleRelayMessage(ctx context.Context, client *http.Client, d *webrtc.DataChannel, target *url.URL, data []byte, maxBody int64) {
	var relayReq lab.RelayRequest
	if err := json.Unmarshal(data, &relayReq); err != nil {
		sendRelayResponse(d, lab.RelayResponse{
			Type:  lab.RelayResponseType,
			Error: "invalid JSON proxy request",
			Time:  time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	if relayReq.Type != lab.RelayRequestType {
		sendRelayResponse(d, errorResponse(relayReq.ID, "unsupported proxy message type"))
		return
	}

	method := strings.ToUpper(strings.TrimSpace(relayReq.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		sendRelayResponse(d, errorResponse(relayReq.ID, "only GET and POST are allowed in this lab"))
		return
	}

	requestURL, err := boundedTargetURL(target, relayReq.Path)
	if err != nil {
		sendRelayResponse(d, errorResponse(relayReq.ID, err.Error()))
		return
	}

	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewBufferString(relayReq.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		sendRelayResponse(d, errorResponse(relayReq.ID, "build target request failed"))
		return
	}
	applyRelayRequestHeaders(req, relayReq)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "text/plain")
	}

	log.Printf("proxy request id=%s method=%s target=%s", relayReq.ID, method, requestURL)
	resp, err := client.Do(req)
	if err != nil {
		sendRelayResponse(d, errorResponse(relayReq.ID, fmt.Sprintf("target request failed: %v", err)))
		return
	}
	defer resp.Body.Close()

	limit := maxBody
	if limit <= 0 {
		limit = 4096
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		sendRelayResponse(d, errorResponse(relayReq.ID, "read target response failed"))
		return
	}

	truncated := int64(len(bodyBytes)) > limit
	if truncated {
		bodyBytes = bodyBytes[:limit]
	}

	responseBody, bodyFormat := relayResponseBody(bodyBytes, resp.Header.Get("Content-Type"))
	preview := responsePreview(bodyBytes, truncated, bodyFormat)

	finalURL := requestURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	sendRelayResponse(d, lab.RelayResponse{
		Type:        lab.RelayResponseType,
		ID:          relayReq.ID,
		Status:      resp.StatusCode,
		Target:      finalURL,
		Bytes:       len(bodyBytes),
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     selectedResponseHeaders(resp.Header),
		BodyFormat:  bodyFormat,
		Body:        responseBody,
		BodyPreview: preview,
		Truncated:   truncated,
		Time:        time.Now().UTC().Format(time.RFC3339),
	})
}

func applyRelayRequestHeaders(req *http.Request, relayReq lab.RelayRequest) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; snowflake-webrtc-proxy-lab/1.1)")
	req.Header.Set("Accept", allowedRequestHeader(relayReq.Headers, "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"))
	req.Header.Set("Accept-Language", allowedRequestHeader(relayReq.Headers, "Accept-Language", "en-US,en;q=0.9"))
	req.Header.Set("X-WebRTC-Proxy-Lab", "true")
	req.Header.Set("X-Proxy-Request-ID", relayReq.ID)
}

func allowedRequestHeader(headers map[string]string, name, fallback string) string {
	if headers == nil {
		return fallback
	}
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			value = strings.TrimSpace(value)
			if value != "" && !strings.ContainsAny(value, "\r\n") {
				return value
			}
		}
	}
	return fallback
}

func selectedResponseHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for _, name := range []string{"Cache-Control", "Content-Language", "Content-Type", "ETag", "Last-Modified", "Location"} {
		if value := headers.Get(name); value != "" {
			out[strings.ToLower(name)] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func relayResponseBody(body []byte, contentType string) (string, string) {
	if shouldTreatBodyAsText(body, contentType) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}

func shouldTreatBodyAsText(body []byte, contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err == nil {
		mediaType = strings.ToLower(mediaType)
		if strings.HasPrefix(mediaType, "text/") {
			return true
		}
		switch mediaType {
		case "application/ecmascript", "application/javascript", "application/json", "application/manifest+json", "application/rss+xml", "application/x-javascript", "application/xhtml+xml", "application/xml", "image/svg+xml":
			return true
		}
	}
	return contentType == "" && utf8.Valid(body)
}

func responsePreview(body []byte, bodyTruncated bool, bodyFormat string) string {
	const previewLimit = 512
	if bodyFormat == "base64" {
		out := fmt.Sprintf("[binary response body: %d bytes]", len(body))
		if bodyTruncated {
			out += "...[body truncated]"
		}
		return out
	}

	preview := body
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}

	out := string(preview)
	if len(body) > previewLimit {
		out += "...[preview truncated]"
	}
	if bodyTruncated {
		out += "...[body truncated]"
	}
	return out
}

func boundedTargetURL(target *url.URL, requestPath string) (string, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "/"
	}
	if strings.HasPrefix(requestPath, "http://") || strings.HasPrefix(requestPath, "https://") || strings.HasPrefix(requestPath, "//") {
		return "", fmt.Errorf("client may only request relative paths")
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	relative, err := url.Parse(requestPath)
	if err != nil {
		return "", fmt.Errorf("invalid request path")
	}
	if relative.IsAbs() || relative.Host != "" || relative.Scheme != "" {
		return "", fmt.Errorf("client may only request relative paths")
	}

	out := *target
	out.Path = joinTargetPath(target.Path, relative.Path)
	out.RawQuery = relative.RawQuery
	out.Fragment = ""

	if hasParentPathSegment(out.Path) || !withinTargetBasePath(target, out.Path) {
		return "", fmt.Errorf("request path must not contain parent-directory segments")
	}
	return out.String(), nil
}

func sameTargetOrigin(target, candidate *url.URL) bool {
	return strings.EqualFold(candidate.Scheme, target.Scheme) && strings.EqualFold(candidate.Host, target.Host)
}

func withinTargetBasePath(target *url.URL, candidatePath string) bool {
	basePath := strings.TrimRight(target.Path, "/")
	if basePath == "" {
		return true
	}
	return candidatePath == basePath || strings.HasPrefix(candidatePath, basePath+"/")
}

func joinTargetPath(basePath, requestPath string) string {
	if requestPath == "" {
		requestPath = "/"
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}

func hasParentPathSegment(pathValue string) bool {
	for _, segment := range strings.Split(pathValue, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func errorResponse(id, message string) lab.RelayResponse {
	return lab.RelayResponse{
		Type:  lab.RelayResponseType,
		ID:    id,
		Error: message,
		Time:  time.Now().UTC().Format(time.RFC3339),
	}
}

func sendRelayResponse(d *webrtc.DataChannel, response lab.RelayResponse) {
	if response.Body != "" {
		body := []byte(response.Body)
		if len(body) > responseChunkBytes {
			sendRelayResponseChunks(d, response, body)
			return
		}
	}

	payload, err := json.Marshal(response)
	if err != nil {
		log.Printf("marshal proxy response: %v", err)
		return
	}
	if err := d.Send(payload); err != nil {
		log.Printf("send proxy response: %v", err)
	}
}

func sendRelayResponseChunks(d *webrtc.DataChannel, response lab.RelayResponse, body []byte) {
	total := (len(body) + responseChunkBytes - 1) / responseChunkBytes
	if total == 0 {
		total = 1
	}

	log.Printf("sending proxy response id=%s in %d DataChannel chunks", response.ID, total)
	for i := 0; i < total; i++ {
		start := i * responseChunkBytes
		end := start + responseChunkBytes
		if end > len(body) {
			end = len(body)
		}

		chunk := response
		chunk.Type = lab.RelayResponseChunkType
		chunk.Body = ""
		chunk.BodyEncoding = "base64"
		chunk.BodyChunk = base64.StdEncoding.EncodeToString(body[start:end])
		chunk.ChunkIndex = i
		chunk.ChunkTotal = total

		payload, err := json.Marshal(chunk)
		if err != nil {
			log.Printf("marshal proxy response chunk id=%s chunk=%d/%d: %v", response.ID, i+1, total, err)
			return
		}
		if err := d.Send(payload); err != nil {
			log.Printf("send proxy response chunk id=%s chunk=%d/%d: %v", response.ID, i+1, total, err)
			return
		}
	}
}
