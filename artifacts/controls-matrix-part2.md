# Part 2 Controls Matrix

Update this matrix only from completed, archived runs. Use `Blocked`, `Allowed`, `Observed only`, or `Inconclusive` in the outcome column.

| Control | Test case(s) | Outcome | What was observed | Evidence reference | Confidence |
| --- | --- | --- | --- | --- | --- |
| Client DNS filter | P2-A through P2-F | Allowed | Target domain resolution delegated to relay; client DNS log shows no lookup | Part 1 & Part 2 logs | High |
| TLS-bump forward proxy | P2-B, P2-D, P2-F | Observed only | mitmproxy logs explicit HTTP(S); does not bump raw DTLS/UDP or TURNS tunnels inline | `captures/mitmproxy/flows.mitm` | High |
| Firewall UDP policy | P2-A through P2-D | Allowed | Direct UDP host pairs (P2-A) and TURN UDP relay (P2-C) connect | P2-A & P2-C logs | High |
| STUN block | P2-A, P2-B | Allowed | Direct ICE candidates gathered via local interface / STUN | P2-A candidate logs | High |
| TURN UDP policy | P2-C, P2-D | Allowed | Coturn relay allocation on port 3478/UDP succeeds; relay candidate pair selected | P2-C Coturn logs | High |
| TURN TLS/TCP 443 policy | P2-E, P2-F | Allowed | Coturn socket connections accepted on TLS 443 (container 5349) | P2-E Coturn logs | High |
| Suricata | P2-A through P2-F | Observed only | `testbed/scripts/analyze-pcap.ps1` harness processes PCAPs with `--network none` | Suricata JSON output | High |
| Zeek/NDR | P2-A through P2-F | Observed only | `testbed/scripts/analyze-pcap.ps1` outputs Zeek JSON logs | Zeek log directory | High |
| Endpoint protection | P2-A through P2-F | Observed only | Process execution of unsigned Go binaries (`relay.exe`, `webclient.exe`) logged | `DETECTION_NOTES.md` | High |
| Controlled target logging | P2-A through P2-F | Allowed | Target server logs relay host IP as requester for all proxied requests | Target log output | High |

## Run References

| Run ID | Test case | Commit | UTC interval | Report | PCAP hashes |
| --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |
