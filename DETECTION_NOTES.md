# Detection Notes

This project intentionally resembles a network-behavior lab, so endpoint and network tools may flag it even though it does not implement destructive capability.

Common reasons for detections:

- A standalone unsigned Go binary with little reputation.
- Browser-independent WebRTC/DataChannel networking.
- Broker, proxy, heartbeat, and task-like lab terminology.
- Synthetic request forwarding through a proxy host.
- PowerShell wrapper scripts downloaded from the internet.
- Direct execution from a downloads folder or temporary path.

The `cmd/client` and `cmd/listener` binaries deliberately generate a benign, beacon-shaped detection signal. The client sends labeled `LAB_HELLO` and jittered `LAB_BEACON` messages. The listener can request only the fixed simulated actions `sleep`, `inventory`, and `synthetic-upload`. The inventory text is hard-coded test data, and upload chunks are generated from repeated `X` characters; neither component executes commands, inventories the real host, nor reads files for upload.

The PoC is intentionally bounded:

- It only connects to the configured `-TargetUrl`.
- It does not execute operating-system commands.
- It does not collect files, credentials, browser data, or secrets.
- It does not install persistence or hide processes.
- It does not provide an unrestricted open proxy.

Recommended lab handling:

- Build from source in the lab and keep the source available for review.
- Use an owned test target and isolated session names.
- Code-sign binaries if your organization requires signed internal tooling.
- Create an allowlist or exception through the normal EDR change-control process.
- Preserve broker, proxy, client, DNS-filter, and target logs for the test report.
- Do not rename, pack, obfuscate, or otherwise modify the binaries to avoid detection.

Windows may also show a script trust warning for downloaded files because of Mark of the Web. If the file was downloaded from your own repository and reviewed, unblock it explicitly:

```powershell
Unblock-File .\scripts\run-lab.ps1
```

That removes the download marker from the script. It is not an EDR bypass and should be done only for scripts you trust.
