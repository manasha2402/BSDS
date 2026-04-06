# Homework 10 : Distributed Key-Value Store

## Overview
This project implements two distributed in-memory key-value stores:
1. **Leader-Follower** — one leader, four followers, configurable W and R quorums
2. **Leaderless** — five identical nodes, any node can accept reads or writes

Both are built in Go, containerized with Docker, and load tested with Locust.

Locust: 
Open http://localhost:8090 — set 10 users, spawn rate 2.

> **Load test parameters used:**
> - Concurrent users: 10
> - Spawn rate: 2 users/second
> - Duration: ~90 seconds per run
> - Key space: 20 keys (ensures reads and writes hit the same key frequently)



## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/set` | POST | Write a key-value pair |
| `/get?key=` | GET | Read a value (quorum read) |
| `/local_read?key=` | GET | Read raw local value (test only) |
| `/internal/set` | POST | Leader→Follower replication (internal) |

### Example
```bash
# Write
curl -X POST http://localhost:8080/set \
  -H "Content-Type: application/json" \
  -d '{"key":"foo","value":"bar"}'

# Read
curl "http://localhost:8080/get?key=foo"
```

---

## How It Works

### Leader-Follower

#### Write (set)
1. Client sends POST /set to the **leader only**
2. Leader generates a version number for the key
3. Leader writes to its local store
4. Leader fans out to all followers concurrently
5. Leader waits for **W** acknowledgements before responding 201
6. Each follower sleeps 100ms (simulated storage delay)
7. Leader sleeps 200ms between each follower message (simulated network delay)

#### Read (get)
1. Client sends GET /get to **any node**
2. Node contacts **R** nodes including itself via `/local_read`
3. Returns the entry with the **highest version number**
4. Followers sleep 50ms when serving reads

#### The three W/R strategies:
- **W=5, R=1** → slow writes (wait for all), fast reads (local only), never stale
- **W=1, R=5** → fast writes (local only), slow reads (contact all), possible staleness
- **W=3, R=3** → balanced, quorum overlap guarantees freshness

### Leaderless

#### Write (set)
1. Client sends POST /set to **any node**
2. That node becomes the **Write Coordinator**
3. Coordinator writes locally first
4. Coordinator propagates to ALL other nodes (W=N)
5. Only responds 201 after every node acks
6. Inconsistency window exists between local write and full propagation

#### Read (get)
1. Client sends GET /get to **any node**
2. Node returns its **local value only** (R=1)
3. If read arrives before propagation completes → stale data returned

---

## N, R, W Configuration

| Strategy | W | R | Write Speed | Read Speed | Stale Reads? |
|----------|---|---|-------------|------------|--------------|
| Leader-Follower 1 | 5 | 1 | Slow | Fast | Never |
| Leader-Follower 2 | 1 | 5 | Fast | Slow | Possible |
| Leader-Follower 3 | 3 | 3 | Medium | Medium | Rare |
| Leaderless | N | 1 | Slow | Fast | Possible |

---

## Load Test Results

### Strategy 1 — W=5, R=1 (AWS)

> All 5 nodes must acknowledge before write returns. Reads served locally.

| Ratio | RPS | Read Median | Write Median | Stale Reads |
|-------|-----|-------------|--------------|-------------|
| 1% writes | 46.7 | 180ms | 400ms | 0 |
| 10% writes | 43.1 | 180ms | 400ms | 0 |
| 50% writes | 31.4 | 180ms | 400ms | 0 |
| 90% writes | 24.7 | 180ms | 390ms | 0 |

#### 1% Writes / 99% Reads
![W5R1 1w99r stats](screenshots/W=5,R=1/1W,99R_stats.png)
![W5R1 1w99r charts](screenshots/W=5,R=1/1W,99R_charts.png)

#### 10% Writes / 90% Reads
![W5R1 10w90r stats](screenshots/W=5,R=1/10W,90R_stats.png)
![W5R1 10w90r charts](screenshots/W=5,R=1/10W,90R_charts.png)

#### 50% Writes / 50% Reads
![W5R1 50w50r stats](screenshots/W=5,R=1/50W,50R_stats.png)
![W5R1 50w50r charts](screenshots/W=5,R=1/50W,50R_charts.png)

#### 90% Writes / 10% Reads
![W5R1 90w10r stats](screenshots/W=5,R=1/90W,10R_stats.png)
![W5R1 90w10r charts](screenshots/W=5,R=1/90W,10R_charts.png)

---

### Strategy 2 — W=1, R=5 (AWS)

> Leader writes locally and returns immediately. Reads contact all 5 nodes.

| Ratio | RPS | Read Median | Write Median | Stale Reads |
|-------|-----|-------------|--------------|-------------|
| 1% writes | 38 | 230ms | 180ms | 0 |
| 10% writes | 39.4 | 230ms | 88ms | 0 |
| 50% writes | 39.25 | 250ms | 88ms | 0 |
| 90% writes | 75.8 | 230ms | 89ms | 0 |

#### 1% Writes / 99% Reads
![W1R5 1w99r stats](screenshots/W=1,R=5/1W,99R_stats.png)
![W1R5 1w99r charts](screenshots/W=1,R=5/1W,99R_charts.png)

#### 10% Writes / 90% Reads
![W1R5 10w90r stats](screenshots/W=1,R=5/10W,90R_stats.png)
![W1R5 10w90r charts](screenshots/W=1,R=5/10W,90R_charts.png)

#### 50% Writes / 50% Reads
![W1R5 50w50r stats](screenshots/W=1,R=5/50W,50R_stats.png)
![W1R5 50w50r charts](screenshots/W=1,R=5/50W,50R_charts.png)

#### 90% Writes / 10% Reads
![W1R5 90w10r stats](screenshots/W=1,R=5/90W,10R_stats.png)
![W1R5 90w10r charts](screenshots/W=1,R=5/90W,10R_charts.png)

---

### Strategy 3 — W=3, R=3 (AWS)

> Quorum of 3 for both reads and writes.

| Ratio | RPS | Read Median | Write Median | Stale Reads |
|-------|-----|-------------|--------------|-------------|
| 1% writes | 36.5 | 230ms | 390ms | 0 |
| 10% writes | 35.5 | 230ms | 390ms | 0 |
| 50% writes | 29.1 | 230ms | 390ms | 0 |
| 90% writes | 23.5 | 230ms | 390ms | 0 |

#### 1% Writes / 99% Reads
![W3R3 1w99r stats](screenshots/W=3,R=3/1W,99R_stats.png)
![W3R3 1w99r charts](screenshots/W=3,R=3/1W,99R_charts.png)

#### 10% Writes / 90% Reads
![W3R3 10w90r stats](screenshots/W=3,R=3/10W,90R_stats.png)
![W3R3 10w90r charts](screenshots/W=3,R=3/10W,90R_charts.png)

#### 50% Writes / 50% Reads
![W3R3 50w50r stats](screenshots/W=3,R=3/50W,50R_stats.png)
![W3R3 50w50r charts](screenshots/W=3,R=3/50W,50R_charts.png)

#### 90% Writes / 10% Reads
![W3R3 90w10r stats](screenshots/W=3,R=3/90W,10R_stats.png)
![W3R3 90w10r charts](screenshots/W=3,R=3/90W,10R_charts.png)

---

### Leaderless — W=N, R=1 (AWS)

> Any node accepts writes, propagates to all. Reads are local only.

| Ratio | RPS | Read Median | Write Median | Stale Reads |
|-------|-----|-------------|--------------|-------------|
| 1% writes | 37.6 | 230ms | 480ms | 0 |
| 10% writes | 35.4 | 230ms | 490ms | 0 |
| 50% writes | 24.8 | 230ms | 480ms | 0 |
| 90% writes | 20.5 | 230ms | 480ms | 0 |

#### 1% Writes / 99% Reads
![LL 1w99r stats](screenshots/leaderless/1W,99R_stats.png)
![LL 1w99r charts](screenshots/leaderless/1W,99R_charts.png)

#### 10% Writes / 90% Reads
![LL 10w90r stats](screenshots/leaderless/10W,90R_stats.png)
![LL 10w90r charts](screenshots/leaderless/10W,90R_charts.png)

#### 50% Writes / 50% Reads
![LL 50w50r stats](screenshots/leaderless/50W,50R_stats.png)
![LL 50w50r charts](screenshots/leaderless/50W,50R_charts.png)

#### 90% Writes / 10% Reads
![LL 90w10r stats](screenshots/leaderless/90W,10R_stats.png)
![LL 90w10r charts](screenshots/leaderless/90W,10R_charts.png)

## Results Analysis

### Load Test Summary

| Strategy | 1% RPS | 10% RPS | 50% RPS | 90% RPS | Write Median | Read Median |
|----------|--------|---------|---------|---------|-------------|-------------|
| W=5, R=1 | 46.7 | 43.1 | 31.4 | 24.7 | ~400ms | 180ms |
| W=1, R=5 | 38.0 | 39.4 | 39.3 | 75.8 | ~89ms | 230ms |
| W=3, R=3 | 36.5 | 35.5 | 29.1 | 23.5 | ~390ms | 230ms |
| Leaderless | 37.6 | 35.4 | 24.8 | 20.5 | ~480ms | 230ms |

### Discussion

**W=5, R=1 — Highest read throughput, degraded by writes**

RPS drops from 46.7 → 24.7 as write ratio increases from 1% → 90%. This is expected: writes require all 5 nodes to ack (~400ms each) while reads are served locally (~180ms). More writes means more time blocked on slow operations. Zero stale reads at all ratios because R=1 reads always hit the leader which has the latest version (the write only returns after all nodes are updated).

**W=1, R=5 — Counterintuitive: RPS *increases* with more writes**

RPS rises from 38 → 75.8 as write ratio increases from 1% → 90%. This seems backwards but is correct: writes complete in ~89ms (leader writes locally and returns immediately), while reads take ~230ms (must contact all 5 nodes). Substituting fast writes for slow reads *increases* overall throughput. Zero stale reads despite W=1 — with only 10 users and a 20-key space, the probability of a quorum read missing the latest write within the ~300ms propagation window is very low at these concurrency levels.

**W=3, R=3 — Balanced but slower than W=5,R=1 for reads**

RPS drops steadily from 36.5 → 23.5. Both reads and writes take ~300-400ms (each must contact 3 nodes). The quorum overlap (W+R=6 > N=5) guarantees freshness. This strategy gives the best balance: faster than W=5,R=1 for heavy writes, more consistent than W=1,R=5.

**Leaderless — Slowest overall, highest write latency**

RPS drops from 37.6 → 20.5. Writes take ~480ms — the coordinator must wait for all 4 peers to ack (W=N), and each peer takes 200ms + 100ms = ~300ms in parallel. With 4 concurrent goroutines all taking ~300ms, the total is ~300ms + network overhead. Zero stale reads in load tests because W=N means every node is updated before 201 is returned; R=1 reads after the ack are always consistent. Stale reads *can* occur during the propagation window (between coordinator's local write and full propagation), but are very rare at low concurrency.

### Which Leader-Follower Strategy Does Best at Each Ratio?

| Ratio | Best Strategy | Why |
|-------|--------------|-----|
| **1% writes / 99% reads** | **W=5, R=1** | Almost all traffic is reads. W=5,R=1 serves reads locally (~180ms) — fastest of all strategies at this ratio. The rare write paying ~400ms barely affects overall RPS (46.7 vs ~37 for the others). |
| **10% writes / 90% reads** | **W=5, R=1** | Still mostly reads. W=5,R=1 leads at 43.1 RPS. The higher write cost (~400ms) is now more noticeable but reads are so fast locally that it still wins. |
| **50% writes / 50% reads** | **W=1, R=5** | Equal mix. W=1,R=5 writes (~89ms) are much faster than its reads (~230ms), and the fast writes offset the slow reads. W=5,R=1 drops to 31.4 RPS because now half the traffic is paying the 400ms write penalty. W=1,R=5 holds at 39.3 RPS. |
| **90% writes / 10% reads** | **W=1, R=5** | Dominated by writes. W=1,R=5 writes complete in ~89ms, and at 90% writes this drives RPS to 75.8 — nearly double any other strategy. W=5,R=1 collapses to 24.7 RPS because almost everything is paying the 5-node ack penalty. |

**Summary:** W=5,R=1 dominates read-heavy workloads; W=1,R=5 dominates write-heavy workloads; W=3,R=3 is the safe balanced choice when you need consistency regardless of ratio.

---

### Which Strategy for Which Application?

| Use Case | Best Strategy | Reason |
|----------|--------------|--------|
| Read-heavy cache (social feed, CDN) | **W=5, R=1** | Fastest reads (180ms, local), acceptable write cost |
| Write-heavy logging / analytics | **W=1, R=5** | Fastest writes (~89ms), high throughput at 90% writes |
| Financial transactions / inventory | **W=3, R=3** | Quorum overlap guarantees no stale reads, balanced latency |
| Simple replication, no single leader | **Leaderless** | No SPOF leader, any node accepts writes, good for geo-distribution |


## Graphs

Each ratio figure has two panels — **Read Latency** and **Write Latency** — with grouped bars showing Median (lighter) and p95 (darker) for all 4 strategies side by side.
---

### 1% Writes / 99% Reads
![1% writes ratio](loadtest/graphs/ratio_1pct.png)

### 10% Writes / 90% Reads
![10% writes ratio](loadtest/graphs/ratio_10pct.png)

### 50% Writes / 50% Reads
![50% writes ratio](loadtest/graphs/ratio_50pct.png)

### 90% Writes / 10% Reads
![90% writes ratio](loadtest/graphs/ratio_90pct.png)

### R/W Interval Distribution — All Strategies
![R/W intervals](loadtest/graphs/rw_intervals.png)

---

## Conclusion

This project explored how the W (write quorum) and R (read quorum) parameters control the consistency/availability trade-off in a replicated key-value store.

**Key findings:**

1. **W+R > N is the only hard consistency guarantee.** W=3, R=3 (with N=5) is the only leader-follower strategy that provably prevents stale reads, because the read quorum always overlaps with the write quorum. W=5,R=1 is also consistent but for a different reason — the write blocks until every node has the data, so any R=1 read is always fresh.

2. **W=1, R=5 inverts the performance curve.** Because writes are fast (~89ms) and reads are slow (~230ms), adding *more* writes actually *increases* overall RPS. At 90% writes the cluster achieved 75.8 RPS — nearly double any other strategy. The trade-off is that stale reads become possible during the propagation window, although they were not observed at this concurrency level.

3. **Leaderless W=N is the slowest but eliminates the single point of failure.** Every write blocks until all 5 nodes ack (~480ms), but once the 201 is returned the entire cluster is consistent. The main advantage over leader-follower is that any node can accept writes — no leader election, no leader failover needed.

4. **Stale reads require high concurrency or tight write/read timing to observe.** All 16 runs showed 0 stale reads because the inconsistency window (~300ms) is short relative to the inter-request time at 10 users. A stress test at 100+ users with a small key space would reliably expose stale reads in W=1 and leaderless strategies.

5. **Simulated delays matter.** The 200ms coordinator-to-follower delay and 100ms follower write delay were deliberately chosen to make quorum behavior observable at human timescales. In a real distributed system these delays would be network RTTs (typically 1-10ms per hop), which would shift all latencies down by ~100× but preserve the *relative* ordering between strategies.

**Bottom line:** the right replication strategy depends entirely on the read/write ratio and consistency requirements of the application. W=5,R=1 for read-heavy workloads, W=1,R=5 for write-heavy workloads, W=3,R=3 when consistency is non-negotiable, and leaderless when you need no single point of failure.

---

## AI Assistance
This project was built with AI assistance (Claude). All code has been reviewed
and understood by the author. The following components were AI-assisted:
- Initial structure and boilerplate for Go HTTP handlers
- Quorum logic in replication.go
- Locust load test files
- Docker Compose configuration