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
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pion/webrtc/v3"

	"snowflakeprotocolpoc/internal/lab"
)

const responseChunkBytes = 24 * 1024

func main() {
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "proxy-session", "shared signaling session id")
	stunServers := flag.String("stun", lab.DefaultSTUN, "comma-separated STUN URLs; empty disables external STUN")
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

	pc, err := webrtc.NewPeerConnection(lab.NewWebRTCConfig(*stunServers))
	if err != nil {
		log.Fatalf("create peer connection: %v", err)
	}
	defer func() {
		if err := pc.Close(); err != nil {
			log.Printf("close peer connection: %v", err)
		}
	}()

	httpClient := &http.Client{Timeout: *requestTimeout}

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE connection state: %s", state.String())
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("peer connection state: %s", state.String())
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
	req.Header.Set("User-Agent", "snowflakeprotocolpoc-proxy/1.0")
	req.Header.Set("X-WebRTC-Proxy-Lab", "true")
	req.Header.Set("X-Proxy-Request-ID", relayReq.ID)
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

	preview := responsePreview(bodyBytes, truncated)

	sendRelayResponse(d, lab.RelayResponse{
		Type:        lab.RelayResponseType,
		ID:          relayReq.ID,
		Status:      resp.StatusCode,
		Target:      requestURL,
		Bytes:       len(bodyBytes),
		ContentType: resp.Header.Get("Content-Type"),
		Body:        string(bodyBytes),
		BodyPreview: preview,
		Truncated:   truncated,
		Time:        time.Now().UTC().Format(time.RFC3339),
	})
}

func responsePreview(body []byte, bodyTruncated bool) string {
	const previewLimit = 512

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
	if relative.IsAbs() || relative.Host != "" {
		return "", fmt.Errorf("client may only request relative paths")
	}
	if hasParentPathSegment(relative.Path) {
		return "", fmt.Errorf("request path must not contain parent-directory segments")
	}

	out := *target
	out.Path = joinTargetPath(target.Path, relative.Path)
	out.RawQuery = relative.RawQuery
	out.Fragment = ""
	return out.String(), nil
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
