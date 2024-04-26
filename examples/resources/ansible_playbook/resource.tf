resource "ansible_playbook" "playbook" {
  playbook   = "playbook.yml"
  inventory  = <<INV
  all:
  children:
    # Define server groups by region
    us_servers:
      hosts:
        server1:
          ansible_host: server1.us.example.com
          ansible_user: admin
          ansible_ssh_private_key_file: /path/to/key1
        server2:
          ansible_host: server2.us.example.com
          ansible_user: admin
          ansible_ssh_private_key_file: /path/to/key2

    eu_servers:
      hosts:
        server3:
          ansible_host: server3.eu.example.com
          ansible_user: admin
          ansible_ssh_private_key_file: /path/to/key3

    # Define groups by function
    webservers:
      hosts:
        server1: {}
        server3: {}

    dbservers:
      hosts:
        server2: {}
  INV

  extra_vars = {
    var_a = "Some variable"
    var_b = "Another variable"
  }
}
