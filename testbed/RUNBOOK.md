# Part 2 TURN and TLS-Inspection Runbook

This runbook creates a local, explicitly configured baseline. It does not claim to reproduce an enterprise transparent interception appliance. Run it only on owned or explicitly authorized systems and targets.

## 1. Generate Temporary TURN Certificates

From the repository root:

```powershell
$TurnHostIP = "192.0.2.10" # Replace with the owned TURN host's reachable IPv4 address.
go run ./cmd/labcert -out testbed/private/turn -dns turn.lab.example -ip $TurnHostIP -valid-for 168h
```

The generator refuses to overwrite files unless `-force` is provided. The output directory and all PEM files are ignored by Git. It creates a temporary CA certificate, TURN server certificate, and TURN private key; it does not install trust automatically.

For `turns:` tests, trust `testbed/private/turn/ca-cert.pem` only in the owned test endpoint's user trust store. Record the imported certificate thumbprint and remove that exact certificate after the test. Do not distribute or reuse this CA.

## 2. Configure the Testbed

```powershell
Copy-Item testbed/.env.example testbed/.env
```

Replace every `CHANGE_ME`. For a two-host run, set `TURN_BIND_IP` to the owned LAN interface and `TURN_EXTERNAL_IP` to the reachable IPv4 address advertised in relay candidates. The Compose file denies all other IPv4 and IPv6 relay destinations, while `TURN_ALLOWED_PEER_IP` explicitly permits the required destination; when both forced-relay peers use this TURN server, that destination is normally `TURN_EXTERNAL_IP`. This value restricts where allocations may relay traffic, not which clients may authenticate.

Validate without starting containers:

```powershell
./testbed/scripts/test-config.ps1
```

## 3. Start the Controlled Services

```powershell
docker compose --env-file testbed/.env -f testbed/compose.yaml up -d turn tls-inspection
docker compose --env-file testbed/.env -f testbed/compose.yaml logs --tail 50 turn tls-inspection
```

The testbed exposes:

- TURN UDP/TCP on `3478`.
- TURN TLS/TCP on host port `443`, mapped to Coturn `5349`.
- A restricted TURN relay allocation range of UDP `49160-49200`.
- Default-deny IPv4 and IPv6 relay-destination rules with one explicit allowed IP or range.
- mitmproxy regular HTTP(S) proxy on loopback `127.0.0.1:8081`.

## 4. Establish the HTTPS-Bump Baseline

Mitmproxy regular mode handles explicitly proxied HTTP(S). After mitmproxy starts, its temporary CA is written under `testbed/private/mitmproxy`.

Use the owned target URL:

```powershell
curl.exe --proxy http://127.0.0.1:8081 `
  --cacert testbed/private/mitmproxy/mitmproxy-ca-cert.pem `
  https://owned-target.example/
```

Preserve the mitmproxy flow file and logs as the HTTPS inspection baseline. The explicit HTTP proxy does not automatically route arbitrary TURN or DTLS traffic. Therefore, success or failure of `turns:` in this local composition must not be described as an inline bump result.

### Explicit Proxy vs. Transparent Inspection
- **Explicit Forward Proxy (`127.0.0.1:8081`)**: Intercepts only HTTP(S) traffic explicitly configured with an HTTP `CONNECT` tunnel. WebRTC DTLS (UDP) and `turns:` (TCP) establish direct peer/relay sockets and bypass explicit forward proxies completely unless proxied at the OS network interface level.
- **Transparent Interception (P2-B, P2-D, P2-F)**: Requires routing all host traffic through an inline transparent gateway (e.g., mitmproxy in `wireguard` or `transparent` mode, Linux `iptables`/`nftables` redirect, or an enterprise NGFW appliance). In transparent inspection environments:
  - Raw DTLS over UDP is typically either blocked (if UDP egress is restricted) or passed uninspected (since forward proxies cannot decrypt custom DTLS sessions without endpoint agent interception).
  - `turns:` over TCP port 443 with ALPN `webrtc-turn` will fail TLS inspection or close with connection errors if the inspection appliance expects valid HTTPS/HTTP2 traffic.

## 5. Run the TURN Cases

Set the same values on both WebRTC peers.

TURN UDP:

```powershell
$TurnHostIP = "192.0.2.10" # Same address as TURN_EXTERNAL_IP.
$env:LAB_TURN_URLS = "turn:${TurnHostIP}:3478?transport=udp"
$env:LAB_TURN_USERNAME = "<value from testbed/.env>"
$env:LAB_TURN_CREDENTIAL = "<value from testbed/.env>"
$env:LAB_ICE_POLICY = "relay"
```

TURN TLS/TCP 443:

```powershell
$env:LAB_TURN_URLS = "turns:turn.lab.example:443?transport=tcp"
$env:LAB_TURN_USERNAME = "<value from testbed/.env>"
$env:LAB_TURN_CREDENTIAL = "<value from testbed/.env>"
$env:LAB_ICE_POLICY = "relay"
```

Verify the logged selected ICE candidate pair contains relay candidates. Configuration alone is not evidence that TURN was selected.

## 6. Capture and Analyze

### Live Interface Capture
Wireshark/Npcap supplies `dumpcap`. List interfaces and run a time-bounded capture:

```powershell
./testbed/scripts/capture-part2.ps1 -ListInterfaces
./testbed/scripts/capture-part2.ps1 -Interface 1 -DurationSeconds 120 -RunId P2-C-run01
```

### Synthetic Protocol Replay (Offline Lab Mode)
If `dumpcap` is not installed on the host, synthesize an exact packet capture representing observed testbed flows using Python:

```powershell
python scripts/generate_lab_pcap.py P2-synth-run01
```

### Sensor Analysis
Analyze a completed PCAP without network access inside the sensor containers:

```powershell
./testbed/scripts/analyze-pcap.ps1 -Pcap captures/P2-synth-run01/P2-synth-run01.pcapng
```

To evaluate custom detection rules against the capture, supply `-SuricataRules`:

```powershell
./testbed/scripts/analyze-pcap.ps1 -Pcap captures/P2-synth-run01/P2-synth-run01.pcapng `
  -SuricataRules rules/webrtc_part2.rules `
  -OutputRoot captures/analysis_rules
```

The analysis harness writes separate Suricata and Zeek outputs plus an analysis manifest.

## 7. Stop and Clean Up

```powershell
docker compose --env-file testbed/.env -f testbed/compose.yaml down
Remove-Item Env:LAB_TURN_URLS,Env:LAB_TURN_USERNAME,Env:LAB_TURN_CREDENTIAL,Env:LAB_ICE_POLICY,Env:LAB_BROKER_TOKEN -ErrorAction SilentlyContinue
```

Remove the temporary CA certificate imported for the test run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File testbed/scripts/remove-lab-ca.ps1
```

Archive evidence hashes and sanitized logs. Delete `testbed/.env`, private keys, and temporary credentials when the measurement window closes.
