package lab

import (
	"fmt"

	"github.com/pion/webrtc/v3"
)

// SelectedCandidatePair returns the active ICE path for measurement logging.
func SelectedCandidatePair(pc *webrtc.PeerConnection) (string, error) {
	if pc == nil || pc.SCTP() == nil || pc.SCTP().Transport() == nil || pc.SCTP().Transport().ICETransport() == nil {
		return "", fmt.Errorf("ICE transport is not ready")
	}
	pair, err := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil {
		return "", fmt.Errorf("get selected ICE candidate pair: %w", err)
	}
	if pair == nil || pair.Local == nil || pair.Remote == nil {
		return "", fmt.Errorf("selected ICE candidate pair is unavailable")
	}
	return pair.String(), nil
}
