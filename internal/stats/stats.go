// Package stats contains the pure math used to turn raw heartbeat and
// upload-time observations into the reporting metrics the API exposes. It
// has no knowledge of HTTP or storage so it can be tested in isolation.
package stats

import "time"

// Snapshot is the computed, reportable uptime and average-upload-time
// metrics for a single device.
type Snapshot struct {
	Uptime        float64
	AvgUploadTime time.Duration
}

// Uptime implements the challenge's uptime formula:
//
//	uptime = (sumHeartbeats / numMinutesBetweenFirstAndLastHeartbeat) * 100
//
// heartbeatMinuteCount ("sumHeartbeats") is the number of distinct minutes
// that had at least one heartbeat (not the minutes themselves, just how
// many there were). first and last are the minute boundaries of the
// earliest and latest heartbeat received. numMinutesBetweenFirstAndLast is
// the plain elapsed-minute distance between them (last - first), which
// does not itself count either endpoint as a "slot" — only
// heartbeatMinuteCount does. A device with a single heartbeat (first ==
// last) is treated as having been up the entire time it was observed,
// i.e. 100%.
func Uptime(heartbeatMinuteCount uint, first time.Time, last time.Time) float64 {
	if heartbeatMinuteCount == 0 {
		return 0
	}

	numMinutes := max(int(last.Sub(first).Minutes()), 1)

	return float64(heartbeatMinuteCount) / float64(numMinutes) * 100
}

// AverageUploadTime returns the mean of a set of upload-time observations,
// expressed as the sum of durations (in nanoseconds) and the count of
// observations that made up that sum. It returns 0 if count is 0.
func AverageUploadTime(sumNanos uint64, count uint64) time.Duration {
	if count == 0 {
		return 0
	}
	return time.Duration(sumNanos / count)
}
