#!/usr/bin/env python3
"""
switch_strategy.py — change W and R on the running AWS ECS cluster
without needing a working terraform state.

Usage:
    python3 switch_strategy.py 5 1    # W=5, R=1
    python3 switch_strategy.py 1 5    # W=1, R=5
    python3 switch_strategy.py 3 3    # W=3, R=3
"""

import json
import subprocess
import sys
import time

# ── config — matches your terraform variable defaults ─────────────────────────
CLUSTER   = "kvstore-cluster"
REGION    = "us-west-2"

# Follower ALB URLs — leader uses these to replicate writes to followers
FOLLOWER_ALBS = [
    "http://kvstore-f1-alb-1718892841.us-west-2.elb.amazonaws.com",
    "http://kvstore-f2-alb-651009101.us-west-2.elb.amazonaws.com",
    "http://kvstore-f3-alb-601209401.us-west-2.elb.amazonaws.com",
    "http://kvstore-f4-alb-1533337818.us-west-2.elb.amazonaws.com",
]
LEADER_ALB = "http://kvstore-leader-alb-439607130.us-west-2.elb.amazonaws.com"

# PEERS string for each role:
# - leader needs all 4 follower ALB URLs
# - followers need leader ALB + other 3 follower ALBs (for quorum reads)
LEADER_PEERS   = ",".join(FOLLOWER_ALBS)
FOLLOWER_PEERS = ",".join([LEADER_ALB] + FOLLOWER_ALBS)
SERVICES  = {
    "leader":    "kvstore-leader",
    "follower1": "kvstore-follower1",
    "follower2": "kvstore-follower2",
    "follower3": "kvstore-follower3",
    "follower4": "kvstore-follower4",
}
TASK_FAMILIES = {
    "leader":    "kvstore-leader",
    "follower1": "kvstore-follower1",
    "follower2": "kvstore-follower2",
    "follower3": "kvstore-follower3",
    "follower4": "kvstore-follower4",
}
# ─────────────────────────────────────────────────────────────────────────────


def aws(args: list[str]) -> dict:
    cmd = ["aws", "--region", REGION, "--output", "json"] + args
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"ERROR running: {' '.join(cmd)}")
        print(result.stderr)
        sys.exit(1)
    return json.loads(result.stdout) if result.stdout.strip() else {}


def get_task_def(family: str) -> dict:
    data = aws(["ecs", "describe-task-definition", "--task-definition", family])
    return data["taskDefinition"]


def patch_env(env_list: list[dict], updates: dict[str, str]) -> list[dict]:
    """Replace or add env vars by name."""
    result = [e for e in env_list if e["name"] not in updates]
    result += [{"name": k, "value": v} for k, v in updates.items()]
    return result


def register_new_revision(task_def: dict, env_updates: dict[str, str]) -> str:
    """Register a new task def revision with updated env vars. Returns new ARN."""
    container = task_def["containerDefinitions"][0].copy()
    container["environment"] = patch_env(
        container.get("environment", []), env_updates
    )

    # Fields required for register-task-definition.
    # runtimePlatform ARM64 is required because the image was built on
    # Apple Silicon — Fargate defaults to x86_64 which won't match the image.
    payload = {
        "family":                  task_def["family"],
        "networkMode":             task_def["networkMode"],
        "requiresCompatibilities": task_def["requiresCompatibilities"],
        "cpu":                     task_def["cpu"],
        "memory":                  task_def["memory"],
        "executionRoleArn":        task_def["executionRoleArn"],
        "taskRoleArn":             task_def.get("taskRoleArn", task_def["executionRoleArn"]),
        "containerDefinitions":    [container],
        "runtimePlatform": {
            "cpuArchitecture":       "X86_64",
            "operatingSystemFamily": "LINUX",
        },
    }

    data = aws(["ecs", "register-task-definition",
                "--cli-input-json", json.dumps(payload)])
    arn = data["taskDefinition"]["taskDefinitionArn"]
    rev = data["taskDefinition"]["revision"]
    print(f"  registered {task_def['family']}:{rev}")
    return arn


def update_service(service_name: str, task_arn: str):
    aws([
        "ecs", "update-service",
        "--cluster", CLUSTER,
        "--service", service_name,
        "--task-definition", task_arn,
        "--force-new-deployment",
    ])
    print(f"  updated service {service_name}")


def wait_stable():
    import urllib.request, urllib.error
    leader_url = "http://kvstore-leader-alb-439607130.us-west-2.elb.amazonaws.com/health"
    print("\nWaiting 30s for ECS to roll out new tasks ...", flush=True)
    time.sleep(30)
    for attempt in range(10):
        try:
            with urllib.request.urlopen(leader_url, timeout=5) as r:
                if r.status == 200:
                    print("Leader is healthy — cluster ready.")
                    return
        except Exception:
            pass
        print(f"  waiting ... ({attempt+1}/10)")
        time.sleep(10)
    print("Leader health check timed out — check AWS console if requests fail.")


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)

    W, R = sys.argv[1], sys.argv[2]
    print(f"\n=== Switching to W={W}, R={R} on cluster '{CLUSTER}' ===\n")

    for role, family in TASK_FAMILIES.items():
        print(f"[{role}]")
        task_def = get_task_def(family)
        peers = LEADER_PEERS if role == "leader" else FOLLOWER_PEERS
        arn = register_new_revision(task_def, {"W": W, "R": R, "PEERS": peers})
        update_service(SERVICES[role], arn)

    wait_stable()
    print(f"\nDone. Cluster is now running W={W}, R={R}.")


if __name__ == "__main__":
    main()
