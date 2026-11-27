output "instance_public_ip" {
  value       = aws_instance.this.public_ip
  description = "SSH to ubuntu@<this>. REST API: http://<this>:8080/v1/claims. Prometheus: http://<this>:9090."
}

output "instance_id" {
  value = aws_instance.this.id
}

output "ssh_command" {
  value = "ssh ubuntu@${aws_instance.this.public_ip}"
}
