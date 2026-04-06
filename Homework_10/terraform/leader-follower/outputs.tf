# These URLs are what you paste into Locust after terraform apply

output "leader_url" {
  description = "Leader ALB URL — send writes here"
  value       = "http://${aws_lb.leader.dns_name}"
}

output "follower_urls" {
  description = "Follower ALB URLs — send reads here"
  value = [
    for lb in aws_lb.follower : "http://${lb.dns_name}"
  ]
}