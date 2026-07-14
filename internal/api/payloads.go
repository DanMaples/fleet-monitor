package api

import "time"

// heartbeatRequest is the body of POST /devices/{device_id}/heartbeat.
type heartbeatRequest struct {
	SentAt time.Time `json:"sent_at"`
}

// statsRequest is the body of POST /devices/{device_id}/stats.
type statsRequest struct {
	SentAt     time.Time `json:"sent_at"`
	UploadTime uint64    `json:"upload_time"` // nanoseconds
}

// statsResponse is the body of GET /devices/{device_id}/stats.
type statsResponse struct {
	Uptime        float64 `json:"uptime"`
	AvgUploadTime string  `json:"avg_upload_time"`
}

// errorResponse is the body returned for 4xx/5xx errors.
type errorResponse struct {
	Msg string `json:"msg"`
}
