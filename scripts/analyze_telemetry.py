#!/usr/bin/env python3
"""
Cross-Platform Offline Telemetry Analyzer for Snowflake WebRTC Proxy Lab (Part 2).
Parses Coturn event logs, Zeek conn.log/ssl.log, and Suricata eve.json to verify
STUN/TURN protocol events, DTLS handshakes, and transport detectability metrics.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


def parse_coturn_log(coturn_path: Path) -> dict:
    summary = {
        "allocations": 0,
        "channel_binds": 0,
        "permissions": 0,
        "tls_connections": 0,
        "peers": set(),
    }
    if not coturn_path.exists():
        return summary

    with coturn_path.open("r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            if "incoming packet ALLOCATE" in line or "allocate request" in line.lower():
                summary["allocations"] += 1
            if "CHANNEL_BIND" in line:
                summary["channel_binds"] += 1
            if "CREATE_PERMISSION" in line:
                summary["permissions"] += 1
            if "TLS/TCP socket accepted" in line or "TLS connection" in line:
                summary["tls_connections"] += 1

            ip_match = re.search(r"peer\s+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)", line)
            if ip_match:
                summary["peers"].add(ip_match.group(1))

    return summary


def parse_zeek_logs(zeek_dir: Path) -> dict:
    summary = {"connections": 0, "dtls_sessions": 0, "tls_sessions": 0, "snis": set()}
    ssl_log = zeek_dir / "ssl.log"
    conn_log = zeek_dir / "conn.log"

    if conn_log.exists():
        with conn_log.open("r", encoding="utf-8", errors="ignore") as f:
            for line in f:
                if not line.startswith("#"):
                    summary["connections"] += 1

    if ssl_log.exists():
        fields: list[str] = []
        with ssl_log.open("r", encoding="utf-8", errors="ignore") as f:
            for line in f:
                if line.startswith("#fields"):
                    fields = line.rstrip("\n").split("\t")[1:]
                    continue
                if line.startswith("#"):
                    continue
                parts = line.strip().split("\t")
                if not fields or len(parts) < len(fields):
                    continue
                record = dict(zip(fields, parts))
                version = record.get("version", "")
                server_name = record.get("server_name", "")
                if "DTLS" in version:
                    summary["dtls_sessions"] += 1
                elif "TLS" in version:
                    summary["tls_sessions"] += 1
                if server_name and server_name != "-":
                    summary["snis"].add(server_name)

    return summary


def parse_suricata_eve(eve_path: Path) -> dict:
    summary = {"alerts": [], "stun_events": 0, "tls_events": 0}
    if not eve_path.exists():
        return summary

    with eve_path.open("r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            try:
                record = json.loads(line.strip())
                event_type = record.get("event_type")
                if event_type == "alert":
                    summary["alerts"].append(record.get("alert", {}).get("signature", "Unknown Alert"))
                elif event_type == "stun":
                    summary["stun_events"] += 1
                elif event_type == "tls":
                    summary["tls_events"] += 1
            except json.JSONDecodeError:
                continue

    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description="Analyze Part 2 WebRTC Telemetry Artifacts")
    parser.add_argument("--coturn-log", type=Path, help="Path to Coturn log file")
    parser.add_argument("--zeek-dir", type=Path, help="Directory containing Zeek logs")
    parser.add_argument("--eve-json", type=Path, help="Path to Suricata eve.json")
    args = parser.parse_args()

    print("=== WebRTC Lab Telemetry Analysis ===")
    if args.coturn_log:
        coturn = parse_coturn_log(args.coturn_log)
        print(f"[Coturn] Allocations: {coturn['allocations']}, Channel Binds: {coturn['channel_binds']}, Permissions: {coturn['permissions']}, TLS Sockets: {coturn['tls_connections']}")
    if args.zeek_dir:
        zeek = parse_zeek_logs(args.zeek_dir)
        print(f"[Zeek] Total Connections: {zeek['connections']}, DTLS: {zeek['dtls_sessions']}, TLS: {zeek['tls_sessions']}, Observed SNIs: {list(zeek['snis'])}")
    if args.eve_json:
        suri = parse_suricata_eve(args.eve_json)
        print(f"[Suricata] Alerts Triggered: {len(suri['alerts'])}, STUN Events: {suri['stun_events']}, TLS Events: {suri['tls_events']}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
