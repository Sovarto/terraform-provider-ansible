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
  group1:
    hosts:
      host1
      host2
  INV

  store_output_in_state = false

  artifact_queries = {
    failures = {
      jsonpath = "$.stats.localhost.failures"
    }
    stdout = {
      jsonpath = "$.plays[?(@.play.name=='Hello World Playbook')].tasks[*].hosts.*.msg"
      json_output = false
      fail_on_missing_key = true
    }
  }
}

output "failures" {
  value = ansible_playbook.hello_world.artifact_queries.failures.result
}

output "stdout" {
  value = ansible_playbook.hello_world.artifact_queries.stdout.result
}