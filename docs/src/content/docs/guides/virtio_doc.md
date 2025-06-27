--- 
title: User-Provided VirtIO Windows Driver Support
description: Adds support for user-uploaded virtio-win.iso files used during Windows VM migrations. If the ISO is present at /home/ubuntu/virtio-win/virtio-win.iso, it is used directly and propagated to agents. If missing, vJailbreak attempts to download it. Migration fails gracefully if both methods are unavailable.
---

:::note
This feature is available from vJailbreak v0.2.0 and later.
:::

## How to use user-provided virtio-win.iso

Users can upload the `virtio-win.iso` to the following path on vJailbreak master node:

```bash
/home/ubuntu/virtio-win/virtio-win.iso
```

:::note
In vjailbreak VM there is already a folder named `virtio-win` at `/home/ubuntu/`. Please make sure to upload to that directory and the name of the file should be `virtio-win.iso`.
:::


## How it works

If the user has scaled up vJailbreak, the ISO is propagated to all the agents. 
When a Windows VM migration is initiated: 

The migration logic checks for `/home/ubuntu/virtio-win/virtio-win.iso` on the source node.

- **If found:**
  - The ISO is used for injecting **VirtIO drivers** into the migrated disk.
  - The ISO is **automatically propagated** to all agent nodes if needed.
- **If not found:**
  - vJailbreak attempts to **download** the ISO from a known upstream source (e.g., [fedoraproject.org](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/)).
- **If both methods fail:**
  - Migration fails gracefully with a clear error message.







