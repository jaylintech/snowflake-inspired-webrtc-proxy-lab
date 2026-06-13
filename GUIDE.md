# WebRTC Proxy Verification Guide

This guide explains how to run the Snowflake-inspired proxy lab and verify whether a monitored test client can reach an owned destination site through a WebRTC proxy server.

In this document, "monitored test client" means a test VM or lab workstation that you own and are authorized to monitor. The client is a simulator. The proxy server is bounded to a configured target URL and is not an open proxy.

## What This Tests

The proxy experiment answers this question:

> Can a test client reach a controlled website/server through WebRTC proxy traffic instead of connecting to that destination directly, and what do content/security controls observe?

Expected path:

```text
[Monitored Test Client] -- HTTP signaling --> [Broker]
[Monitored Test Client] == WebRTC DataChannel ==> [Proxy Server]
[Proxy Server] -- HTTP/HTTPS --> [Controlled Website/Server]
```

The client-side firewall can still see broker traffic and WebRTC/UDP to the proxy server. It should not see a direct client connection to the controlled target if the test is working.

## Roles

- `broker`: HTTP signaling broker for SDP offer/answer exchange.
- `proxy`: WebRTC proxy server role. It maps to the `cmd/relay` implementation.
- `webclient`: test client that sends synthetic HTTP requests over the DataChannel.
- `browserui`: local browser-like viewer that creates a WebRTC DataChannel from the browser.
- `target`: controlled HTTP server that logs proxied requests.

The older `listener` and `client` roles are still present for direct beacon testing, but they are not the main proxy experiment.

## Do You Need External STUN?

Not always. STUN helps WebRTC peers discover public-facing candidates for NAT traversal. It does not carry the proxied HTTP request.

| Scenario | External STUN needed? | Notes |
| --- | --- | --- |
| Broker, proxy server, webclient, and target on the same host | No | Use `-NoStun` or `--no-stun`. |
| Webclient and proxy server on the same LAN with direct UDP allowed | Usually no | Host candidates are often enough. |
| Webclient and proxy server on different NATed networks | Usually yes | STUN helps peers discover public candidates. |
| Strict enterprise network | Maybe blocked | Public STUN and arbitrary peer UDP may be denied. |
| HTTP signaling works but UDP is blocked | STUN will not fix it | You need UDP allowed or a TURN relay. |

If signaling succeeds but ICE never connects, record that as a defensive result. It means rendezvous was allowed but WebRTC tunnel establishment was blocked.

## Local Windows Verification

From the repository root:

```powershell
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role proxy-local -Session proxy-local -NoStun
```

This opens four PowerShell windows:

- broker
- target
- proxy
- webclient

Success indicators:

- Broker logs `stored offer for session "proxy-local"`.
- Broker logs `stored answer for session "proxy-local"`.
- Proxy logs `proxy data channel "lab-proxy" open`.
- Proxy logs `proxy request id=proxy-001`.
- Target logs `target hit`.
- Webclient logs `proxy response id=proxy-001 status=200`.

## Local Linux Verification

From the repository root:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py proxy-local --session proxy-local --no-stun
```

The Linux runner starts all proxy components in the same terminal and stops broker, proxy, and target after the webclient exits.

## Split Lab Verification

Use this setup to test from a monitored client network.

Example:

```text
[Monitored Test Client] --> broker on SERVER_IP:8080
[Monitored Test Client] == WebRTC/UDP ==> proxy server on SERVER_IP
[Proxy Server] --> target on 127.0.0.1:9090 or another owned target
```

Replace `SERVER_IP` with the proxy server's reachable IP address.

### Step 1: Start Broker

On the proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
```

Verify from the monitored client:

```powershell
Invoke-WebRequest -Uri http://SERVER_IP:8080/healthz -UseBasicParsing
```

Expected status is `204 No Content`.

### Step 2: Start Controlled Target

On the proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role target -TargetListen 127.0.0.1:9090
```

Using `127.0.0.1` is useful because the monitored client cannot directly reach this target. If the target logs a hit, it came through the proxy server.

### Step 3: Start Proxy Server

On the proxy-server side:

```powershell
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session proxy-split -TargetUrl http://127.0.0.1:9090
```

Expected proxy log:

```text
proxy server waiting for SDP offer at http://127.0.0.1:8080 session "proxy-split"
```

### Step 4: Start WebRTC Client

On the monitored test client:

```powershell
.\scripts\run-lab.ps1 -Role webclient -BrokerUrl http://SERVER_IP:8080 -Session proxy-split -Paths "/,/healthz,/article-proof?via=webrtc"
```

Expected webclient logs:

```text
offer posted to http://SERVER_IP:8080 session "proxy-split"; waiting for proxy answer
ICE connection state: connected
peer connection state: connected
proxy data channel "lab-proxy" open
sent proxy request id=proxy-001 method=GET path=/
proxy response id=proxy-001 status=200
```

Expected proxy logs:

```text
proxy data channel "lab-proxy" created by client
proxy data channel "lab-proxy" open
proxy request id=proxy-001 method=GET target=http://127.0.0.1:9090/
```

Expected target logs:

```text
target hit: method=GET path=/ request_id=proxy-001
target hit: method=GET path=/article-proof?via=webrtc request_id=proxy-003
```

If those logs appear, the proxy path is working.

## Browser-Like Viewer Verification

The browser viewer is useful when you want something closer to browsing while still keeping the same Snowflake-inspired WebRTC proxy path. It is not a full browser isolation product. It renders sanitized HTML only, and it disables scripts, forms, external assets, and cross-site links so the monitored test client does not directly load the target site's resources.

Expected path:

```text
[Monitored Test Client Browser] -- HTTP signaling --> [Broker]
[Monitored Test Client Browser] == WebRTC DataChannel ==> [Proxy Server]
[Proxy Server] -- HTTP/HTTPS --> [Configured Target Site]
```

### Browser Viewer: Proxy-Server Side

Open one PowerShell window for the broker:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
```

Open another PowerShell window for the proxy server:

```powershell
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session browser-test -TargetUrl https://example.com
```

Replace `https://example.com` with a site or server you own or are explicitly authorized to test.

### Browser Viewer: Monitored Client Side

Start the local UI on the monitored test client:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://SERVER_IP:8080 -Session browser-test -TargetUrl https://example.com -UiListen 127.0.0.1:7777
```

Open this URL locally on the monitored test client:

```text
http://127.0.0.1:7777
```

Click `Connect`. When the DataChannel opens, the viewer automatically requests `/`. You can then enter relative paths such as:

```text
/
/robots.txt
/article-proof?via=browserui
```

You can also enter a full URL under the configured target, such as:

```text
https://example.com/
https://example.com/robots.txt
example.com/robots.txt
```

The viewer converts those to relative paths before sending the request over WebRTC. A different hostname is blocked in the UI because the proxy session is bounded to the configured target.

Expected browser UI logs:

```text
posted SDP offer to broker
applied SDP answer from proxy
proxy data channel "lab-proxy" open
sent proxy request id=browser-001 path=/
proxy response id=browser-001 status=200
```

Expected proxy-server logs:

```text
proxy data channel "lab-proxy" created by client
proxy data channel "lab-proxy" open
proxy request id=browser-001 method=GET target=https://example.com/
```

For DNS-filter testing, the monitored test client should resolve and connect to the broker/proxy infrastructure, not the configured target site. The configured target site should see the proxy-server host as the requester.

## What To Observe

On the monitored client network:

- HTTP signaling to `SERVER_IP:8080`.
- WebRTC/UDP traffic to the proxy server.
- STUN traffic if STUN is enabled.
- No direct client connection to the controlled target if the target is only reachable from the proxy-server side.

On the target server:

- Requests should come from the proxy server host.
- The `X-WebRTC-Proxy-Lab: true` header marks proxied lab requests.
- The `X-Proxy-Request-ID` header maps target hits back to webclient logs.

## Failure Modes

Broker receives no offer:

- The client cannot reach `http://SERVER_IP:8080`.
- The broker is not running.
- Host firewall or cloud security group blocks TCP `8080`.

Broker stores offer and answer, but WebRTC never connects:

- UDP between client and proxy server is blocked.
- Public STUN is blocked or unreachable.
- Both peers are behind NATs that need TURN.

WebRTC connects, but target receives no request:

- Proxy target URL is wrong.
- Target is not reachable from the proxy server host.
- Client paths are invalid.

Target receives requests directly from the client:

- The test is not isolated correctly.
- You are also browsing or curling the target directly from the client.
- Use a target only reachable from the proxy-server side if you need stronger proof.

## Safety Notes

Run this only in environments you own or have explicit permission to test. Keep the proxy target bounded to systems you control. Do not modify the proxy server into an unrestricted proxy for third-party destinations.
