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

// ptr returns a pointer to a copy of v, for populating the pointer fields
// on heartbeatRequest/statsRequest from a literal or function result.
func ptr[T any](v T) *T {
	return &v
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

func TestRequests(t *testing.T) {
	tests := map[string]struct {
		method     string
		path       string
		body       any
		wantStatus int
		wantErr    error
	}{
		"POST heartbeat for unknown device returns 404": {
			method: http.MethodPost,
			path:   "/api/v1/devices/unknown/heartbeat",
			body: heartbeatRequest{
				SentAt: ptr(mustParseTime("2024-01-01T00:00:00Z")),
			},
			wantStatus: http.StatusNotFound,
		},
		// sent_at is a required field (see openapi.json);
		// omitting it must not silently default to the zero time.
		"POST heartbeat missing sent_at returns 400": {
			method:     http.MethodPost,
			path:       "/api/v1/devices/dev-1/heartbeat",
			body:       heartbeatRequest{},
			wantStatus: http.StatusBadRequest,
			wantErr:    errSentAtRequired,
		},
		"POST heartbeat for known device returns 204": {
			method: http.MethodPost,
			path:   "/api/v1/devices/dev-1/heartbeat",
			body: heartbeatRequest{
				SentAt: ptr(mustParseTime("2024-01-01T00:00:00Z")),
			},
			wantStatus: http.StatusNoContent,
		},
		"POST stats missing sent_at returns 400": {
			method: http.MethodPost,
			path:   "/api/v1/devices/dev-1/stats",
			body: statsRequest{
				UploadTime: ptr(uint64(1_000_000_000)),
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    errSentAtRequired,
		},
		// upload_time is a required field (see openapi.json);
		// omitting it must not silently default to 0.
		"POST stats missing upload_time returns 400": {
			method: http.MethodPost,
			path:   "/api/v1/devices/dev-1/stats",
			body: statsRequest{
				SentAt: ptr(mustParseTime("2024-01-01T00:00:00Z")),
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    errUploadTimeRequired,
		},
		// An explicit upload_time of 0 is a legitimate (if unusual)
		// measurement, not a missing field, so it must be accepted.
		"POST stats with upload_time of 0 is accepted": {
			method: http.MethodPost,
			path:   "/api/v1/devices/dev-1/stats",
			body: statsRequest{
				SentAt:     ptr(mustParseTime("2024-01-01T00:00:00Z")),
				UploadTime: ptr(uint64(0)),
			},
			wantStatus: http.StatusNoContent,
		},
		"POST stats for known device returns 204": {
			method: http.MethodPost,
			path:   "/api/v1/devices/dev-1/stats",
			body: statsRequest{
				SentAt:     ptr(mustParseTime("2024-01-01T00:00:00Z")),
				UploadTime: ptr(uint64(1_000_000_000)),
			},
			wantStatus: http.StatusNoContent,
		},
		"GET stats with no data yet returns 204": {
			method:     http.MethodGet,
			path:       "/api/v1/devices/dev-1/stats",
			wantStatus: http.StatusNoContent,
		},
		"GET stats for unknown device returns 404": {
			method:     http.MethodGet,
			path:       "/api/v1/devices/unknown/stats",
			wantStatus: http.StatusNotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := newTestServer()
			rec := doRequest(t, h, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantErr != nil {
				var resp errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal error response: %v", err)
				}
				if resp.Msg != tt.wantErr.Error() {
					t.Errorf("msg = %q, want %q", resp.Msg, tt.wantErr.Error())
				}
			}
		})
	}
}

func TestFullFlow_HeartbeatAndStatsThenGet(t *testing.T) {
	h := newTestServer()

	// Minutes 0, 1, 2, 4 (a gap at minute 3): 4 distinct heartbeat minutes
	// over a 4 minute span -> 100% uptime.
	base := "2024-01-01T00:0%d:00Z"
	for _, minute := range []int{0, 1, 2, 4} {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/heartbeat", heartbeatRequest{
			SentAt: ptr(mustParseTime(fmt.Sprintf(base, minute))),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("heartbeat %d status = %d, want 204", minute, rec.Code)
		}
	}

	for _, nanos := range []uint64{1_000_000_000, 3_000_000_000} {
		rec := doRequest(t, h, http.MethodPost, "/api/v1/devices/dev-1/stats", statsRequest{
			SentAt:     ptr(mustParseTime("2024-01-01T00:00:00Z")),
			UploadTime: ptr(nanos),
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
