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
      jsonpath = "$.plays[?(@.play.name=='Hello World Playbook')].tasks[*].hosts[*]"
    }
  }
}
