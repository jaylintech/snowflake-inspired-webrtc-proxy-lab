package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/webrtc/v3"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
	"snowflake-inspired-webrtc-proxy-lab/internal/relay"
)

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

	targetURL, err := relay.ParseTarget(*target)
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

	httpClient, err := relay.NewBoundedHTTPClient(targetURL, *requestTimeout)
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

func handleRelayMessage(ctx context.Context, client *http.Client, d *webrtc.DataChannel, targetURL *url.URL, data []byte, maxBody int64) {
	var relayReq lab.RelayRequest
	if err := json.Unmarshal(data, &relayReq); err != nil {
		relay.SendRelayResponse(d, relay.ErrorResponse("", "invalid JSON proxy request"))
		return
	}

	if relayReq.Type != lab.RelayRequestType {
		relay.SendRelayResponse(d, relay.ErrorResponse(relayReq.ID, "unsupported proxy message type"))
		return
	}

	httpReq, err := relay.BuildRelayRequest(ctx, relayReq, targetURL)
	if err != nil {
		relay.SendRelayResponse(d, relay.ErrorResponse(relayReq.ID, err.Error()))
		return
	}

	log.Printf("proxy request id=%s method=%s target=%s", relayReq.ID, httpReq.Method, httpReq.URL.String())
	resp, err := client.Do(httpReq)
	if err != nil {
		relay.SendRelayResponse(d, relay.ErrorResponse(relayReq.ID, fmt.Sprintf("target request failed: %v", err)))
		return
	}
	defer resp.Body.Close()

	response := relay.BuildRelayResponse(relayReq, resp, maxBody)
	relay.SendRelayResponse(d, response)
}
