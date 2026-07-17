# Part 2 TURN and TLS-Inspection Runbook

This runbook creates a local, explicitly configured baseline. It does not claim to reproduce an enterprise transparent interception appliance. Run it only on owned or explicitly authorized systems and targets.

## 1. Generate Temporary TURN Certificates

From the repository root:

```powershell
go run ./cmd/labcert -out testbed/private/turn -dns turn.lab.example -ip 127.0.0.1 -valid-for 168h
```

The generator refuses to overwrite files unless `-force` is provided. The output directory and all PEM files are ignored by Git. It creates a temporary CA certificate, TURN server certificate, and TURN private key; it does not install trust automatically.

For `turns:` tests, trust `testbed/private/turn/ca-cert.pem` only in the owned test endpoint's user trust store. Record the imported certificate thumbprint and remove that exact certificate after the test. Do not distribute or reuse this CA.

## 2. Configure the Testbed

```powershell
Copy-Item testbed/.env.example testbed/.env
```

Replace every `CHANGE_ME`. `TURN_ALLOWED_PEER_IP` must be the authorized WebRTC peer IP or range for this run; this prevents the local TURN service from relaying to arbitrary peers.

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
- mitmproxy regular HTTP(S) proxy on loopback `127.0.0.1:8081`.

## 4. Establish the HTTPS-Bump Baseline

Mitmproxy regular mode handles explicitly proxied HTTP(S). After mitmproxy starts, its temporary CA is written under `testbed/private/mitmproxy`.

Use the owned target URL:

```powershell
curl.exe --proxy http://127.0.0.1:8081 `
  --cacert testbed/private/mitmproxy/mitmproxy-ca-cert.pem `
  https://owned-target.example/
```

Preserve the mitmproxy flow file and logs as the HTTPS inspection baseline. The explicit HTTP proxy does not automatically route arbitrary TURN or DTLS traffic. Therefore, success or failure of `turns:` in this local composition must not be described as an inline bump result. P2-F requires an authorized transparent/TUN/appliance topology documented separately.

## 5. Run the TURN Cases

Set the same values on both WebRTC peers.

TURN UDP:

```powershell
$env:LAB_TURN_URLS = "turn:127.0.0.1:3478?transport=udp"
$env:LAB_TURN_USERNAME = "<value from testbed/.env>"
$env:LAB_TURN_CREDENTIAL = "<value from testbed/.env>"
$env:LAB_ICE_POLICY = "relay"
```

TURN TLS/TCP 443:

```powershell
$env:LAB_TURN_URLS = "turns:127.0.0.1:443?transport=tcp"
$env:LAB_TURN_USERNAME = "<value from testbed/.env>"
$env:LAB_TURN_CREDENTIAL = "<value from testbed/.env>"
$env:LAB_ICE_POLICY = "relay"
```

Verify the logged selected ICE candidate pair contains relay candidates. Configuration alone is not evidence that TURN was selected.

## 6. Capture and Analyze

Wireshark/Npcap supplies `dumpcap`. List interfaces and run a time-bounded capture:

```powershell
./testbed/scripts/capture-part2.ps1 -ListInterfaces
./testbed/scripts/capture-part2.ps1 -Interface 1 -DurationSeconds 120 -RunId P2-C-run01
```

Analyze a completed PCAP without network access inside the sensor containers:

```powershell
./testbed/scripts/analyze-pcap.ps1 -Pcap captures/P2-C-run01/P2-C-run01.pcapng
```

The analysis harness writes separate Suricata and Zeek outputs plus an analysis manifest. Supplying `-SuricataRules` is optional and should be used only for capture-derived rules with documented controls.

## 7. Stop and Clean Up

```powershell
docker compose --env-file testbed/.env -f testbed/compose.yaml down
Remove-Item Env:LAB_TURN_URLS,Env:LAB_TURN_USERNAME,Env:LAB_TURN_CREDENTIAL,Env:LAB_ICE_POLICY,Env:LAB_BROKER_TOKEN -ErrorAction SilentlyContinue
```

Remove the exact temporary CA certificates imported for the run, then archive evidence hashes and sanitized logs. Delete `testbed/.env`, private keys, and temporary credentials when the measurement window closes.
