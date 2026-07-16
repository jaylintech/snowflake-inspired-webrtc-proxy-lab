# Part 2 Testbed

This directory holds sanitized, reusable configuration and procedures for the authorized TLS-inspection and TURN measurements. Do not commit private keys, CA material, TURN credentials, broker tokens, packet captures, or production addresses.

## TURN and Signaling Environment

Set the same TURN values on both peers. `LAB_ICE_POLICY=relay` is required for forced-TURN test cases; leaving it unset or setting it to `all` permits direct candidates.

```powershell
$env:LAB_TURN_URLS = "turns:turn.lab.example:443?transport=tcp"
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
- Record the selected ICE candidate pair; forced relay must be verified, not assumed.
- Establish an ordinary HTTPS bump baseline before testing WebRTC/TURN.
- Capture on clearly named legs and synchronize clocks.
- Use synthetic identities and content only.
- Keep raw evidence outside the repository and commit hashes plus sanitized summaries.

## Planned Config Layout

- `turn/`: sanitized TURN server templates and certificate expectations.
- `tls-inspection/`: proxy-specific templates and CA-install/cleanup notes.
- `sensors/`: capture placement and Suricata/Zeek loading instructions.

Templates will be added only after the selected lab products and versions are recorded.
