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

## Remote Router WebRTC Test

Date: 2026-06-18

Configuration:

- Proxy host: Device A behind a router.
- Monitored client: Device B outside the proxy host LAN.
- Router forwards: TCP `8080` and UDP `40000` to Device A.
- Proxy ICE settings: `-IcePortMin 40000 -IcePortMax 40000 -AdvertiseIP YOUR_PUBLIC_IP`.
- Client interface: `browserui` pointed at `http://PUBLIC_PROXY_IP:8080`.

Result:

- Broker health check returned HTTP `204`, confirming TCP signaling reachability.
- Before `-AdvertiseIP`, the remote firewall view showed UDP `40000` checks toward private or local adapter addresses such as `192.168.x.x` and `172.16.x.x`.
- After forwarding UDP `40000` and advertising the public IP, the WebRTC proxy path connected successfully.

Interpretation:

```text
For an off-LAN router test, broker reachability alone is insufficient. The proxy host must expose a reachable UDP ICE candidate. A fixed ICE UDP port plus an advertised public IP produced a working remote WebRTC DataChannel path in this lab.
```

This finding does not imply that all remote networks will allow WebRTC/UDP. Enterprise egress policy, NAT behavior, firewall rules, STUN restrictions, and lack of TURN can still prevent connection.

## Off-LAN Test Shape

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
- UDP `40000` inbound to the proxy host for WebRTC/ICE, when using the documented fixed-port setup.
- Proxy bounded to one `-TargetUrl`.
- VM removed or shut down after testing.

Expected failure points:

- Broker blocked: TCP `8080` or host firewall issue.
- ICE failure: UDP, NAT, STUN, or cloud firewall issue.
- Private candidates: use `-AdvertiseIP YOUR_PUBLIC_IP` if the remote side attempts UDP to `192.168.x.x`, `172.16.x.x`, or other non-public addresses.
- Target failure: proxy host cannot reach the target.
- Truncation: raise `-MaxBody` for large HTML responses.
