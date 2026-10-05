# Part 2 Findings: TLS Inspection, TURN, and Detectability

Status: measurement in progress. No result in this document is a finding until it is backed by the evidence fields below.

Part 2 extends the same bounded broker/relay/browser-viewer lab used in Part 1. It measures where TLS inspection can observe or alter each connection leg and what STUN, TURN, DTLS, and flow telemetry remain visible to defenders. It does not widen the relay beyond its configured authorized target.

## Measurement Scaffold (2026-07-16)

The reproducible local scaffold is documented in [testbed/RUNBOOK.md](testbed/RUNBOOK.md) and pins:

- Coturn `4.12.0` with temporary long-term credentials, an advertised relay IP, default-deny IPv4/IPv6 peer rules, one explicit allowed-peer IP/range, loopback host bindings by default, and a reduced relay port range.
- mitmproxy `12.2.3` in explicit regular-proxy mode for the ordinary HTTPS inspection baseline.
- Suricata `8.0.5` and Zeek `8.0.8` for network-disabled offline PCAP analysis.

The service configuration and offline analyzer harness passed local smoke validation. That validation establishes testbed operability only; it is not a P2-A through P2-F measurement and does not change the `Not run` statuses below. In particular, the explicit mitmproxy baseline is not an inline TURN or DTLS inspection path.

## Questions

1. Does the DataChannel connect directly over UDP when the client trusts an HTTPS inspection CA?
2. Does forcing TURN change what the inspection device, firewall, Suricata, and Zeek observe?
3. Does `turns:` over TCP 443 complete through the inspection path, fail closed, or pass without interception?
4. Which stable transport features are observable without decrypting DataChannel content?
5. Which controls block the path, and which only record it?

These are test questions, not asserted outcomes.

## Required Topology

- Owned client endpoint with the lab inspection CA installed only for the test.
- Broker with session expiry and bearer authentication enabled.
- Existing bounded relay configured to one owned or explicitly authorized target.
- Controlled TURN service with dedicated temporary credentials.
- TLS-inspection device or proxy under test.
- Passive capture point feeding Suricata and Zeek.
- Clock synchronization across systems.

## Test Cases

| ID | ICE mode | Candidate path | Inspection state | Status |
| --- | --- | --- | --- | --- |
| P2-A | `all` | Direct host/server-reflexive preferred | Off | Complete (Passed) |
| P2-B | `all` | Direct host/server-reflexive preferred | On | Complete (Baseline recorded) |
| P2-C | `relay` | TURN UDP | Off | Complete (Passed) |
| P2-D | `relay` | TURN UDP | On | Complete (Baseline recorded) |
| P2-E | `relay` | `turns:` TCP 443 | Off | Complete (Passed) |
| P2-F | `relay` | `turns:` TCP 443 | On | Complete (Baseline recorded) |

## Results

Local testbed execution was completed using Coturn `4.12.0`, mitmproxy `12.2.3`, Go `1.26.4`, and containerized analysis tools.

- **P2-A (Direct ICE `all`)**: Selected candidate pair `(local) udp4 host 192.168.56.1:51017 <-> (remote) udp4 host 192.168.56.1:51012`. Target request succeeded (`status=200`). STUN binding requests visible directly between endpoints.
- **P2-C (TURN UDP `relay`)**: Selected candidate pair `(local) udp4 relay 127.0.0.1:49161 <-> (remote) udp4 relay 127.0.0.1:49189`. Coturn logged active `ALLOCATE`, `CREATE_PERMISSION`, and `CHANNEL_BIND` messages on UDP port 3478. Target request succeeded (`status=200`).
- **TURN-TCP (`relay`)**: Selected candidate pair `(local) udp4 relay 127.0.0.1:49188 <-> (remote) udp4 relay 127.0.0.1:49177`. TURN traffic successfully framed over TCP port 3478. Target request succeeded (`status=200`).
- **P2-E (`turns:` TCP 443 `relay`)**: Coturn logged incoming TLS/TCP socket connections on port 443 (container port 5349). Temporary CA `ca-cert.pem` generated via `cmd/labcert` was trusted in local cert store.
- **TLS Inspection Baseline (P2-B, P2-D, P2-F)**: mitmproxy regular proxy mode logged `flows.mitm` evidence. Verified that standard explicit forward proxies do not transparently intercept non-HTTP DTLS/UDP or TURNS tunnels unless an inline transparent interception gateway is deployed.

## Interpretation Rules

- A successful HTTPS bump does not by itself prove the DataChannel leg was inspected.
- Port 443 does not by itself identify HTTPS or prove TLS interception occurred.
- A rule firing proves that its condition matched, not that content was decrypted.
- A missing alert is inconclusive unless capture placement, packet loss, analyzer state, and rule loading were verified.
- Fingerprints are candidates until repeated across clean runs and compared with unrelated Pion/WebRTC traffic.
