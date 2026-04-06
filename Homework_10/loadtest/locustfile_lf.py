"""
Leader-Follower Load Test
=========================
Sends writes to the leader, reads to random followers.
Both reads AND writes show up in Locust UI and CSV.

Tracks:
  - Stale reads (version comparison)
  - R/W intervals: time between a write and the next read on the same key

Run with:
  locust -f locustfile_lf.py --host http://localhost:8080 --web-port 8090

Change read/write ratio by adjusting @task weights below.
"""

import csv
import os
import random
import threading
import time
import requests
from locust import HttpUser, task, between, events

# Follower ALB URLs (AWS). Override for local testing:
#   LF_FOLLOWERS="http://localhost:8081,...,http://localhost:8084" locust ...
_followers_env = os.getenv("LF_FOLLOWERS", "")
FOLLOWER_URLS = (
    [u.strip() for u in _followers_env.split(",") if u.strip()]
    if _followers_env
    else [
        "http://kvstore-f1-alb-1718892841.us-west-2.elb.amazonaws.com",
        "http://kvstore-f2-alb-651009101.us-west-2.elb.amazonaws.com",
        "http://kvstore-f3-alb-601209401.us-west-2.elb.amazonaws.com",
        "http://kvstore-f4-alb-1533337818.us-west-2.elb.amazonaws.com",
    ]
)

NUM_KEYS = 20
KEYS = [f"key{i}" for i in range(NUM_KEYS)]

# ── Shared state (thread-safe) ───────────────────────────────────────────────
_lock = threading.Lock()
_versions: dict = {}        # key → latest known version
_last_write_ts: dict = {}   # key → timestamp of last write
_stale_reads = 0
_total_reads = 0
_rw_intervals: list = []    # seconds between a write and the next read on same key


def record_write(key, version, ts):
    with _lock:
        if version > _versions.get(key, 0):
            _versions[key] = version
            _last_write_ts[key] = ts


def check_read(key, version, read_ts):
    """Update stale counter and record R/W interval. Returns True if stale."""
    global _stale_reads, _total_reads
    with _lock:
        known = _versions.get(key, 0)
        stale = known > 0 and version < known
        _total_reads += 1
        if stale:
            _stale_reads += 1
        wt = _last_write_ts.get(key)
        if wt is not None:
            interval = read_ts - wt
            if interval >= 0:
                _rw_intervals.append(interval)
    return stale


@events.quitting.add_listener
def on_quit(environment, **kwargs):
    with _lock:
        stale = _stale_reads
        total = _total_reads
        intervals = list(_rw_intervals)

    print(f"\n{'='*50}")
    print(f"Stale reads : {stale} / {total}")
    pct = (stale / total * 100) if total > 0 else 0
    print(f"Stale rate  : {pct:.1f}%")
    if intervals:
        import statistics
        print(f"R/W intervals — n={len(intervals)}, "
              f"median={statistics.median(intervals)*1000:.0f}ms, "
              f"max={max(intervals)*1000:.0f}ms")
    print(f"{'='*50}\n")

    # Save R/W intervals to CSV for graph generation
    if intervals:
        out = os.path.join(os.path.dirname(__file__), "csv", "rw_intervals_lf.csv")
        os.makedirs(os.path.dirname(out), exist_ok=True)
        with open(out, "w", newline="") as f:
            w = csv.writer(f)
            w.writerow(["interval_ms"])
            for v in intervals:
                w.writerow([round(v * 1000, 2)])
        print(f"R/W intervals saved to {out}")


# ── User ─────────────────────────────────────────────────────────────────────

class LeaderFollowerUser(HttpUser):
    wait_time = between(0.01, 0.05)

    # Adjust weights for each ratio:
    #   1%  writes → @task(1)  + @task(99)
    #   10% writes → @task(10) + @task(90)
    #   50% writes → @task(50) + @task(50)
    #   90% writes → @task(90) + @task(10)
    @task(1)
    def write(self):
        key = random.choice(KEYS)
        value = f"val-{random.randint(0, 999999)}"
        ts = time.time()

        with self.client.post(
            "/set",
            json={"key": key, "value": value},
            name="/set [write]",
            catch_response=True,
        ) as resp:
            if resp.status_code == 201:
                data = resp.json()
                record_write(key, data.get("version", 0), ts)
                resp.success()
            else:
                resp.failure(f"write failed: {resp.status_code}")

    @task(99)
    def read(self):
        key = random.choice(KEYS)
        follower = random.choice(FOLLOWER_URLS)
        read_ts = time.time()

        start = time.perf_counter()
        try:
            resp = requests.get(f"{follower}/get", params={"key": key}, timeout=5)
            elapsed = int((time.perf_counter() - start) * 1000)
        except Exception as e:
            elapsed = int((time.perf_counter() - start) * 1000)
            self.environment.events.request.fire(
                request_type="GET", name="/get [read]",
                response_time=elapsed, response_length=0,
                exception=e, context={},
            )
            return

        if resp.status_code == 404:
            self.environment.events.request.fire(
                request_type="GET", name="/get [read]",
                response_time=elapsed, response_length=0,
                exception=None, context={},
            )
            return

        if resp.status_code == 200:
            data = resp.json()
            stale = check_read(key, data.get("version", 0), read_ts)
            if stale:
                print(f"[STALE] key={key} got={data.get('version')} "
                      f"expected>={_versions.get(key)}")

        self.environment.events.request.fire(
            request_type="GET", name="/get [read]",
            response_time=elapsed,
            response_length=len(resp.content) if resp.status_code == 200 else 0,
            exception=None if resp.status_code == 200 else Exception(f"status {resp.status_code}"),
            context={},
        )
