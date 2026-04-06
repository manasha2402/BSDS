#!/usr/bin/env python3
"""
deploy_leaderless.py — Deploy the leaderless KV store cluster on AWS ECS/Fargate.

Usage:
    python3 deploy_leaderless.py

Creates 5 identical nodes, each with its own ALB.
PEERS are set automatically after ALBs are created.
"""

import json
import subprocess
import sys
import time

# ── Config ─────────────────────────────────────────────────────────────────
REGION      = "us-west-2"
ACCOUNT_ID  = "992497562928"
CLUSTER     = "kvstore-ll-cluster"
ECR_REPO    = "kvstore-ll-node"
LOG_GROUP   = "/ecs/kvstore-ll"
SERVICE_NAME = "kvstore-ll"

NODES = ["node1", "node2", "node3", "node4", "node5"]

# Reuse existing networking from leader-follower deployment
SUBNETS        = ["subnet-0389fd7974caf7b77", "subnet-050c751a87b8598b5",
                  "subnet-0e2ecd99a26cc2db0", "subnet-0bda079cf2960edf8"]
NODE_SG        = "sg-0eb47fa64508cec84"   # kvstore-nodes-sg
ALB_SG         = "sg-09a99a015f3e8439e"   # kvstore-alb-sg
LAB_ROLE_ARN   = f"arn:aws:iam::{ACCOUNT_ID}:role/LabRole"
ECR_URL        = f"{ACCOUNT_ID}.dkr.ecr.{REGION}.amazonaws.com/{ECR_REPO}"
# ───────────────────────────────────────────────────────────────────────────


def run(args, check=True):
    cmd = ["aws", "--region", REGION, "--output", "json"] + args
    result = subprocess.run(cmd, capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"ERROR: {' '.join(cmd)}\n{result.stderr}")
        sys.exit(1)
    return json.loads(result.stdout) if result.stdout.strip() else {}


def sh(cmd, check=True):
    print(f"  $ {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"ERROR:\n{result.stderr}")
        sys.exit(1)
    return result.stdout.strip()


# ── Step 1: ECR ─────────────────────────────────────────────────────────────
def setup_ecr():
    print("\n[1/6] Setting up ECR...")
    existing = run(["ecr", "describe-repositories", "--repository-names", ECR_REPO], check=False)
    if not existing.get("repositories"):
        run(["ecr", "create-repository", "--repository-name", ECR_REPO,
             "--image-scanning-configuration", "scanOnPush=false"])
        print(f"  Created ECR repo: {ECR_REPO}")
    else:
        print(f"  ECR repo already exists: {ECR_REPO}")

    print("  Logging into ECR...")
    pwd = sh(f"aws ecr get-login-password --region {REGION}")
    sh(f"echo '{pwd}' | docker login --username AWS --password-stdin {ACCOUNT_ID}.dkr.ecr.{REGION}.amazonaws.com")


# ── Step 2: Build & Push ─────────────────────────────────────────────────────
def build_and_push():
    print("\n[2/6] Building and pushing leaderless image...")
    src = "../../leaderless"
    sh(f"docker build --platform linux/amd64 -t {ECR_URL}:latest {src}")
    sh(f"docker push {ECR_URL}:latest")
    print("  Image pushed.")


# ── Step 3: ECS Cluster & CloudWatch ─────────────────────────────────────────
def setup_cluster():
    print("\n[3/6] Setting up ECS cluster and CloudWatch log group...")
    run(["ecs", "create-cluster", "--cluster-name", CLUSTER], check=False)
    run(["logs", "create-log-group", "--log-group-name", LOG_GROUP], check=False)
    print(f"  Cluster: {CLUSTER}")


# ── Step 4: ALBs ─────────────────────────────────────────────────────────────
def create_albs():
    print("\n[4/6] Creating ALBs, target groups, and listeners...")
    alb_dns = {}

    for node in NODES:
        alb_name = f"{SERVICE_NAME}-{node}-alb"
        tg_name  = f"{SERVICE_NAME}-{node}-tg"

        # Create ALB
        alb = run(["elbv2", "create-load-balancer",
                   "--name", alb_name,
                   "--type", "application",
                   "--scheme", "internet-facing",
                   "--security-groups", ALB_SG,
                   "--subnets"] + SUBNETS)
        alb_arn = alb["LoadBalancers"][0]["LoadBalancerArn"]
        alb_dns[node] = "http://" + alb["LoadBalancers"][0]["DNSName"]
        print(f"  {node} ALB: {alb_dns[node]}")

        # Create target group
        tg = run(["elbv2", "create-target-group",
                  "--name", tg_name,
                  "--protocol", "HTTP",
                  "--port", "8080",
                  "--vpc-id", "vpc-04b3c2bb6e561aae4",
                  "--target-type", "ip",
                  "--health-check-path", "/health",
                  "--health-check-interval-seconds", "30",
                  "--health-check-timeout-seconds", "5",
                  "--healthy-threshold-count", "2",
                  "--unhealthy-threshold-count", "3",
                  "--matcher", "HttpCode=200"])
        tg_arn = tg["TargetGroups"][0]["TargetGroupArn"]

        # Create listener
        run(["elbv2", "create-listener",
             "--load-balancer-arn", alb_arn,
             "--protocol", "HTTP",
             "--port", "80",
             "--default-actions", f"Type=forward,TargetGroupArn={tg_arn}"])

    print("  Waiting 15s for ALBs to provision...")
    time.sleep(15)
    return alb_dns


# ── Step 5: Task Definitions ─────────────────────────────────────────────────
def register_task_defs(alb_dns):
    print("\n[5/6] Registering ECS task definitions...")
    task_arns = {}

    for node in NODES:
        peers = ",".join(v for k, v in alb_dns.items() if k != node)
        family = f"{SERVICE_NAME}-{node}"

        container = {
            "name": node,
            "image": f"{ECR_URL}:latest",
            "essential": True,
            "portMappings": [{"containerPort": 8080}],
            "environment": [
                {"name": "NODE_ID", "value": node},
                {"name": "PORT",    "value": "8080"},
                {"name": "PEERS",   "value": peers},
            ],
            "logConfiguration": {
                "logDriver": "awslogs",
                "options": {
                    "awslogs-group":         LOG_GROUP,
                    "awslogs-region":        REGION,
                    "awslogs-stream-prefix": node,
                }
            }
        }

        payload = {
            "family":                  family,
            "networkMode":             "awsvpc",
            "requiresCompatibilities": ["FARGATE"],
            "cpu":                     "256",
            "memory":                  "512",
            "executionRoleArn":        LAB_ROLE_ARN,
            "taskRoleArn":             LAB_ROLE_ARN,
            "containerDefinitions":    [container],
            "runtimePlatform": {
                "cpuArchitecture":       "X86_64",
                "operatingSystemFamily": "LINUX",
            },
        }

        data = run(["ecs", "register-task-definition",
                    "--cli-input-json", json.dumps(payload)])
        task_arns[node] = data["taskDefinition"]["taskDefinitionArn"]
        print(f"  registered {family}:{data['taskDefinition']['revision']}")

    return task_arns


# ── Step 6: ECS Services ─────────────────────────────────────────────────────
def create_services(task_arns, alb_dns):
    print("\n[6/6] Creating ECS services...")

    for node in NODES:
        tg_name = f"{SERVICE_NAME}-{node}-tg"
        tg_data = run(["elbv2", "describe-target-groups", "--names", tg_name])
        tg_arn  = tg_data["TargetGroups"][0]["TargetGroupArn"]

        run(["ecs", "create-service",
             "--cluster",         CLUSTER,
             "--service-name",    f"{SERVICE_NAME}-{node}",
             "--task-definition", task_arns[node],
             "--desired-count",   "1",
             "--launch-type",     "FARGATE",
             "--network-configuration",
             json.dumps({
                 "awsvpcConfiguration": {
                     "subnets":        SUBNETS,
                     "securityGroups": [NODE_SG],
                     "assignPublicIp": "ENABLED"
                 }
             }),
             "--load-balancers",
             json.dumps([{
                 "targetGroupArn": tg_arn,
                 "containerName":  node,
                 "containerPort":  8080
             }])])
        print(f"  created service kvstore-ll-{node}")


def wait_healthy(alb_dns):
    import urllib.request
    url = alb_dns["node1"] + "/health"
    print(f"\nWaiting for cluster to be healthy ({url})...")
    for i in range(20):
        time.sleep(15)
        try:
            with urllib.request.urlopen(url, timeout=5) as r:
                if r.status == 200:
                    print("Cluster is healthy!")
                    return
        except Exception:
            pass
        print(f"  still waiting... ({i+1}/20)")
    print("Timed out — check AWS console.")


def main():
    setup_ecr()
    build_and_push()
    setup_cluster()
    alb_dns = create_albs()
    task_arns = register_task_defs(alb_dns)
    create_services(task_arns, alb_dns)

    print("\n=== Deployment complete! ===")
    print("Node ALB URLs:")
    for node, dns in alb_dns.items():
        print(f"  {node}: {dns}")

    wait_healthy(alb_dns)

    print("\nSave these ALB URLs — you'll need them for locustfile_ll.py")
    for node, dns in alb_dns.items():
        print(f"  {dns}")


if __name__ == "__main__":
    main()
