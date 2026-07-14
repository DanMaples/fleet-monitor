package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fleet-monitor/internal/device"
	"fleet-monitor/internal/telemetry"
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func newTestServer() http.Handler {
	s := telemetry.NewMemoryStore([]device.Device{{ID: "dev-1"}})
	return NewRouter(NewHandlers(s))
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostHeartbeat_UnknownDeviceReturns404(t *testing.T) {
	h := newTestServer()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/unknown/heartbeat", heartbeatRequest{})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPostHeartbeat_KnownDeviceReturns204(t *testing.T) {
	h := newTestServer()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/heartbeat", heartbeatRequest{SentAt: mustParseTime("2024-01-01T00:00:00Z")})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestPostStats_KnownDeviceReturns204(t *testing.T) {
	h := newTestServer()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/stats", statsRequest{
		SentAt:     mustParseTime("2024-01-01T00:00:00Z"),
		UploadTime: 1_000_000_000,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestGetStats_NoDataReturns204(t *testing.T) {
	h := newTestServer()
	rec := doRequest(t, h, http.MethodGet, "/api/v1/devices/dev-1/stats", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestGetStats_UnknownDeviceReturns404(t *testing.T) {
	h := newTestServer()
	rec := doRequest(t, h, http.MethodGet, "/api/v1/devices/unknown/stats", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestFullFlow_HeartbeatAndStatsThenGet(t *testing.T) {
	h := newTestServer()

	// Minutes 0, 1, 2, 4 (a gap at minute 3): 4 distinct heartbeat minutes
	// over a 4 minute span -> 100% uptime.
	base := "2024-01-01T00:0%d:00Z"
	for _, minute := range []int{0, 1, 2, 4} {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/heartbeat", heartbeatRequest{
			SentAt: mustParseTime(fmt.Sprintf(base, minute)),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("heartbeat %d status = %d, want 204", minute, rec.Code)
		}
	}

	for _, nanos := range []uint64{1_000_000_000, 3_000_000_000} {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/stats", statsRequest{
			SentAt:     mustParseTime("2024-01-01T00:00:00Z"),
			UploadTime: nanos,
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("stats status = %d, want 204", rec.Code)
		}
	}

	rec := doRequest(t, h, http.MethodGet, "/api/v1/devices/dev-1/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp statsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Uptime != 100 {
		t.Errorf("Uptime = %v, want 100", resp.Uptime)
	}
	if resp.AvgUploadTime != "2s" {
		t.Errorf("AvgUploadTime = %q, want %q", resp.AvgUploadTime, "2s")
	}
}
