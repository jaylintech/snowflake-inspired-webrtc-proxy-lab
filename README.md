# Snowflake-Inspired WebRTC Proxy Lab

Controlled proof of concept for testing how DNS filters, content filters, firewalls, and monitoring tools observe a bounded WebRTC proxy path.

The lab compares direct client access to a destination with access through a WebRTC DataChannel proxy host:

```text
[Test Client] -- HTTP signaling --> [Broker]
[Test Client] == WebRTC DataChannel ==> [Proxy Server]
[Proxy Server] -- HTTP/HTTPS --> [Controlled Target]
```

Additional documentation:

- [GUIDE.md](GUIDE.md): runbook and verification steps
- [REMOTE_TEST.md](REMOTE_TEST.md): off-LAN remote proxy host test guide
- [FINDINGS.md](FINDINGS.md): observed DNS-filter behavior and off-LAN test plan
- [DETECTION_NOTES.md](DETECTION_NOTES.md): why endpoint tools may flag the lab and how to handle that transparently

## Scope

This project is a defensive network-behavior lab. It is intentionally bounded:

- The proxy connects only to the configured `-TargetUrl`.
- Clients send relative-path requests under that target, including same-origin static asset requests initiated by the browser viewer.
- Proxy mode allows only `GET` and `POST`.
- Targets must be owned or explicitly authorized for testing.
- The project does not implement command execution, persistence, credential access, file collection, process hiding, or an open proxy.

## Components

- `cmd/broker`: HTTP SDP offer/answer signaling service.
- `cmd/relay`: bounded WebRTC proxy implementation. User-facing scripts expose it as `proxy`.
- `cmd/webclient`: CLI client that sends synthetic requests over the DataChannel.
- `cmd/browserui`: local browser-based viewer that creates the WebRTC DataChannel from the browser and renders sanitized responses.
- `cmd/target`: controlled HTTP target for local verification.

## Build

Windows:

```powershell
Unblock-File .\scripts\run-lab.ps1
.\scripts\run-lab.ps1 -Role build
```

Linux:

```bash
python3 scripts/run_lab.py build
```

Manual:

```bash
go mod tidy
go test ./...
go build -o bin ./cmd/...
```

## Quick Local Test

Runs broker, target, proxy, and client on one machine.

Windows:

```powershell
.\scripts\run-lab.ps1 -Role proxy-local -Session proxy-local -NoStun
```

Linux:

```bash
python3 scripts/run_lab.py proxy-local --session proxy-local --no-stun
```

Success indicators:

- Broker logs `stored offer` and `stored answer`.
- Proxy logs `proxy data channel "lab-proxy" open`.
- Proxy logs `proxy request id=... target=...`.
- Target logs `target hit`.
- Client logs `proxy response id=... status=200`.

## Browser Viewer

The browser viewer provides a lightweight "browser within a browser" experience while keeping the same bounded proxy path. It renders sanitized HTML, follows same-origin links under the configured target, and can fetch same-origin CSS/images through the DataChannel. Scripts, forms, frames, cross-site resources, and direct target-site subresource loads remain disabled.

Proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session browser-test -TargetUrl https://example.com -MaxBody 1048576
```

Client side:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://SERVER_IP:8080 -Session browser-test -TargetUrl https://example.com -UiListen 127.0.0.1:7777
```

Open on the client:

```text
http://127.0.0.1:7777
```

The viewer accepts `/`, `/robots.txt`, or full URLs under the configured target such as `https://example.com/robots.txt`. Large responses are sent in DataChannel-safe chunks and reassembled by the client. The proxy still enforces `-MaxBody`.

If a page returns `200` but has little visible sanitized content, the viewer shows a raw HTML preview. This usually means the target page depends on disabled scripts, frames, cross-site resources, service-worker behavior, or other browser features outside this bounded PoC.

## Split Test

Use this for content-filter and DNS-filter experiments.

Proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session split-test -TargetUrl https://controlled-target.example -MaxBody 1048576
```

Client side:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://SERVER_IP:8080 -Session split-test -TargetUrl https://controlled-target.example -UiListen 127.0.0.1:7777
```

Expected observation:

- Client sees broker/proxy signaling and WebRTC traffic.
- Proxy host resolves and connects to the configured target.
- Target sees the proxy host as the requester.
- Client-side DNS logs should not show the configured target if the client never resolves it directly.

## Network Notes

- Same host or same LAN: `-NoStun` may be sufficient.
- NAT-to-NAT paths usually require STUN.
- Strict networks may block public STUN or peer UDP.
- For remote router tests, forward `TCP 8080` and a fixed UDP ICE port such as `UDP 40000` to the proxy host, then run the proxy with `-IcePortMin 40000 -IcePortMax 40000 -AdvertiseIP YOUR_PUBLIC_IP`.
- If the remote firewall shows UDP checks to private addresses such as `192.168.x.x` or `172.16.x.x`, the proxy is advertising private ICE candidates instead of the public IP.
- A successful off-LAN router test used TCP `8080`, UDP `40000`, and `-AdvertiseIP`; see [FINDINGS.md](FINDINGS.md) and [REMOTE_TEST.md](REMOTE_TEST.md).
- If signaling succeeds but ICE never connects, UDP/NAT traversal is the likely failure point.
- TURN is the normal fallback for WebRTC paths where direct UDP cannot connect; this PoC does not bundle TURN.

## PoC Artifacts

The `artifacts/` directory contains templates for repeatable evidence collection:

- `artifacts/poc-report-template.md`: run summary, topology, commands, evidence, and interpretation.
- `artifacts/ssl-bump-test-checklist.md`: checklist for authorized SSL-bump/TLS-inspection lab runs.

The current viewer is more useful for static/server-rendered sites because it can fetch same-origin CSS and images through the DataChannel. It is still not a transparent browser proxy: JavaScript-heavy applications, login flows, service workers, media, fonts, WebSockets, and complex CSP behavior may not render correctly.

## Defensive Analysis

Compare these views during a test:

- Client DNS/content filter: broker/proxy infrastructure vs. final target domain.
- Firewall/NDR: HTTP signaling, STUN, and WebRTC/UDP flows.
- Target logs: proxy host IP vs. client IP.
- Endpoint tools: unusual standalone WebRTC/DataChannel process behavior.

Use only in owned or explicitly authorized environments.
