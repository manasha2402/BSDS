"""
Leaderless Load Test
====================
Sends reads AND writes to random nodes — any node can be the
Write Coordinator. Tracks stale reads by comparing version numbers.
Also tracks R/W intervals: time between a write and the next read on same key.

Run with:
  locust -f locustfile_ll.py \
    --host http://kvstore-ll-node1-alb-1666949106.us-west-2.elb.amazonaws.com \
    --web-port 8091

Adjust read/write ratio by changing the weights on the tasks below.
"""

import csv
import os
import random
import threading
import time
import requests
from locust import HttpUser, task, between, events

# All nodes in the leaderless cluster
ALL_NODES = [
    "http://kvstore-ll-node1-alb-1666949106.us-west-2.elb.amazonaws.com",
    "http://kvstore-ll-node2-alb-1172564231.us-west-2.elb.amazonaws.com",
    "http://kvstore-ll-node3-alb-737984554.us-west-2.elb.amazonaws.com",
    "http://kvstore-ll-node4-alb-2129094127.us-west-2.elb.amazonaws.com",
    "http://kvstore-ll-node5-alb-233498980.us-west-2.elb.amazonaws.com",
]

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
        out = os.path.join(os.path.dirname(__file__), "csv", "rw_intervals_ll.csv")
        os.makedirs(os.path.dirname(out), exist_ok=True)
        with open(out, "w", newline="") as f:
            w = csv.writer(f)
            w.writerow(["interval_ms"])
            for v in intervals:
                w.writerow([round(v * 1000, 2)])
        print(f"R/W intervals saved to {out}")


# ── User ─────────────────────────────────────────────────────────────────────

class LeaderlessUser(HttpUser):
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
        node = random.choice(ALL_NODES)
        ts = time.time()
        start = time.perf_counter()

        try:
            resp = requests.post(
                f"{node}/set",
                json={"key": key, "value": value},
                timeout=10,
            )
            elapsed = int((time.perf_counter() - start) * 1000)
        except Exception as e:
            elapsed = int((time.perf_counter() - start) * 1000)
            self.environment.events.request.fire(
                request_type="POST", name="/set [write]",
                response_time=elapsed, response_length=0,
                exception=e, context={},
            )
            return

        if resp.status_code == 201:
            data = resp.json()
            record_write(key, data.get("version", 0), ts)

        self.environment.events.request.fire(
            request_type="POST", name="/set [write]",
            response_time=elapsed,
            response_length=len(resp.content),
            exception=None if resp.status_code == 201 else Exception(f"status {resp.status_code}"),
            context={},
        )

    @task(99)
    def read(self):
        key = random.choice(KEYS)
        node = random.choice(ALL_NODES)
        read_ts = time.time()
        start = time.perf_counter()

        try:
            resp = requests.get(f"{node}/get", params={"key": key}, timeout=5)
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
                print(f"[STALE] node={node} key={key} got={data.get('version')} "
                      f"expected>={_versions.get(key)}")

        self.environment.events.request.fire(
            request_type="GET", name="/get [read]",
            response_time=elapsed,
            response_length=len(resp.content) if resp.status_code == 200 else 0,
            exception=None if resp.status_code == 200 else Exception(f"status {resp.status_code}"),
            context={},
        )
