package main

import (
	"net/url"
	"testing"
)

func mustTarget(t *testing.T, raw string) *url.URL {
	t.Helper()

	target, err := parseTarget(raw)
	if err != nil {
		t.Fatalf("parse target %q: %v", raw, err)
	}
	return target
}

func TestBoundedTargetURLUsesTargetBasePath(t *testing.T) {
	target := mustTarget(t, "https://example.com/lab")

	got, err := boundedTargetURL(target, "/robots.txt?via=webrtc")
	if err != nil {
		t.Fatalf("bounded target URL: %v", err)
	}

	want := "https://example.com/lab/robots.txt?via=webrtc"
	if got != want {
		t.Fatalf("bounded target URL = %q, want %q", got, want)
	}
}

func TestBoundedTargetURLRejectsAbsoluteURL(t *testing.T) {
	target := mustTarget(t, "https://example.com")

	if _, err := boundedTargetURL(target, "https://other.example/"); err == nil {
		t.Fatal("expected absolute request URL to be rejected")
	}
}

func TestBoundedTargetURLRejectsParentPathSegment(t *testing.T) {
	target := mustTarget(t, "https://example.com/lab")

	if _, err := boundedTargetURL(target, "/../admin"); err == nil {
		t.Fatal("expected parent-directory request path to be rejected")
	}
}
func TestRedirectAllowedRequiresSameTargetBasePath(t *testing.T) {
	target := mustTarget(t, "https://example.com/lab")

	allowed, err := url.Parse("https://example.com/lab/next")
	if err != nil {
		t.Fatal(err)
	}
	if !redirectAllowed(target, allowed) {
		t.Fatal("expected same-origin redirect under target base path to be allowed")
	}

	outsidePath, err := url.Parse("https://example.com/admin")
	if err != nil {
		t.Fatal(err)
	}
	if redirectAllowed(target, outsidePath) {
		t.Fatal("expected same-origin redirect outside target base path to be blocked")
	}

	otherOrigin, err := url.Parse("https://other.example/lab/next")
	if err != nil {
		t.Fatal(err)
	}
	if redirectAllowed(target, otherOrigin) {
		t.Fatal("expected cross-origin redirect to be blocked")
	}
}

func TestRelayResponseBodyBase64EncodesBinary(t *testing.T) {
	body, format := relayResponseBody([]byte{0x00, 0x01, 0xff}, "image/png")
	if format != "base64" {
		t.Fatalf("format = %q, want base64", format)
	}
	if body != "AAH/" {
		t.Fatalf("body = %q, want AAH/", body)
	}
}

func TestParseTargetRejectsParentPathSegment(t *testing.T) {
	if _, err := parseTarget("https://example.com/lab/../admin"); err == nil {
		t.Fatal("expected parent-directory target path to be rejected")
	}
}
