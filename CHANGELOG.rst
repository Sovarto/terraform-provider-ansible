================================================
The Terraform Provider for Ansible Release Notes
================================================

.. contents:: Topics

v1.2.0
======

Major Changes
-------------

- Added ansible_playbook2 resource which automatically creates a dynamic inventory based on the ansible_host and ansible_group resources.

Minor Changes
-------------

- Update dependencies (google.golang.org/grpc and golang.org/x/net) to resolve security alerts https://github.com/ansible/terraform-provider-ansible/security/dependabot (https://github.com/ansible/terraform-provider-ansible/pull/72).
- Updates the provider to use SDKv2 (https://github.com/ansible/terraform-provider-ansible/issues/39).

Bugfixes
--------

- provider/resource_playbook - Fix race condition between multiple ansible_playbook resources (https://github.com/ansible/terraform-provider-ansible/issues/38).
