# WebRTC Relay Verification Guide

This guide explains how to run the Snowflake-inspired relay lab and verify whether a monitored test client can reach an owned destination through a WebRTC relay.

In this document, "victim" means a test VM or lab workstation that you own and are authorized to monitor. The client is a simulator. The relay is bounded to a configured target URL and is not an open proxy.

## What This Tests

The relay experiment answers this question:

> Can a test client reach a controlled website/server through WebRTC relay traffic instead of connecting to that destination directly, and what do content/security controls observe?

Expected path:

```text
[Victim/Test Client] -- HTTP signaling --> [Broker]
[Victim/Test Client] == WebRTC DataChannel ==> [Relay]
[Relay] -- HTTP/HTTPS --> [Controlled Target]
```

The client-side firewall can still see broker traffic and WebRTC/UDP to the relay. It should not see a direct client connection to the controlled target if the test is working.

## Roles

- `broker`: HTTP signaling broker for SDP offer/answer exchange.
- `relay`: WebRTC peer that receives bounded relay requests and forwards them to one configured target URL.
- `webclient`: test client that sends synthetic HTTP requests over the DataChannel.
- `target`: controlled HTTP server that logs relayed requests.

The older `listener` and `client` roles are still present for direct C2-like beacon testing, but they are not the main relay experiment.

## Do You Need External STUN?

Not always. STUN helps WebRTC peers discover public-facing candidates for NAT traversal. It does not carry the relayed HTTP request.

| Scenario | External STUN needed? | Notes |
| --- | --- | --- |
| Broker, relay, webclient, and target on the same host | No | Use `-NoStun` or `--no-stun`. |
| Webclient and relay on the same LAN with direct UDP allowed | Usually no | Host candidates are often enough. |
| Webclient and relay on different NATed networks | Usually yes | STUN helps peers discover public candidates. |
| Strict enterprise network | Maybe blocked | Public STUN and arbitrary peer UDP may be denied. |
| HTTP signaling works but UDP is blocked | STUN will not fix it | You need UDP allowed or a TURN relay. |

If signaling succeeds but ICE never connects, record that as a defensive result. It means rendezvous was allowed but WebRTC tunnel establishment was blocked.

## Local Windows Verification

From the repository root:

```powershell
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role relay-local -Session relay-local -NoStun
```

This opens four PowerShell windows:

- broker
- target
- relay
- webclient

Success indicators:

- Broker logs `stored offer for session "relay-local"`.
- Broker logs `stored answer for session "relay-local"`.
- Relay logs `relay data channel "lab-relay" open`.
- Relay logs `relay request id=relay-001`.
- Target logs `target hit`.
- Webclient logs `relay response id=relay-001 status=200`.

## Local Linux Verification

From the repository root:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py relay-local --session relay-local --no-stun
```

The Linux runner starts all relay components in the same terminal and stops broker, relay, and target after the webclient exits.

Success indicators are the same as the Windows local test.

## Split Lab Verification

Use this setup to test from a monitored client network.

Example:

```text
[Monitored Test Client] --> broker on SERVER_IP:8080
[Monitored Test Client] == WebRTC/UDP ==> relay on SERVER_IP
[Relay] --> target on 127.0.0.1:9090 or another owned target
```

Replace `SERVER_IP` with the server's reachable IP address.

### Step 1: Start Broker

On the server:

```bash
python3 scripts/run_lab.py broker --listen :8080
```

Verify from the monitored client:

```bash
curl -i http://SERVER_IP:8080/healthz
```

Expected status:

```text
HTTP/1.1 204 No Content
```

Windows check:

```powershell
Invoke-WebRequest -Uri http://SERVER_IP:8080/healthz -UseBasicParsing
```

### Step 2: Start Controlled Target

On the server:

```bash
python3 scripts/run_lab.py target --target-listen :9090
```

Verify locally on the server:

```bash
curl -i http://127.0.0.1:9090/healthz
```

### Step 3: Start Relay

On the server:

```bash
python3 scripts/run_lab.py relay --broker-url http://127.0.0.1:8080 --session relay-split --target-url http://127.0.0.1:9090
```

Expected relay log:

```text
relay waiting for SDP offer at http://127.0.0.1:8080 session "relay-split"
```

### Step 4: Start WebRTC Client

On the monitored test client:

```bash
python3 scripts/run_lab.py webclient --broker-url http://SERVER_IP:8080 --session relay-split --paths "/,/healthz,/article-proof?via=webrtc"
```

Windows equivalent:

```powershell
.\scripts\run-lab.ps1 -Role webclient -BrokerUrl http://SERVER_IP:8080 -Session relay-split -Paths "/,/healthz,/article-proof?via=webrtc"
```

Expected webclient logs:

```text
offer posted to http://SERVER_IP:8080 session "relay-split"; waiting for relay answer
ICE connection state: connected
peer connection state: connected
relay data channel "lab-relay" open
sent relay request id=relay-001 method=GET path=/
relay response id=relay-001 status=200
```

Expected relay logs:

```text
relay data channel "lab-relay" created by client
relay data channel "lab-relay" open
relay request id=relay-001 method=GET target=http://127.0.0.1:9090/
```

Expected target logs:

```text
target hit: method=GET path=/ request_id=relay-001
target hit: method=GET path=/article-proof?via=webrtc request_id=relay-003
```

If those logs appear, the relay path is working.

## What To Observe

On the monitored client network:

- HTTP signaling to `SERVER_IP:8080`.
- WebRTC/UDP traffic to the relay.
- STUN traffic if STUN is enabled.
- No direct client connection to the controlled target if the target is only reachable from the relay side.

On the target server:

- Requests should come from the relay host.
- The `X-WebRTC-Relay-Lab: true` header marks relayed lab requests.
- The `X-Relay-Request-ID` header maps target hits back to webclient logs.

On the broker:

- `stored offer`
- `stored answer`

On the relay:

- `ICE connection state: connected`
- `relay data channel "lab-relay" open`
- `relay request id=... target=...`

## Network Checks

Windows client:

```powershell
Test-NetConnection SERVER_IP -Port 8080
netstat -ano | findstr ":8080"
netstat -ano -p udp
```

Linux client:

```bash
curl -i http://SERVER_IP:8080/healthz
ss -tunap | grep -E ':8080|webclient|relay'
```

Packet capture examples:

```bash
sudo tcpdump -ni any 'tcp port 8080 or udp'
```

Wireshark display filters:

```text
http
stun
udp
ip.addr == SERVER_IP
```

The DataChannel payload is encrypted on the wire, so use application logs for message contents.

## Failure Modes

Broker receives no offer:

- The client cannot reach `http://SERVER_IP:8080`.
- The broker is not running.
- Host firewall or cloud security group blocks TCP `8080`.

Broker stores offer and answer, but WebRTC never connects:

- UDP between client and relay is blocked.
- Public STUN is blocked or unreachable.
- Both peers are behind NATs that need TURN.

WebRTC connects, but target receives no request:

- Relay `-target-url` is wrong.
- Target is not reachable from the relay host.
- Client paths are invalid.

Target receives requests directly from the client:

- The test is not isolated correctly.
- You are also browsing or curling the target directly from the client.
- Use a target only reachable from the relay side if you need stronger proof.

## Direct Beacon Comparison

For direct C2-shaped telemetry, use:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role local
```

Linux:

```bash
python3 scripts/run_lab.py local
```

That mode connects the client directly to a listener over WebRTC and emits beacon/task messages. It does not test Snowflake-style relay indirection.

## Safety Notes

Run this only in environments you own or have explicit permission to test. Keep the relay target bounded to systems you control. Do not modify the relay into an unrestricted proxy for third-party destinations.
