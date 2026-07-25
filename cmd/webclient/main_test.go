package main

import (
	"encoding/base64"
	"testing"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

func TestAssemblerSingleChunk(t *testing.T) {
	a := newResponseAssembler()
	body := "hello world"
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "test-001",
		BodyEncoding: "base64",
		BodyChunk:    base64.StdEncoding.EncodeToString([]byte(body)),
		ChunkIndex:   0,
		ChunkTotal:   1,
	}

	_, complete, err := a.add(chunk)
	if err != nil {
		t.Fatalf("add chunk: %v", err)
	}
	if !complete {
		t.Fatal("expected single chunk to complete immediately")
	}
}

func TestAssemblerMultipleChunks(t *testing.T) {
	a := newResponseAssembler()
	parts := []string{"hello ", "world ", "from ", "chunks"}

	for i, part := range parts {
		chunk := lab.RelayResponse{
			Type:         lab.RelayResponseChunkType,
			ID:           "multi-001",
			BodyEncoding: "base64",
			BodyChunk:    base64.StdEncoding.EncodeToString([]byte(part)),
			ChunkIndex:   i,
			ChunkTotal:   len(parts),
		}
		_, complete, err := a.add(chunk)
		if err != nil {
			t.Fatalf("add chunk %d: %v", i, err)
		}
		if i < len(parts)-1 && complete {
			t.Fatalf("chunk %d prematurely completed", i)
		}
		if i == len(parts)-1 && !complete {
			t.Fatal("final chunk did not complete")
		}
	}
}

func TestAssemblerOutOfOrderChunks(t *testing.T) {
	a := newResponseAssembler()
	parts := map[int]string{0: "first ", 1: "second ", 2: "third"}

	order := []int{1, 0, 2}
	for _, i := range order {
		chunk := lab.RelayResponse{
			Type:         lab.RelayResponseChunkType,
			ID:           "ooo-001",
			BodyEncoding: "base64",
			BodyChunk:    base64.StdEncoding.EncodeToString([]byte(parts[i])),
			ChunkIndex:   i,
			ChunkTotal:   len(parts),
		}
		_, complete, err := a.add(chunk)
		if err != nil {
			t.Fatalf("add chunk %d: %v", i, err)
		}
		if i == 2 && !complete {
			t.Fatal("final out-of-order chunk did not complete")
		}
	}
}

func TestAssemblerRejectsDuplicateChunk(t *testing.T) {
	a := newResponseAssembler()
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "dup-001",
		BodyEncoding: "base64",
		BodyChunk:    base64.StdEncoding.EncodeToString([]byte("data")),
		ChunkIndex:   0,
		ChunkTotal:   2,
	}

	_, _, _ = a.add(chunk)

	chunk2 := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "dup-001",
		BodyEncoding: "base64",
		BodyChunk:    base64.StdEncoding.EncodeToString([]byte("data2")),
		ChunkIndex:   1,
		ChunkTotal:   2,
	}
	_, complete, err := a.add(chunk2)
	if err != nil {
		t.Fatalf("add second chunk: %v", err)
	}
	if !complete {
		t.Fatal("expected completion after second chunk")
	}
}

func TestAssemblerRejectsMissingID(t *testing.T) {
	a := newResponseAssembler()
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "",
		BodyEncoding: "base64",
		BodyChunk:    "dGVzdA==",
		ChunkIndex:   0,
		ChunkTotal:   1,
	}
	_, _, err := a.add(chunk)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestAssemblerRejectsMissingChunkTotal(t *testing.T) {
	a := newResponseAssembler()
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "test-001",
		BodyEncoding: "base64",
		BodyChunk:    "dGVzdA==",
		ChunkIndex:   0,
		ChunkTotal:   0,
	}
	_, _, err := a.add(chunk)
	if err == nil {
		t.Fatal("expected error for missing chunk total")
	}
}

func TestAssemblerRejectsIndexOutOfRange(t *testing.T) {
	a := newResponseAssembler()
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "test-001",
		BodyEncoding: "base64",
		BodyChunk:    "dGVzdA==",
		ChunkIndex:   5,
		ChunkTotal:   3,
	}
	_, _, err := a.add(chunk)
	if err == nil {
		t.Fatal("expected error for chunk index outside total")
	}
}

func TestAssemblerRejectsUnsupportedEncoding(t *testing.T) {
	a := newResponseAssembler()
	chunk := lab.RelayResponse{
		Type:         lab.RelayResponseChunkType,
		ID:           "test-001",
		BodyEncoding: "plain",
		BodyChunk:    "test",
		ChunkIndex:   0,
		ChunkTotal:   1,
	}
	_, _, err := a.add(chunk)
	if err == nil {
		t.Fatal("expected error for unsupported encoding")
	}
}

func TestAssemblerProducesCompleteResponse(t *testing.T) {
	a := newResponseAssembler()
	parts := []string{"part1", "part2", "part3_"}
	var result lab.RelayResponse
	var complete bool
	var err error

	for i, part := range parts {
		chunk := lab.RelayResponse{
			Type:         lab.RelayResponseChunkType,
			ID:           "full-001",
			BodyEncoding: "base64",
			BodyChunk:    base64.StdEncoding.EncodeToString([]byte(part)),
			ChunkIndex:   i,
			ChunkTotal:   len(parts),
			Status:       200,
			Target:       "https://example.com/",
			ContentType:  "text/plain",
			Time:         "2026-01-01T00:00:00Z",
		}
		result, complete, err = a.add(chunk)
		if err != nil {
			t.Fatalf("add chunk %d: %v", i, err)
		}
		if i < len(parts)-1 && complete {
			t.Fatalf("chunk %d prematurely completed", i)
		}
	}

	if !complete {
		t.Fatal("expected completion after all chunks")
	}
	if result.Type != lab.RelayResponseType {
		t.Fatalf("type = %q, want %q", result.Type, lab.RelayResponseType)
	}
	if result.Body != "part1part2part3_" {
		t.Fatalf("assembled body = %q, want %q", result.Body, "part1part2part3_")
	}
}
