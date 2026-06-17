# Testing WebRTC Proxy Traffic Against DNS And Content Filtering

This project tests a bounded proxy pattern built on WebRTC DataChannels. A monitored client connects to a broker for SDP signaling, establishes a WebRTC channel to a proxy host, and asks that proxy host to fetch a configured target site.

The goal is not to build an open proxy. The goal is to measure what different controls observe when the final destination is reached by the proxy host instead of by the monitored client.

## Research Question

```text
Can client-side DNS or content filtering identify and block a destination when the monitored client never resolves that destination and instead requests it through a WebRTC proxy host?
```

## Architecture

```text
[Monitored Client] -- HTTP signaling --> [Broker]
[Monitored Client] == WebRTC DataChannel ==> [Proxy Host]
[Proxy Host] -- HTTP/HTTPS --> [Configured Target]
```

Components:

- `broker`: SDP offer/answer exchange.
- `proxy`: bounded WebRTC proxy server.
- `webclient`: CLI request client.
- `browserui`: local browser-based viewer.
- `target`: controlled local HTTP server for verification.

## Safety Boundary

The proxy host accepts only relative-path requests under a configured `-TargetUrl`. The PoC does not implement command execution, persistence, credential access, file collection, process hiding, or unrestricted proxying.

## Expected Observations

On the monitored client:

- HTTP signaling to the broker.
- WebRTC/UDP traffic to the proxy host.
- STUN traffic when enabled.
- No direct DNS lookup or direct connection to the final target if the test is isolated correctly.

On the proxy host:

- DNS lookup and HTTP/HTTPS connection to the configured target.
- Proxy logs containing `X-Proxy-Request-ID` correlation IDs.

On the target:

- Requests from the proxy host, not the monitored client.
- `X-WebRTC-Proxy-Lab: true`.
- `X-Proxy-Request-ID`.

## Lab Finding

In one controlled test, a monitored client with the NextDNS client installed had the target site blocked by policy and block bypass methods enabled. Direct access from the client was blocked. The same target loaded through the WebRTC proxy viewer because the client did not resolve or connect to the target directly; the proxy host resolved and fetched it.

Careful conclusion:

```text
Client-side DNS filtering did not observe the final destination domain when that destination was resolved and fetched by a separate WebRTC proxy host.
```

This does not imply invisibility or bypass of firewall, EDR, NDR, TLS inspection, or WebRTC-aware controls.

## Detection And Measurement

Useful evidence:

- DNS-filter logs for direct blocked access.
- Absence of target-domain DNS queries during proxied access.
- Browser viewer DataChannel logs.
- Proxy host request logs.
- Target server logs showing proxy-host source IP.
- Packet captures showing signaling, STUN, and WebRTC/UDP.

## Network Notes

STUN is not required for same-host testing and may not be required on the same LAN. Across NATed networks, STUN is usually required. If signaling succeeds but ICE never connects, UDP policy or NAT traversal is the likely blocker. TURN is the standard fallback for WebRTC paths that cannot connect directly, but this PoC intentionally does not bundle TURN.

## Claim Boundary

This lab supports narrow, evidence-based claims about DNS-filter visibility and WebRTC transport behavior. It should not be described as a general-purpose bypass or stealth channel.
