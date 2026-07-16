package lab

import "testing"

func TestPeerConnectionOptionsFromEnvironment(t *testing.T) {
	t.Setenv("LAB_TURN_URLS", "turns:turn.example.test:443?transport=tcp")
	t.Setenv("LAB_TURN_USERNAME", "lab-user")
	t.Setenv("LAB_TURN_CREDENTIAL", "lab-password")
	t.Setenv("LAB_ICE_POLICY", "relay")

	options := PeerConnectionOptionsFromEnvironment("stun:stun.example.test:3478", 40000, 40000, "203.0.113.10")
	if options.TURNServers != "turns:turn.example.test:443?transport=tcp" || options.ICEPolicy != "relay" {
		t.Fatalf("TURN environment was not applied: %+v", options)
	}
	if options.ICEPortMin != 40000 || options.ICEPortMax != 40000 || options.AdvertiseIP != "203.0.113.10" {
		t.Fatalf("explicit ICE settings were not preserved: %+v", options)
	}
}
