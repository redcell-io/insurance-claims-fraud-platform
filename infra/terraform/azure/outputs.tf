output "instance_public_ip" {
  value       = azurerm_public_ip.this.ip_address
  description = "SSH to ubuntu@<this>. REST API: http://<this>:8080/v1/claims. Prometheus: http://<this>:9090."
}

output "instance_id" {
  value = azurerm_linux_virtual_machine.this.id
}

output "ssh_command" {
  value = "ssh ubuntu@${azurerm_public_ip.this.ip_address}"
}
