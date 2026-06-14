# Lab Findings And Test Plan

This file records observed lab behavior and follow-up tests for the bounded WebRTC proxy proof of concept.

## Finding: NextDNS Client Blocked Direct Access But Not Proxied Access

Date observed: 2026-06-14

Lab setup:

- Device B was the monitored test client.
- Device B had the NextDNS client installed.
- The target site was blocked in the NextDNS policy.
- NextDNS block bypass methods were enabled.
- Device A ran the signaling broker and bounded WebRTC proxy server.
- Device A's proxy server was configured to fetch the blocked target site.
- Device B used the local browser viewer at `http://127.0.0.1:7777`.

Observed result:

- Direct access from Device B to the target site was blocked by NextDNS.
- The same target site loaded inside the WebRTC proxy viewer on Device B.
- The proxy path succeeded because Device B did not resolve or connect to the target site directly.
- Device B connected to the broker/proxy infrastructure and used a WebRTC DataChannel to the proxy server.
- Device A resolved and fetched the configured target site.

Careful interpretation:

```text
This result does not show that the traffic is invisible. It shows that client-side DNS filtering on Device B did not observe the final destination domain when that destination was resolved and fetched by a separate proxy host over the WebRTC proxy path.
```

Evidence to preserve for an article or report:

- A screenshot or log showing direct access from Device B was blocked.
- NextDNS logs showing the direct blocked request.
- Browser viewer logs showing `proxy data channel "lab-proxy" open`.
- Browser viewer logs showing `proxy response id=... status=200`.
- Proxy server logs showing `proxy request id=... target=https://blocked-target/...`.
- Target server logs, when using an owned target, showing Device A/proxy host as the requester.

## Recommended Off-LAN Public Test

Yes, the next useful test is to run the proxy host off the same LAN. That tests whether the PoC survives real NAT, public routing, firewall, and STUN behavior.

Use only systems you own or are explicitly authorized to test.

Recommended topology:

```text
[Device B with NextDNS] -- HTTP signaling --> [Public proxy host TCP 8080]
[Device B with NextDNS] == WebRTC/UDP ==> [Public proxy host]
[Public proxy host] -- HTTPS --> [Owned target site]
```

The cleanest version is:

- Put Device A/proxy on a temporary VPS or cloud VM with a public IP.
- Use a target domain you own and can inspect in server logs.
- Keep the proxy bounded to that one `-TargetUrl`.
- Do not turn the proxy into an open proxy.
- Shut the VM down after the test.

Proxy host requirements:

- TCP `8080` inbound for the signaling broker.
- UDP inbound to the proxy host for WebRTC/ICE.
- Outbound HTTPS from the proxy host to the configured target site.
- Outbound STUN from Device B if STUN is enabled.

Current limitation:

The PoC currently lets WebRTC choose its UDP candidate ports. On a cloud VM, this may require a broader UDP firewall allowance during the test. If that is too loose for your environment, add a fixed ICE UDP port range to the PoC before public testing, then open only that range on the proxy host.

## Off-LAN Test Steps

On the public proxy host:

```powershell
git pull
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role broker -Listen :8080
```

In a second terminal on the public proxy host:

```powershell
.\scripts\run-lab.ps1 -Role proxy -BrokerUrl http://127.0.0.1:8080 -Session public-dns-test -TargetUrl https://blocked-target.example -MaxBody 1048576
```

On Device B:

```powershell
git pull
.\scripts\run-lab.ps1 -Role build
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://PUBLIC_PROXY_IP:8080 -Session public-dns-test -TargetUrl https://blocked-target.example -UiListen 127.0.0.1:7777
```

Open on Device B:

```text
http://127.0.0.1:7777
```

Then request:

```text
https://blocked-target.example/
https://blocked-target.example/robots.txt
```

## How To Classify Off-LAN Results

Broker unreachable:

- TCP `8080` is blocked.
- The public IP or DNS name is wrong.
- The broker is not running.

Broker works, but WebRTC never connects:

- UDP between Device B and the public proxy host is blocked.
- Public STUN is blocked or unreliable.
- The cloud firewall/security group is not allowing the selected UDP path.
- A TURN relay would be needed for that network path, but this PoC intentionally does not bundle TURN.

WebRTC connects, but target does not load:

- The proxy host cannot reach the target site.
- The proxy `-TargetUrl` is wrong.
- The target blocks the proxy host's outbound request.
- The response exceeds `-MaxBody` and is truncated.

Target loads through the viewer:

- Record whether Device B's DNS logs show the target domain.
- Record whether Device B connects only to the broker/proxy host plus STUN.
- Record whether the target site sees the proxy host IP instead of Device B.

## Claim Boundary

Strong, defensible claim:

```text
In this lab, client-side DNS filtering did not block the destination when the monitored client never resolved the destination domain and instead requested it through a bounded WebRTC proxy host.
```

Avoid overclaiming:

- Do not claim this bypasses all security filtering.
- Do not claim the traffic is invisible.
- Do not claim it defeats EDR, NDR, firewall policy, TLS inspection, or WebRTC-aware controls.
- Do not test against third-party targets without authorization.
