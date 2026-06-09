package lab

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v3"
)

type Signal struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func FromDescription(desc webrtc.SessionDescription) Signal {
	return Signal{
		Type: desc.Type.String(),
		SDP:  desc.SDP,
	}
}

func (s Signal) Description() (webrtc.SessionDescription, error) {
	var sdpType webrtc.SDPType

	switch strings.ToLower(strings.TrimSpace(s.Type)) {
	case "offer":
		sdpType = webrtc.SDPTypeOffer
	case "answer":
		sdpType = webrtc.SDPTypeAnswer
	default:
		return webrtc.SessionDescription{}, fmt.Errorf("unsupported SDP type %q", s.Type)
	}

	if strings.TrimSpace(s.SDP) == "" {
		return webrtc.SessionDescription{}, fmt.Errorf("empty SDP body")
	}

	return webrtc.SessionDescription{
		Type: sdpType,
		SDP:  s.SDP,
	}, nil
}
