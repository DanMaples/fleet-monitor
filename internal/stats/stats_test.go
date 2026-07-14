package stats

import (
	"testing"
	"time"
)

func TestUptime(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	type testCase struct {
		heartbeatMinuteCount uint
		first                time.Time
		last                 time.Time
		want                 float64
	}

	tests := map[string]testCase{
		"no heartbeats": {
			heartbeatMinuteCount: 0,
			first:                base,
			last:                 base,
			want:                 0,
		},
		"single heartbeat is 100%": {
			heartbeatMinuteCount: 1,
			first:                base,
			last:                 base,
			want:                 100,
		},
		"half of a nine minute span": {
			heartbeatMinuteCount: 5,
			first:                base,
			last:                 base.Add(9 * time.Minute),
			want:                 float64(5) / float64(9) * 100,
		},
		"one missed minute out of a three minute span": {
			heartbeatMinuteCount: 3,
			first:                base,
			last:                 base.Add(3 * time.Minute),
			want:                 100,
		},
		// The next two cases document a known, deliberately-accepted
		// limitation of the formula: because the denominator is the
		// non-inclusive elapsed span (last-first) rather than the true
		// number of one-minute slots (last-first+1), a device that
		// misses exactly one heartbeat reports exactly 100% uptime —
		// identical to a device that missed none.
		//
		// This is intentional, not a bug: the reference device
		// simulator's own expected values are computed the same,
		// non-inclusive way (see "Design notes" in README.md).
		// If either of these two cases starts
		// failing, that's a sign the formula changed, not that the
		// test is wrong.
		"zero heartbeats missed reports above 100% (formula's known bias)": {
			// Minutes 0, 1, 2, 3, 4 all present: 5 distinct
			// heartbeat-minutes over a 4 minute span. Uptime reports
			// *above* 100% because the denominator (last-first = 4)
			// is one less than the true number of one-minute slots
			// in [first, last] (5).
			heartbeatMinuteCount: 5,
			first:                base,
			last:                 base.Add(4 * time.Minute),
			want:                 125, // 5/4*100
		},
		"one heartbeat missed is indistinguishable from zero missed": {
			// Only minute 3 is missing out of 0..4: 4 distinct
			// heartbeat-minutes over the same 4 minute span as above.
			// This reports exactly 100% — the same as the zero-gap
			// case above would report if its last heartbeat had
			// simply landed one minute earlier. The single missed
			// heartbeat is silently absorbed by the formula's
			// off-by-one denominator.
			heartbeatMinuteCount: 4,
			first:                base,
			last:                 base.Add(4 * time.Minute),
			want:                 100,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Uptime(tc.heartbeatMinuteCount, tc.first, tc.last)
			if got != tc.want {
				t.Errorf("Uptime() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAverageUploadTime(t *testing.T) {
	type testCase struct {
		sumNanos uint64
		count    uint64
		want     time.Duration
	}
	tests := map[string]testCase{
		"no samples":    {sumNanos: 0, count: 0, want: 0},
		"single sample": {sumNanos: uint64(5 * time.Second), count: 1, want: 5 * time.Second},
		"average of three samples": {
			sumNanos: uint64(1*time.Second) + uint64(2*time.Second) + uint64(3*time.Second),
			count:    3,
			want:     2 * time.Second,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := AverageUploadTime(tc.sumNanos, tc.count)
			if got != tc.want {
				t.Errorf("AverageUploadTime() = %v, want %v", got, tc.want)
			}
		})
	}
}
