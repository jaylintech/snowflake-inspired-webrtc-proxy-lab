# PoC Report Template

## Run Summary

- Run ID:
- Date/time:
- Operator:
- Purpose:
- Target authorization:

## Topology

- Monitored client:
- SSL-bumping router / TLS-inspection device:
- Broker URL:
- Proxy host:
- Configured target URL:
- STUN/TURN configuration:
- Fixed ICE port / advertised IP:

## Commands

### Broker

```text

```

### Proxy

```text

```

### Browser Viewer

```text

```

## Expected Observations

- Direct client target access result:
- Broker reachability result:
- WebRTC/DataChannel result:
- Rendered page result:
- Same-origin CSS/image asset result:
- Client DNS observation for final target:
- Router/firewall observation for WebRTC/STUN:
- Target source IP observation:

## Evidence

- Browser viewer screenshot:
- Browser viewer log excerpt:
- Broker log excerpt:
- Proxy log excerpt:
- Target log excerpt:
- Router/firewall log excerpt:
- Packet capture path, if collected:

## Interpretation

- What loaded through the DataChannel:
- What did not load, and why:
- Whether TLS inspection saw final-target HTTPS directly from the monitored client:
- Whether controls observed or blocked WebRTC/STUN/UDP:
- Residual limitations:

## Cleanup

- Temporary firewall rules removed:
- Temporary proxy host stopped/destroyed:
- Logs preserved:
