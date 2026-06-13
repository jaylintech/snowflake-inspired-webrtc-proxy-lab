package lab

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRelayResponseChunkIncludesZeroIndex(t *testing.T) {
	payload, err := json.Marshal(RelayResponse{
		Type:         RelayResponseChunkType,
		ID:           "browser-001",
		BodyEncoding: "base64",
		BodyChunk:    "SGVsbG8=",
		ChunkIndex:   0,
		ChunkTotal:   4,
		Time:         "2026-06-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal chunk response: %v", err)
	}

	if !strings.Contains(string(payload), `"chunk_index":0`) {
		t.Fatalf("expected first chunk to include chunk_index=0, got %s", string(payload))
	}
}
