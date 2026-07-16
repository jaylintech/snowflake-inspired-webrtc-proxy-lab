# PoC Artifacts

Use these templates to keep defensive lab runs repeatable and easy to review.

Recommended bundle for each run:

- `poc-report-template.md`: copy this into a dated run folder and fill in the topology, commands, and observations.
- `ssl-bump-test-checklist.md`: use this while testing behind an SSL-bumping router or TLS-inspection gateway.
- Broker log: SDP offer/answer and client source IP.
- Proxy log: ICE candidates, DataChannel state, target request URLs, status codes, and truncation notes.
- Browser viewer screenshot: connected state, requested path, rendered output, and log panel.
- Network evidence: router/firewall logs for broker signaling, STUN, and WebRTC/UDP traffic.
- Target evidence: access log showing the proxy host as the requester.

Keep targets owned or explicitly authorized. The lab is intentionally bounded to one configured target URL and is not an open proxy.
