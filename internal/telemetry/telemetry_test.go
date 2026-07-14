package telemetry

import (
	"errors"
	"sync"
	"testing"
	"time"

	"fleet-monitor/internal/device"
)

func newTestStore() Store {
	return NewMemoryStore([]device.Device{{ID: "dev-1"}, {ID: "dev-2"}})
}

func TestUnknownDevice(t *testing.T) {
	tests := map[string]struct {
		op func(s Store) error
	}{
		"RecordHeartbeat": {
			op: func(s Store) error { return s.RecordHeartbeat("unknown", time.Now()) },
		},
		"RecordUploadStat": {
			op: func(s Store) error { return s.RecordUploadStat("unknown", time.Now(), 100) },
		},
		"Stats": {
			op: func(s Store) error { _, err := s.Stats("unknown"); return err },
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newTestStore()
			if err := tt.op(s); !errors.Is(err, ErrDeviceNotFound) {
				t.Fatalf("got %v, want ErrDeviceNotFound", err)
			}
		})
	}
}

func TestStats(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		// heartbeatOffsets are added to base and recorded as heartbeats.
		heartbeatOffsets []time.Duration
		// uploadTimes are each recorded as an upload-time sample at base.
		uploadTimes []time.Duration

		wantErr       error
		wantUptime    float64
		wantAvgUpload time.Duration
	}{
		"no data yet": {
			wantErr: ErrNoData,
		},
		// Heartbeats at minutes 0, 1, 2, 4 (a gap at minute 3): 4 distinct
		// minutes over a 4 minute span between first and last -> 100%,
		// matching the literal (non-inclusive) reading of the challenge's
		// uptime formula. Upload times of 1s, 2s, 3s average to 2s.
		"full uptime and average": {
			heartbeatOffsets: []time.Duration{
				0, 1 * time.Minute, 2 * time.Minute, 4 * time.Minute,
			},
			uploadTimes:   []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second},
			wantUptime:    100,
			wantAvgUpload: 2 * time.Second,
		},
		// Two heartbeats land in minute 0 (should collapse to one), one
		// lands in minute 1, and one lands in minute 3 (a gap at minute
		// 2). Distinct minutes = {0, 1, 3} = 3, span = 3 -> 100%. If the
		// duplicate in minute 0 were double-counted, this would
		// incorrectly read 133%.
		"duplicate heartbeats in same minute don't double count": {
			heartbeatOffsets: []time.Duration{
				0, 30 * time.Second, 1 * time.Minute, 3 * time.Minute,
			},
			wantUptime: 100,
		},
		// Heartbeats at minute 0 and minute 4 only: a 4 minute span
		// between first and last, with only 2 distinct heartbeat minutes
		// -> 50%.
		"partial uptime": {
			heartbeatOffsets: []time.Duration{0, 4 * time.Minute},
			wantUptime:       50,
		},
		// The next two cases document a deliberate simplification of the
		// API contract (see Store.Stats): ErrNoData only fires when a
		// device has reported neither heartbeats nor upload stats at
		// all. A device that's missing just one of the two still gets a
		// successful Snapshot back, with that metric reported as 0,
		// indistinguishable from a genuine zero measurement. If these
		// cases start returning ErrNoData, or the zero value changes,
		// that's a sign Stats's error/zero-value behavior changed, not
		// that the tests are wrong.
		"heartbeats only, no upload stats, is not an error": {
			// A single heartbeat is 100% uptime (see stats.Uptime); no
			// upload samples means AvgUploadTime reports its zero value,
			// 0s, rather than ErrNoData.
			heartbeatOffsets: []time.Duration{0},
			wantUptime:       100,
			wantAvgUpload:    0,
		},
		"upload stats only, no heartbeats, is not an error": {
			// No heartbeats means Uptime reports its zero value, 0,
			// rather than ErrNoData.
			uploadTimes:   []time.Duration{2 * time.Second},
			wantUptime:    0,
			wantAvgUpload: 2 * time.Second,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newTestStore()

			for _, offset := range tt.heartbeatOffsets {
				if err := s.RecordHeartbeat("dev-1", base.Add(offset)); err != nil {
					t.Fatalf("RecordHeartbeat: %v", err)
				}
			}
			for _, d := range tt.uploadTimes {
				if err := s.RecordUploadStat("dev-1", base, uint64(d)); err != nil {
					t.Fatalf("RecordUploadStat: %v", err)
				}
			}

			got, err := s.Stats("dev-1")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Stats() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Stats: %v", err)
			}
			if got.Uptime != tt.wantUptime {
				t.Errorf("Uptime = %v, want %v", got.Uptime, tt.wantUptime)
			}
			if got.AvgUploadTime != tt.wantAvgUpload {
				t.Errorf("AvgUploadTime = %v, want %v", got.AvgUploadTime, tt.wantAvgUpload)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := newTestStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = s.RecordHeartbeat("dev-1", base.Add(time.Duration(i)*time.Minute))
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = s.RecordUploadStat("dev-1", base, uint64(i))
		}(i)
	}
	wg.Wait()

	got, err := s.Stats("dev-1")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// 100 distinct heartbeat minutes (0..99) over a 99 minute span. This
	// mainly exists to be run with -race: a lost update would report a
	// count below 100 and a lower uptime.
	want := float64(100) / float64(99) * 100
	if got.Uptime != want {
		t.Errorf("Uptime = %v, want %v", got.Uptime, want)
	}
}
