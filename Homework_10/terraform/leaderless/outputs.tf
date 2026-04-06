# All 5 node URLs — Locust can send reads and writes to any of these

output "node_urls" {
  description = "Node ALB URLs — send reads and writes to any node"
  value = [
    for lb in aws_lb.node : "http://${lb.dns_name}"
  ]
}