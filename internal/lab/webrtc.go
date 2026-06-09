package lab

import (
	"strings"

	"github.com/pion/webrtc/v3"
)

const DefaultSTUN = "stun:stun.l.google.com:19302"

// NewWebRTCConfig returns a minimal WebRTC configuration for a lab run.
// Pass an empty stunCSV value to disable external STUN and test only local ICE.
func NewWebRTCConfig(stunCSV string) webrtc.Configuration {
	urls := splitCSV(stunCSV)
	if len(urls) == 0 {
		return webrtc.Configuration{}
	}

	return webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: urls},
		},
	}
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
