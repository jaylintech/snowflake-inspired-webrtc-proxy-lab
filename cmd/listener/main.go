package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	taskEvery := flag.Int("task-every", 3, "send a simulated task every N beacons; 0 disables tasking")
	taskSequence := flag.String("task-sequence", "sleep,inventory,synthetic-upload", "comma-separated simulated task actions")
	syntheticBytes := flag.Int("synthetic-bytes", 4096, "synthetic upload bytes requested for synthetic-upload tasks")
	chunkBytes := flag.Int("chunk-bytes", 512, "synthetic upload chunk size requested for synthetic-upload tasks")
	pollInterval := flag.Duration("poll", time.Second, "broker polling interval")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum time to wait for signaling")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	signalCtx, cancelSignal := context.WithTimeout(ctx, *timeout)
	defer cancelSignal()

	pc, err := lab.NewPeerConnection(*stunServers, *icePortMin, *icePortMax, *advertiseIP)
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

	var ackCounter uint64
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		log.Printf("data channel %q created by client", d.Label())

		d.OnOpen(func() {
			log.Printf("data channel %q open", d.Label())
		})

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			text := string(msg.Data)
			if strings.HasPrefix(text, "LAB_BEACON:") {
				seq := atomic.AddUint64(&ackCounter, 1)
				log.Printf("received lab beacon #%d: %s", seq, text)

				ack := fmt.Sprintf("LAB_ACK: seq=%d time=%s", seq, time.Now().UTC().Format(time.RFC3339))
				if err := d.SendText(ack); err != nil {
					log.Printf("send ack: %v", err)
				}

				if shouldSendTask(*taskEvery, seq) {
					task := nextTask(*taskSequence, seq)
					taskID := fmt.Sprintf("task-%03d", seq)
					request := formatTask(taskID, task, *syntheticBytes, *chunkBytes)
					log.Printf("sending simulated task: %s", request)
					if err := d.SendText(request); err != nil {
						log.Printf("send simulated task: %v", err)
					}
				}
				return
			}

			log.Printf("received lab message: %s", text)
		})
	})

	log.Printf("waiting for SDP offer at %s session %q", *brokerURL, *sessionID)
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

	log.Printf("answer posted; listener is ready for lab DataChannel traffic")
	<-ctx.Done()
}

func shouldSendTask(taskEvery int, beaconSeq uint64) bool {
	return taskEvery > 0 && beaconSeq > 0 && beaconSeq%uint64(taskEvery) == 0
}

func nextTask(taskSequence string, beaconSeq uint64) string {
	tasks := splitTasks(taskSequence)
	if len(tasks) == 0 {
		return "sleep"
	}
	return tasks[(int(beaconSeq)-1)%len(tasks)]
}

func splitTasks(taskSequence string) []string {
	var tasks []string
	for _, task := range strings.Split(taskSequence, ",") {
		task = strings.TrimSpace(task)
		if task != "" {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func formatTask(taskID, action string, syntheticBytes, chunkBytes int) string {
	if syntheticBytes < 0 {
		syntheticBytes = 0
	}
	if chunkBytes <= 0 {
		chunkBytes = 512
	}

	return strings.Join([]string{
		"LAB_TASK:",
		"id=" + taskID,
		"action=" + action,
		"bytes=" + strconv.Itoa(syntheticBytes),
		"chunk=" + strconv.Itoa(chunkBytes),
		"time=" + time.Now().UTC().Format(time.RFC3339),
	}, " ")
}
