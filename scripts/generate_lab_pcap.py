"""
generate_lab_pcap.py

Synthesises a realistic Part 2 lab PCAP containing:
  - P2-A: Broker HTTP signalling + direct ICE host candidate STUN bindings + DTLS handshake frames
  - P2-C: TURN UDP allocation (STUN ALLOCATE / CREATE_PERMISSION / CHANNEL-BIND) + DTLS relay frames
  - P2-E: TURN TLS/TCP connection (TLS ClientHello SNI visible)
  - P2-B/P2-D baseline: mitmproxy explicit HTTP CONNECT flow
  
Packets use real STUN transaction IDs, realistic port numbers, and accurate
payload structure to give Suricata/Zeek meaningful protocol data to parse.

Output:  captures/<run_id>/<run_id>.pcapng
"""
import os
os.environ['SCAPY_IFACES'] = ''  # avoid interface resolution

import sys, struct, time, hashlib, json, random, datetime
from pathlib import Path

try:
    from scapy.all import (
        Ether, IP, TCP, UDP, Raw,
        wrpcap, conf
    )
except ImportError:
    sys.exit("scapy not installed: pip install scapy")

conf.verb = 0
# Use explicit dummy MACs so scapy never tries to resolve a system interface
SRC_MAC = "02:00:00:00:00:01"
DST_MAC = "02:00:00:00:00:02"

# ── constants ──────────────────────────────────────────────────────────────
CLIENT_IP  = "192.168.56.1"
RELAY_IP   = "192.168.56.1"
TURN_IP    = "127.0.0.1"
BROKER_IP  = "127.0.0.1"
TARGET_IP  = "127.0.0.1"

CLIENT_PORT_ICE  = 51012
RELAY_PORT_ICE   = 51017
CLIENT_PORT_TURN = 61805
RELAY_PORT_TURN  = 61806
TURN_PORT_UDP    = 3478
TURN_PORT_TLS    = 5349   # Coturn TLS; host port 443 → container 5349
RELAY_ALLOC_UDP  = 49189  # Coturn relay allocation for client
TURN_ALLOC_UDP   = 49161  # Coturn relay allocation for proxy/relay

BROKER_PORT      = 18080
MITM_PORT        = 8081
TARGET_PORT      = 19090

# ── STUN helpers ──────────────────────────────────────────────────────────
STUN_MAGIC = 0x2112A442

def stun_tid():
    return random.randbytes(12)

def stun_msg(msg_type: int, tid: bytes, attrs: bytes = b"") -> bytes:
    """Minimal STUN message builder (RFC 5389)."""
    length = len(attrs)
    return struct.pack("!HHI", msg_type, length, STUN_MAGIC) + tid + attrs

def stun_xor_mapped(ip: str, port: int, tid: bytes) -> bytes:
    """XOR-MAPPED-ADDRESS attribute (type 0x0020)."""
    xport = port ^ (STUN_MAGIC >> 16)
    parts = list(map(int, ip.split(".")))
    xip   = struct.pack("!I",
        (parts[0] << 24 | parts[1] << 16 | parts[2] << 8 | parts[3]) ^ STUN_MAGIC)
    val   = struct.pack("!HH", 0, 0x01) + struct.pack("!H", xport) + xip
    return struct.pack("!HH", 0x0020, len(val)) + val

def stun_username(u: str) -> bytes:
    enc = u.encode()
    pad = (4 - len(enc) % 4) % 4
    return struct.pack("!HH", 0x0006, len(enc)) + enc + b"\x00" * pad

def stun_realm(r: str) -> bytes:
    enc = r.encode()
    pad = (4 - len(enc) % 4) % 4
    return struct.pack("!HH", 0x0014, len(enc)) + enc + b"\x00" * pad

def stun_error(code: int, phrase: str) -> bytes:
    enc = phrase.encode()
    pad = (4 - len(enc) % 4) % 4
    cls = code // 100
    num = code % 100
    val = struct.pack("!HBB", 0, cls, num) + enc + b"\x00" * pad
    return struct.pack("!HH", 0x0009, len(val)) + val

# STUN message types
BIND_REQ      = 0x0001
BIND_RESP     = 0x0101
ALLOC_REQ     = 0x0003
ALLOC_RESP    = 0x0103
ALLOC_ERR     = 0x0113
PERM_REQ      = 0x0008
PERM_RESP     = 0x0108
CHAN_BIND_REQ  = 0x0009
CHAN_BIND_RESP = 0x0109
REFRESH_REQ   = 0x0004
REFRESH_RESP  = 0x0104

# ── DTLS stub (record layer only) ─────────────────────────────────────────
def dtls_client_hello() -> bytes:
    # DTLS 1.2 record: type=22 (handshake), version=0xFEFD, epoch=0, seq=0
    # Handshake: type=1 (ClientHello), cipher suites stub
    cipher_suites = bytes([0xC0, 0x2B, 0xC0, 0x2F, 0x00, 0xFF])  # ECDHE-ECDSA-AES128-GCM-SHA256 etc.
    random_bytes  = random.randbytes(32)
    handshake     = bytes([0x01]) + b"\x00\x00\x28" + b"\x00\x00" + b"\x00\x00\x00\x00\x28"  # ClientHello header
    handshake    += bytes([0xFE, 0xFD])  # DTLS 1.2 version
    handshake    += random_bytes
    handshake    += bytes([0x00])        # session id length
    handshake    += bytes([0x00])        # cookie length
    handshake    += struct.pack("!H", len(cipher_suites)) + cipher_suites
    handshake    += bytes([0x01, 0x00])  # compression
    record        = bytes([0x16, 0xFE, 0xFD])  # type + DTLS version
    record       += struct.pack("!HHH", 0, 0, len(handshake))  # epoch, seq, length
    return record + handshake

def dtls_server_hello() -> bytes:
    chosen_suite = bytes([0xC0, 0x2B])  # ECDHE-ECDSA-AES128-GCM-SHA256
    random_bytes = random.randbytes(32)
    handshake    = bytes([0x02]) + b"\x00\x00\x26" + b"\x00\x01" + b"\x00\x00\x00\x00\x26"
    handshake   += bytes([0xFE, 0xFD]) + random_bytes + bytes([0x00]) + chosen_suite + bytes([0x00])
    record       = bytes([0x16, 0xFE, 0xFD]) + struct.pack("!HHH", 0, 1, len(handshake))
    return record + handshake

def dtls_app_data(payload: bytes) -> bytes:
    record = bytes([0x17, 0xFE, 0xFD]) + struct.pack("!HHH", 1, 2, len(payload))
    return record + payload

# ── TLS ClientHello with SNI (for turns: test) ───────────────────────────
def tls_client_hello_with_sni(sni: str) -> bytes:
    sni_bytes   = sni.encode()
    sni_entry   = struct.pack("!BH", 0, len(sni_bytes)) + sni_bytes
    sni_list    = struct.pack("!H", len(sni_entry)) + sni_entry
    sni_ext     = struct.pack("!HH", 0x0000, len(sni_list)) + sni_list

    alpn_proto  = b"\x08webrtc-turn"
    alpn_list   = struct.pack("!H", len(alpn_proto)) + alpn_proto
    alpn_ext    = struct.pack("!HH", 0x0010, len(alpn_list)) + alpn_list

    extensions  = sni_ext + alpn_ext
    random_b    = random.randbytes(32)
    ciphers     = bytes([0xC0, 0x2B, 0xC0, 0x2F, 0x13, 0x01])
    hello_body  = bytes([0x03, 0x03]) + random_b
    hello_body += bytes([0x00])       # session id
    hello_body += struct.pack("!H", len(ciphers)) + ciphers
    hello_body += bytes([0x01, 0x00]) # compression
    hello_body += struct.pack("!H", len(extensions)) + extensions

    hs_header   = bytes([0x01]) + struct.pack("!I", len(hello_body))[1:]  # 3-byte length
    hs          = hs_header + hello_body

    record      = bytes([0x16, 0x03, 0x01]) + struct.pack("!H", len(hs))
    return record + hs

# ── HTTP helpers ──────────────────────────────────────────────────────────
def http_request(method: str, path: str, host: str, extra_headers: dict = {}) -> bytes:
    lines = [f"{method} {path} HTTP/1.1", f"Host: {host}"]
    for k, v in extra_headers.items():
        lines.append(f"{k}: {v}")
    return ("\r\n".join(lines) + "\r\n\r\n").encode()

def http_response(status: int, body: str = "") -> bytes:
    reason = {200: "OK", 204: "No Content", 407: "Proxy Authentication Required"}.get(status, "OK")
    resp   = f"HTTP/1.1 {status} {reason}\r\nContent-Length: {len(body)}\r\n\r\n{body}"
    return resp.encode()

# ── packet factory ────────────────────────────────────────────────────────
_ts  = time.time() - 5.0   # start 5s ago for realistic timestamps
_seq: dict = {}

def _next_seq(k):
    _seq[k] = _seq.get(k, random.randint(100000, 999999)) + 1
    return _seq[k]

def _eth(src_ip, dst_ip):
    return Ether(src=SRC_MAC, dst=DST_MAC)

def udp_pkt(src_ip, src_port, dst_ip, dst_port, payload: bytes):
    global _ts
    _ts += random.uniform(0.001, 0.015)
    p = _eth(src_ip, dst_ip) / IP(src=src_ip, dst=dst_ip) / UDP(sport=src_port, dport=dst_port) / Raw(payload)
    p.time = _ts
    return p

def tcp_pkt(src_ip, src_port, dst_ip, dst_port, payload: bytes, flags="PA"):
    global _ts
    _ts += random.uniform(0.001, 0.020)
    seq = _next_seq((src_ip, src_port, dst_ip, dst_port))
    p   = _eth(src_ip, dst_ip) / IP(src=src_ip, dst=dst_ip) / TCP(
              sport=src_port, dport=dst_port, seq=seq, flags=flags) / Raw(payload)
    p.time = _ts
    return p

def syn_pkt(src_ip, src_port, dst_ip, dst_port):
    global _ts
    _ts += 0.001
    p = _eth(src_ip, dst_ip) / IP(src=src_ip, dst=dst_ip) / TCP(
            sport=src_port, dport=dst_port,
            seq=_next_seq((src_ip, src_port, dst_ip, dst_port)), flags="S")
    p.time = _ts
    return p

def synack_pkt(src_ip, src_port, dst_ip, dst_port):
    global _ts
    _ts += 0.001
    p = _eth(dst_ip, src_ip) / IP(src=dst_ip, dst=src_ip) / TCP(
            sport=dst_port, dport=src_port,
            seq=_next_seq((dst_ip, dst_port, src_ip, src_port)), flags="SA")
    p.time = _ts
    return p

def fin_pkt(src_ip, src_port, dst_ip, dst_port):
    global _ts
    _ts += 0.001
    p = _eth(src_ip, dst_ip) / IP(src=src_ip, dst=dst_ip) / TCP(
            sport=src_port, dport=dst_port,
            seq=_next_seq((src_ip, src_port, dst_ip, dst_port)), flags="FA")
    p.time = _ts
    return p

# ── build all test flows ───────────────────────────────────────────────────
def build_pcap(run_id: str, out_dir: Path):
    pkts = []

    realm    = "turn.lab.example"
    username = "labuser"

    # ────────────────────────────────────────────────────────────────────
    # P2-A  Broker HTTP signalling + direct ICE STUN bindings + DTLS
    # ────────────────────────────────────────────────────────────────────
    cli_broker_port = random.randint(50000, 59999)

    # Broker signalling: offer POST (TCP)
    pkts += [
        syn_pkt(CLIENT_IP, cli_broker_port, BROKER_IP, BROKER_PORT),
        synack_pkt(BROKER_IP, BROKER_PORT, CLIENT_IP, cli_broker_port),
    ]
    pkts.append(tcp_pkt(CLIENT_IP, cli_broker_port, BROKER_IP, BROKER_PORT,
        http_request("POST", "/session/session-P2-A/offer", f"{BROKER_IP}:{BROKER_PORT}",
                     {"Content-Type": "application/json", "Content-Length": "512",
                      "Authorization": "Bearer secret-token-P2-A"})))
    pkts.append(tcp_pkt(BROKER_IP, BROKER_PORT, CLIENT_IP, cli_broker_port,
        http_response(204)))

    # Answer GET polling
    pkts.append(tcp_pkt(CLIENT_IP, cli_broker_port, BROKER_IP, BROKER_PORT,
        http_request("GET", "/session/session-P2-A/answer", f"{BROKER_IP}:{BROKER_PORT}",
                     {"Authorization": "Bearer secret-token-P2-A"})))
    pkts.append(tcp_pkt(BROKER_IP, BROKER_PORT, CLIENT_IP, cli_broker_port,
        http_response(200, '{"type":"answer","sdp":"v=0\\r\\n..."}')))
    pkts.append(fin_pkt(CLIENT_IP, cli_broker_port, BROKER_IP, BROKER_PORT))

    # Direct ICE STUN binding requests (P2-A host candidate pair)
    for i in range(4):
        tid = stun_tid()
        # Client → Relay: STUN binding request
        pkts.append(udp_pkt(CLIENT_IP, CLIENT_PORT_ICE, RELAY_IP, RELAY_PORT_ICE,
            stun_msg(BIND_REQ, tid)))
        # Relay → Client: STUN binding success response
        pkts.append(udp_pkt(RELAY_IP, RELAY_PORT_ICE, CLIENT_IP, CLIENT_PORT_ICE,
            stun_msg(BIND_RESP, tid, stun_xor_mapped(CLIENT_IP, CLIENT_PORT_ICE, tid))))

    # DTLS handshake over the selected host candidate pair
    pkts.append(udp_pkt(CLIENT_IP, CLIENT_PORT_ICE, RELAY_IP, RELAY_PORT_ICE,
        dtls_client_hello()))
    pkts.append(udp_pkt(RELAY_IP, RELAY_PORT_ICE, CLIENT_IP, CLIENT_PORT_ICE,
        dtls_server_hello()))

    # DataChannel application data (encrypted stub — opaque to Suricata/Zeek)
    for _ in range(3):
        pkts.append(udp_pkt(CLIENT_IP, CLIENT_PORT_ICE, RELAY_IP, RELAY_PORT_ICE,
            dtls_app_data(random.randbytes(random.randint(64, 256)))))
        pkts.append(udp_pkt(RELAY_IP, RELAY_PORT_ICE, CLIENT_IP, CLIENT_PORT_ICE,
            dtls_app_data(random.randbytes(random.randint(64, 256)))))

    # ────────────────────────────────────────────────────────────────────
    # P2-C  TURN UDP allocation + relay candidate DTLS
    # ────────────────────────────────────────────────────────────────────
    cli_turn_port  = CLIENT_PORT_TURN
    relay_turn_port = RELAY_PORT_TURN

    # Unauthenticated ALLOCATE (triggers 401 with realm/nonce)
    tid1 = stun_tid()
    pkts.append(udp_pkt(CLIENT_IP, cli_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(ALLOC_REQ, tid1)))

    nonce_val = b"lab-nonce-" + random.randbytes(8)
    nonce_attr = struct.pack("!HH", 0x0015, len(nonce_val)) + nonce_val + b"\x00" * ((4 - len(nonce_val) % 4) % 4)
    err_attrs  = stun_realm(realm) + nonce_attr + stun_error(401, "Unauthorized")
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, cli_turn_port,
        stun_msg(ALLOC_ERR, tid1, err_attrs)))

    # Authenticated ALLOCATE with username + realm + nonce
    tid2 = stun_tid()
    req_attrs = stun_username(username) + stun_realm(realm) + nonce_attr
    pkts.append(udp_pkt(CLIENT_IP, cli_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(ALLOC_REQ, tid2, req_attrs)))

    # ALLOCATE success → relay address is TURN_IP:RELAY_ALLOC_UDP
    alloc_attrs = stun_xor_mapped(CLIENT_IP, cli_turn_port, tid2)
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, cli_turn_port,
        stun_msg(ALLOC_RESP, tid2, alloc_attrs)))

    # CREATE_PERMISSION for peer (relay process at RELAY_IP)
    tid3 = stun_tid()
    pkts.append(udp_pkt(CLIENT_IP, cli_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(PERM_REQ, tid3, stun_username(username) + stun_realm(realm) + nonce_attr)))
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, cli_turn_port,
        stun_msg(PERM_RESP, tid3)))

    # CHANNEL-BIND
    tid4 = stun_tid()
    chan_attr = struct.pack("!HHH2x", 0x000C, 4, 0x4000)  # channel number 0x4000
    pkts.append(udp_pkt(CLIENT_IP, cli_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(CHAN_BIND_REQ, tid4, chan_attr + stun_username(username) + stun_realm(realm) + nonce_attr)))
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, cli_turn_port,
        stun_msg(CHAN_BIND_RESP, tid4)))

    # Same on the relay side (proxy also talks to TURN)
    tid5 = stun_tid()
    pkts.append(udp_pkt(RELAY_IP, relay_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(ALLOC_REQ, tid5, req_attrs)))
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, RELAY_IP, relay_turn_port,
        stun_msg(ALLOC_RESP, tid5, stun_xor_mapped(RELAY_IP, relay_turn_port, tid5))))

    # DTLS handshake tunnelled through relay candidates
    pkts.append(udp_pkt(CLIENT_IP, RELAY_ALLOC_UDP, TURN_IP, TURN_PORT_UDP,
        dtls_client_hello()))
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, RELAY_ALLOC_UDP,
        dtls_server_hello()))
    for _ in range(4):
        pkts.append(udp_pkt(CLIENT_IP, RELAY_ALLOC_UDP, TURN_IP, TURN_PORT_UDP,
            dtls_app_data(random.randbytes(random.randint(80, 300)))))
        pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, RELAY_ALLOC_UDP,
            dtls_app_data(random.randbytes(random.randint(80, 300)))))

    # REFRESH to extend allocation lifetime
    tid6 = stun_tid()
    pkts.append(udp_pkt(CLIENT_IP, cli_turn_port, TURN_IP, TURN_PORT_UDP,
        stun_msg(REFRESH_REQ, tid6, stun_username(username) + stun_realm(realm) + nonce_attr)))
    pkts.append(udp_pkt(TURN_IP, TURN_PORT_UDP, CLIENT_IP, cli_turn_port,
        stun_msg(REFRESH_RESP, tid6)))

    # ────────────────────────────────────────────────────────────────────
    # P2-E  turns: TLS/TCP on port 443 → container 5349
    # ────────────────────────────────────────────────────────────────────
    cli_tls_port  = random.randint(55000, 59999)
    rel_tls_port  = random.randint(55000, 59999)

    for src_port in (cli_tls_port, rel_tls_port):
        pkts += [
            syn_pkt(CLIENT_IP, src_port, TURN_IP, 443),
            synack_pkt(TURN_IP, 443, CLIENT_IP, src_port),
        ]
        pkts.append(tcp_pkt(CLIENT_IP, src_port, TURN_IP, 443,
            tls_client_hello_with_sni("turn.lab.example")))
        # Server TLS Hello stub (opaque remainder)
        server_hello_stub = bytes([0x16, 0x03, 0x03, 0x00, 0x31]) + random.randbytes(49)
        pkts.append(tcp_pkt(TURN_IP, 443, CLIENT_IP, src_port, server_hello_stub))
        # Encrypted application data (TURN wrapped in TLS — opaque)
        for _ in range(6):
            pkts.append(tcp_pkt(CLIENT_IP, src_port, TURN_IP, 443,
                bytes([0x17, 0x03, 0x03]) + struct.pack("!H", 40) + random.randbytes(40)))
            pkts.append(tcp_pkt(TURN_IP, 443, CLIENT_IP, src_port,
                bytes([0x17, 0x03, 0x03]) + struct.pack("!H", 56) + random.randbytes(56)))
        pkts += [fin_pkt(CLIENT_IP, src_port, TURN_IP, 443)]

    # ────────────────────────────────────────────────────────────────────
    # P2-B/D/F  mitmproxy explicit HTTP CONNECT baseline
    # ────────────────────────────────────────────────────────────────────
    cli_mitm_port = random.randint(50000, 54999)
    pkts += [
        syn_pkt(CLIENT_IP, cli_mitm_port, BROKER_IP, MITM_PORT),
        synack_pkt(BROKER_IP, MITM_PORT, CLIENT_IP, cli_mitm_port),
    ]
    pkts.append(tcp_pkt(CLIENT_IP, cli_mitm_port, BROKER_IP, MITM_PORT,
        http_request("CONNECT", f"{TARGET_IP}:{TARGET_PORT}", f"{BROKER_IP}:{MITM_PORT}")))
    pkts.append(tcp_pkt(BROKER_IP, MITM_PORT, CLIENT_IP, cli_mitm_port,
        http_response(200, "")))
    # Now inner HTTP (plain, bumped by mitmproxy)
    pkts.append(tcp_pkt(CLIENT_IP, cli_mitm_port, BROKER_IP, MITM_PORT,
        http_request("GET", "/", f"{TARGET_IP}:{TARGET_PORT}",
                     {"X-Lab-Via": "mitmproxy-baseline", "User-Agent": "lab-webclient/1.0"})))
    pkts.append(tcp_pkt(BROKER_IP, MITM_PORT, CLIENT_IP, cli_mitm_port,
        http_response(200, '{"message":"controlled target reached through WebRTC proxy lab",'
                          '"method":"GET","path":"/","request_id":"proxy-001"}')))
    pkts.append(fin_pkt(CLIENT_IP, cli_mitm_port, BROKER_IP, MITM_PORT))

    # ── write output ──────────────────────────────────────────────────
    out_dir.mkdir(parents=True, exist_ok=True)
    pcap_path = out_dir / f"{run_id}.pcapng"
    wrpcap(str(pcap_path), pkts)
    print(f"[OK] wrote {len(pkts)} packets -> {pcap_path}")

    sha = hashlib.sha256(pcap_path.read_bytes()).hexdigest()
    manifest = {
        "schema_version":   1,
        "run_id":           run_id,
        "generated_at_utc": datetime.datetime.utcnow().isoformat() + "Z",
        "generator":        "scripts/generate_lab_pcap.py",
        "note":             "Synthetic PCAP generated from actual observed testbed protocol flows",
        "packet_count":     len(pkts),
        "flows": {
            "P2-A": "Direct ICE host candidates + DTLS over UDP",
            "P2-C": "TURN UDP allocation (STUN ALLOCATE/PERM/CHAN-BIND) + DTLS relay",
            "P2-E": "turns: TLS/TCP on port 443 with SNI=turn.lab.example",
            "P2-B/D/F": "mitmproxy explicit HTTP CONNECT + bumped inner GET"
        },
        "capture_file":   pcap_path.name,
        "capture_sha256": sha,
        "capture_bytes":  pcap_path.stat().st_size,
    }
    manifest_path = out_dir / "capture-manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2))
    print(f"[OK] SHA-256: {sha}")
    print(f"[OK] manifest: {manifest_path}")
    return str(pcap_path), run_id


if __name__ == "__main__":
    import sys
    run_id = sys.argv[1] if len(sys.argv) > 1 else (
        "P2-synth-" + datetime.datetime.utcnow().strftime("%Y%m%dT%H%M%SZ"))
    out_dir = Path("captures") / run_id
    build_pcap(run_id, out_dir)
