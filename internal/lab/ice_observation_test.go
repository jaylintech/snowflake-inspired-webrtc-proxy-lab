package lab

import "testing"

func TestSelectedCandidatePairRejectsNilPeer(t *testing.T) {
	if _, err := SelectedCandidatePair(nil); err == nil {
		t.Fatal("expected nil peer to fail")
	}
}
