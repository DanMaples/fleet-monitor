// Package api implements the HTTP contract described in openapi.json:
// recording device heartbeats and upload-time stats, and reporting
// computed per-device metrics back out.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"fleet-monitor/internal/telemetry"
)

// Handlers serves the fleet API's HTTP endpoints.
type Handlers interface {
	// PostHeartbeat handles POST /devices/{device_id}/heartbeat.
	PostHeartbeat(w http.ResponseWriter, r *http.Request)

	// PostStats handles POST /devices/{device_id}/stats.
	PostStats(w http.ResponseWriter, r *http.Request)

	// GetStats handles GET /devices/{device_id}/stats.
	GetStats(w http.ResponseWriter, r *http.Request)
}

// handlers is the concrete, telemetry-store-backed implementation of
// Handlers.
type handlers struct {
	store telemetry.Store
}

// NewHandlers builds Handlers backed by the given store.
func NewHandlers(s telemetry.Store) Handlers {
	return &handlers{store: s}
}

func (h *handlers) PostHeartbeat(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")

	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.RecordHeartbeat(deviceID, req.SentAt); err != nil {
		writeTelemetryError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) PostStats(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")

	var req statsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.RecordUploadStat(deviceID, req.SentAt, req.UploadTime); err != nil {
		writeTelemetryError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")

	snapshot, err := h.store.Stats(deviceID)
	if err != nil {
		if errors.Is(err, telemetry.ErrNoData) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeTelemetryError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, statsResponse{
		Uptime:        snapshot.Uptime,
		AvgUploadTime: snapshot.AvgUploadTime.String(),
	})
}

func writeTelemetryError(w http.ResponseWriter, err error) {
	if errors.Is(err, telemetry.ErrDeviceNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Msg: msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("failed to encode response body: %v", err)
	}
}
