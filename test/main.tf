terraform {
  required_providers {
    ansible = {
      version = "~> 2.0.9"
      source  = "sovarto/ansible"
    }
  }
}

resource "ansible_playbook" "hello_world" {
  playbook  = "hello_world.yml"
  inventory = <<INV

  INV

  artifact_queries = {
    stats2 = "$.stats"
  }
}

output "stats" {
  value = ansible_playbook.hello_world.artifact_queries_results.stats
}
