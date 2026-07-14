// Package telemetry holds the in-memory, concurrency-safe state for each
// device's heartbeats and upload-time samples, and turns them into the
// metrics defined in the stats package.
package telemetry

import (
	"errors"
	"sync"
	"time"

	"fleet-monitor/internal/device"
	"fleet-monitor/internal/stats"
)

// ErrDeviceNotFound is returned when an operation references a device_id
// that wasn't present in the devices.csv roster the Store was built from.
var ErrDeviceNotFound = errors.New("device not found")

// ErrNoData is returned by Stats when a known device has reported neither
// a heartbeat nor an upload-time sample. A device that has reported only
// one of the two does not trigger ErrNoData; see Store.Stats.
var ErrNoData = errors.New("no data recorded for device")

// Store is what the rest of the program depends on to record and query
// fleet telemetry. New returns an in-memory, concurrency-safe
// implementation, but callers should code against this interface so
// other implementations (e.g. backed by a database) can be substituted
// without changes elsewhere.
type Store interface {
	// RecordHeartbeat registers that deviceID was alive at sentAt.
	RecordHeartbeat(deviceID string, sentAt time.Time) error

	// RecordUploadStat registers an upload-time sample for deviceID.
	RecordUploadStat(deviceID string, sentAt time.Time, uploadTimeNanos uint64) error

	// Stats computes the current uptime and average upload time for
	// deviceID. It returns ErrNoData only if deviceID has reported
	// neither a heartbeat nor an upload-time sample. A device that has
	// reported only one of the two still returns a Snapshot
	// successfully, with the missing metric reported as its zero value:
	// Uptime 0 for a device with upload samples but no heartbeats, or
	// AvgUploadTime 0 for a device with heartbeats but no upload
	// samples. That zero is indistinguishable from a genuine
	// zero-valued measurement (e.g. a real 0s average upload time).
	// This is a deliberate simplification of the API contract (which
	// has no "field not present" representation), not a bug. See the
	// partial-data cases in TestStats (telemetry_test.go), which assert
	// this behavior explicitly.
	Stats(deviceID string) (stats.Snapshot, error)
}

// record accumulates the raw observations for a single device. Heartbeats
// are kept as a set of distinct minute boundaries (to dedupe multiple
// heartbeats landing in the same minute and to know the first/last minute
// seen); upload times are folded into a running sum/count rather than
// stored individually, since only their mean is ever reported.
type record struct {
	mu sync.RWMutex

	heartbeatMinutes map[int64]struct{} // unix-minute -> present
	hasHeartbeat     bool
	firstMinute      int64
	lastMinute       int64

	uploadCount    uint64
	uploadSumNanos uint64
}

// heartbeatWindow returns the minute boundaries of the earliest and latest
// heartbeat recorded for this device. ok is false if no heartbeat has ever
// been recorded, in which case first and last are meaningless: firstMinute
// and lastMinute are 0-valued in that state, which time.Unix would
// otherwise silently read back as the Unix epoch (1970-01-01) rather than
// "no data yet". Reading firstMinute/lastMinute directly instead of
// through this accessor risks introducing that bug. Callers must hold
// r.mu (for reading) before calling this.
func (r *record) heartbeatWindow() (first, last time.Time, ok bool) {
	if !r.hasHeartbeat {
		return time.Time{}, time.Time{}, false
	}
	return time.Unix(r.firstMinute, 0).UTC(), time.Unix(r.lastMinute, 0).UTC(), true
}

// memoryStore is the concurrency-safe, in-memory implementation of Store.
// The set of known devices is fixed at construction time from
// devices.csv; only the per-device counters mutate at runtime, so no
// lock is needed around the top-level map.
type memoryStore struct {
	records map[string]*record
}

// NewMemoryStore builds a Store pre-populated with the given devices. Heartbeats or
// stats posted for a device_id outside this set are rejected with
// ErrDeviceNotFound.
func NewMemoryStore(devices []device.Device) Store {
	records := make(map[string]*record, len(devices))
	for _, device := range devices {
		records[device.ID] = &record{heartbeatMinutes: make(map[int64]struct{})}
	}
	return &memoryStore{records: records}
}

func (s *memoryStore) RecordHeartbeat(deviceID string, sentAt time.Time) error {
	r, exists := s.records[deviceID]
	if !exists {
		return ErrDeviceNotFound
	}

	minute := sentAt.UTC().Truncate(time.Minute).Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.heartbeatMinutes[minute] = struct{}{}
	if !r.hasHeartbeat || minute < r.firstMinute {
		r.firstMinute = minute
	}
	if !r.hasHeartbeat || minute > r.lastMinute {
		r.lastMinute = minute
	}
	r.hasHeartbeat = true

	return nil
}

func (s *memoryStore) RecordUploadStat(deviceID string, sentAt time.Time, uploadTimeNanos uint64) error {
	r, exists := s.records[deviceID]
	if !exists {
		return ErrDeviceNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.uploadCount++
	r.uploadSumNanos += uploadTimeNanos

	return nil
}

func (s *memoryStore) Stats(deviceID string) (stats.Snapshot, error) {
	r, exists := s.records[deviceID]
	if !exists {
		return stats.Snapshot{}, ErrDeviceNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.hasHeartbeat && r.uploadCount == 0 {
		return stats.Snapshot{}, ErrNoData
	}

	var uptime float64
	if first, last, ok := r.heartbeatWindow(); ok {
		uptime = stats.Uptime(uint(len(r.heartbeatMinutes)), first, last)
	}

	return stats.Snapshot{
		Uptime:        uptime,
		AvgUploadTime: stats.AverageUploadTime(r.uploadSumNanos, r.uploadCount),
	}, nil
}
