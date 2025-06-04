---
title: "vJailbreak Installation Debug Guide"
description: "Learn how to debug installation issues related to vJailbreak, including where the install script is located, how it works, and what to check when things go wrong."
---

### 1. Check Installation Logs

All logs related to the install process are written to:

`/var/log/pf9-install.log`  

Look here for:
- Image pull errors
- Authentication issues
- YAML apply failures
- Proxy or network errors

> 🔍 **Tip:** If you're seeing errors related to pulling images, verify that the image registry URL is accessible from within the vJailbreak VM.

---

### 2. Test Registry Access (Image Pull Failures)

If the logs show image pull issues, run this on the vJailbreak VM:

```bash
curl -v <image-url>
```

🔁 What If the URL Is Accessible but Installation Still Fails?
Even if the URL is accessible, transient network issues or Kubernetes API hiccups might cause failures.

Recheck /var/log/pf9-install.log for intermittent or recoverable errors.

In such cases, you can safely re-run the installer:

```bash
sudo bash /etc/pf9/install.sh
```
