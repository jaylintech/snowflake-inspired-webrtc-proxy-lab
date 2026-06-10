# Snowflake-Inspired WebRTC Relay Lab

This repository contains a controlled WebRTC relay proof of concept for defensive testing. It evaluates whether a test client can reach an owned destination server through a WebRTC DataChannel relay instead of connecting to that destination directly.

For a step-by-step local and split-machine walkthrough, see [GUIDE.md](GUIDE.md).

## Architecture

Primary relay mode:

```text
[Test Client] -- HTTP signaling --> [Broker]
[Test Client] == WebRTC DataChannel ==> [Relay]
[Relay] -- HTTP/HTTPS --> [Controlled Target Server]
```

Roles:

- `cmd/broker`: HTTP signaling service that exchanges SDP offers and answers.
- `cmd/relay`: WebRTC relay that accepts DataChannel requests and forwards them only to one configured target URL.
- `cmd/webclient`: test client that sends synthetic HTTP requests through the WebRTC relay.
- `cmd/target`: controlled HTTP target server that logs proof of relayed access.

The repository also keeps the earlier direct C2-shaped beacon emulator:

- `cmd/listener`: direct WebRTC listener emulator.
- `cmd/client`: direct WebRTC beacon/tasking client emulator.

## Safety Boundary

The relay is intentionally bounded. It is not an open proxy.

- The relay only connects to the configured `-target` base URL.
- The WebRTC client can request only relative paths, not arbitrary URLs.
- Only `GET` and `POST` are allowed in relay mode.
- The target server is intended to be a system you own or are explicitly authorized to test.

This PoC does not execute commands, collect files, install persistence, steal credentials, hide processes, or bypass host controls. It tests network behavior and security-filter visibility.

## Prerequisites

- Go 1.22 or newer.
- Network egress that allows HTTP signaling from the client to the broker.
- Network egress that allows WebRTC/UDP between the client and relay, unless both peers are local.
- A controlled target server, either the included `cmd/target` server or an owned web service.

The project uses Pion WebRTC v3.3.6.

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

On Windows, the build produces:

- `bin/broker.exe`
- `bin/relay.exe`
- `bin/webclient.exe`
- `bin/target.exe`
- `bin/listener.exe`
- `bin/client.exe`

On Linux, the build produces the same names without `.exe`.

## Quick Relay Demo

Windows:

```powershell
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role relay-local -Session relay-local -NoStun
```

Linux:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py relay-local --session relay-local --no-stun
```

The relay-local mode starts:

1. broker on `:8080`
2. controlled target on `:9090`
3. WebRTC relay pointed at `http://127.0.0.1:9090`
4. WebRTC client sending synthetic requests through the relay

Success indicators:

- Broker logs `stored offer` and `stored answer`.
- Relay logs `relay data channel "lab-relay" open`.
- Relay logs `relay request id=... target=http://127.0.0.1:9090/...`.
- Target logs `target hit`.
- Webclient logs `relay response id=... status=200`.

## Split Relay Lab

Use this when you want to test content/security filtering from a monitored client network.

On the server side, run the broker, relay, and target. Replace `SERVER_IP` with the reachable server IP.

Terminal 1:

```bash
python3 scripts/run_lab.py broker --listen :8080
```

Terminal 2:

```bash
python3 scripts/run_lab.py target --target-listen :9090
```

Terminal 3:

```bash
python3 scripts/run_lab.py relay --broker-url http://127.0.0.1:8080 --session relay-split --target-url http://127.0.0.1:9090
```

On the monitored test client:

```bash
python3 scripts/run_lab.py webclient --broker-url http://SERVER_IP:8080 --session relay-split --paths "/,/healthz,/article-proof?via=webrtc"
```

Windows client equivalent:

```powershell
.\scripts\run-lab.ps1 -Role webclient -BrokerUrl http://SERVER_IP:8080 -Session relay-split -Paths "/,/healthz,/article-proof?via=webrtc"
```

The monitored client should not connect directly to the controlled target. It should connect to the broker for signaling and to the relay over WebRTC. The relay then connects to the target.

## STUN And Network Reality

You do not always need an external STUN server.

- Same host: no external STUN is needed. Use `-NoStun` or `--no-stun`.
- Same LAN with direct UDP allowed: external STUN is often not needed.
- NAT to NAT across networks: STUN is usually needed so each peer can discover public-facing candidates.
- Strict enterprise networks: public STUN may be blocked, and arbitrary peer UDP may also be blocked.

If HTTP signaling succeeds but ICE/WebRTC never connects, that is still a useful defensive result: the environment allowed rendezvous but blocked the WebRTC tunnel at NAT traversal or UDP policy.

TURN is the normal WebRTC fallback when direct peer-to-peer connectivity fails. This PoC does not bundle TURN because direct WebRTC behavior is easier to observe and reason about. Adding a TURN relay would change the telemetry because client traffic would go to the TURN server rather than directly to the relay.

## Direct Beacon Mode

The older direct C2-shaped emulator is still available for comparison:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role local
```

Linux:

```bash
python3 scripts/run_lab.py local
```

This mode uses:

```text
[Test Client] == WebRTC DataChannel ==> [Listener]
```

It emits `LAB_HELLO`, `LAB_BEACON`, `LAB_ACK`, `LAB_TASK`, `LAB_RESULT`, and optional `LAB_CHUNK` messages. Use it for direct WebRTC C2 telemetry testing. Use relay mode for Snowflake-inspired content-filter testing.

## Defensive Analysis Notes

For relay mode, compare what each control plane observes:

- Client DNS/content filter: should see broker/relay infrastructure, not the controlled target URL directly.
- Firewall: should see HTTP signaling and WebRTC/UDP to the relay.
- Target server: should see requests coming from the relay host, not the client.
- EDR/NDR: may detect unusual DataChannel use, STUN traffic, long-lived UDP, or unexpected process behavior.

This PoC should be run only in systems you own or have explicit permission to test.
