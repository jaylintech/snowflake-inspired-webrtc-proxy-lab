package lab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func PutSignal(ctx context.Context, brokerURL, sessionID, kind string, payload Signal) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s signal: %w", kind, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, signalURL(brokerURL, sessionID, kind), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", kind, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post %s signal: %w", kind, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("post %s signal: broker returned %s", kind, resp.Status)
	}

	return nil
}

func GetSignal(ctx context.Context, brokerURL, sessionID, kind string) (Signal, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signalURL(brokerURL, sessionID, kind), nil)
	if err != nil {
		return Signal{}, false, fmt.Errorf("build %s request: %w", kind, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Signal{}, false, fmt.Errorf("get %s signal: %w", kind, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Signal{}, false, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Signal{}, false, fmt.Errorf("get %s signal: broker returned %s", kind, resp.Status)
	}

	var payload Signal
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Signal{}, false, fmt.Errorf("decode %s signal: %w", kind, err)
	}

	return payload, true, nil
}

func PollSignal(ctx context.Context, brokerURL, sessionID, kind string, interval time.Duration) (Signal, error) {
	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		payload, ok, err := GetSignal(ctx, brokerURL, sessionID, kind)
		if err != nil {
			return Signal{}, err
		}
		if ok {
			return payload, nil
		}

		select {
		case <-ctx.Done():
			return Signal{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func signalURL(brokerURL, sessionID, kind string) string {
	return fmt.Sprintf(
		"%s/sessions/%s/%s",
		strings.TrimRight(brokerURL, "/"),
		url.PathEscape(sessionID),
		url.PathEscape(kind),
	)
}
