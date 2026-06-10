package main

import (
	"context"
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

	"snowflakeprotocolpoc/internal/lab"
)

func main() {
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "relay-session", "shared signaling session id")
	stunServers := flag.String("stun", lab.DefaultSTUN, "comma-separated STUN URLs; empty disables external STUN")
	paths := flag.String("paths", "/,/healthz,/article-proof?via=webrtc", "comma-separated relative paths to request through the relay")
	method := flag.String("method", httpMethodGet, "GET or POST")
	body := flag.String("body", "synthetic relay lab body", "POST body used when method is POST")
	interval := flag.Duration("interval", time.Second, "delay between relayed requests")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time to wait for signaling and responses")
	pollInterval := flag.Duration("poll", time.Second, "broker polling interval")
	flag.Parse()

	requestPaths := splitPaths(*paths)
	if len(requestPaths) == 0 {
		log.Fatal("at least one relative path is required")
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

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE connection state: %s", state.String())
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("peer connection state: %s", state.String())
	})

	dataChannel, err := pc.CreateDataChannel("lab-relay", nil)
	if err != nil {
		log.Fatalf("create data channel: %v", err)
	}

	var responses uint64
	done := make(chan struct{})
	var closeDone sync.Once

	dataChannel.OnOpen(func() {
		log.Printf("relay data channel %q open; sending %d bounded target request(s)", dataChannel.Label(), len(requestPaths))
		go sendRelayRequests(ctx, dataChannel, requestPaths, strings.ToUpper(*method), *body, *interval)
	})
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var response lab.RelayResponse
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			log.Printf("invalid relay response: %s", string(msg.Data))
			return
		}
		if response.Error != "" {
			log.Printf("relay response id=%s error=%s", response.ID, response.Error)
		} else {
			log.Printf("relay response id=%s status=%d bytes=%d target=%s preview=%q", response.ID, response.Status, response.Bytes, response.Target, response.BodyPreview)
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
	log.Printf("offer posted to %s session %q; waiting for relay answer", *brokerURL, *sessionID)

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
		log.Printf("received all relay responses; exiting")
	case <-signalCtx.Done():
		log.Printf("timed out waiting for relay responses: %v", signalCtx.Err())
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
			ID:     fmt.Sprintf("relay-%03d", i+1),
			Method: method,
			Path:   path,
		}
		if method == "POST" {
			request.Body = body
		}

		payload, err := json.Marshal(request)
		if err != nil {
			log.Printf("marshal relay request: %v", err)
			return
		}
		if err := dataChannel.Send(payload); err != nil {
			log.Printf("send relay request: %v", err)
			return
		}
		log.Printf("sent relay request id=%s method=%s path=%s", request.ID, request.Method, request.Path)

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
