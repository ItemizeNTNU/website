package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// message is the single-field envelope the previous API used for every status
// and error. Clients key off it, so the shape is preserved.
type message struct {
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already sent, so there is nothing to salvage —
		// but a truncated response should not pass unnoticed.
		slog.Error("writing JSON response failed", "err", err)
	}
}
