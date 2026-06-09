# Local Verification Guide

This guide shows how to run the lab in a controlled environment and verify that the test client connects to the command-and-control listener emulator.

In this document, "victim" means a test VM or lab workstation that you own and are authorized to monitor. The client is a simulator: it does not execute commands, collect files, install persistence, hide itself, steal credentials, or bypass security tools. It only produces C2-shaped WebRTC telemetry for defensive research.

## What You Are Running

The lab has three roles:

- `broker`: HTTP signaling service used to exchange WebRTC SDP offers and answers.
- `listener`: command-and-control listener emulator that receives lab beacons and sends simulated tasks.
- `client`: victim/test-client simulator that opens a WebRTC DataChannel and sends synthetic beacons/results.

The connection has three observable phases:

1. The client posts an SDP offer to the broker.
2. The listener reads the offer, posts an SDP answer, and ICE/STUN negotiation starts.
3. The client and listener open a WebRTC DataChannel and exchange `LAB_HELLO`, `LAB_BEACON`, `LAB_ACK`, `LAB_TASK`, `LAB_RESULT`, and optional `LAB_CHUNK` messages.

## Option 1: One Windows Machine

Use this to prove the application flow works before moving to separate machines.

From the repository root:

```powershell
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role local -Session lab-local
```

The `local` role opens three PowerShell windows:

- broker
- listener
- client

Expected proof that it works:

- Broker window shows `stored offer for session "lab-local"`.
- Broker window shows `stored answer for session "lab-local"`.
- Listener window shows `data channel "lab-beacon" created by client`.
- Listener window shows `received lab beacon`.
- Client window shows `data channel "lab-beacon" open`.
- Client window shows `listener response: LAB_ACK`.

The local run is useful for functional verification, but it mostly uses loopback traffic. Use the split-machine setup below when you want network telemetry that looks like a client connecting to an external listener.

## Option 2: One Linux Machine

From the repository root:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py local --session lab-local
```

The Python runner starts broker, listener, and client in the same terminal. Because the default client sends a counted run, the runner stops the background broker and listener after the client finishes.

Expected proof that it works:

- The broker logs show `stored offer` and `stored answer`.
- The listener logs show `received lab beacon`.
- The client logs show `ICE connection state: connected` or `peer connection state: connected`.
- The client logs show `listener response: LAB_ACK`.

## Option 3: Split Lab with C2 Server and Victim Simulator

This is the setup to use for a technical article or monitoring experiment.

Example topology:

```text
[Victim/Test Client] ---> HTTP signaling ---> [C2 Server: broker]
[Victim/Test Client] <== WebRTC DataChannel ==> [C2 Server: listener]
```

Use two machines:

- C2 server emulator: runs `broker` and `listener`.
- Victim/test-client simulator: runs `client`.

Replace `C2_SERVER_IP` with the server's reachable IP address.

### Step 1: Start the C2 Broker

On the C2 server:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role broker -Listen :8080
```

Linux:

```bash
python3 scripts/run_lab.py broker --listen :8080
```

Verify that the broker is reachable.

From Windows victim/test client:

```powershell
Invoke-WebRequest -Uri http://C2_SERVER_IP:8080/healthz -UseBasicParsing
```

The expected HTTP status is `204 No Content`.

From Linux victim/test client:

```bash
curl -i http://C2_SERVER_IP:8080/healthz
```

The expected HTTP status is `204 No Content`.

If this fails, fix basic routing, host firewall, or cloud security group rules before continuing. The client cannot signal WebRTC until it can reach the broker.

### Step 2: Start the C2 Listener

On the C2 server, in a second terminal:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role listener -BrokerUrl http://127.0.0.1:8080 -Session lab-split -TaskEvery 2 -SyntheticBytes 8192
```

Linux:

```bash
python3 scripts/run_lab.py listener --broker-url http://127.0.0.1:8080 --session lab-split --task-every 2 --synthetic-bytes 8192
```

Expected listener log:

```text
waiting for SDP offer at http://127.0.0.1:8080 session "lab-split"
```

### Step 3: Start the Victim/Test Client Simulator

On the victim/test-client machine:

Windows:

```powershell
.\scripts\run-lab.ps1 -Role client -BrokerUrl http://C2_SERVER_IP:8080 -Session lab-split -Interval 8s -Jitter 35 -Count 8 -TaskDelay 1s -ChunkDelay 250ms
```

Linux:

```bash
python3 scripts/run_lab.py client --broker-url http://C2_SERVER_IP:8080 --session lab-split --interval 8s --jitter 35 --count 8 --task-delay 1s --chunk-delay 250ms
```

Expected client logs:

```text
offer posted to http://C2_SERVER_IP:8080 session "lab-split"; waiting for answer
ICE connection state: connected
peer connection state: connected
data channel "lab-beacon" open
sent lab beacon #1
listener response: LAB_ACK
listener response: LAB_TASK
```

Expected C2 listener logs:

```text
data channel "lab-beacon" created by client
data channel "lab-beacon" open
received lab beacon #1
sending simulated task: LAB_TASK
received lab message: LAB_RESULT
received lab message: LAB_CHUNK
```

Expected broker logs:

```text
stored offer for session "lab-split"
stored answer for session "lab-split"
```

If you see those messages on both sides, the PoC is working.

## Verification Checklist

Use this checklist when validating the run for screenshots or article evidence.

- Broker is reachable from the test client: `/healthz` returns HTTP `204`.
- Broker records the SDP offer.
- Broker records the SDP answer.
- Listener reports a DataChannel created by the client.
- Client reports `ICE connection state: connected` or `peer connection state: connected`.
- Client reports `data channel "lab-beacon" open`.
- Listener receives at least one `LAB_BEACON`.
- Client receives at least one `LAB_ACK`.
- Listener sends at least one `LAB_TASK` when `TaskEvery` is greater than zero.
- Listener receives `LAB_RESULT`.
- Listener receives `LAB_CHUNK` when `synthetic-upload` is in the task sequence.

## Network Checks

The application logs are the best functional proof. Network tools help prove what the environment observed.

### Windows

Confirm the broker port is listening on the C2 server:

```powershell
Get-NetTCPConnection -LocalPort 8080 -State Listen
```

Check the client can reach the broker:

```powershell
Test-NetConnection C2_SERVER_IP -Port 8080
```

During a run, view active TCP and UDP entries:

```powershell
netstat -ano | findstr ":8080"
netstat -ano -p udp
```

For packet capture, use Wireshark on the client or server and start with these display filters:

```text
http
stun
udp
ip.addr == C2_SERVER_IP
```

### Linux

Confirm the broker port is listening on the C2 server:

```bash
ss -ltnp | grep ':8080'
```

Check the client can reach the broker:

```bash
curl -i http://C2_SERVER_IP:8080/healthz
```

During a run, view active TCP and UDP entries:

```bash
ss -tunap | grep -E ':8080|client|listener|broker'
```

For packet capture:

```bash
sudo tcpdump -ni any 'tcp port 8080 or udp'
```

You should see HTTP signaling on TCP `8080` and UDP traffic for WebRTC ICE/DataChannel activity. The DataChannel payload is encrypted on the wire, so verify message contents in the application logs rather than packet payloads.

## Local-Only Mode Without Public STUN

For an isolated local test, disable public STUN on both peers.

Windows:

```powershell
.\scripts\run-lab.ps1 -Role listener -BrokerUrl http://127.0.0.1:8080 -Session no-stun -NoStun
.\scripts\run-lab.ps1 -Role client -BrokerUrl http://127.0.0.1:8080 -Session no-stun -NoStun -Count 3
```

Linux:

```bash
python3 scripts/run_lab.py listener --broker-url http://127.0.0.1:8080 --session no-stun --no-stun
python3 scripts/run_lab.py client --broker-url http://127.0.0.1:8080 --session no-stun --no-stun --count 3
```

Use normal STUN for split-machine testing unless both peers can discover a direct local candidate without it.

## Troubleshooting

If the client never reaches `waiting for answer`, check:

- The broker URL is reachable from the client.
- The listener and client use the same `Session` value.
- The broker is running before the client starts.

If the answer is stored but ICE never connects, check:

- Host firewalls allow UDP between the client and listener.
- The configured STUN server is reachable, or use `NoStun` only for a local-only test.
- Both peers are on networks that allow WebRTC-style UDP traffic.

If you see `LAB_ACK` but no `LAB_TASK`, check:

- `TaskEvery` is greater than zero on the listener.
- The client sends enough beacons to reach the task cadence.
- The listener `TaskSequence` is not empty.

If you see `LAB_TASK` but no `LAB_CHUNK`, check:

- `synthetic-upload` is included in the listener task sequence.
- `SyntheticBytes` is greater than zero.
- The client did not exit before the task result finished.

## Safety Notes

Keep this lab in systems you own or have explicit permission to test. The PoC is intentionally designed as a network-behavior emulator, not an implant. Use the logs and traffic shape to validate detection ideas, not to operate against third-party systems.
