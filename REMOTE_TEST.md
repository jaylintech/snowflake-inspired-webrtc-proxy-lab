# Remote Server Test Guide

This guide describes how to test the WebRTC proxy lab when the proxy host is a remote server instead of a machine on the same LAN.

Use only owned or explicitly authorized systems.

## Goal

Validate this path:

```text
[Monitored Client] -- HTTP signaling --> [Remote Proxy Host]
[Monitored Client] == WebRTC/UDP ==> [Remote Proxy Host]
[Remote Proxy Host] -- HTTP/HTTPS --> [Controlled Target]
```

This test is useful for measuring:

- Whether the monitored client can reach the remote broker.
- Whether WebRTC/ICE can establish a DataChannel across NAT and firewall boundaries.
- Whether client-side DNS filtering observes the final target domain.
- Whether the controlled target sees the proxy host as the requester.

## Recommended Topology

- **Remote proxy host**: temporary VPS/cloud VM with a public IP.
- **Monitored client**: the filtered endpoint running `browserui`.
- **Controlled target**: owned or authorized website/server.

Avoid third-party targets for evidence collection. An owned target provides server logs, source IP, and timestamps.

## Remote Host Network Requirements

Open these only for the test window:

| Direction | Protocol | Port | Purpose |
| --- | --- | --- | --- |
| Inbound | TCP | `8080` | Broker signaling |
| Inbound | UDP | `40000` recommended | WebRTC/ICE DataChannel path |
| Outbound | UDP | `19302` or configured STUN | STUN, if enabled |
| Outbound | TCP | `443` or target port | Fetch controlled target |

Router or cloud firewall rule:

```text
TCP 8080  -> remote proxy host TCP 8080
UDP 40000 -> remote proxy host UDP 40000
```

Restrict inbound rules to the monitored client's public IP when your router or firewall supports it. Remove the rules after testing.

## Remote Host Setup

Clone and build:

```bash
git clone https://github.com/jaylintech/snowflakeprotocolpoc.git
cd snowflakeprotocolpoc
python3 scripts/run_lab.py build
```

If Go is not installed, install Go 1.22 or newer, then rerun the build.

Start the broker:

```bash
python3 scripts/run_lab.py broker --listen :8080
```

In a second terminal, start the proxy:

```bash
python3 scripts/run_lab.py proxy \
  --broker-url http://127.0.0.1:8080 \
  --session remote-test \
  --target-url https://controlled-target.example \
  --max-body 1048576 \
  --ice-port-min 40000 \
  --ice-port-max 40000 \
  --advertise-ip YOUR_PUBLIC_IP
```

Replace `https://controlled-target.example` with the owned or authorized target.
Replace `YOUR_PUBLIC_IP` with the public IP that the monitored client should reach, such as the WAN IP on your router.

The proxy log should include a local ICE candidate containing the public IP and UDP `40000`. If it only advertises private addresses such as `192.168.x.x` or `172.16.x.x`, stop the proxy and restart it with `--advertise-ip YOUR_PUBLIC_IP`.

## Monitored Client Setup

Update and build:

```powershell
git pull
.\scripts\run-lab.ps1 -Role build
```

Verify broker reachability:

```powershell
Invoke-WebRequest -Uri http://PUBLIC_PROXY_IP:8080/healthz -UseBasicParsing
```

Expected result:

```text
StatusCode : 204
```

Start the browser viewer:

```powershell
.\scripts\run-lab.ps1 -Role browserui -BrokerUrl http://PUBLIC_PROXY_IP:8080 -Session remote-test -TargetUrl https://controlled-target.example -UiListen 127.0.0.1:7777
```

Open locally on the monitored client:

```text
http://127.0.0.1:7777
```

Click `Connect`, then request:

```text
/
/robots.txt
https://controlled-target.example/
```

## Expected Logs

Browser viewer:

```text
posted SDP offer to broker
applied SDP answer from proxy
proxy data channel "lab-proxy" open
proxy response id=browser-001 status=200
```

Remote proxy host:

```text
local ICE candidate: ... YOUR_PUBLIC_IP ... 40000 ...
proxy data channel "lab-proxy" created by client
proxy request id=browser-001 method=GET target=https://controlled-target.example/
```

Controlled target:

```text
source_ip=<REMOTE_PROXY_HOST_IP>
path=/
```

## DNS-Filter Evidence

For DNS-filter testing, collect:

- Direct access from the monitored client blocked by DNS policy.
- DNS logs showing the direct blocked request.
- No target-domain DNS query from the monitored client during proxied access.
- Proxy logs showing the configured target URL.
- Target logs showing the remote proxy host as source.

Expected interpretation:

```text
The monitored client did not resolve the final target domain during proxied access. The remote proxy host resolved and fetched the target instead.
```

## Failure Triage

### Broker Health Check Fails

Likely causes:

- TCP `8080` is closed on the remote host firewall or cloud security group.
- Broker is not running.
- Wrong public IP or DNS name.
- Local network blocks outbound TCP `8080`.

### Broker Works, WebRTC Does Not Connect

Likely causes:

- Inbound UDP to the remote proxy host is blocked.
- UDP `40000` is not forwarded to the proxy host.
- The proxy was started without `--advertise-ip YOUR_PUBLIC_IP`, causing remote clients to try private ICE candidates such as `192.168.x.x`, `172.16.x.x`, or virtual adapter IPs.
- STUN is blocked from the monitored client or proxy host.
- NAT traversal fails without TURN.

This PoC does not bundle TURN. If this failure is consistent across networks even with UDP `40000` forwarded, the next engineering step is TURN support for controlled lab use.

Observed lab symptom:

```text
TCP broker health worked, but firewall logs showed UDP 40000 attempts to private/local ICE candidate addresses. Adding the fixed UDP port and public advertised IP corrected the candidate path.
```

### DataChannel Connects, Target Fails

Likely causes:

- Proxy `--target-url` / `-TargetUrl` is incorrect.
- Remote proxy host cannot resolve or reach the target.
- Target blocks the proxy host IP.
- Response exceeds `-MaxBody`.

### Target Logs Show Client IP

Likely causes:

- The monitored client browsed the target directly outside the viewer.
- Browser UI target hint does not match proxy `-TargetUrl`.
- Test evidence mixes direct-control and proxied runs.

## Cleanup

After the test:

- Stop broker and proxy processes.
- Remove temporary firewall/security-group rules.
- Shut down or destroy the temporary VM.
- Preserve logs and packet captures needed for reporting.

## Claim Boundary

A successful remote test supports a narrow claim:

```text
In this lab, the monitored client reached a blocked destination through a bounded WebRTC proxy host without resolving the destination domain locally.
```

It does not prove invisibility or bypass of all security controls.
