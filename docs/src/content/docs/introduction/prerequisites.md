---
title: FAQ & Prerequisites
description: prerequisites for vJailbreak
---

### Are IPs and MAC addresses persisted?
Yes, if your OpenStack network has a valid subnet range that allows the IP to be allocated, vJailbreak will create a port with the same MAC address and IP address as the source VM.

### What OS versions are supported?
We internally use virt-v2v, so all operating systems supported for conversion by virt-v2v are supported by vJailbreak. You can find a detailed list of them [here](https://libguestfs.org/virt-v2v-support.1.html#guests).

### Do I need to perform any manual steps to remove VMware Tools?
No, vJailbreak will remove them for you, with the help of virt-v2v. The process that virt-v2v uses along with alternative approaches can be found [here](https://libguestfs.org/virt-v2v.1.html#converting-a-windows-guest).

### Do I need to perform any manual steps to install drivers for Linux and Windows VMs?
No, vJailbreak will install it for you. For Windows, we allow you to specify a URL for a specific version of virtio drivers. This is useful for older Windows versions, eg. Windows Server 2012, which specifically need [v0.1.189](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/archive-virtio/virtio-win-0.1.189-1/virtio-win-0.1.189.iso) in order to work.

### How does Vjailbreak handle flavors of the vm in the target openstack environment? 
vJailbreak provides users the flexibility to assign desired OpenStack flavors to virtual machines during the migration setup. If the user specifies a flavor in the migration form, vJailbreak will honor that choice during provisioning on the target OpenStack environment.

If no flavor is explicitly chosen, vJailbreak automatically selects the most appropriate flavor based on the VM's resource requirements (We always try to find the exact match if not the next best match). In cases where no suitable flavor is found, the UI will display a warning. If the user proceeds despite the warning, the migration will fail with a clear error message indicating that a compatible flavor could not be found.

### What access do I need for my vCenter user to be able to perform this migration?
Please refer to the following table for the required privileges:

| Privilege | Description |
| --- | --- |
| `Virtual machine.Interaction` privileges: |     |
| `Virtual machine.Interaction.Power Off` | Allows powering off a powered-on virtual machine. This operation powers down the guest operating system. |
| `Virtual machine.Interaction.Power On` | Allows powering on a powered-off virtual machine and resuming a suspended virtual machine. |
| `Virtual machine.Config.ChangeTracking`| Allows enabling or disabling change tracking on a virtual machine. |
| `Virtual machine.Guest operating system management by VIX API` | Allows managing a virtual machine by the VMware VIX API. |
| `Virtual machine.Provisioning` | Note: All `Virtual machine.Provisioning` privileges are required.  |
| `Virtual machine.Provisioning.Allow disk access` | Allows opening a disk on a virtual machine for random read and write access. Used mostly for remote disk mounting. |
| `Virtual machine.Provisioning.Allow file access` | Allows operations on files associated with a virtual machine, including VMX, disks, logs, and NVRAM. |
| `Virtual machine.Provisioning.Allow read-only disk access` | Allows opening a disk on a virtual machine for random read access. Used mostly for remote disk mounting. |
| `Virtual machine.Provisioning.Allow virtual machine download` | Allows read operations on files associated with a virtual machine, including VMX, disks, logs, and NVRAM. |
| `Virtual machine.Provisioning.Allow virtual machine files upload` | Allows write operations on files associated with a virtual machine, including VMX, disks, logs, and NVRAM. |
| `Virtual machine.Provisioning.Clone template` | Allows cloning of a template. |
| `Virtual machine.Provisioning.Clone virtual machine` | Allows cloning of an existing virtual machine and allocation of resources. |
| `Virtual machine.Provisioning.Create template from virtual machine` | Allows creation of a new template from a virtual machine. |
| `Virtual machine.Provisioning.Customize guest` | Allows customization of a virtual machine’s guest operating system without moving the virtual machine. |
| `Virtual machine.Provisioning.Deploy template` | Allows deployment of a virtual machine from a template. |
| `Virtual machine.Provisioning.Mark as template` | Allows marking an existing powered-off virtual machine as a template. |
| `Virtual machine.Provisioning.Mark as virtual machine` | Allows marking an existing template as a virtual machine. |
| `Virtual machine.Provisioning.Modify customization specification` | Allows creation, modification, or deletion of customization specifications. |
| `Virtual machine.Provisioning.Promote disks` | Allows promote operations on a virtual machine’s disks. |
| `Virtual machine.Provisioning.Read customization specifications` | Allows reading a customization specification. |
| `Virtual machine.Snapshot management` privileges: |     |
| `Virtual machine.Snapshot management.Create snapshot` | Allows creation of a snapshot from the virtual machine’s current state. |
| `Virtual machine.Snapshot management.Remove Snapshot` | Allows removal of a snapshot from the snapshot history. |
| `Datastore` privileges: |     |
| `Datastore.Browse datastore` | Allows exploring the contents of a datastore. |
| `Datastore.Low level file operations` | Allows performing low-level file operations - read, write, delete, and rename - in a datastore. |
| `Sessions` privileges: |     |
| `Sessions.Validate session` | Allows verification of the validity of a session. |
| `Cryptographic` privileges: |     |
| `Cryptographic.Decrypt` | Allows decryption of an encrypted virtual machine. |
| `Cryptographic.Direct access` | Allows access to encrypted resources. |

### What ports do I need to open for vJailbreak to work?
Please refer the following table for the required ports:

| Port | Protocol | Source | Destination | Purpose |
| --- | --- | --- | --- | --- |
| 443 | TCP | PCD nodes | VMware vCenter API endpiont | VMware provider inventory<br><br>Disk transfer authentication |
| 443 | TCP | PCD nodes | VMware ESXi hosts | Disk transfer authentication |
| 902 | TCP | PCD nodes | VMware ESXi hosts | Disk transfer data copy |
| 5480 | TCP | PCD nodes | VMware vCenter API endpoint | VMware Site Recovery Manager Appliance Management Interface |


### What network connectivity do I need for vJailbreak?

<!-- The vJailbreak VM and any helper nodes must be able to resolve & connect to your VMware vCenter environment and all ESXi hosts, and must be able to resolve & connect to [quay.io](https://quay.io). -->

The vJailbreak VM and any helper nodes must be able to resolve and connect to the following:

- **vCenter, ESXi, and OpenStack API endpoints** — required for API communication.
- **Cloud-init certificate endpoints**:
- [`https://<FQDN>:443`](https://<FQDN>) — the FQDN is typically the hostname or IP of the VM where vJailbreak is deployed, used to retrieve certificates during cloud-init.
- **Virtio ISO download source**:
  - [https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso)
- **Health-check endpoints on migrated guest VMs** — over user-defined HTTP/HTTPS ports.
- **External tooling sources**:
  - [https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml](https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml)
  - [https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml](https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml)
- **K3s installation sources** — includes scripts and supporting binaries fetched from:
  - [https://get.k3s.io](https://get.k3s.io)
  - [https://github.com/k3s-io/k3s](https://github.com/k3s-io/k3s)
  - [https://update.k3s.io](https://update.k3s.io)
  - [https://github.com/rancher/k3s-root](https://github.com/rancher/k3s-root)
- **Helm chart repository for NGINX ingress** — used during setup:
  - [https://kubernetes.github.io/ingress-nginx](https://kubernetes.github.io/ingress-nginx)
- **Container registries required to pull images** — needed for K3s, vJailbreak components (controller, UI), Prometheus, Grafana, CoreDNS, NGINX ingress, exporters, etc.:
  - [https://docker.io](https://docker.io)
  - [https://ghcr.io](https://ghcr.io)
  - [https://quay.io](https://quay.io)
  - [https://registry.k8s.io](https://registry.k8s.io)
- **ICMP (ping) access to guest VM IPs** — for connectivity verification




### Required Ingress Rules for Kubernetes Node with Kubelet, Metrics Server, and Prometheus

| **Component**      | **Port**  | **Protocol** | **Source** | **Purpose** |
|--------------------|----------|-------------|------------|-------------|
| **Kubelet API**    | 10250   | TCP         | Control Plane / Prometheus | Health checks, logs, metrics |
| **Kubelet Read-Only (Optional)** | 10255 | TCP | Internal Only | Deprecated but might be used in some cases |
| **Metrics Server** | 4443    | TCP         | Internal Cluster | K8s resource metrics (`kubectl top`) |
| **Prometheus**     | 9090    | TCP         | Internal Cluster / Monitoring Server | Prometheus UI and API |
| **Node Exporter** (if used) | 9100 | TCP | Prometheus | Node-level metrics |
| **Cadvisor (Optional)** | 4194 | TCP | Internal Cluster / Prometheus | Container metrics collection |