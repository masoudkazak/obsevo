# Performance benchmark

The question this benchmark answers is narrow and practical: **can a small team
run this on a small VPS?** It measures ingestion latency, how far the async
worker falls behind, and what the data costs in CPU, memory and disk.

Run it yourself with `make benchmark` (see [Reproducing](#reproducing)).

## Results

Measured 2026-08-30 against the commit that introduced this file.

Each trace carries 4 observations, so the event rate is 5× the trace rate.
Events are sent in batches of 20, which is what the SDKs do.

| Scenario | Events/s | Ingest p50 | Ingest p95 | Ingest p99 | Read p95 | Worker drain | Peak queue | Postgres growth | Failures |
|---|---|---|---|---|---|---|---|---|---|
| 100 traces/min | 8 | 1.6 ms | 1.8 ms | 1.8 ms | 2.9 ms | 0.2 s | 0 | 0.38 MB | 0 |
| 1,000 traces/min | 83 | 1.5 ms | 2.0 ms | 2.1 ms | 1.6 ms | 0.2 s | 0 | 2.31 MB | 0 |
| 10,000 traces/min | 833 | 1.2 ms | 1.7 ms | 2.0 ms | 2.0 ms | 0.2 s | 0 | 22.78 MB | 0 |
| 60,000 traces/min | 4,883 | 1.0 ms | 1.5 ms | 1.7 ms | 9.4 ms | 1.4 s | 15,884 | 120.56 MB | 0 |

"Worker drain" is how long after the last request the queue took to empty — the
lag between an event being accepted and being queryable. A drain of 0.2 s is the
measurement floor (the sampler's poll interval), meaning the worker never fell
behind at all.

### Resource footprint

After ingesting ~177,000 events (35,550 traces, 142,188 observations, 155 MB of
database):

| Component | Memory | Notes |
|---|---|---|
| API + worker (single Go binary) | **26 MB RSS** | 26 OS threads |
| PostgreSQL 16 | 186 MB | container limited to 256 MB |
| Redis 7 | 5 MB | container limited to 128 MB |
| **Total** | **~217 MB** | |

Storage cost is roughly **1.1 KB per event** with realistic input/output
payloads, so 1M events is about 1.1 GB before indexes settle.

### What this means for sizing

The brief's target is 2 CPU / 2–4 GB RAM. At the top scenario the brief asks
for — 10,000 traces/min — the system runs with the queue permanently empty and
sub-2 ms ingestion latency, using about 220 MB. **The target is met with a wide
margin**; the practical limits are disk for retention and Postgres shared
buffers, not CPU or the application.

The 60,000 traces/min row is included to show where the shape changes: ingestion
latency stays flat (the API only validates and enqueues), but the queue starts
to build because writes are now the constraint. It still drains in 1.4 s, so
even 6× the brief's top scenario is served, just no longer instantly.

## The bottleneck this benchmark found, and the fix

The first run of this benchmark exposed a real ceiling. At 10,000 traces/min the
API accepted everything at p95 = 1.4 ms, but:

```
worker drain   9.80s after the last request (peak queue 5838)
```

The worker was a single sequential loop: pop one event, write it, repeat. Each
event costs a database round trip, which caps a serial writer at a few hundred
events per second. At 833 events/s the queue grew for the whole run and took ten
seconds to clear — meaning a trace could be accepted but not visible in the UI
for ten seconds.

The fix was to shard the worker. One goroutine reads Redis and routes each event
to one of N shards **by trace id**, so every event for a given trace is still
handled in arrival order — a span's `create` and its later `update` cannot be
reordered — while unrelated traces are written concurrently. N defaults to
`NumCPU - 1`, clamped to 2–8, and is overridable with `WORKER_CONCURRENCY`.

After the change, the same scenario reports:

```
worker drain   0.20s after the last request (peak queue 0)
```

The queue never builds at all, and the ceiling moved out by roughly 6×.

A second issue surfaced at 60,000 traces/min: 4,831 requests were rejected with
429. The rate limiter was enforcing per-scope defaults even though
`RATE_LIMIT_PER_MINUTE` was 0, which is documented as "disabled". Rate limiting
is now genuinely opt-in; the per-scope numbers only apply once a global limit is
configured.

## Reproducing

Bring up the stack, create a project and an API key, then run the load
generator:

```bash
docker compose up -d postgres redis
make run &                              # or: docker compose up -d api

# Register, then create an API key in Settings → API Keys, or via the API.
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
export DATABASE_URL='postgres://obsevo:obsevo@localhost:5432/obsevo?sslmode=disable'
export REDIS_URL='redis://localhost:6379'

make benchmark                          # runs the three scenarios above
```

Or drive one scenario directly:

```bash
go run ./cmd/benchmark \
  -host http://localhost:3001 \
  -rate 10000 -duration 30s -observations 4 \
  -stats-container langfuse-api \
  -json
```

Flags worth knowing:

| Flag | Meaning |
|---|---|
| `-rate` | traces per minute to sustain |
| `-duration` | how long to sustain it |
| `-observations` | observations per trace (half are generations) |
| `-batch` | events per ingestion request |
| `-concurrency` | concurrent ingestion requests |
| `-database-url` | enables Postgres size and row-count sampling |
| `-redis-url` | enables queue-depth sampling and the drain measurement |
| `-stats-container` | samples CPU and memory from a Docker container |
| `-json` | machine-readable report |

## Measurement caveats

- Numbers were taken on a 20-core development machine with Postgres and Redis in
  containers and the API running natively. A 2-core VPS will show higher
  latency; the shape of the curve — flat ingestion latency, queue depth as the
  first thing to move — is what transfers.
- Ingestion latency is measured client-side and includes the network hop, which
  is a loopback here.
- The load generator sends realistic but uniform payloads. Production traffic
  with much larger inputs or outputs will store more per event than the 1.1 KB
  measured here.
- `-stats-container` shells out to `docker stats`, so it only works when the API
  runs in a container you can name.
