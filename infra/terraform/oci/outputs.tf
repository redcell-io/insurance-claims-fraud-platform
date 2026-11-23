output "instance_public_ip" {
  value       = data.oci_core_vnic.this.public_ip_address
  description = "SSH to ubuntu@<this>. REST API: http://<this>:8080/v1/claims. Prometheus: http://<this>:9090."
}

output "instance_ocid" {
  value = oci_core_instance.this.id
}

output "ssh_command" {
  value = "ssh ubuntu@${data.oci_core_vnic.this.public_ip_address}"
}
