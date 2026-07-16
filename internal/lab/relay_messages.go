package lab

const (
	RelayRequestType       = "LAB_PROXY_REQUEST"
	RelayResponseType      = "LAB_PROXY_RESPONSE"
	RelayResponseChunkType = "LAB_PROXY_RESPONSE_CHUNK"
)

type RelayRequest struct {
	Type    string            `json:"type"`
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type RelayResponse struct {
	Type         string            `json:"type"`
	ID           string            `json:"id"`
	Status       int               `json:"status"`
	Target       string            `json:"target"`
	Bytes        int               `json:"bytes"`
	ContentType  string            `json:"content_type,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyEncoding string            `json:"body_encoding,omitempty"`
	BodyFormat   string            `json:"body_format,omitempty"`
	Body         string            `json:"body,omitempty"`
	BodyChunk    string            `json:"body_chunk,omitempty"`
	BodyPreview  string            `json:"body_preview,omitempty"`
	ChunkIndex   int               `json:"chunk_index"`
	ChunkTotal   int               `json:"chunk_total,omitempty"`
	Truncated    bool              `json:"truncated,omitempty"`
	Error        string            `json:"error,omitempty"`
	Time         string            `json:"time"`
}
