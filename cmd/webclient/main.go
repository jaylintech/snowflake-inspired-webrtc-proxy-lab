package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/webrtc/v3"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func main() {
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "proxy-session", "shared signaling session id")
	stunServers := flag.String("stun", lab.DefaultSTUN, "comma-separated STUN URLs; empty disables external STUN")
	icePortMin := flag.Uint("ice-port-min", 0, "minimum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	icePortMax := flag.Uint("ice-port-max", 0, "maximum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	advertiseIP := flag.String("advertise-ip", "", "public IP to advertise for ICE host candidates when using a router port forward")
	paths := flag.String("paths", "/,/healthz,/article-proof?via=webrtc", "comma-separated relative paths to request through the proxy server")
	method := flag.String("method", httpMethodGet, "GET or POST")
	body := flag.String("body", "synthetic proxy lab body", "POST body used when method is POST")
	interval := flag.Duration("interval", time.Second, "delay between proxied requests")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time to wait for signaling and responses")
	pollInterval := flag.Duration("poll", time.Second, "broker polling interval")
	transportPreset := flag.String("transport", "", "transport preset: direct, stun, turn-udp, turn-tcp, turns-tls")
	flag.Parse()

	requestPaths := splitPaths(*paths)
	if len(requestPaths) == 0 {
		log.Fatal("at least one relative path is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signalCtx, cancelSignal := context.WithTimeout(ctx, *timeout)
	defer cancelSignal()

	opts := lab.PeerConnectionOptionsFromEnvironment(*stunServers, *icePortMin, *icePortMax, *advertiseIP)
	if *transportPreset != "" {
		if err := lab.ApplyTransportPreset(&opts, *transportPreset, os.Getenv("LAB_TURN_HOST")); err != nil {
			log.Fatalf("apply transport preset: %v", err)
		}
	}

	pc, err := lab.NewPeerConnectionWithOptions(opts)
	if err != nil {
		log.Fatalf("create peer connection: %v", err)
	}
	defer func() {
		if err := pc.Close(); err != nil {
			log.Printf("close peer connection: %v", err)
		}
	}()

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

	dataChannel, err := pc.CreateDataChannel("lab-proxy", nil)
	if err != nil {
		log.Fatalf("create data channel: %v", err)
	}

	var responses uint64
	done := make(chan struct{})
	var closeDone sync.Once
	assembler := newResponseAssembler()

	dataChannel.OnOpen(func() {
		log.Printf("proxy data channel %q open; sending %d bounded target request(s)", dataChannel.Label(), len(requestPaths))
		go sendRelayRequests(ctx, dataChannel, requestPaths, strings.ToUpper(*method), *body, *interval)
	})
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var response lab.RelayResponse
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			log.Printf("invalid proxy response: %s", string(msg.Data))
			return
		}

		if response.Type == lab.RelayResponseChunkType {
			assembled, complete, err := assembler.add(response)
			if err != nil {
				log.Printf("invalid proxy response chunk id=%s: %v", response.ID, err)
				return
			}
			log.Printf("proxy response chunk id=%s chunk=%d/%d", response.ID, response.ChunkIndex+1, response.ChunkTotal)
			if !complete {
				return
			}
			response = assembled
		}

		if response.Error != "" {
			log.Printf("proxy response id=%s error=%s", response.ID, response.Error)
		} else {
			log.Printf("proxy response id=%s status=%d bytes=%d target=%s preview=%q", response.ID, response.Status, response.Bytes, response.Target, response.BodyPreview)
		}

		if atomic.AddUint64(&responses, 1) >= uint64(len(requestPaths)) {
			closeDone.Do(func() { close(done) })
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("create offer: %v", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Fatalf("set local offer: %v", err)
	}
	<-gatherComplete

	localOffer := pc.LocalDescription()
	if localOffer == nil {
		log.Fatal("missing local offer after ICE gathering")
	}

	if err := lab.PutSignal(signalCtx, *brokerURL, *sessionID, "offer", lab.FromDescription(*localOffer)); err != nil {
		log.Fatalf("post offer: %v", err)
	}
	log.Printf("offer posted to %s session %q; waiting for proxy answer", *brokerURL, *sessionID)

	answerSignal, err := lab.PollSignal(signalCtx, *brokerURL, *sessionID, "answer", *pollInterval)
	if err != nil {
		log.Fatalf("poll answer: %v", err)
	}

	answer, err := answerSignal.Description()
	if err != nil {
		log.Fatalf("parse answer: %v", err)
	}
	if answer.Type != webrtc.SDPTypeAnswer {
		log.Fatalf("expected answer, got %s", answer.Type.String())
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		log.Fatalf("set remote answer: %v", err)
	}

	select {
	case <-done:
		log.Printf("received all proxy responses; exiting")
	case <-signalCtx.Done():
		log.Printf("timed out waiting for proxy responses: %v", signalCtx.Err())
	}
}

const httpMethodGet = "GET"

func sendRelayRequests(ctx context.Context, dataChannel *webrtc.DataChannel, paths []string, method, body string, interval time.Duration) {
	if method == "" {
		method = httpMethodGet
	}
	for i, path := range paths {
		request := lab.RelayRequest{
			Type:   lab.RelayRequestType,
			ID:     fmt.Sprintf("proxy-%03d", i+1),
			Method: method,
			Path:   path,
		}
		if method == "POST" {
			request.Body = body
		}

		payload, err := json.Marshal(request)
		if err != nil {
			log.Printf("marshal proxy request: %v", err)
			return
		}
		if err := dataChannel.Send(payload); err != nil {
			log.Printf("send proxy request: %v", err)
			return
		}
		log.Printf("sent proxy request id=%s method=%s path=%s", request.ID, request.Method, request.Path)

		if i == len(paths)-1 || interval <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func splitPaths(raw string) []string {
	var out []string
	for _, path := range strings.Split(raw, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

type responseAssembler struct {
	mu      sync.Mutex
	pending map[string]*chunkedResponse
}

type chunkedResponse struct {
	response lab.RelayResponse
	chunks   []string
	received int
}

func newResponseAssembler() *responseAssembler {
	return &responseAssembler{pending: make(map[string]*chunkedResponse)}
}

func (a *responseAssembler) add(chunk lab.RelayResponse) (lab.RelayResponse, bool, error) {
	if chunk.ID == "" {
		return lab.RelayResponse{}, false, fmt.Errorf("missing response id")
	}
	if chunk.ChunkTotal <= 0 {
		return lab.RelayResponse{}, false, fmt.Errorf("missing chunk total")
	}
	if chunk.ChunkIndex < 0 || chunk.ChunkIndex >= chunk.ChunkTotal {
		return lab.RelayResponse{}, false, fmt.Errorf("chunk index %d outside total %d", chunk.ChunkIndex, chunk.ChunkTotal)
	}
	if chunk.BodyEncoding != "base64" {
		return lab.RelayResponse{}, false, fmt.Errorf("unsupported body encoding %q", chunk.BodyEncoding)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	state := a.pending[chunk.ID]
	if state == nil {
		state = &chunkedResponse{
			response: chunk,
			chunks:   make([]string, chunk.ChunkTotal),
		}
		a.pending[chunk.ID] = state
	}
	if len(state.chunks) != chunk.ChunkTotal {
		delete(a.pending, chunk.ID)
		return lab.RelayResponse{}, false, fmt.Errorf("chunk total changed from %d to %d", len(state.chunks), chunk.ChunkTotal)
	}
	if state.chunks[chunk.ChunkIndex] == "" {
		state.received++
	}
	state.chunks[chunk.ChunkIndex] = chunk.BodyChunk

	if state.received < len(state.chunks) {
		return lab.RelayResponse{}, false, nil
	}

	var body []byte
	for i, encoded := range state.chunks {
		if encoded == "" {
			return lab.RelayResponse{}, false, fmt.Errorf("missing chunk %d", i)
		}
		part, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return lab.RelayResponse{}, false, fmt.Errorf("decode chunk %d: %w", i, err)
		}
		body = append(body, part...)
	}

	delete(a.pending, chunk.ID)

	response := state.response
	response.Type = lab.RelayResponseType
	response.BodyEncoding = ""
	response.BodyChunk = ""
	response.ChunkIndex = 0
	response.ChunkTotal = 0
	response.Body = string(body)
	return response, true, nil
}
