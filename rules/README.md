# Detection Rules

This directory is reserved for validated Suricata and Zeek detections derived from Part 2 packet captures.

Rules are intentionally not pre-populated from assumptions. A rule belongs here only when its source run, sensor version, capture point, expected match, negative controls, and known false-positive scope are documented.

## Validation Checklist

1. Confirm the sensor loaded the rule/analyzer successfully.
2. Replay or rerun the named positive capture.
3. Test unrelated browser WebRTC and unrelated Pion traffic as negative controls.
4. Record alert identifiers, counts, and flow tuples.
5. Separate generic STUN/TURN/DTLS visibility from tool-specific fingerprinting.
6. Avoid payload signatures for synthetic strings unless the test explicitly measures decrypted or endpoint-visible content.

Raw PCAP files and secrets must not be committed.
