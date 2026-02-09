#!/usr/bin/env python3

# Your actual performance data - UPDATE THESE!
sequential_time = 8.5  # Change to your actual sequential time
parallel_time = 3.1    # Change to your actual parallel time

# Calculate metrics
speedup = sequential_time / parallel_time
efficiency = (speedup / 3) * 100
time_saved = sequential_time - parallel_time

# ASCII Bar Chart
print("\n" + "="*70)
print("  MAPREDUCE PERFORMANCE ANALYSIS - SHAKESPEARE'S HAMLET")
print("="*70)

print("\n📊 EXECUTION TIME COMPARISON\n")

max_time = max(sequential_time, parallel_time)
scale = 50 / max_time

# Sequential bar
seq_bar_length = int(sequential_time * scale)
seq_bar = "█" * seq_bar_length
print(f"Sequential (1 mapper)    {seq_bar} {sequential_time:.2f}s")

# Parallel bar
par_bar_length = int(parallel_time * scale)
par_bar = "█" * par_bar_length
print(f"Parallel (3 mappers)     {par_bar} {parallel_time:.2f}s")

print("\n" + "-"*70)

# Speedup visualization
print("\n⚡ SPEEDUP ANALYSIS\n")
speedup_bar = "█" * int(speedup * 10)
ideal_bar = "░" * 30
print(f"Actual Speedup:  {speedup_bar} {speedup:.2f}x")
print(f"Ideal Speedup:   {ideal_bar} 3.00x (linear)")

print("\n" + "-"*70)

# Efficiency
print("\n📈 PARALLEL EFFICIENCY\n")
eff_bar = "█" * int(efficiency / 2)
print(f"Efficiency: {eff_bar} {efficiency:.1f}%")
print(f"Target:     {'█' * 50} 100%")

print("\n" + "="*70)

# Summary table
print("\n📋 PERFORMANCE SUMMARY\n")
print(f"{'Metric':<25} {'Value':>15}")
print("-"*42)
print(f"{'Sequential Time':<25} {sequential_time:>12.2f}s")
print(f"{'Parallel Time':<25} {parallel_time:>12.2f}s")
print(f"{'Time Saved':<25} {time_saved:>12.2f}s")
print(f"{'Speedup Factor':<25} {speedup:>12.2f}x")
print(f"{'Parallel Efficiency':<25} {efficiency:>11.1f}%")
print(f"{'Time Reduction':<25} {(time_saved/sequential_time*100):>11.1f}%")

print("\n" + "="*70)

# Key findings
print("\n💡 KEY FINDINGS\n")
print(f"✓ Achieved {speedup:.2f}x speedup with 3 mappers")
print(f"✓ Parallel efficiency of {efficiency:.1f}% (excellent!)")
print(f"✓ Reduced execution time by {(time_saved/sequential_time*100):.1f}%")
print(f"✓ Demonstrates effective distributed processing")

if speedup < 2.5:
    print(f"\n⚠️  Speedup below 2.5x may be due to:")
    print("   - Network latency")
    print("   - S3 I/O overhead")
    print("   - Sequential bottlenecks (split/reduce)")
elif speedup > 2.7:
    print(f"\n🎉 Excellent speedup! Close to ideal linear scaling!")

print("\n" + "="*70 + "\n")

# Save data
with open('performance_summary.txt', 'w') as f:
    f.write(f"MapReduce Performance Test Results\n")
    f.write(f"="*40 + "\n\n")
    f.write(f"Sequential: {sequential_time:.2f}s\n")
    f.write(f"Parallel: {parallel_time:.2f}s\n")
    f.write(f"Speedup: {speedup:.2f}x\n")
    f.write(f"Efficiency: {efficiency:.1f}%\n")

print("✅ Summary saved to 'performance_summary.txt'")
