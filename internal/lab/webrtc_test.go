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
