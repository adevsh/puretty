package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIntegrationPTYPipeline(t *testing.T) {
	session, err := NewSession("/bin/sh")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	h := &handler{session: session}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/input":
			h.handleInput(w, r)
		case "/output":
			h.handleOutput(w, r)
		case "/resize":
			h.handleResize(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Poll 1: get initial output (shell prompt).
	offset := getOutput(t, ts.URL, 0)
	t.Logf("initial offset after prompt: %d", offset)

	// Send a command via /input.
	sendInput(t, ts.URL, "echo INTEGRATION_TEST_XYZ\n")

	// Poll until we see the expected output or timeout.
	deadline := time.After(5 * time.Second)
	var allOutput strings.Builder
	for {
		data, newOffset, err := pollOutput(ts.URL, offset, 2*time.Second)
		if err != nil {
			t.Fatalf("pollOutput: %v", err)
		}
		if data != nil {
			allOutput.Write(data)
			offset = newOffset
		}
		if strings.Contains(allOutput.String(), "INTEGRATION_TEST_XYZ") {
			t.Logf("found expected output: %q", allOutput.String())
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for INTEGRATION_TEST_XYZ in output: %q", allOutput.String())
		default:
		}
	}
}

func TestIntegrationResize(t *testing.T) {
	session, err := NewSession("/bin/sh")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	h := &handler{session: session}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/resize":
			h.handleResize(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Resize should succeed.
	body := bytes.NewReader([]byte(`{"rows":30,"cols":120}`))
	resp, err := http.Post(ts.URL+"/resize", "application/json", body)
	if err != nil {
		t.Fatalf("POST /resize: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Invalid JSON should get 400.
	body = bytes.NewReader([]byte(`not json`))
	resp, err = http.Post(ts.URL+"/resize", "application/json", body)
	if err != nil {
		t.Fatalf("POST /resize: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

func TestIntegrationSessionClosed(t *testing.T) {
	session, err := NewSession("/bin/sh")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	session.Close()

	h := &handler{session: session}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/input":
			h.handleInput(w, r)
		case "/resize":
			h.handleResize(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Input after close should get 503.
	resp, err := http.Post(ts.URL+"/input", "application/octet-stream", bytes.NewReader([]byte("test")))
	if err != nil {
		t.Fatalf("POST /input: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	// Resize after close should get 503.
	body := bytes.NewReader([]byte(`{"rows":30,"cols":120}`))
	resp, err = http.Post(ts.URL+"/resize", "application/json", body)
	if err != nil {
		t.Fatalf("POST /resize: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func getOutput(t *testing.T, baseURL string, offset int64) int64 {
	t.Helper()
	data, newOffset, err := pollOutput(baseURL, offset, 5*time.Second)
	if err != nil {
		t.Fatalf("getOutput: %v", err)
	}
	if data == nil {
		t.Fatal("expected initial output (shell prompt)")
	}
	return newOffset
}

func pollOutput(baseURL string, offset int64, timeout time.Duration) ([]byte, int64, error) {
	url := fmt.Sprintf("%s/output?offset=%d", baseURL, offset)
	client := &http.Client{Timeout: timeout + 1*time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, offset, err
	}
	defer resp.Body.Close()

	newOffsetStr := resp.Header.Get("X-Offset")
	if newOffsetStr == "" {
		return nil, offset, fmt.Errorf("missing X-Offset header")
	}
	newOffset, err := strconv.ParseInt(newOffsetStr, 10, 64)
	if err != nil {
		return nil, offset, fmt.Errorf("invalid X-Offset: %w", err)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, offset, err
	}

	if len(data) == 0 && newOffset == offset {
		return nil, offset, nil
	}
	return data, newOffset, nil
}

func sendInput(t *testing.T, baseURL string, data string) {
	t.Helper()
	resp, err := http.Post(baseURL+"/input", "application/octet-stream", strings.NewReader(data))
	if err != nil {
		t.Fatalf("sendInput: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sendInput: expected 204, got %d", resp.StatusCode)
	}
}
