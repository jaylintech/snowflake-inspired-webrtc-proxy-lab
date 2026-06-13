# Testing WebRTC Proxy Traffic Against Content Filtering

WebRTC is usually discussed in the context of browsers, voice, video, and collaboration tools. The same peer-to-peer transport pattern can also be used as a proxy layer: a client establishes a WebRTC DataChannel to an intermediate proxy server, and that proxy server reaches a final website or server on the client's behalf.

This article describes a bounded, defensive proof of concept that tests that network pattern. The goal is to evaluate what content filters, firewalls, and monitoring tools see when a client reaches an owned destination site through a Snowflake-inspired WebRTC proxy server instead of connecting to the destination directly.

## Research Question

Can a monitored network identify or block access to a controlled destination when the test client sends the request through a WebRTC proxy server?

The PoC is intentionally scoped to answer that question. It does not implement command execution, persistence, credential access, process hiding, filesystem collection, or an unrestricted proxy. The proxy server is configured with one allowed target URL, and the client can request only relative paths under that target.

## Lab Architecture

The proxy path has four components:

- `broker`: an explicit HTTP signaling broker that exchanges SDP offers and answers.
- `proxy`: a WebRTC proxy server that receives bounded DataChannel requests and forwards them to a configured target URL.
- `webclient`: a test client that sends synthetic HTTP requests through the DataChannel.
- `target`: a controlled HTTP server that logs proof of proxied requests.

The connection flow has three phases:

1. Signaling: the webclient posts an SDP offer to the broker, and the proxy server posts an SDP answer.
2. ICE/STUN negotiation: the webclient and proxy server discover whether a WebRTC path can be established.
3. Proxy exchange: the webclient sends bounded request messages through the encrypted DataChannel, and the proxy server forwards them to the controlled target.

The project also includes a browser-like viewer. In that mode, a local page on the monitored client creates the WebRTC DataChannel from the browser itself, sends relative-path requests to the proxy server, and renders sanitized HTML responses. It is intentionally not a full browser proxy: scripts, forms, external assets, and cross-site links are disabled so the monitored client does not directly browse the target site's resources.

## Why This Matters

A direct WebRTC client still connects to the final server IP. A proxy model changes the observation point:

```text
client == WebRTC DataChannel ==> proxy server --> controlled target
```

The client-side network sees signaling and WebRTC traffic to the proxy server. The controlled target sees traffic from the proxy server. That distinction is the Snowflake-inspired part: indirection, not invisibility.

## Expected Observations

On the monitored client network, defenders should look for:

- HTTP signaling to the broker.
- STUN traffic if STUN is enabled.
- WebRTC/UDP traffic to the proxy server.
- No direct client connection to the controlled target when the target is reachable only from the proxy side.

On the controlled target, defenders should see:

- Requests from the proxy server host.
- `X-WebRTC-Proxy-Lab: true`.
- `X-Proxy-Request-ID`, which maps target hits back to webclient logs.

## STUN And Enterprise Networks

External STUN is not always required. Same-host and same-LAN tests often work without it. Across NATed networks, STUN is often needed so peers can discover public-facing candidates.

Enterprise networks may block public STUN and arbitrary peer UDP. In that case, signaling can succeed while WebRTC fails to connect. That is still a valid result: it means the network allowed rendezvous but blocked the proxy channel at NAT traversal or UDP policy.

## Running The Proxy Test

Build:

```powershell
.\scripts\run-lab.ps1 -Role build
```

Run a local Windows test:

```powershell
.\scripts\run-lab.ps1 -Role proxy-local -Session proxy-local -NoStun
```

For split testing, run broker, target, and proxy on the server side, then run `webclient` from the monitored client network.

## Detection Ideas

Signals worth measuring:

- Whether the client resolves or connects to the final target directly.
- Whether the final target sees the client IP or the proxy server IP.
- WebRTC DataChannel session duration.
- UDP flow timing and byte distribution.
- STUN reachability.
- Differences between allowed collaboration WebRTC and this standalone client process.

## Safety Boundary

This project is a defensive network-behavior emulator. It uses synthetic requests against controlled targets. The proxy server is deliberately bounded so it cannot be used as a general-purpose proxy to arbitrary third-party destinations.
