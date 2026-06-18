# WebRTC Proxy Lab Runbook

This runbook describes how to verify the bounded WebRTC proxy path locally, across two devices, and against DNS-filter controls.

Terminology:

- **Proxy host**: runs `broker` and `proxy`.
- **Monitored client**: runs `webclient` or `browserui`.
- **Controlled target**: an owned or authorized site/server used for testing.

## Roles

- `broker`: HTTP SDP signaling service.
- `proxy`: WebRTC proxy role backed by `cmd/relay`.
- `webclient`: CLI request client.
- `browserui`: browser-based viewer using native browser WebRTC.
- `target`: local controlled HTTP target.

## Local Verification

Use this first to confirm the code works on one machine.

Windows:

```powershell
Unblock-File .\scripts\run-lab.ps1
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role proxy-local -Session proxy-local -NoStun
```

Linux:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py proxy-local --session proxy-local --no-stun
```

Expected logs:

```text
stored offer for session "proxy-local"
stored answer for session "proxy-local"
proxy data channel "lab-proxy" open
proxy request id=proxy-001
target hit
proxy response id=proxy-001 status=200
```

## Two-Device Browser Viewer Test

Use this setup for DNS-filter and content-filter observations.

### Proxy Host

Build:

```powershell
git pull
.\scripts\run-lab.ps1 -Role build
```

Start broker:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
```

Start proxy:

```powershell
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session browser-test -TargetUrl https://controlled-target.example -MaxBody 1048576
```

### Monitored Client

Build:

```powershell
git pull
.\scripts\run-lab.ps1 -Role build
```

Verify broker reachability:

```powershell
Invoke-WebRequest -Uri http://SERVER_IP:8080/healthz -UseBasicParsing
```

Start browser UI:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://SERVER_IP:8080 -Session browser-test -TargetUrl https://controlled-target.example -UiListen 127.0.0.1:7777
```

Open:

```text
http://127.0.0.1:7777
```

Request:

```text
/
/robots.txt
https://controlled-target.example/
https://controlled-target.example/robots.txt
```

Expected browser UI logs:

```text
posted SDP offer to broker
applied SDP answer from proxy
proxy data channel "lab-proxy" open
proxy response id=browser-001 status=200
```

Expected proxy logs:

```text
proxy data channel "lab-proxy" created by client
proxy request id=browser-001 method=GET target=https://controlled-target.example/
```

## DNS-Filter Test Method

Recommended target:

```text
blocked-test.yourdomain.example
```

Use a controlled domain so server-side logs can confirm source IP and request timing.

1. Put the monitored client in the DNS-filter policy.
2. Block the controlled target domain in that policy.
3. Keep the proxy host outside that policy, or explicitly allow it to resolve/fetch the target.
4. Verify direct access from the monitored client is blocked.
5. Run the browser viewer through the WebRTC proxy.
6. Compare DNS logs, proxy logs, and target logs.

Evidence to collect:

- Direct client access blocked by the DNS filter.
- DNS-filter logs for the direct blocked request.
- Browser UI logs showing DataChannel connection and `status=200`.
- Proxy logs showing the target URL.
- Target logs showing the proxy host as requester.

Expected interpretation:

```text
Client-side DNS filtering did not observe the final target domain when the monitored client never resolved that domain and requested it through a separate WebRTC proxy host.
```

## Off-LAN Test

An off-LAN test validates real NAT, routing, firewall, STUN, and UDP behavior.

For detailed remote-host commands and failure triage, see [REMOTE_TEST.md](REMOTE_TEST.md).

Recommended topology:

```text
[Monitored Client] -- HTTP signaling --> [Public Proxy Host TCP 8080]
[Monitored Client] == WebRTC/UDP ==> [Public Proxy Host]
[Public Proxy Host] -- HTTPS --> [Controlled Target]
```

Proxy host requirements:

- Inbound TCP `8080` for the broker.
- Inbound UDP for WebRTC/ICE. Use `UDP 40000` with `-IcePortMin 40000 -IcePortMax 40000` for a narrow router rule.
- Outbound HTTPS to the controlled target.
- Outbound STUN from the monitored client, unless testing without STUN.

Recommended temporary forwards to the proxy host:

```text
TCP 8080
UDP 40000
```

## STUN And Failure Modes

STUN guidance:

| Scenario | STUN Needed? | Notes |
| --- | --- | --- |
| Same host | No | Use `-NoStun`. |
| Same LAN, UDP allowed | Usually no | Host candidates often work. |
| Different NATed networks | Usually yes | STUN exposes public-facing candidates. |
| UDP blocked | No | STUN cannot fix blocked UDP. TURN is required. |

Common outcomes:

- Broker unreachable: TCP `8080`, host firewall, security group, or wrong IP.
- Broker works but ICE fails: UDP/NAT/STUN issue.
- WebRTC connects but target fails: proxy target URL, proxy egress, or target-side block.
- `status=200` with sparse viewer output: target page likely depends on scripts, frames, or external assets disabled by the sanitized viewer.
- Target sees client directly: test isolation error or direct browsing/curling from the client.

## Claim Boundary

Do not overstate results. A successful DNS-filter test shows that a DNS filter on the monitored client did not see a destination resolved by the proxy host. It does not prove invisibility or bypass of firewall, EDR, NDR, TLS inspection, or WebRTC-aware controls.

Use only in owned or explicitly authorized environments.

If endpoint tooling flags the binaries or wrapper scripts, see [DETECTION_NOTES.md](DETECTION_NOTES.md) for expected reasons and lab-safe handling.
