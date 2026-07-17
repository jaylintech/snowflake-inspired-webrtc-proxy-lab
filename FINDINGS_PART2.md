# Part 2 Findings: TLS Inspection, TURN, and Detectability

Status: measurement in progress. No result in this document is a finding until it is backed by the evidence fields below.

Part 2 extends the same bounded broker/relay/browser-viewer lab used in Part 1. It measures where TLS inspection can observe or alter each connection leg and what STUN, TURN, DTLS, and flow telemetry remain visible to defenders. It does not widen the relay beyond its configured authorized target.

## Measurement Scaffold (2026-07-16)

The reproducible local scaffold is documented in [testbed/RUNBOOK.md](testbed/RUNBOOK.md) and pins:

- Coturn `4.12.0` with temporary long-term credentials, an explicit allowed-peer IP/range, loopback host bindings by default, and a reduced relay port range.
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
| P2-A | `all` | Direct host/server-reflexive preferred | Off | Not run |
| P2-B | `all` | Direct host/server-reflexive preferred | On | Not run |
| P2-C | `relay` | TURN UDP | Off | Not run |
| P2-D | `relay` | TURN UDP | On | Not run |
| P2-E | `relay` | `turns:` TCP 443 | Off | Not run |
| P2-F | `relay` | `turns:` TCP 443 | On | Not run |

For each run, record the selected ICE candidate pair rather than inferring the path from configuration alone.

## Evidence Required Per Run

- UTC start/end and unique session ID.
- Exact commit and sanitized configuration.
- Client, broker, relay, TURN, target, inspection-device, Suricata, and Zeek logs.
- Packet capture on each available leg with a recorded SHA-256 digest.
- Selected local/remote ICE candidate types and transport.
- Whether signaling, ICE, DataChannel, and bounded target request each completed.
- Exact alert/rule identifiers and relevant flow metadata.
- Negative evidence and capture blind spots.

Store raw captures outside Git. Commit only sanitized summaries, rule sources, config templates, and hashes.

## Results

No Part 2 measurements have been completed yet. Use [artifacts/controls-matrix-part2.md](artifacts/controls-matrix-part2.md) for the cross-control summary and the existing report/checklist templates for each run.

## Interpretation Rules

- A successful HTTPS bump does not by itself prove the DataChannel leg was inspected.
- Port 443 does not by itself identify HTTPS or prove TLS interception occurred.
- A rule firing proves that its condition matched, not that content was decrypted.
- A missing alert is inconclusive unless capture placement, packet loss, analyzer state, and rule loading were verified.
- Fingerprints are candidates until repeated across clean runs and compared with unrelated Pion/WebRTC traffic.
