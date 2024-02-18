resource "ansible_playbook" "playbook" {
  playbook     = "playbook.yml"
  replayable   = true
  state_file   = "terraform.my-stack.tfstate"
  project_path = "cdktf.out/stacks/my-stack"

  extra_vars = {
    var_a = "Some variable"
    var_b = "Another variable"
  }
}
