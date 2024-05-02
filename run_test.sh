#!/bin/bash

set -e

go build -o terraform-provider-ansible
cd test
export TF_CLI_CONFIG_FILE="$(pwd)/ansible-dev.tfrc"
terraform apply --auto-approve