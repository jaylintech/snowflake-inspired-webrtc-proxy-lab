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

func TestParseTargetRejectsParentPathSegment(t *testing.T) {
	if _, err := parseTarget("https://example.com/lab/../admin"); err == nil {
		t.Fatal("expected parent-directory target path to be rejected")
	}
}
