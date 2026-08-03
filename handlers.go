package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

// handler is a container for handlers that close over the global Session.
type handler struct {
	session *Session
}

// handleInput reads raw bytes from the request body and writes them to the PTY.
//
// POST /input
func (h *handler) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if _, err := h.session.Write(body); err != nil {
		http.Error(w, "session closed", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleOutput long-polls the output ring buffer and returns new PTY data.
//
// GET /output?offset=<N>
func (h *handler) handleOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	offsetStr := r.URL.Query().Get("offset")
	var offset int64
	if offsetStr != "" {
		var err error
		offset, err = strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
	}

	data, newOffset, err := h.session.Output().ReadSince(r.Context(), offset, 25*time.Second)
	if err != nil {
		// Context cancelled = client disconnected; nothing to send.
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Offset", strconv.FormatInt(newOffset, 10))

	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleResize changes the PTY window dimensions.
//
// POST /resize
// Body: {"rows": N, "cols": N}
func (h *handler) handleResize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Rows int `json:"rows"`
		Cols int `json:"cols"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.session.Resize(req.Rows, req.Cols); err != nil {
		http.Error(w, "session closed", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
