## Questions

**1. Roughly how many submissions did it take before you passed all critical scenarios, and what was the most common failure?**

Around 10-12 submissions. The most common early failure was S3_PHOTO_UPLOAD_ASYNC timing out — the service was uploading to S3 synchronously inside the POST handler before sending 202, so ChaosArena's 30-second poll window expired before the status ever reached `completed`.

**2. Where are your photo files stored, and why did you pick that over other options?**

Photos are stored in an S3 bucket with a public-read policy. S3 gives strong read-after-write consistency, permanent public URLs (no presigning on every GET), and scales without any configuration. Local disk was ruled out because ECS tasks are ephemeral and don't share storage; RDS binary columns would be slow and expensive; EFS adds unnecessary complexity.

**3. Describe your deployment setup — how many instances, what cloud services, and how they connect to each other.**

Three ECS Fargate tasks (4 vCPU / 8 GB each) sit behind an Application Load Balancer. Each task holds 50 background worker goroutines. The tasks read and write to two DynamoDB tables (albums, photos) and upload files to an S3 bucket. A VPC Gateway Endpoint routes ECS to S3 traffic over AWS's private backbone. All infrastructure is provisioned by a single Terraform file.

**4. Did you use a reverse proxy or load balancer? If so, what role does it play in your architecture?**

Yes, ALB. It distributes incoming requests across the three ECS tasks, performs health checks on `/health` every 15 seconds to remove unhealthy targets, and provides a single stable DNS name for ChaosArena to submit to. `idle_timeout` is set to 300 seconds so large file uploads aren't dropped mid-stream.

**5. How does your background worker get notified that there's a new photo to process? Did you use a queue, polling, or something else?**

An in-process Go channel (`jobQueue`, buffered at 10 000). The POST handler non-blockingly sends a `PhotoJob` struct (containing the file bytes and metadata) onto the channel; 50 goroutines per task read from it continuously. No SQS or external queue — keeping it in-process eliminated a network round-trip and kept the p95 of the accept path under 10 ms.

**6. The spec requires that `seq` is assigned in the POST handler, not the background worker. Why does that matter, and how did you ensure correctness under concurrent uploads to the same album?**

If the worker assigned seq, two concurrent POSTs could both return 202 before either worker ran, making seq ordering undefined and potentially non-unique. Assigning it in the handler means the client gets the correct seq in the 202 response and ordering matches arrival order. Correctness under concurrency is guaranteed by DynamoDB's `UpdateItem ADD photo_seq :1` — an atomic server-side increment. No two ECS tasks can get the same value regardless of how many fire simultaneously.

**7. What happens in your system if the worker crashes or fails halfway through processing a photo?**

The photo stays in `processing` status permanently. There is no retry loop or dead-letter queue — a worker failure (e.g. S3 timeout) sets the status to `failed` and stops. A future improvement would be a retry with exponential back-off before marking `failed`. The 5-minute context timeout on each job prevents workers from hanging indefinitely on a stuck upload.

**8. What does your database schema look like? What tables or collections did you create and why?**

Two DynamoDB tables:
- **albums** — hash key `album_id` (String). Attributes: `title`, `description`, `owner`, `photo_seq` (a numeric counter, incremented atomically on each photo upload).
- **photos** — hash key `photo_id` (String). Attributes: `album_id`, `seq`, `status`, `s3_key`.

Keeping photos in a separate table avoids unbounded item growth on album records and allows photo lookups by `photo_id` directly without scanning.

**9. Did you add any indexes to your database? If so, on which columns and why?**

No secondary indexes. Every read is by primary key, albums by `album_id`, photos by `photo_id`. `GET /albums` uses a full table scan (`Scan`) which is acceptable because the albums table is small and DynamoDB Scan is parallelisable. Adding a GSI on `album_id` in the photos table would speed up a "list photos per album" query, but no such endpoint exists in the spec.

**10. Which load testing scenario was the hardest for you, and what bottleneck did you discover?**

S12 (Concurrent Photo Uploads). The bottleneck was that the POST handler was blocking on the S3 upload before sending 202, turning an "async" endpoint effectively synchronous. Under 50+ concurrent requests each waiting 200–800 ms for S3, the p95 ballooned to ~22 seconds. S12's high error rate was dragging down the overall score.

**11. What was the single most impactful change you made to improve your load test scores?**

Two changes were most impactful. First, switching from MySQL to DynamoDB unblocked the entire load test suite — MySQL's connection pool saturated under concurrent load and caused bulk 504s on S11, which prevented S12–S15 from running at all. DynamoDB has no connection pool; every request is an independent HTTPS call. Second, making small-file uploads async — reading the bytes in the handler, returning 202 immediately, and handing the S3 upload off to a goroutine pool — dropped the POST handler time from ~500 ms to under 10 ms, directly fixing S12 and unblocking S13–S15.

**12. How did you handle concurrent writes — for example, many album creates or photo uploads happening at the same time?**

DynamoDB handles concurrent writes natively, each `PutItem` / `UpdateItem` is an independent atomic HTTP call with no connection pool or row locking. For the `seq` counter specifically, `ADD photo_seq :1` is a compare-and-swap style atomic increment; no two concurrent callers across any number of ECS tasks can receive the same value.

**13. Describe a specific bug you ran into and how you diagnosed it using the ChaosArena event logs or your own logs.**

S9 (Delete Super) showed a photo returning 200 with `status: completed` roughly 200 ms after a DELETE had already returned 204. The ChaosArena event log showed the DELETE succeeded at `t=50ms`, then a GET at `t=250ms` got a 200 with `completed`. Cross-referencing the ECS CloudWatch logs showed a worker finishing an S3 upload at `t=200ms` and calling `UpdateItem` with no condition — re-creating the record. The fix was adding `ConditionExpression: attribute_exists(photo_id)` so a post-delete write throws `ConditionalCheckFailedException`, which the worker handles by deleting the orphaned S3 object.

**14. How did you test your service locally before submitting to ChaosArena?**

All testing was done against our own AWS deployment (ECS + ALB) before submitting to ChaosArena. Manual `curl` commands exercised each endpoint in sequence: PUT album → GET album → POST photo → poll GET until `completed` → DELETE → confirm 404. For concurrency, I wrote a loop that fired concurrent DELETE requests against the same photo and verified every response returned 204, confirming idempotent delete behaviour held under concurrency. I also tested large file uploads directly against the ALB to check for 504 timeouts, and monitored CloudWatch logs to verify worker goroutines were completing S3 uploads and updating photo status correctly. After each fix I re-ran the full curl sequence and checked the ALB target group in the AWS console to confirm all three ECS tasks were healthy before submitting.

**15. If you had another week, what is the one thing you would change or add to your system to improve your score?**

Add a Redis layer (ElastiCache) in front of DynamoDB for album and photo metadata reads. Under S13 and S14, the same album and photo records are read repeatedly by concurrent clients — every GET currently hits DynamoDB. With Redis acting as an in-memory read layer, the first request loads the record and subsequent requests are served in under 1 ms without touching DynamoDB at all. This would directly improve p95 on S13 (Mixed Read/Write Metadata) and the read half of S14. Any PUT album or photo status update writes through to both DynamoDB and Redis to keep them in sync.

**16. How did you add value over and above what Claude could do in this assignment?**

Debugging the live deployment required judgment that no tool could automate, recognising that 504s only appeared under load (not individually) pointed to a pool exhaustion problem rather than a network issue, deciding to replace MySql entirely with DynamoDB rather than just tuning the pool size; identifying the VPC endpoint as the source of the S3 latency by correlating CloudWatch timing with upload sizes. The architectural decisions - hybrid upload threshold, worker pool size, ALB timeout, were calibrated against actual ChaosArena run reports, not theoretical analysis.

**ChaosArena URL:** `chaosarena-alb-938452724.us-west-2.elb.amazonaws.com`

---

## Architecture

- 3 ECS Fargate tasks (4 vCPU / 8 GB each) behind an AWS Application Load Balancer
- Each task runs 50 background worker goroutines for async S3 uploads
- DynamoDB (PAY_PER_REQUEST) — two tables: albums and photos
- S3 bucket (public-read) for photo storage, connected via VPC Gateway Endpoint
- All infrastructure provisioned with Terraform

---

## Tech Stack

| Layer | Choice | Reason |
|---|---|---|
| Language | Go 1.24 | Goroutines handle thousands of concurrent connections natively |
| HTTP | `net/http` (stdlib) | No framework overhead; Go 1.22+ routing with `r.PathValue()` |
| Database | DynamoDB PAY_PER_REQUEST | No connection pool limits; atomic `ADD` counter for seq numbers |
| File storage | S3 (public-read bucket) | Permanent public URLs; strong consistency after PutObject |
| Deployment | ECS Fargate | Horizontal scaling; 3 tasks behind ALB |
| IaC | Terraform | Single `main.tf` |

---


### GET /health

```json
{ "status": "ok" }
```

`status` must be the exact lowercase string `"ok"`.

---

### PUT /albums/:album_id

**Request body:**
```json
{
  "album_id": "a1b2c3d4-...",
  "title": "My Summer Trip",
  "description": "Photos from Cancun",
  "owner": "student@northeastern.edu"
}
```

Returns `201` on create, `200` on update. Idempotent — calling twice with the same `:album_id` does not create two records.

---

### GET /albums/:album_id

Returns `200` with the album JSON or `404 {"error":"not found"}`.

---

### GET /albums

Returns all albums (bare array or `{"albums":[...]}` both accepted). Must include every album ever created — ChaosArena accumulates albums across all scenarios including load tests.

---

### POST /albums/:album_id/photos

`Content-Type: multipart/form-data`, field name `photo`.

**Response — 202 Accepted (immediate):**
```json
{ "photo_id": "f1e2d3c4-...", "seq": 4, "status": "processing" }
```

- `seq` is assigned synchronously in the POST handler (not by the background worker).
- Monotonically increasing and unique within each album; counters are per-album starting at 1.
- ChaosArena polls GET status until `completed`.

---

### GET /albums/:album_id/photos/:photo_id

```json
{ "photo_id": "...", "album_id": "...", "seq": 4, "status": "completed", "url": "https://..." }
```

`url` must be present when `status` is `completed` and must return 200 when fetched directly. Valid status values: `processing` · `completed` · `failed`.

---

### DELETE /albums/:album_id/photos/:photo_id

Returns `204`. After deletion:
- `GET` on the photo must return `404`
- The file at the captured `url` must no longer return 200

Deletion must complete within 5 seconds. Always returns `204` — even if the photo was already deleted (idempotent).

---

### Load Testing Scenarios (80 pts)

Run as long as S1–S5 pass. Scored by p95 latency:

```
If Student_P95 > 5 × Ref_P95:  Score = 0

Otherwise:
    Latency Score = min(MaxPoints, MaxPoints × (Ref_P95 / Student_P95))
    Error Penalty = min(1.0, ErrorRate / 0.10)
    Score         = Latency Score × (1 − Error Penalty)
```

`ErrorRate` counts 5xx + timeouts only. At 10% error rate the score is zeroed regardless of latency.

| Scenario | Max Points | What is measured |
|---|---|---|
| S11 Concurrent Album Creates | 15 | p95 of concurrent PUT /albums |
| S12 Concurrent Photo Uploads | 15 | p95 of POST→completed under concurrent load |
| S13 Mixed Read/Write Metadata | 15 | p95 of mixed GET/PUT metadata concurrently |
| S14 Mixed Metadata + Uploads | 15 (10+5) | metadata ops and uploads simultaneously; two sub-scores |
| S15 Large Payload Upload | 20 (10+10) | large concurrent uploads; POST→202 and POST→completed sub-scores |
| **Subtotal** | **80** | |

**Grand total: 190 pts**

---

## Key Design Decisions

### DynamoDB over RDS

The per-album `seq` counter must be atomic across all ECS tasks. RDS requires `SELECT ... FOR UPDATE` — a row lock that serialises under load and exhausts the connection pool.

DynamoDB's `UpdateItem ADD photo_seq :1` is one atomic API call with no locks and no connection pool. Every ECS task gets a unique, monotonically increasing seq value with one round-trip.

### Hybrid Upload

Two upload paths based on `Content-Length`:

**Small files (< 5 MB) — optimised for S12 concurrent load:**
```
POST → read bytes → assign seq → insert photo → return 202
                                                      ↓
                                         worker pool (50 goroutines)
                                              → S3 upload
                                              → mark completed
```

**Large files (≥ 5 MB) — optimised for S15 large payload:**
```
POST → assign seq → insert photo → stream body directly to S3
                                   (receive + upload overlap)
     → mark completed → return 202
```

Streaming means total time ≈ `max(receive, upload)` instead of `receive + upload`.

### S3 VPC Gateway Endpoint

Routes ECS→S3 traffic over AWS's private backbone instead of the internet gateway. Free, zero code changes, ~1500 ms improvement on large uploads.

### Public S3 URLs

Bucket has a public-read policy. No presigned URL generation on every GET (eliminates a per-request AWS API call):
```
https://{bucket}.s3.us-west-2.amazonaws.com/photos/{album_id}/{photo_id}
```

### S9 Race Condition (DELETE while upload in progress)

```
t=0ms   POST → photo inserted as 'processing', bytes enqueued
t=50ms  DELETE → DynamoDB record deleted
t=200ms Worker finishes S3 upload → UpdatePhotoStatus('completed')
                  ConditionExpression: attribute_exists(photo_id)
                → ConditionalCheckFailedException → ErrPhotoDeleted
t=200ms Worker catches ErrPhotoDeleted → deletes orphaned S3 object
t=500ms GET → 404 ✓
```

---


## Environment Variables

Set automatically by Terraform in the ECS task definition.

| Variable | Description |
|---|---|
| `ALBUMS_TABLE` | DynamoDB albums table name |
| `PHOTOS_TABLE` | DynamoDB photos table name |
| `S3_BUCKET` | S3 bucket name |
| `AWS_REGION` | AWS region (us-west-2) |
| `WORKER_COUNT` | Background goroutine pool size (50) |
| `PORT` | HTTP port (8080) |

---

## Debugging Failed Runs

### Correctness event log

```json
{
  "name": "S3_PHOTO_UPLOAD_ASYNC",
  "status": "FAILED",
  "points_awarded": 0,
  "events": [
    { "t_ms": 0,     "type": "REQUEST",   "detail": "POST /albums/xyz/photos" },
    { "t_ms": 45,    "type": "RESPONSE",  "detail": "status=202 latency=45ms" },
    { "t_ms": 30041, "type": "VIOLATION", "detail": "photo still 'processing' after 30s timeout" }
  ]
}
```

### Load test results

```json
{
  "name": "S11_CONCURRENT_CREATES_LOAD",
  "status": "PASSED",
  "points_awarded": 12,
  "metrics": { "p95_ms": 187, "p99_ms": 312, "error_rate": 0.002 }
}
```

For S14 and S15 the `extra` field breaks down sub-scores:

```json
"extra": {
  "accept_p95_ms": 4200,   "accept_score": 10,
  "complete_p95_ms": 9100, "complete_score": 7,
  "complete_error_rate_pct": 0
}
```

---

## What Actually Broke

A log of every real failure hit during development, what the symptom was, and what fixed it.

---

### 1. Every request returns 504 Bad Gateway

**Symptom:** `curl $ALB/health` returns `504 Gateway Timeout` immediately after `terraform apply`. ECS tasks show as `RUNNING` in the console but the ALB target group shows all targets as `unhealthy`.

**Cause:** The health check path in the ALB target group was misconfigured (pointing to `/` instead of `/health`), so the ALB never marked any target healthy and dropped every request before it reached the container.

**Fix:**
```hcl
# terraform/main.tf
resource "aws_lb_target_group" "app" {
  health_check {
    path                = "/health"
    healthy_threshold   = 2
    interval            = 15
  }
}
```

After `terraform apply`, wait ~30 seconds for the health check to pass, then retry.

---

### 2. 504s only under load (not on individual requests)

**Symptom:** Correctness scenarios S1–S5 all pass. The moment S11 (concurrent album creates) starts, the ALB starts returning 504s in bulk. ECS logs show nothing — the container appears fine.

**Cause:** The original stack used **PostgreSQL via `pgxpool`**. The default `pgxpool.New` without any config creates a pool of only **4 connections**. Under S11's concurrent load, all 4 connections were held, new requests queued, hit the ALB idle timeout, and the ALB dropped them as 504.

**What we tried first (wrong):** Increasing ECS task count and vCPU. Made no difference — the bottleneck was the DB pool, not CPU.

**Fix:** Replaced PostgreSQL entirely with DynamoDB. DynamoDB uses an HTTP API — there is no connection pool. Each request is an independent HTTPS call. 150 concurrent goroutines across 3 ECS tasks can all fire simultaneously with zero contention.

```go
// Before — pool exhaustion waiting to happen
pool, err := pgxpool.New(ctx, connStr) // default max_conns = 4

// After — no pool, no limit
client := dynamodb.NewFromConfig(cfg) // HTTP client, unlimited concurrency
```

---

### 3. S12 p95 was 22 000 ms — load test effectively 0 points

**Symptom:** S12 (concurrent photo uploads) timed out for most requests. The p95 came back at ~22 seconds. ChaosArena awarded 0 points.

**Cause:** The POST handler was uploading the file to S3 **synchronously before returning 202**. Under concurrent load, every goroutine blocked on the S3 upload (each taking 200–800 ms). The 202 response only went out after S3 confirmed the upload, turning "async" into synchronous in practice.

**Fix:** Hybrid upload — small files (< 5 MB) are read into memory immediately, 202 is returned, and the actual S3 upload happens in a background worker pool. The handler returns in < 5 ms.

```go
// Before — blocks on S3, 202 only sent after upload completes
s3.upload(ctx, key, fileBytes)
w.WriteHeader(http.StatusAccepted)

// After — enqueue job, return 202 immediately
go func() { jobQueue <- PhotoJob{...} }()
w.WriteHeader(http.StatusAccepted)
```

---

### 4. S15 large upload timing out — 504 on big files

**Symptom:** S15 sends a large file (tens of MB). The ALB returns 504 before the upload completes. The ECS logs confirm the handler is still running — the connection is being cut by the ALB, not the server.

**Cause:** ALB default `idle_timeout` is **60 seconds**. A large file upload over a slow connection takes longer than 60 seconds of "idle" time on the TCP connection from the ALB's perspective (it sees no bytes flowing on the response side while the body is still being received).

**Fix:**
```hcl
resource "aws_lb" "app" {
  idle_timeout = 300  # 5 minutes — large upload buffer
}
```

Additionally switched large files to **streaming** — instead of reading the entire body first then uploading, the body is piped directly into the S3 uploader. This means receive and upload overlap, cutting total time roughly in half.

---

### 5. S9 — photo reappeared after DELETE

**Symptom:** S9 sends DELETE while the upload worker is still in progress. After deletion the GET correctly returns 404. But ~200 ms later, the GET started returning 200 again with `status: completed`.

**Cause:** The background worker finishing its S3 upload called `UpdatePhotoStatus("completed")` with a plain `PutItem` / `UpdateItem` — no condition check. It re-created (or re-updated) the DynamoDB record regardless of whether the photo had been deleted.

**Fix:** Added `ConditionExpression: attribute_exists(photo_id)` to `UpdatePhotoStatus`. If the photo was deleted, DynamoDB throws `ConditionalCheckFailedException`. The worker catches this as `ErrPhotoDeleted` and then deletes the orphaned S3 object.

```go
var ErrPhotoDeleted = errors.New("photo deleted before worker completed")

// UpdatePhotoStatus — conditional update
_, err = client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
    ConditionExpression: aws.String("attribute_exists(photo_id)"),
    ...
})
// ConditionalCheckFailedException → return ErrPhotoDeleted

// Worker — clean up orphaned S3 object
if errors.Is(err, ErrPhotoDeleted) {
    s3.delete(ctx, job.S3Key)
}
```

---

### 6. S3 uploads slow — ~1500 ms extra latency

**Symptom:** Even after fixing the async path, individual S3 upload times in CloudWatch logs were consistently 1200–1800 ms slower than expected for same-region transfers.

**Cause:** ECS tasks were routing S3 traffic through the **internet gateway** — out to the public internet and back — instead of staying on the AWS private network.

**Fix:** Added an S3 VPC Gateway Endpoint in Terraform. Zero code changes, free, and the traffic now stays entirely on AWS's backbone.

```hcl
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = data.aws_vpc.default.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = data.aws_route_tables.default.ids
}
```

---

### 7. DELETE returning 404 on already-deleted photos (S7/S8 failing)

**Symptom:** S7 and S8 test double-delete and concurrent delete. Both scenarios failed because the second DELETE call was returning `404` instead of `204`.

**Cause:** The handler returned 404 when the DynamoDB `DeleteItem` found no item to delete.

**Fix:** `DeleteItem` is always idempotent, if the item doesn't exist, that's still a success. Return `204` unconditionally.

```go
// Before
if item == nil {
    writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
    return
}

// After — DELETE is always 204
store.DeletePhoto(ctx, albumID, photoID) // ConditionalCheckFailedException → treated as success
w.WriteHeader(http.StatusNoContent)
```

---

### 8. Handler panics returning 502 from ALB

**Symptom:** Occasionally the ALB returned `502 Bad Gateway` with no corresponding error in the ECS application logs. The container kept running but the specific request silently failed.

**Cause:** A nil pointer dereference (or other panic) inside a handler caused Go's default behaviour, close the connection. The ALB interprets a dropped connection as a 502.

**Fix:** Added a recovery middleware that wraps every handler. Any panic is caught, logged with a stack trace, and converted to a proper `500` JSON response.

```go
func recoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                log.Printf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, err, debug.Stack())
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusInternalServerError)
                w.Write([]byte(`{"error":"internal error"}`))
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```
