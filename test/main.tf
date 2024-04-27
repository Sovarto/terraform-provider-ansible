terraform {
  required_providers {
    ansible = {
      version = "~> 2.0.7"
      source  = "sovarto/ansible"
    }
  }
}

resource "ansible_playbook" "hello_world" {
  playbook= "hello_world.yml"
  inventory=<<INV
  test:
    hosts:
      abc:
      localhost:
  INV
}