<div align="center">

# vJailbreak

A free and open-source tool that simplifies the migration of virtual machines from VMware to any OpenStack-compliant cloud.

[![Build Status](https://github.com/platform9/vjailbreak/actions/workflows/packer.yml/badge.svg)](https://github.com/platform9/vjailbreak/actions/workflows/packer.yml)
[![Latest Release](https://badgen.net/github/release/platform9/vjailbreak/latest)](https://github.com/platform9/vjailbreak/releases/latest)
[![All Releases](https://badgen.net/github/releases/platform9/vjailbreak)](https://github.com/platform9/vjailbreak/releases)
[![GitHub Stars](https://img.shields.io/github/stars/platform9/vjailbreak)](https://github.com/platform9/vjailbreak/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/platform9/vjailbreak)](https://github.com/platform9/vjailbreak/network/members)
[![Go Report Card](https://goreportcard.com/badge/github.com/platform9/vjailbreak/v2v-helper)](https://goreportcard.com/report/github.com/platform9/vjailbreak/v2v-helper)

</div>

## Overview

vJailbreak is a powerful migration tool designed to seamlessly transfer virtual machines from VMware environments to any OpenStack-compliant cloud platform. The project aims to simplify the complex process of VM migration while ensuring compatibility and performance in the target environment.

### Key Features

- **VMware Integration**: Connect directly to vCenter to access and discover source VMs

- **Automated VM Conversion**: Convert VM disks from `vmdk` to `qcow2` format and prepare for OpenStack environment

- **OpenStack Integration**: Direct deployment to any OpenStack-compliant cloud with network and storage mapping

- **Flexible Migration Options**: Support for both hot and cold migrations with customizable scheduling

- **Rolling Conversion**: Staged migration process that minimizes downtime and resource usage

- **Advanced Migration Options**: Customizable migration parameters including network configuration, resource allocation, and storage mappings

- **Health Verification**: Post-migration health checks to ensure successful migration

- **Scalable Architecture**: Scale up or down migration agents based on workload

- **Enhanced Security**: Secure handling of credentials and data throughout the migration process

## Documentation

Comprehensive documentation for vJailbreak is available on our documentation site:

[📚 vJailbreak Documentation](https://platform9.github.io/vjailbreak/introduction/getting_started/)

## Demonstration

### Video Overview

Click the image below to watch a demonstration of vJailbreak in action:

[![vJailbreak demo](https://img.youtube.com/vi/seThilJ5ujM/0.jpg)](https://www.youtube.com/watch?v=seThilJ5ujM)

## Screenshots

### Migration Setup

<details>
<summary>Click to view migration setup screenshots</summary>

#### Migration Form (Step 1)
![Migration Form Step 1](assets/migrationform1.png)

#### Migration Form (Step 2)
![Migration Form Step 2](assets/migrationform2.png)
</details>

### Migration Progress

<details>
<summary>Click to view migration progress screenshots</summary>

#### Progress View (Stage 1)
![Migration Progress View 1](assets/migrationprogress1.png)

#### Progress View (Stage 2)
![Migration Progress View 2](assets/migrationprogress2.png)
</details>

### Agent Scaling

<details>
<summary>Click to view agent scaling screenshots</summary>

#### Scaling Up
![Scale Up Interface](assets/scaleup.png)
![Scale Up Agents View](assets/scaleupagents.png)

#### Scaling Down
![Scale Down Interface](assets/scaledown.png)
</details>

## License

This project is available under the [Business Source License 1.1](LICENSE). See the LICENSE file for full details.

Key points from the license:
- Licensor: Platform9 Systems, Inc.
- Licensed Work: All versions of vJailbreak (c) 2024 Platform9 Systems, Inc.
- Change Date: Four years from the date the Licensed Work is published
- Change License: MPL 2.0

For information about alternative licensing arrangements, contact info@platform9.com.

## Installation

### Prerequisites

- **VMware Environment**:
  - VMware vCenter with appropriate permissions
  - vSphere environment accessible to vJailbreak

- **OpenStack Environment**:
  - OpenStack-compliant cloud target
  - Network and storage access

- **Network Connectivity**:
  - Access to vCenter, ESXi, and OpenStack API endpoints
  - Access to download virtio drivers and other components
  - ICMP (ping) access for connectivity verification

- **Supported OS**:
  - All operating systems supported by virt-v2v
  - See [virt-v2v support page](https://libguestfs.org/virt-v2v-support.1.html) for details

> **Note:** For an extensive and detailed list of all prerequisites, including network ports, access requirements, and FAQ, please refer to the [Prerequisites documentation](https://platform9.github.io/vjailbreak/introduction/prerequisites/).

### Quick Start

1. **Install ORAS and download vJailbreak**:
   ```bash
   # Install ORAS (see https://oras.land/docs/installation)
   
   # Download the latest vJailbreak image
   oras pull quay.io/platform9/vjailbreak:<tag>
   # This downloads the image to vjailbreak_qcow2/vjailbreak-image.qcow2
   ```

2. **Upload image and create vJailbreak VM**:
   ```bash
   # Upload the image to your OpenStack environment
   openstack image create --os-interface admin --insecure \
     --container-format bare --disk-format qcow2 \
     --file vjailbreak_qcow2/vjailbreak-image.qcow2 vjailbreak-image
   
   # Deploy a VM from this image (use UI or CLI)
   # - Choose m1.xlarge flavor (or larger for large VM migrations)
   # - Select a network that can reach your VMware vCenter
   # - Configure security group to allow required traffic
   ```

3. **Copy VDDK Libraries**:
   - Download [VDDK libraries](https://developer.broadcom.com/sdks/vmware-virtual-disk-development-kit-vddk/)
   - Copy to `/home/ubuntu` on the vJailbreak VM
   - Untar to create `vmware-vix-disklib-distrib` in `/home/ubuntu`

4. **Configure DNS Resolution**:
   - Proper DNS resolution for your VMware and OpenStack URLs is required for vJailbreak to function correctly
   
   - **Static Entries**: Adding entries to `/etc/hosts` will apply changes immediately
     ```bash
     # Example /etc/hosts entry
     192.168.1.100 vcenter.example.com
     192.168.2.100 openstack.example.com
     ```

   - **DNS Configuration**: If modifying `/etc/resolv.conf`, you must restart the controller pod for changes to take effect
     ```bash
     # After modifying resolv.conf
     kubectl -n vjailbreak rollout restart deployment migration-controller-manager
     ```

5. **Launch vJailbreak**:
   - Connect to the vJailbreak UI using the VM's IP address
   - Provide VMware vCenter and OpenStack credentials
   - Begin migrating VMs

## Usage

Refer to the [documentation](https://platform9.github.io/vjailbreak/introduction/getting_started/) for detailed usage instructions.

## Community and Support

- **GitHub Issues**: For bug reports and feature requests
- **Documentation**: [Complete vJailbreak Documentation](https://platform9.github.io/vjailbreak/)
- **Reddit Community**: Join our [r/vjailbreak](https://www.reddit.com/r/vjailbreak/) subreddit
- **Commercial Support**: For enterprise support, contact [Platform9](https://platform9.com/)

## Acknowledgements

vJailbreak builds upon several open-source technologies:

- [virt-v2v](https://github.com/virt-manager/virt-manager)
- [nbdkit](https://gitlab.com/nbdkit/nbdkit)
- [k3s](https://k3s.io/)
- [OpenStack](https://www.openstack.org/)
- [govmomi](https://github.com/vmware/govmomi)
- [virtio-win](https://github.com/virtio-win/kvm-guest-drivers-windows)

## Contributing

Contributions to vJailbreak are welcome! Please feel free to submit issues and pull requests.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please ensure your code follows the project's coding standards and includes appropriate tests.
