# Snowflake-Inspired WebRTC Proxy Lab

This repository contains a controlled WebRTC proxy proof of concept for defensive testing. It evaluates whether a test client can reach an owned website or server through a WebRTC DataChannel proxy server instead of connecting to that site directly.

For a step-by-step walkthrough, see [GUIDE.md](GUIDE.md).

## Architecture

Primary proxy mode:

```text
[Test Client] -- HTTP signaling --> [Broker]
[Test Client] == WebRTC DataChannel ==> [Proxy Server]
[Proxy Server] -- HTTP/HTTPS --> [Controlled Website/Server]
```

Roles:

- `cmd/broker`: HTTP signaling service that exchanges SDP offers and answers.
- `cmd/relay`: bounded WebRTC proxy server. It accepts DataChannel requests and forwards them only to one configured target URL.
- `cmd/webclient`: test client that sends synthetic HTTP requests through the WebRTC proxy server.
- `cmd/browserui`: local browser-like viewer. It uses the browser's native WebRTC DataChannel support and renders sanitized proxy responses.
- `cmd/target`: controlled HTTP target server that logs proof of proxied access.

The `relay` name remains in the code because the implementation is a WebRTC relay internally. User-facing docs and scripts also expose `proxy` and `proxy-local` aliases.

## Safety Boundary

The proxy server is intentionally bounded. It is not an open proxy.

- The proxy server only connects to the configured `-target` base URL.
- The WebRTC client can request only relative paths, not arbitrary URLs.
- Only `GET` and `POST` are allowed in proxy mode.
- The target website/server must be one you own or are explicitly authorized to test.

This PoC does not execute commands, collect files, install persistence, steal credentials, hide processes, or bypass host controls. It tests network behavior and security-filter visibility.

## Build

Install Go 1.22 or newer, then build from the repository root.

Windows:

```powershell
.\scripts\run-lab.ps1 -Role build
```

Linux:

```bash
python3 scripts/run_lab.py build
```

Manual build:

```bash
mkdir -p bin
go mod tidy
go test ./...
go build -o bin ./cmd/...
```

## Quick Local Proxy Test

Use this only to prove the code works. It runs everything on one machine.

Windows:

```powershell
.\scripts\run-lab.ps1 -Role proxy-local -Session proxy-local -NoStun
```

Linux:

```bash
python3 scripts/run_lab.py proxy-local --session proxy-local --no-stun
```

The local test starts:

1. broker on `:8080`
2. controlled target on `:9090`
3. WebRTC proxy server pointed at `http://127.0.0.1:9090`
4. WebRTC client sending synthetic requests through the proxy server

Success indicators:

- Broker logs `stored offer` and `stored answer`.
- Proxy server logs `proxy data channel "lab-proxy" open`.
- Proxy server logs `proxy request id=... target=http://127.0.0.1:9090/...`.
- Target logs `target hit`.
- Webclient logs `proxy response id=... status=200`.

## Browser-Like Viewer

The `browserui` role serves a local page that uses the browser's native WebRTC implementation instead of the Go `webclient`. It is still the same bounded proxy path:

```text
local browser UI -> broker for signaling
local browser UI == WebRTC DataChannel ==> proxy server
proxy server -> configured target website/server
```

Device A, proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session browser-test -TargetUrl https://example.com
```

Device B, monitored client side:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://SERVER_IP:8080 -Session browser-test -TargetUrl https://example.com -UiListen 127.0.0.1:7777
```

Then open:

```text
http://127.0.0.1:7777
```

The viewer accepts relative paths such as `/` or `/robots.txt`. It also accepts full URLs under the configured target, such as `https://example.com/robots.txt`, and safely normalizes them to relative paths before sending anything over WebRTC. Returned HTML is sanitized before rendering: scripts, forms, external assets, and cross-site links are disabled so the monitored client does not directly browse the configured target site.

Large responses are split into DataChannel-safe chunks and reassembled by the viewer. The proxy still enforces the `-MaxBody` cap, which defaults to 262144 bytes.

For a one-machine browser UI check:

```powershell
.\scripts\run-lab.ps1 -Role browser-local -Session browser-local -TargetUrl http://127.0.0.1:9090 -NoStun
```

## Split Proxy Test

Use this for the meaningful content-filter/security-filter test.

Server side:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
.\scripts\run-lab.ps1 -Role target -TargetListen 127.0.0.1:9090
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session proxy-split -TargetUrl http://127.0.0.1:9090
```

Client side:

```powershell
.\scripts\run-lab.ps1 -Role webclient -BrokerUrl http://SERVER_IP:8080 -Session proxy-split -Paths "/,/healthz,/article-proof?via=webrtc"
```

Expected flow:

```text
client/webclient -> broker over HTTP
client/webclient -> proxy server over WebRTC
proxy server -> controlled website/server over HTTP
```

The monitored client should not connect directly to the controlled target. It should connect to the broker for signaling and to the proxy server over WebRTC. The proxy server then connects to the target site.

## STUN And Network Reality

You do not always need an external STUN server.

- Same host: no external STUN is needed. Use `-NoStun` or `--no-stun`.
- Same LAN with direct UDP allowed: external STUN is often not needed.
- NAT to NAT across networks: STUN is usually needed so each peer can discover public-facing candidates.
- Strict enterprise networks: public STUN may be blocked, and arbitrary peer UDP may also be blocked.

If HTTP signaling succeeds but ICE/WebRTC never connects, that is still a useful defensive result: the environment allowed rendezvous but blocked the WebRTC proxy channel at NAT traversal or UDP policy.

TURN is the normal WebRTC fallback when direct peer-to-peer connectivity fails. This PoC does not bundle TURN because direct WebRTC behavior is easier to observe and reason about.

## Direct Beacon Comparison

The older direct beacon emulator is still available for comparison:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role local
```

Linux:

```bash
python3 scripts/run_lab.py local
```

That mode connects a client directly to a listener over WebRTC and emits beacon/task messages. Use proxy mode for Snowflake-inspired content-filter testing.

## Defensive Analysis Notes

For proxy mode, compare what each control plane observes:

- Client DNS/content filter: should see broker/proxy infrastructure, not the controlled target URL directly.
- Firewall: should see HTTP signaling and WebRTC/UDP to the proxy server.
- Target server: should see requests coming from the proxy server host, not the client.
- EDR/NDR: may detect unusual DataChannel use, STUN traffic, long-lived UDP, or unexpected process behavior.

This PoC should be run only in systems you own or have explicit permission to test.
