# SSL-Bump Static Viewer Checklist

This checklist is for an authorized lab where the monitored client sits behind an SSL-bumping router or TLS-inspection gateway.

## Preconditions

- The controlled target is owned or explicitly authorized.
- The proxy is configured with one target URL only.
- The broker is reachable from the monitored client.
- A reachable ICE candidate exists, such as a fixed UDP port forwarded to the proxy host.
- The browser viewer target hint exactly matches the proxy `-TargetUrl`.
- You have a way to collect router/firewall, proxy, browser-viewer, and target logs.

## Test Steps

1. Confirm direct browsing behavior through the SSL-bumping router.
   - Result:
   - Evidence:

2. Start broker and proxy with the bounded target URL.
   - Broker command:
   - Proxy command:
   - Evidence:

3. Start `browserui` on the monitored client and connect.
   - Browser UI command:
   - DataChannel state:
   - Evidence:

4. Request `/` and one known same-origin static page path.
   - Paths:
   - Status codes:
   - Render result:

5. Confirm same-origin static assets were fetched through the DataChannel.
   - CSS fetch log lines:
   - Image fetch log lines:
   - Any skipped assets:

6. Confirm the monitored client did not directly resolve or connect to the final target outside the viewer.
   - DNS observation:
   - Firewall observation:

7. Confirm the target saw the proxy host as source.
   - Target log:

## Pass Criteria

- Broker signaling works through the inspected network path.
- WebRTC ICE reaches connected/completed state.
- The page HTML returns through the DataChannel.
- Same-origin CSS/images needed by a static page return through the DataChannel.
- Cross-origin assets, scripts, forms, and frames remain disabled by the viewer.
- The controlled target logs the proxy host as requester.

## Known Limitations

- This is not a full transparent browser proxy.
- JavaScript-heavy applications, login flows, service workers, WebSockets, media, fonts, and complex CSP behavior may not render correctly.
- Networks can still detect or block broker signaling, STUN, ICE, UDP, DTLS, or WebRTC behavior.
- TURN is not bundled in this PoC.
