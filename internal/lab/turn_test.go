package lab

import (
	"strings"
	"testing"

	"github.com/pion/webrtc/v3"
)

func TestNewWebRTCConfigWithTURNRelayPolicy(t *testing.T) {
	config, err := NewWebRTCConfigWithOptions(PeerConnectionOptions{
		STUNServers:    "stun:stun.example.test:3478",
		TURNServers:    "turns:turn.example.test:443?transport=tcp",
		TURNUsername:   "lab-user",
		TURNCredential: "lab-password",
		ICEPolicy:      "relay",
	})
	if err != nil {
		t.Fatalf("TURN configuration failed: %v", err)
	}
	if config.ICETransportPolicy != webrtc.ICETransportPolicyRelay {
		t.Fatalf("ICE policy = %v, want relay", config.ICETransportPolicy)
	}
	if len(config.ICEServers) != 2 {
		t.Fatalf("ICE server groups = %d, want 2", len(config.ICEServers))
	}
	if config.ICEServers[1].Username != "lab-user" || config.ICEServers[1].Credential != "lab-password" {
		t.Fatalf("TURN credentials were not preserved")
	}
}

func TestNewWebRTCConfigRejectsPartialTURNCredentials(t *testing.T) {
	_, err := NewWebRTCConfigWithOptions(PeerConnectionOptions{
		TURNServers:  "turn:turn.example.test:3478",
		TURNUsername: "lab-user",
	})
	if err == nil || !strings.Contains(err.Error(), "both be set") {
		t.Fatalf("error = %v, want paired credential validation", err)
	}
}

func TestNewWebRTCConfigRejectsWrongServerScheme(t *testing.T) {
	_, err := NewWebRTCConfigWithOptions(PeerConnectionOptions{
		TURNServers: "https://turn.example.test:443",
	})
	if err == nil || !strings.Contains(err.Error(), "turn or turns") {
		t.Fatalf("error = %v, want TURN scheme validation", err)
	}
}

func TestNewWebRTCConfigRejectsUnknownICEPolicy(t *testing.T) {
	_, err := NewWebRTCConfigWithOptions(PeerConnectionOptions{ICEPolicy: "prefer-relay"})
	if err == nil || !strings.Contains(err.Error(), "all or relay") {
		t.Fatalf("error = %v, want ICE policy validation", err)
	}
}
