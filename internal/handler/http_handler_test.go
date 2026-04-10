package handler

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDecodeMonitorInputFromForm(t *testing.T) {
	form := url.Values{}
	form.Set("name", "example")
	form.Set("url", "https://example.com")
	form.Set("interval_seconds", "30")
	form.Set("timeout_seconds", "5")
	form.Set("is_enabled", "on")

	r := httptest.NewRequest("POST", "/monitors", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	in, err := decodeMonitorInput(r)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !in.IsEnabled || in.IntervalSeconds != 30 || in.TimeoutSeconds != 5 {
		t.Fatalf("unexpected input: %+v", in)
	}
}

func TestDecodeMonitorInputInvalidNumber(t *testing.T) {
	form := url.Values{}
	form.Set("name", "example")
	form.Set("url", "https://example.com")
	form.Set("interval_seconds", "x")
	form.Set("timeout_seconds", "5")

	r := httptest.NewRequest("POST", "/monitors", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if _, err := decodeMonitorInput(r); err == nil {
		t.Fatal("expected error for invalid interval_seconds")
	}
}
