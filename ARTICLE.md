# Testing WebRTC Relay Traffic Against Content Filtering

WebRTC is usually discussed in the context of browsers, voice, video, and collaboration tools. The same peer-to-peer transport pattern can also be used as a relay layer: a client establishes a WebRTC DataChannel to an intermediate peer, and that peer reaches a final website or server on the client's behalf.

This article describes a bounded, defensive proof of concept that tests that network pattern. The goal is to evaluate what content filters, firewalls, and monitoring tools see when a client reaches an owned destination server through a Snowflake-inspired WebRTC relay instead of connecting to the destination directly.

## Research Question

Can a monitored network identify or block access to a controlled destination when the test client sends the request through a WebRTC relay?

The PoC is intentionally scoped to answer that question. It does not implement command execution, persistence, credential access, process hiding, filesystem collection, or an unrestricted proxy. The relay is configured with one allowed target URL, and the client can request only relative paths under that target.

## Lab Architecture

The relay path has four components:

- `broker`: an explicit HTTP signaling broker that exchanges SDP offers and answers.
- `relay`: a WebRTC peer that receives bounded DataChannel requests and forwards them to a configured target URL.
- `webclient`: a test client that sends synthetic HTTP requests through the DataChannel.
- `target`: a controlled HTTP server that logs proof of relayed requests.

The connection flow has three phases:

1. Signaling: the webclient posts an SDP offer to the broker, and the relay posts an SDP answer.
2. ICE/STUN negotiation: the webclient and relay discover whether a WebRTC path can be established.
3. Relay exchange: the webclient sends `LAB_RELAY_REQUEST` messages through the encrypted DataChannel, and the relay forwards them to the controlled target.

## Why This Differs From Direct C2 Emulation

A direct WebRTC C2 emulator looks like this:

```text
client == WebRTC DataChannel ==> listener
```

That is useful for studying WebRTC beacon cadence and peer UDP visibility, but the client still connects directly to the listener IP.

The relay model looks like this:

```text
client == WebRTC DataChannel ==> relay --> controlled target
```

The client-side network sees signaling and WebRTC traffic to the relay. The controlled target sees traffic from the relay. That distinction is the Snowflake-inspired part: indirection, not invisibility.

## Expected Observations

On the monitored client network, defenders should look for:

- HTTP signaling to the broker.
- STUN traffic if STUN is enabled.
- WebRTC/UDP traffic to the relay.
- No direct client connection to the controlled target when the target is reachable only from the relay side.

On the controlled target, defenders should see:

- Requests from the relay host.
- `X-WebRTC-Relay-Lab: true`.
- `X-Relay-Request-ID`, which maps target hits back to webclient logs.

On the relay, logs should show:

- DataChannel establishment.
- Bounded target requests.
- The configured target URL.

## STUN And Enterprise Networks

External STUN is not always required. Same-host and same-LAN tests often work without it. Across NATed networks, STUN is often needed so peers can discover public-facing candidates.

Enterprise networks may block public STUN and arbitrary peer UDP. In that case, signaling can succeed while WebRTC fails to connect. That is still a valid result: it means the network allowed rendezvous but blocked the relay channel at NAT traversal or UDP policy.

TURN is the normal WebRTC fallback for strict networks, but this PoC does not bundle TURN. Direct relay behavior is easier to observe and explain without introducing a relay-of-a-relay.

## Running The Relay Test

Build:

```powershell
.\scripts\run-lab.ps1 -Role build
```

Run a local Windows test:

```powershell
.\scripts\run-lab.ps1 -Role relay-local -Session relay-local -NoStun
```

Run a local Linux test:

```bash
python3 scripts/run_lab.py build
python3 scripts/run_lab.py relay-local --session relay-local --no-stun
```

For split testing, run broker, target, and relay on the server side, then run `webclient` from the monitored client network.

## Detection Ideas

Signals worth measuring:

- Whether the client resolves or connects to the final target directly.
- Whether the final target sees the client IP or the relay IP.
- WebRTC DataChannel session duration.
- UDP flow timing and byte distribution.
- STUN reachability.
- Differences between allowed collaboration WebRTC and this standalone client process.

## Safety Boundary

This project is a defensive network-behavior emulator. It uses synthetic requests against controlled targets. The relay is deliberately bounded so it cannot be used as a general-purpose proxy to arbitrary third-party destinations.
