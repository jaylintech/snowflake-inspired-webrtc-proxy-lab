package lab

import (
	"fmt"
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

// NewPeerConnection creates a PeerConnection with optional local ICE UDP port bounds.
// Pass 0 for both port values to use Pion's default dynamic UDP port behavior.
func NewPeerConnection(stunCSV string, icePortMin, icePortMax uint) (*webrtc.PeerConnection, error) {
	config := NewWebRTCConfig(stunCSV)
	if icePortMin == 0 && icePortMax == 0 {
		return webrtc.NewPeerConnection(config)
	}
	if icePortMin == 0 || icePortMax == 0 {
		return nil, fmt.Errorf("ice port min and max must both be set, or both be 0")
	}
	if icePortMin > 65535 || icePortMax > 65535 {
		return nil, fmt.Errorf("ice port values must be between 1 and 65535")
	}

	var settingEngine webrtc.SettingEngine
	if err := settingEngine.SetEphemeralUDPPortRange(uint16(icePortMin), uint16(icePortMax)); err != nil {
		return nil, fmt.Errorf("set ICE UDP port range: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	return api.NewPeerConnection(config)
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
