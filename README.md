# Snowflake Protocol WebRTC Lab

This repository contains a benign WebRTC DataChannel lab for observing signaling, ICE/STUN negotiation, and encrypted peer-to-peer heartbeat traffic in a monitored network. It is intended for defensive testing and lab documentation only.

The harness has three pieces:

- `cmd/broker`: an explicit HTTP signaling broker that stores SDP offers and answers for a named lab session.
- `cmd/listener`: the external listener emulator. It waits for an offer, answers it, receives heartbeats, and replies with acknowledgements.
- `cmd/client`: the internal client emulator. It creates a WebRTC DataChannel and sends periodic benign heartbeat strings.

The broker intentionally uses ordinary, visible HTTP signaling. It does not implement domain fronting, SNI manipulation, covert routing, payload execution, persistence, or command execution.

## Prerequisites

- Go 1.22 or newer.
- Network egress that allows the configured STUN server if you use the default `stun:stun.l.google.com:19302`.
- Two hosts or two terminals on one host for a local-only first run.

The project uses Pion WebRTC v3.3.6, the current tagged v3 package shown by Go package documentation as of June 2026.

## Build

If Go is installed locally:

```powershell
go mod tidy
go test ./...
go build -o bin ./cmd/...
```

If Go is not installed, build with Docker from the repository root:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.22 go test ./...
docker run --rm -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 -v "${PWD}:/src" -w /src golang:1.22 go build -o bin ./cmd/...
```

The Windows build produces:

- `bin/broker.exe`
- `bin/listener.exe`
- `bin/client.exe`

## Easy Runners

Windows PowerShell:

```powershell
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role local
```

The `local` role opens broker, listener, and client in separate PowerShell windows with safe C2-shaped defaults. To run a single role:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
.\scripts\run-lab.ps1 -Role listener -BrokerUrl http://127.0.0.1:8080 -Session lab-demo
.\scripts\run-lab.ps1 -Role client -BrokerUrl http://127.0.0.1:8080 -Session lab-demo -Count 8
```

Linux Python:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py local
```

The Linux runner keeps all local demo processes attached to the same terminal and stops broker/listener when the counted client run finishes. To run a single role:

```bash
python3 scripts/run_lab.py broker --listen :8080
python3 scripts/run_lab.py listener --broker-url http://127.0.0.1:8080 --session lab-demo
python3 scripts/run_lab.py client --broker-url http://127.0.0.1:8080 --session lab-demo --count 8
```

Both runners expose the same lab knobs: session id, STUN disablement, beacon interval, jitter, task cadence, synthetic upload size, chunk size, and task delay. Use `-NoStun` in PowerShell or `--no-stun` in Python for a local-only run.

## Local Run

Use three terminals from the repository root.

Terminal 1:

```powershell
go run ./cmd/broker -listen :8080
```

Or with the compiled binary:

```powershell
.\bin\broker.exe -listen :8080
```

Terminal 2:

```powershell
go run ./cmd/listener -broker http://127.0.0.1:8080 -session lab-1
```

Or with the compiled binary:

```powershell
.\bin\listener.exe -broker http://127.0.0.1:8080 -session lab-1
```

Terminal 3:

```powershell
go run ./cmd/client -broker http://127.0.0.1:8080 -session lab-1 -interval 10s -count 6
```

Or with the compiled binary:

```powershell
.\bin\client.exe -broker http://127.0.0.1:8080 -session lab-1 -interval 10s -count 6
```

For a same-LAN or two-host test, run the broker and listener where the monitored client can reach the broker URL. Then point the client at that broker:

```powershell
go run ./cmd/client -broker http://BROKER_HOST:8080 -session lab-1
```

To avoid public STUN during a local-only test, pass an empty STUN value to both peers:

```powershell
go run ./cmd/listener -broker http://127.0.0.1:8080 -session local-only -stun ""
go run ./cmd/client -broker http://127.0.0.1:8080 -session local-only -stun "" -count 3
```

## Safe Malware-Like Emulation

The lab can produce C2-shaped traffic without performing real host actions:

- Jittered recurring beacons with a fake host identifier.
- A fake initial check-in containing synthetic user, OS, and profile fields.
- Listener-issued simulated tasks every N beacons.
- Client-side simulated task results after a delay.
- Synthetic chunk uploads that generate payload-shaped traffic without reading local files.

Example:

```powershell
go run ./cmd/listener -broker http://127.0.0.1:8080 -session lab-2 -task-every 2 -task-sequence sleep,inventory,synthetic-upload -synthetic-bytes 8192 -chunk-bytes 1024
go run ./cmd/client -broker http://127.0.0.1:8080 -session lab-2 -interval 8s -jitter 35 -count 8 -task-delay 1s -chunk-delay 250ms
```

This intentionally does not execute shell commands, collect credentials, read files, install persistence, hide processes, or bypass security controls. It only emulates the timing and message shapes needed for network monitoring research.

## Expected Telemetry

You should see three phases:

1. Signaling: HTTP requests to the broker paths `/sessions/{session}/offer` and `/sessions/{session}/answer`.
2. ICE/STUN: UDP traffic for candidate gathering and connectivity checks. With the default configuration, this includes STUN traffic to `stun.l.google.com:19302`.
3. DataChannel traffic: encrypted peer-to-peer WebRTC traffic carrying `LAB_HELLO`, `LAB_BEACON`, `LAB_ACK`, `LAB_TASK`, `LAB_RESULT`, and optional `LAB_CHUNK` messages.

Useful log lines include:

- `stored offer for session`
- `stored answer for session`
- `ICE connection state: connected`
- `data channel "lab-beacon" open`
- `sent lab heartbeat`
- `received lab heartbeat`

## Defensive Analysis Notes

This harness helps compare firewall, proxy, EDR, and packet telemetry across the three observable phases without introducing malware-like behavior. For a report, capture:

- Broker access logs and TLS/proxy metadata if you put the broker behind HTTPS.
- DNS and UDP telemetry for the configured STUN server.
- Peer-to-peer UDP flow duration, packet size distribution, and cadence.
- DataChannel heartbeat intervals and total message count.

The main defensive takeaway to test is whether monitoring can distinguish short-lived interactive WebRTC use from long-duration, low-bandwidth DataChannel sessions with regular heartbeat timing.
