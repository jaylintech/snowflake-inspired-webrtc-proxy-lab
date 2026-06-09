# WebRTC Data Channels as a Defensive C2 Emulation Lab

WebRTC is usually discussed in the context of browsers, voice, video, and collaboration tools. The same peer-to-peer transport pattern also makes it useful for defensive research because it produces a network shape that many organizations already allow: HTTPS signaling, ICE/STUN negotiation, and encrypted UDP peer traffic.

This article describes a benign proof of concept that emulates that network behavior without implementing malware capabilities. The goal is to help defenders collect telemetry, validate detections, and reason about WebRTC-based tunneling patterns in a controlled lab.

## Research Question

Can a monitored network reliably identify long-duration, low-bandwidth WebRTC DataChannel sessions that behave like command-and-control traffic, even when the traffic uses legitimate WebRTC primitives?

The PoC is intentionally scoped to answer that question. It models the timing and message shapes of C2-like traffic while avoiding real endpoint actions such as command execution, persistence, credential access, process hiding, filesystem collection, or security control bypass.

## Lab Architecture

The lab has three components:

- `broker`: an explicit HTTP signaling broker that exchanges SDP offers and answers.
- `listener`: an external peer that accepts the WebRTC DataChannel and returns simulated tasking.
- `client`: an internal peer that creates the DataChannel, sends beacons, and reports simulated task results.

The connection flow has three phases:

1. Signaling over HTTP: the client posts an SDP offer and the listener posts an SDP answer.
2. ICE/STUN negotiation: the peers discover viable UDP paths.
3. DataChannel exchange: the peers send encrypted WebRTC messages that mimic beaconing and tasking.

## Emulated Behaviors

The PoC includes several safe behaviors that create malware-like telemetry without becoming malware:

- `LAB_HELLO`: a fake initial host check-in with synthetic identity fields.
- `LAB_BEACON`: recurring beacons with configurable interval and jitter.
- `LAB_ACK`: listener acknowledgement messages.
- `LAB_TASK`: simulated operator tasking from the listener.
- `LAB_RESULT`: simulated client task results.
- `LAB_CHUNK`: synthetic chunk upload traffic generated from repeated bytes, not local files.

These messages help analysts observe cadence, session duration, packet sizing, burst behavior, and peer-to-peer flow metadata.

## Why This Matters

Traditional network controls often focus on known domains, destination IPs, ports, or signatures. WebRTC complicates that model because the observable pieces are split across multiple phases. The signaling path may look like ordinary web traffic, while the final peer-to-peer channel may look like collaboration or media infrastructure.

This does not mean WebRTC traffic is invisible. It means defenders need behavioral analytics:

- long-running UDP sessions with low but regular throughput,
- recurring heartbeat cadence,
- unusual DataChannel use without corresponding audio or video behavior,
- repeated STUN negotiation patterns,
- client systems establishing peer-like sessions outside expected application contexts.

## Running the PoC

Start the broker:

```powershell
.\bin\broker.exe -listen :8080
```

Start the listener:

```powershell
.\bin\listener.exe -broker http://127.0.0.1:8080 -session lab-2 -task-every 2 -task-sequence sleep,inventory,synthetic-upload -synthetic-bytes 8192 -chunk-bytes 1024
```

Start the client:

```powershell
.\bin\client.exe -broker http://127.0.0.1:8080 -session lab-2 -interval 8s -jitter 35 -count 8 -task-delay 1s -chunk-delay 250ms
```

For a local-only test without public STUN, pass `-stun ""` to both peers.

## Detection Ideas

Useful telemetry sources include firewall flow records, DNS logs, proxy logs, endpoint network telemetry, packet captures, and STUN/UDP metadata. A practical detection experiment should compare this PoC against normal WebRTC-heavy applications such as conferencing tools.

Signals worth measuring:

- beacon interval distribution,
- UDP flow duration,
- bytes sent per direction,
- burst size and timing after task messages,
- frequency of STUN binding checks,
- whether a process has a normal user-facing WebRTC reason to exist.

## Safety Boundary

This project is a defensive emulator. It deliberately avoids harmful capabilities and uses synthetic data. The point is to make the network theory testable without creating an operational implant.

