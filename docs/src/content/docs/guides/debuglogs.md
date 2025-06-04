---
title: "vJailbreak Debug Log Collection Guide"
description: "Learn how vJailbreak collects and stores migration debug logs directly on the host."
---


This guide outlines how vJailbreak handles debug log collection for VM migrations. Traditionally, enabling debug logs required editing ConfigMaps and restarting pods. With the current setup, debug logs are automatically collected and stored without any manual intervention. In kubectl logs of the pod, normal logs will be displayed as usual. 

## How It Works

- For every migration executed via vJailbreak, a dedicated debug log file is generated.
- The log file is written to the host system at:

  `/var/log/pf9/<migration-name>.log`

- These logs are centrally accessible from the **vjailbreak node**, simplifying the debugging process.

## Log File Location

| Node Type   | Path                              | Description                               |
|------------|-----------------------------------|-------------------------------------------|
| vjailbreak-master    | `/var/log/pf9/<migration>.log`    | Centralized location for all logs         |

## Example

If a migration is named `vm-migrate-001`, its log will be available at:

`/var/log/pf9/vm-migrate-001.log` in the vjailbreak node.


