package lab

import "testing"

func TestNewPeerConnectionRejectsPartialICEPortRange(t *testing.T) {
	pc, err := NewPeerConnection("", 40000, 0, "")
	if err == nil {
		if pc != nil {
			_ = pc.Close()
		}
		t.Fatal("expected partial ICE port range to fail")
	}
}

func TestNewPeerConnectionRejectsOutOfRangeICEPort(t *testing.T) {
	pc, err := NewPeerConnection("", 40000, 70000, "")
	if err == nil {
		if pc != nil {
			_ = pc.Close()
		}
		t.Fatal("expected out-of-range ICE port to fail")
	}
}

func TestNewPeerConnectionAcceptsSingleICEPort(t *testing.T) {
	pc, err := NewPeerConnection("", 40000, 40000, "")
	if err != nil {
		t.Fatalf("single-port ICE range failed: %v", err)
	}
	defer pc.Close()
}

func TestNewPeerConnectionRejectsInvalidAdvertiseIP(t *testing.T) {
	pc, err := NewPeerConnection("", 40000, 40000, "not-an-ip")
	if err == nil {
		if pc != nil {
			_ = pc.Close()
		}
		t.Fatal("expected invalid advertised IP to fail")
	}
}

func TestNewPeerConnectionAcceptsAdvertiseIP(t *testing.T) {
	pc, err := NewPeerConnection("", 40000, 40000, "203.0.113.10")
	if err != nil {
		t.Fatalf("advertised IP with single-port ICE range failed: %v", err)
	}
	defer pc.Close()
}

func TestApplyTransportPreset(t *testing.T) {
	tests := []struct {
		preset      string
		wantSTUN    string
		wantTURN    string
		wantPolicy  string
		expectError bool
	}{
		{preset: "direct", wantSTUN: "", wantTURN: "", wantPolicy: "all", expectError: false},
		{preset: "stun", wantSTUN: DefaultSTUN, wantTURN: "", wantPolicy: "all", expectError: false},
		{preset: "turn-udp", wantSTUN: "", wantTURN: "turn:turn.lab.example:3478", wantPolicy: "relay", expectError: false},
		{preset: "turn-tcp", wantSTUN: "", wantTURN: "turn:turn.lab.example:3478?transport=tcp", wantPolicy: "relay", expectError: false},
		{preset: "turns-tls", wantSTUN: "", wantTURN: "turns:turn.lab.example:443?transport=tcp", wantPolicy: "relay", expectError: false},
		{preset: "invalid", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			opts := PeerConnectionOptions{}
			err := ApplyTransportPreset(&opts, tt.preset, "turn.lab.example")
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for preset %q", tt.preset)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.STUNServers != tt.wantSTUN {
				t.Errorf("STUNServers = %q, want %q", opts.STUNServers, tt.wantSTUN)
			}
			if opts.TURNServers != tt.wantTURN {
				t.Errorf("TURNServers = %q, want %q", opts.TURNServers, tt.wantTURN)
			}
			if opts.ICEPolicy != tt.wantPolicy {
				t.Errorf("ICEPolicy = %q, want %q", opts.ICEPolicy, tt.wantPolicy)
			}
		})
	}
}

func TestApplyTransportPresetRejectsNilOptions(t *testing.T) {
	if err := ApplyTransportPreset(nil, "direct", ""); err == nil {
		t.Fatal("expected nil options to be rejected")
	}
}
