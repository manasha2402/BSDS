import json
import statistics
from collections import defaultdict

# Load and combine
with open("mysql_test_results.json") as f:
    mysql_data = json.load(f)
with open("dynamodb_test_results.json") as f:
    dynamo_data = json.load(f)

for e in mysql_data:  e["database"] = "mysql"
for e in dynamo_data: e["database"] = "dynamodb"

combined = mysql_data + dynamo_data

with open("combined_results.json", "w") as f:
    json.dump(combined, f, indent=2)
print("\nJson write successful: combined_results.json\n")

# Helper methods
def percentile(data, p):
    sorted_d = sorted(data)
    idx = (p / 100) * (len(sorted_d) - 1)
    lo, hi = int(idx), min(int(idx) + 1, len(sorted_d) - 1)
    return sorted_d[lo] + (idx - lo) * (sorted_d[hi] - sorted_d[lo])

def stats(times):
    return {
        "avg":  round(statistics.mean(times), 2),
        "p50":  round(percentile(times, 50), 2),
        "p95":  round(percentile(times, 95), 2),
        "p99":  round(percentile(times, 99), 2),
        "min":  round(min(times), 2),
        "max":  round(max(times), 2),
    }

def bucket(data, db):
    return [e for e in data if e["database"] == db]

ops_order = ["create_cart", "add_items", "get_cart"]

print("Pre-Analysis Data Verification:")
for db, data in [("MySQL", mysql_data), ("DynamoDB", dynamo_data)]:
    counts = defaultdict(int)
    for e in data: counts[e["operation"]] += 1
    total = len(data)
    successes = sum(1 for e in data if e["success"])
    print(f"\n{db}: {total} ops | {successes} successes")
    for op in ops_order:
        print(f"  {op}: {counts[op]}")
    if total != 150: print(f"  WARNING: expected 150, got {total}")
    else: print("  150 operations confirmed")
print()

print("Overall Statistics:")
for db, data in [("MySQL", mysql_data), ("DynamoDB", dynamo_data)]:
    times = [e["response_time"] for e in data]
    s = stats(times)
    success_rate = round(sum(1 for e in data if e["success"]) / len(data) * 100, 1)
    print(f"\n{db} Statistics")
    print(f"  Avg: {s['avg']} ms | P50: {s['p50']} ms | P95: {s['p95']} ms | P99: {s['p99']} ms")
    print(f"  Min: {s['min']} ms | Max: {s['max']} ms | Success: {success_rate}%")
print()

print("\nOperation-Specific Breakdown:\n")
print(f"{'Operation':<15} {'MySQL Avg':>10} {'DDB Avg':>10} {'Faster By':>10}")
print("-" * 50)
for op in ops_order:
    m_times = [e["response_time"] for e in mysql_data  if e["operation"] == op]
    d_times = [e["response_time"] for e in dynamo_data if e["operation"] == op]
    m_avg = round(statistics.mean(m_times), 2) if m_times else None
    d_avg = round(statistics.mean(d_times), 2) if d_times else None
    if m_avg and d_avg:
        winner = "MySQL" if m_avg < d_avg else "DynamoDB"
        margin = round(abs(m_avg - d_avg), 2)
        print(f"{op:<15} {m_avg:>10} {d_avg:>10}   {winner} by {margin} ms")
print()

print("\nComparison Table:\n")
m_times = [e["response_time"] for e in mysql_data]
d_times = [e["response_time"] for e in dynamo_data]
ms = stats(m_times)
ds = stats(d_times)
m_sr = round(sum(1 for e in mysql_data  if e["success"]) / 150 * 100, 1)
d_sr = round(sum(1 for e in dynamo_data if e["success"]) / 150 * 100, 1)

metrics = [
    ("Avg Response Time (ms)", ms["avg"], ds["avg"]),
    ("P50 Response Time (ms)", ms["p50"], ds["p50"]),
    ("P95 Response Time (ms)", ms["p95"], ds["p95"]),
    ("P99 Response Time (ms)", ms["p99"], ds["p99"]),
    ("Success Rate (%)",        m_sr,      d_sr),
]

print(f"{'Metric':<28} {'MySQL':>8} {'DynamoDB':>10} {'Winner':>10} {'Margin':>10}")
print("-" * 70)
for name, mv, dv in metrics:
    if name == "Success Rate (%)":
        winner = "MySQL" if mv > dv else ("DynamoDB" if dv > mv else "Tie")
        margin = f"{abs(mv - dv):.1f}%"
    else:
        winner = "MySQL" if mv < dv else ("DynamoDB" if dv < mv else "Tie")
        margin = f"{abs(mv - dv):.2f} ms"
    print(f"{name:<28} {mv:>8} {dv:>10} {winner:>10} {margin:>10}")
print(f"{'Total Operations':<28} {'150':>8} {'150':>10}\n")