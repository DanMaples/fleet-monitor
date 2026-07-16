package api

import (
	"log/slog"
	"net/http"
	"time"
)

// apiPrefix matches the server base path declared in openapi.json
// (http://127.0.0.1:6733/api/v1).
const apiPrefix = "/api/v1"

// NewRouter wires up the fleet API's routes.
func NewRouter(h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST "+apiPrefix+"/devices/{device_id}/heartbeat", h.PostHeartbeat)
	mux.HandleFunc("POST "+apiPrefix+"/devices/{device_id}/stats", h.PostStats)
	mux.HandleFunc("GET "+apiPrefix+"/devices/{device_id}/stats", h.GetStats)

	return withLogging(mux)
}

// withLogging logs each request's method, path, status, and latency.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader records status before delegating, so withLogging can log the
// status code a handler actually sent.
func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}
