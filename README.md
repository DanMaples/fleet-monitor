# Fleet Monitoring API: SafelyYou Coding Challenge

An HTTP API that ingests per-device heartbeats and video upload-time
telemetry, and reports each device's uptime and average upload time, per
the contract in [`openapi.json`](openapi.json).

## Running it

Requires Go 1.22+ (uses the standard-library `net/http` method+path
routing added in 1.22).

```bash
go run ./cmd/server -devices devices.csv -port 6733
```

Flags:

| Flag        | Default        | Description                          |
|-------------|----------------|---------------------------------------|
| `-devices`  | `devices.csv`  | Path to the device roster CSV         |
| `-port`     | `6733`         | Port to listen on                     |

The server loads the device roster on startup, then serves the API under
`/api/v1` (matching the `servers` entry in `openapi.json`), e.g.
`http://127.0.0.1:6733/api/v1/devices/{device_id}/heartbeat`.

To exercise it against the reference device simulator:
(Development was done on a mac, so I used `device-simulator-mac-arm64`)

```bash
go run ./cmd/server -devices devices.csv -port 6733 &
./device-simulator-mac-arm64 -host 127.0.0.1 -port 6733
```

The simulator prints a comparison of expected vs. actual results to the
console and to `results.txt` (included in this repo from the most recent
run, in which all 5 devices matched exactly).

## Running the tests

```bash
go test ./... -race -cover
```

## Project layout

```
cmd/server/         entrypoint: flag parsing, wiring, graceful shutdown
internal/device/     devices.csv loading
internal/stats/      pure uptime / average-upload-time math (no I/O)
internal/telemetry/  in-memory, concurrency-safe per-device counters
internal/api/         HTTP handlers, routing, request/response DTOs
```

The split keeps the uptime/average formulas (`internal/stats`) testable
without any HTTP or concurrency concerns, and keeps the storage layer
(`internal/telemetry`, exposed as the `Store` interface) swappable. See
"extending the data model" below.

## Design notes

**Uptime formula.** The challenge defines:

```
uptime = (sumHeartbeats / numMinutesBetweenFirstAndLastHeartbeat) * 100
```

I initially implemented `numMinutesBetweenFirstAndLastHeartbeat` as an
*inclusive* count of one-minute slots between the first and last heartbeat
(i.e. `(last-first) + 1`). That produced values consistently slightly below
the simulator's expected output for every device. Switching to the
non-inclusive elapsed-minute distance between the two timestamps instead
(`last-first`, i.e. dropping the `+1`) matched the simulator's expected
values exactly for all five devices, so that's the implementation I
kept; it's documented in `internal/stats/stats.go`. Heartbeats are
bucketed by truncating `sent_at` to the minute in UTC, deduplicating
multiple heartbeats landing in the same minute.

**Known limitation: a single missed heartbeat is invisible.** Because the
denominator is the non-inclusive `last-first` rather than the true number
of one-minute slots between first and last (`last-first + 1`), a device
that misses *exactly one* heartbeat reports exactly 100% uptime,
indistinguishable from a device that missed none. (Two or more missed
heartbeats are still visible: the percentage drops below 100%, just not
by quite as much as an inclusive formula would show.) I'm aware of this
gap and chose to leave the formula as-is in order to match the simulator 
output, rather than "fix" it. I would not use this formula in production
code because of this blind spot. This decision is captured directly in
`internal/stats/stats_test.go`,
which asserts the blind spot is expected, intentional behavior so a
future change to the formula can't silently alter it. 

**Storage.** Rather than storing every individual heartbeat timestamp and
upload-time sample, each device tracks:
- a set of distinct minute-buckets that received a heartbeat (needed to
  dedupe and to derive first/last), and
- a running `(sum, count)` for upload times, since only the mean is ever
  reported.

This keeps memory bounded by the number of distinct heartbeat-minutes
rather than the number of requests, and makes both writes and the stats
read O(1) (aside from the heartbeat map, whose size is bounded by the
device's observed lifetime in minutes, not by request volume).

**Concurrency.** Each device has its own mutex, so heartbeats/stats for
different devices never contend. The top-level device map is populated
once at startup from `devices.csv` and never mutated afterward, so it
needs no lock.

**HTTP status codes.** `404` for an unknown `device_id` (not in
`devices.csv`), `204` from `GET .../stats` if the device is known but has
no data yet, `200` with the JSON body once there's at least one heartbeat
or upload-time sample, and `400` for a malformed request body (not
specified in `openapi.json`, but reasonable, since the simulator never
triggers it).

## Write-up

### How long did you spend, and what was the hardest part?

Implementing the straightforward version of the API (routing, storage,
CSV loading, the calculations) took under an hour. Almost all the
remaining time went into one thing: getting the uptime calculation to
match the simulator's expected values exactly. The formula as written in
the PDF is ambiguous about whether the "minutes between first and last
heartbeat" is an inclusive span or not, and the intuitive (inclusive)
reading was off by a small, consistent margin against the simulator's
output. I resolved it by trying the literal, non-inclusive reading
instead and rerunning the simulator, which matched its expected values
exactly across all five devices. This makes the discrepancy exact and
unambiguous rather than a guessing game, however it does leave a blind
spot, which I documented.

### How would you modify the data model to support more kinds of metrics?

Right now `internal/stats` has one pure function per metric, and
`internal/telemetry`'s `record` struct has one set of fields per metric
(heartbeat minute-set; upload-time sum/count). Adding a new metric (say,
error-rate or CPU temperature) means adding a new field (or small
sub-struct) to `record`, a new pure calculation function in
`internal/stats`, a new `Record*` method on `Store`, a new field on the
`Stats` struct it returns, a new ingestion endpoint in `internal/api`, and
a new field on `statsResponse`. That's mechanical but touches five files,
which is the natural growing pain of a hand-rolled, per-metric struct.

If the metric surface were going to grow past a handful of types, I'd
restructure around a generic `Report{DeviceID, Kind, SentAt, Value}`
ingestion path and a `map[Kind]Aggregator` in the store, where an
`Aggregator` is an interface (`Add(value)`, `Result() any`) implemented
per metric kind (running-mean, minute-set uptime, max, percentile, ...).
That turns "add a metric" into "register a new `Aggregator` and payload
type" instead of touching the store's core struct, at the cost of losing
some type safety in the ingestion payload (would want a `json.RawMessage`
+ per-kind decode, or generics keyed by kind). I didn't do this upfront
because with two metrics it would be premature abstraction: three
concrete fields is clearer to read than a generic aggregator registry.

If metrics needed to be queried by time range, compared across devices, or
persisted across restarts, I'd also replace the in-memory store with a
real time-series-shaped store (e.g., a proper TSDB or a Postgres table
with device_id/metric/timestamp/value, aggregated in SQL or in a
read-time rollup). See "deployment" below.

### Runtime complexity

- `POST .../heartbeat`: O(1) amortized. One map insert (device is a map
  lookup; the minute is inserted into a `map[int64]struct{}`) plus at most
  two comparisons to update first/last.
- `POST .../stats`: O(1). Increments a running sum and count.
- `GET .../stats`: O(1). `len()` on a Go map is O(1), and both first/last
  minute and the upload sum/count are already tracked incrementally, so no
  scan is needed at read time.
- Memory: O(D + sum of M_d) where D is the number of devices and M_d is
  the number of *distinct* heartbeat-minutes observed for device d (not
  the number of requests: duplicate/retried heartbeats in the same
  minute don't grow it). Upload-time storage is O(D) regardless of request
  volume, since only a running sum/count is kept, not the individual
  samples.
- Startup: O(N) to parse an N-row `devices.csv`.

### AI tool usage

This solution was built with the assistance of Claude Code. I used
it to help outline and write both the production code and the unit tests.

### Security, testing, and deployment (if this were going to production)

- **Security**: the API currently trusts any caller to report telemetry
  for any known `device_id` with no authentication. In production I'd put
  per-device credentials (mTLS client certs or signed tokens minted at
  device-provisioning time) in front of it, rate-limit per device, and
  validate `sent_at` isn't wildly in the future/past to prevent a
  compromised or misconfigured device from skewing another device's stats
  (impossible today anyway, since `sent_at` doesn't carry device identity
  beyond the path param, but worth bounding regardless).
- **Testing**: unit tests cover the pure formulas (`internal/stats`), the
  concurrent store (including a `-race` test), CSV loading, and the HTTP
  handlers end-to-end via `httptest`. For production I'd add a black-box
  contract test that runs the real device-simulator binary against a
  freshly started server in CI, so a regression in the formula (like the
  one this write-up describes) is caught automatically rather than by
  hand.
- **Deployment**: this is intentionally an in-memory, single-process
  service: fine for the challenge, not for production, since all state
  is lost on restart and it can't be horizontally scaled (two replicas
  would each see only half of a device's heartbeats). For a real alpha
  prototype I'd put a durable, shared store behind the same `Store`
  interface (e.g. Postgres with a `heartbeats(device_id, minute)` unique
  constraint and an `upload_stats(device_id, sent_at, upload_time)` table,
  or a time-series DB), run the API as a stateless container behind a load
  balancer so it can scale horizontally, and add `/healthz`, structured
  logs, and metrics (Prometheus) for operability. Rough shape:

  ```
  devices ──HTTP──▶ [load balancer] ──▶ [API replicas] ──▶ [Postgres/TSDB]
                                                │
                                                └──▶ [Prometheus/metrics]
  ```
