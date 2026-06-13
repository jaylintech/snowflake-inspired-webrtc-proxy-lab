package lab

const (
	RelayRequestType  = "LAB_PROXY_REQUEST"
	RelayResponseType = "LAB_PROXY_RESPONSE"
)

type RelayRequest struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

type RelayResponse struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Status      int    `json:"status"`
	Target      string `json:"target"`
	Bytes       int    `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body,omitempty"`
	BodyPreview string `json:"body_preview,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Error       string `json:"error,omitempty"`
	Time        string `json:"time"`
}
