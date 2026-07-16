package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/webrtc/v3"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func main() {
	brokerURL := flag.String("broker", "http://127.0.0.1:8080", "signaling broker URL")
	sessionID := flag.String("session", "lab-session", "shared signaling session id")
	stunServers := flag.String("stun", lab.DefaultSTUN, "comma-separated STUN URLs; empty disables external STUN")
	icePortMin := flag.Uint("ice-port-min", 0, "minimum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	icePortMax := flag.Uint("ice-port-max", 0, "maximum local UDP port for Pion ICE candidates; 0 uses dynamic ports")
	advertiseIP := flag.String("advertise-ip", "", "public IP to advertise for ICE host candidates when using a router port forward")
	interval := flag.Duration("interval", 10*time.Second, "heartbeat interval")
	jitter := flag.Int("jitter", 20, "heartbeat jitter percentage from 0 to 90")
	count := flag.Int("count", 0, "number of heartbeats to send; 0 sends until interrupted")
	hostID := flag.String("host-id", "Host_ID_8842_Active", "benign host identifier included in heartbeat text")
	taskDelay := flag.Duration("task-delay", 750*time.Millisecond, "delay before returning simulated task results")
	chunkDelay := flag.Duration("chunk-delay", 150*time.Millisecond, "delay between synthetic upload chunks")
	pollInterval := flag.Duration("poll", time.Second, "broker polling interval")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time to wait for signaling")
	flag.Parse()

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

	dataChannel, err := pc.CreateDataChannel("lab-beacon", nil)
	if err != nil {
		log.Fatalf("create data channel: %v", err)
	}

	doneSending := make(chan struct{})
	dataChannel.OnOpen(func() {
		log.Printf("data channel %q open; sending lab beacons every %s with %d%% jitter", dataChannel.Label(), *interval, *jitter)
		sendHello(dataChannel, *hostID)
		go sendHeartbeats(ctx, dataChannel, *interval, *jitter, *count, *hostID, doneSending)
	})
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		text := string(msg.Data)
		log.Printf("listener response: %s", text)
		if strings.HasPrefix(text, "LAB_TASK:") {
			go handleSimulatedTask(ctx, dataChannel, text, *taskDelay, *chunkDelay)
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
	log.Printf("offer posted to %s session %q; waiting for answer", *brokerURL, *sessionID)

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

	if *count > 0 {
		select {
		case <-doneSending:
			log.Printf("sent requested heartbeat count; exiting")
		case <-signalCtx.Done():
			log.Printf("timed out waiting for data channel traffic: %v", signalCtx.Err())
		}
		return
	}

	<-ctx.Done()
}

func sendHello(dataChannel *webrtc.DataChannel, hostID string) {
	msg := fmt.Sprintf(
		"LAB_HELLO: host=%s user=lab-user os=windows profile=simulated-webrtc-client time=%s",
		hostID,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err := dataChannel.SendText(msg); err != nil {
		log.Printf("send hello: %v", err)
	}
}

func sendHeartbeats(ctx context.Context, dataChannel *webrtc.DataChannel, interval time.Duration, jitterPercent int, count int, hostID string, done chan<- struct{}) {
	defer close(done)

	if interval <= 0 {
		interval = 10 * time.Second
	}
	if jitterPercent < 0 {
		jitterPercent = 0
	}
	if jitterPercent > 90 {
		jitterPercent = 90
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var seq uint64
	for {
		current := atomic.AddUint64(&seq, 1)
		msg := fmt.Sprintf("LAB_BEACON: host=%s seq=%d idle=%ds time=%s", hostID, current, 120+current, time.Now().UTC().Format(time.RFC3339))
		if err := dataChannel.SendText(msg); err != nil {
			log.Printf("send beacon: %v", err)
			return
		}
		log.Printf("sent lab beacon #%d", current)

		if count > 0 && int(current) >= count {
			return
		}

		delay := intervalWithJitter(interval, jitterPercent, rng)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func intervalWithJitter(base time.Duration, jitterPercent int, rng *rand.Rand) time.Duration {
	if jitterPercent <= 0 {
		return base
	}

	delta := base.Nanoseconds() * int64(jitterPercent) / 100
	if delta <= 0 {
		return base
	}

	offset := rng.Int63n((delta*2)+1) - delta
	next := base + time.Duration(offset)
	if next < 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	return next
}

func handleSimulatedTask(ctx context.Context, dataChannel *webrtc.DataChannel, rawTask string, taskDelay, chunkDelay time.Duration) {
	fields := parseFields(rawTask)
	taskID := valueOrDefault(fields["id"], "task-unknown")
	action := valueOrDefault(fields["action"], "sleep")

	if taskDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(taskDelay):
		}
	}

	switch action {
	case "sleep":
		sendTaskResult(dataChannel, taskID, action, "simulated delay complete")
	case "inventory":
		sendTaskResult(dataChannel, taskID, action, "synthetic os=windows hostname=LAB-WS-8842 user=lab-user groups=lab")
	case "synthetic-upload":
		bytesTotal := clampInt(parseInt(fields["bytes"], 4096), 0, 1<<20)
		chunkBytes := clampInt(parseInt(fields["chunk"], 512), 64, 32*1024)
		sendSyntheticUpload(ctx, dataChannel, taskID, bytesTotal, chunkBytes, chunkDelay)
	default:
		sendTaskResult(dataChannel, taskID, action, "unsupported simulated action")
	}
}

func sendTaskResult(dataChannel *webrtc.DataChannel, taskID, action, detail string) {
	msg := fmt.Sprintf(
		"LAB_RESULT: id=%s action=%s status=simulated detail=%q time=%s",
		taskID,
		action,
		detail,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err := dataChannel.SendText(msg); err != nil {
		log.Printf("send simulated result: %v", err)
	}
}

func sendSyntheticUpload(ctx context.Context, dataChannel *webrtc.DataChannel, taskID string, bytesTotal, chunkBytes int, chunkDelay time.Duration) {
	sent := 0
	chunkIndex := 0
	for sent < bytesTotal {
		size := chunkBytes
		if remaining := bytesTotal - sent; remaining < size {
			size = remaining
		}

		payload := strings.Repeat("X", size)
		msg := fmt.Sprintf("LAB_CHUNK: id=%s idx=%d bytes=%d data=%s", taskID, chunkIndex, size, payload)
		if err := dataChannel.SendText(msg); err != nil {
			log.Printf("send synthetic chunk: %v", err)
			return
		}

		sent += size
		chunkIndex++

		if chunkDelay > 0 && sent < bytesTotal {
			select {
			case <-ctx.Done():
				return
			case <-time.After(chunkDelay):
			}
		}
	}

	sendTaskResult(dataChannel, taskID, "synthetic-upload", fmt.Sprintf("synthetic bytes sent=%d chunks=%d", sent, chunkIndex))
}

func parseFields(line string) map[string]string {
	fields := make(map[string]string)
	for _, part := range strings.Fields(line) {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			fields[key] = value
		}
	}
	return fields
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
