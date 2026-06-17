# Findings

Observed findings from controlled lab runs.

## NextDNS Client DNS-Filter Test

Date: 2026-06-14

Configuration:

- Monitored client: Device B
- DNS filter: NextDNS client
- Target state: blocked by policy
- NextDNS block bypass methods: enabled
- Proxy host: Device A
- Client interface: `browserui` at `http://127.0.0.1:7777`

Result:

- Direct access from Device B to the target was blocked.
- The target loaded through the WebRTC proxy viewer.
- Device B did not resolve or connect to the target directly.
- Device A resolved and fetched the target through the bounded proxy.

Interpretation:

```text
Client-side DNS filtering did not block the destination when the monitored client did not resolve the destination domain and instead requested it through a separate WebRTC proxy host.
```

This finding does not imply traffic invisibility or bypass of EDR, NDR, firewall policy, TLS inspection, or WebRTC-aware controls.

## Evidence Checklist

Preserve these artifacts for repeatable reporting:

- DNS-filter log showing the direct block.
- Direct client request failure.
- Browser viewer log showing DataChannel connection.
- Browser viewer log showing `status=200`.
- Proxy log showing the requested target URL.
- Target server log showing the proxy host as requester.
- Packet capture or flow log showing client-to-proxy WebRTC/UDP.

## Off-LAN Test Plan

Purpose:

```text
Validate whether the same behavior holds when the proxy host is outside the client's LAN and real NAT/firewall/STUN behavior applies.
```

Recommended topology:

```text
[Monitored Client + DNS Filter] -- HTTP signaling --> [Public Proxy Host TCP 8080]
[Monitored Client + DNS Filter] == WebRTC/UDP ==> [Public Proxy Host]
[Public Proxy Host] -- HTTPS --> [Owned Target]
```

Requirements:

- Temporary public VM or VPS for the proxy host.
- Owned target domain with server logs.
- TCP `8080` inbound to the proxy host.
- UDP inbound to the proxy host for WebRTC/ICE.
- Proxy bounded to one `-TargetUrl`.
- VM removed or shut down after testing.

Expected failure points:

- Broker blocked: TCP `8080` or host firewall issue.
- ICE failure: UDP, NAT, STUN, or cloud firewall issue.
- Target failure: proxy host cannot reach the target.
- Truncation: raise `-MaxBody` for large HTML responses.

Current engineering note:

The PoC currently lets WebRTC choose UDP candidate ports. A fixed ICE UDP port range would make public cloud tests cleaner because the proxy host could expose a narrow UDP range instead of broad UDP.

## Claim Boundary

Strong claim:

```text
In this lab, DNS filtering on the monitored client did not observe the final destination because the proxy host resolved and fetched that destination.
```

Avoid claiming:

- The traffic is invisible.
- All filtering was bypassed.
- Endpoint or network detection was defeated.
- The proxy is suitable for unrestricted browsing.
