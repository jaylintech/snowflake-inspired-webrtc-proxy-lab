package relay

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"snowflake-inspired-webrtc-proxy-lab/internal/lab"
)

const ResponseChunkBytes = 24 * 1024

func ParseTarget(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("target scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("target host is required")
	}
	if HasParentPathSegment(parsed.Path) {
		return nil, fmt.Errorf("target path must not contain parent-directory segments")
	}
	parsed.Fragment = ""
	return parsed, nil
}

func NewBoundedHTTPClient(target *url.URL, timeout time.Duration) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: timeout,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if !RedirectAllowed(target, req.URL) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}, nil
}

func RedirectAllowed(target, redirectURL *url.URL) bool {
	if redirectURL == nil {
		return false
	}
	return SameTargetOrigin(target, redirectURL) && WithinTargetBasePath(target, redirectURL.Path)
}

func SameTargetOrigin(target, candidate *url.URL) bool {
	return strings.EqualFold(candidate.Scheme, target.Scheme) && strings.EqualFold(candidate.Host, target.Host)
}

func WithinTargetBasePath(target *url.URL, candidatePath string) bool {
	basePath := strings.TrimRight(target.Path, "/")
	if basePath == "" {
		return true
	}
	return candidatePath == basePath || strings.HasPrefix(candidatePath, basePath+"/")
}

func JoinTargetPath(basePath, requestPath string) string {
	if requestPath == "" {
		requestPath = "/"
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}

func HasParentPathSegment(pathValue string) bool {
	for _, segment := range strings.Split(pathValue, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func BoundedTargetURL(target *url.URL, requestPath string) (string, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "/"
	}
	if strings.HasPrefix(requestPath, "http://") || strings.HasPrefix(requestPath, "https://") || strings.HasPrefix(requestPath, "//") {
		return "", fmt.Errorf("client may only request relative paths")
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	relative, err := url.Parse(requestPath)
	if err != nil {
		return "", fmt.Errorf("invalid request path")
	}
	if relative.IsAbs() || relative.Host != "" || relative.Scheme != "" {
		return "", fmt.Errorf("client may only request relative paths")
	}

	out := *target
	out.Path = JoinTargetPath(target.Path, relative.Path)
	out.RawQuery = relative.RawQuery
	out.Fragment = ""

	if HasParentPathSegment(out.Path) || !WithinTargetBasePath(target, out.Path) {
		return "", fmt.Errorf("request path must not contain parent-directory segments")
	}
	return out.String(), nil
}

func ApplyRelayRequestHeaders(req *http.Request, relayReq lab.RelayRequest) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; snowflake-webrtc-proxy-lab/1.1)")
	req.Header.Set("Accept", AllowedRequestHeader(relayReq.Headers, "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"))
	req.Header.Set("Accept-Language", AllowedRequestHeader(relayReq.Headers, "Accept-Language", "en-US,en;q=0.9"))
	req.Header.Set("X-WebRTC-Proxy-Lab", "true")
	req.Header.Set("X-Proxy-Request-ID", relayReq.ID)
}

func AllowedRequestHeader(headers map[string]string, name, fallback string) string {
	if headers == nil {
		return fallback
	}
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			value = strings.TrimSpace(value)
			if value != "" && !strings.ContainsAny(value, "\r\n") {
				return value
			}
		}
	}
	return fallback
}

func SelectedResponseHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for _, name := range []string{"Cache-Control", "Content-Language", "Content-Type", "ETag", "Last-Modified", "Location"} {
		if value := headers.Get(name); value != "" {
			out[strings.ToLower(name)] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func RelayResponseBody(body []byte, contentType string) (string, string) {
	if ShouldTreatBodyAsText(body, contentType) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}

func ShouldTreatBodyAsText(body []byte, contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err == nil {
		mediaType = strings.ToLower(mediaType)
		if strings.HasPrefix(mediaType, "text/") {
			return true
		}
		switch mediaType {
		case "application/ecmascript", "application/javascript", "application/json", "application/manifest+json", "application/rss+xml", "application/x-javascript", "application/xhtml+xml", "application/xml", "image/svg+xml":
			return true
		}
	}
	return contentType == "" && utf8.Valid(body)
}

func ResponsePreview(body []byte, bodyTruncated bool, bodyFormat string) string {
	const previewLimit = 512
	if bodyFormat == "base64" {
		out := fmt.Sprintf("[binary response body: %d bytes]", len(body))
		if bodyTruncated {
			out += "...[body truncated]"
		}
		return out
	}

	preview := body
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}

	out := string(preview)
	if len(body) > previewLimit {
		out += "...[preview truncated]"
	}
	if bodyTruncated {
		out += "...[body truncated]"
	}
	return out
}

func ErrorResponse(id, message string) lab.RelayResponse {
	return lab.RelayResponse{
		Type:  lab.RelayResponseType,
		ID:    id,
		Error: message,
		Time:  time.Now().UTC().Format(time.RFC3339),
	}
}

func BuildRelayRequest(ctx context.Context, relayReq lab.RelayRequest, target *url.URL) (*http.Request, error) {
	method := strings.ToUpper(strings.TrimSpace(relayReq.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, fmt.Errorf("only GET and POST are allowed")
	}

	requestURL, err := BoundedTargetURL(target, relayReq.Path)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewBufferString(relayReq.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("build target request failed")
	}
	ApplyRelayRequestHeaders(req, relayReq)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "text/plain")
	}
	return req, nil
}

func BuildRelayResponse(relayReq lab.RelayRequest, resp *http.Response, maxBody int64) lab.RelayResponse {
	limit := maxBody
	if limit <= 0 {
		limit = 4096
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return ErrorResponse(relayReq.ID, "read target response failed")
	}

	truncated := int64(len(bodyBytes)) > limit
	if truncated {
		bodyBytes = bodyBytes[:limit]
	}

	responseBody, bodyFormat := RelayResponseBody(bodyBytes, resp.Header.Get("Content-Type"))
	preview := ResponsePreview(bodyBytes, truncated, bodyFormat)

	finalURL := resp.Request.URL.String()

	return lab.RelayResponse{
		Type:        lab.RelayResponseType,
		ID:          relayReq.ID,
		Status:      resp.StatusCode,
		Target:      finalURL,
		Bytes:       len(bodyBytes),
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     SelectedResponseHeaders(resp.Header),
		BodyFormat:  bodyFormat,
		Body:        responseBody,
		BodyPreview: preview,
		Truncated:   truncated,
		Time:        time.Now().UTC().Format(time.RFC3339),
	}
}

func SendRelayResponse(d lab.DataChannelWriter, response lab.RelayResponse) {
	if response.Body != "" {
		body := []byte(response.Body)
		if len(body) > ResponseChunkBytes {
			SendRelayResponseChunks(d, response, body)
			return
		}
	}

	payload, err := lab.MarshalJSON(response)
	if err != nil {
		log.Printf("marshal proxy response: %v", err)
		return
	}
	if err := d.Send(payload); err != nil {
		log.Printf("send proxy response: %v", err)
	}
}

func SendRelayResponseChunks(d lab.DataChannelWriter, response lab.RelayResponse, body []byte) {
	total := (len(body) + ResponseChunkBytes - 1) / ResponseChunkBytes
	if total == 0 {
		total = 1
	}

	log.Printf("sending proxy response id=%s in %d DataChannel chunks", response.ID, total)
	for i := 0; i < total; i++ {
		start := i * ResponseChunkBytes
		end := start + ResponseChunkBytes
		if end > len(body) {
			end = len(body)
		}

		chunk := response
		chunk.Type = lab.RelayResponseChunkType
		chunk.Body = ""
		chunk.BodyEncoding = "base64"
		chunk.BodyChunk = base64.StdEncoding.EncodeToString(body[start:end])
		chunk.ChunkIndex = i
		chunk.ChunkTotal = total

		payload, err := lab.MarshalJSON(chunk)
		if err != nil {
			log.Printf("marshal proxy response chunk id=%s chunk=%d/%d: %v", response.ID, i+1, total, err)
			return
		}
		if err := d.Send(payload); err != nil {
			log.Printf("send proxy response chunk id=%s chunk=%d/%d: %v", response.ID, i+1, total, err)
			return
		}
	}
}
