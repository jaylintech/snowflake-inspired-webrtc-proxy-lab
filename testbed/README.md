# Part 2 Testbed

This directory holds sanitized, reusable configuration and procedures for the authorized TLS-inspection and TURN measurements. Do not commit private keys, CA material, TURN credentials, broker tokens, packet captures, or production addresses.

## TURN and Signaling Environment

Set the same TURN values on both peers. `LAB_ICE_POLICY=relay` is required for forced-TURN test cases; leaving it unset or setting it to `all` permits direct candidates.

```powershell
$TurnTLSHost = "192.0.2.10" # IP SAN from the lab certificate, or a controlled resolvable DNS name.
$TurnTLSPort = 443 # Same value as TURN_TLS_HOST_PORT in testbed/.env.
$env:LAB_TURN_URLS = "turns:${TurnTLSHost}:${TurnTLSPort}?transport=tcp"
$env:LAB_TURN_USERNAME = "temporary-lab-user"
$env:LAB_TURN_CREDENTIAL = "temporary-lab-password"
$env:LAB_ICE_POLICY = "relay"
$env:LAB_BROKER_TOKEN = "temporary-high-entropy-token"
```

The Go peers read these variables automatically. `cmd/browserui` embeds them in the page served by its local HTTP listener, so keep the default loopback bind and do not expose that UI to another host.

Run the broker with bounded session retention:

```powershell
go run ./cmd/broker -listen :8080 -session-ttl 15m -cleanup-interval 1m
```

Use dedicated temporary credentials for each measurement window and clear them afterward:

```powershell
Remove-Item Env:LAB_TURN_URLS,Env:LAB_TURN_USERNAME,Env:LAB_TURN_CREDENTIAL,Env:LAB_ICE_POLICY,Env:LAB_BROKER_TOKEN -ErrorAction SilentlyContinue
```

## Testbed Rules

- Keep the relay configured to one owned or explicitly authorized target.
- Keep Coturn's default-deny peer rules enabled and allow only the remote relay-candidate IP or range needed by the run.
- Record the selected ICE candidate pair; forced relay must be verified, not assumed.
- Establish an ordinary HTTPS bump baseline before testing WebRTC/TURN.
- Capture on clearly named legs and synchronize clocks.
- Use synthetic identities and content only.
- Keep raw evidence outside the repository and commit hashes plus sanitized summaries.

## Implemented Testbed

- `compose.yaml`: pinned Coturn and mitmproxy services.
- `.env.example`: non-secret version and configuration template.
- `RUNBOOK.md`: certificate, startup, baseline, TURN, capture, analysis, and cleanup procedure.
- `scripts/capture-part2.ps1`: time-bounded dumpcap capture with SHA-256 manifest.
- `scripts/analyze-pcap.ps1`: network-disabled offline Suricata and Zeek execution.
- `scripts/test-config.ps1`: preflight validation for secrets, certificates, and Compose rendering.

The local mitmproxy service is an explicit HTTP(S) proxy baseline. It is not represented as an inline TURN or DTLS interception appliance.

The analyzer manifest records the configured image references together with the local image IDs and available repository digests. Version tags remain useful labels, but the recorded IDs/digests identify the exact analyzer images used for a run.
