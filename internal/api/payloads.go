package api

import (
	"errors"
	"time"
)

// errSentAtRequired is returned by validate when a request body omits
// the required "sent_at" field.
var errSentAtRequired = errors.New("sent_at is required")

// errUploadTimeRequired is returned by statsRequest.validate when a
// request body omits the required "upload_time" field.
var errUploadTimeRequired = errors.New("upload_time is required")

// heartbeatRequest is the body of POST /devices/{device_id}/heartbeat.
// SentAt is a pointer so that a request body omitting "sent_at" (nil)
// can be told apart from one that supplied it, during validate().
type heartbeatRequest struct {
	SentAt *time.Time `json:"sent_at"`
}

// validate reports which required fields, if any, are missing.
func (r heartbeatRequest) validate() error {
	if r.SentAt == nil {
		return errSentAtRequired
	}
	return nil
}

// statsRequest is the body of POST /devices/{device_id}/stats.
type statsRequest struct {
	SentAt     *time.Time `json:"sent_at"`
	UploadTime *uint64    `json:"upload_time"` // nanoseconds
}

// validate reports which required fields, if any, are missing.
func (r statsRequest) validate() error {
	if r.SentAt == nil {
		return errSentAtRequired
	}
	if r.UploadTime == nil {
		return errUploadTimeRequired
	}
	return nil
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
