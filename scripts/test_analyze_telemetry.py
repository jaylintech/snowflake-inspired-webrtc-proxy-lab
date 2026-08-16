import json
import tempfile
import unittest
from pathlib import Path

from analyze_telemetry import parse_coturn_log, parse_suricata_eve, parse_zeek_logs


class TelemetryAnalyzerTests(unittest.TestCase):
    def test_parse_coturn_log(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "coturn.log"
            path.write_text(
                "allocate request\nCHANNEL_BIND peer 192.0.2.20\n"
                "CREATE_PERMISSION peer 192.0.2.20\nTLS connection\n",
                encoding="utf-8",
            )
            result = parse_coturn_log(path)
            self.assertEqual(result["allocations"], 1)
            self.assertEqual(result["channel_binds"], 1)
            self.assertEqual(result["permissions"], 1)
            self.assertEqual(result["tls_connections"], 1)
            self.assertEqual(result["peers"], {"192.0.2.20"})

    def test_parse_zeek_uses_fields_header(self):
        with tempfile.TemporaryDirectory() as directory:
            zeek_dir = Path(directory)
            (zeek_dir / "conn.log").write_text(
                "#fields\tts\tuid\n1.0\tC1\n", encoding="utf-8"
            )
            (zeek_dir / "ssl.log").write_text(
                "#separator \\x09\n"
                "#fields\tts\tuid\tid.orig_h\tid.orig_p\tid.resp_h\tid.resp_p\tversion\tcipher\tcurve\tserver_name\n"
                "1.0\tC1\t192.0.2.1\t50000\t192.0.2.10\t443\tTLSv13\tTLS_AES_128_GCM_SHA256\tx25519\tturn.lab.example\n"
                "2.0\tC2\t192.0.2.1\t50001\t192.0.2.10\t3478\tDTLSv12\tTLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256\tx25519\t-\n",
                encoding="utf-8",
            )
            result = parse_zeek_logs(zeek_dir)
            self.assertEqual(result["connections"], 1)
            self.assertEqual(result["tls_sessions"], 1)
            self.assertEqual(result["dtls_sessions"], 1)
            self.assertEqual(result["snis"], {"turn.lab.example"})

    def test_parse_suricata_eve(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "eve.json"
            records = [
                {"event_type": "stun"},
                {"event_type": "tls"},
                {"event_type": "alert", "alert": {"signature": "Lab STUN"}},
            ]
            path.write_text(
                "\n".join(json.dumps(record) for record in records) + "\n",
                encoding="utf-8",
            )
            result = parse_suricata_eve(path)
            self.assertEqual(result["stun_events"], 1)
            self.assertEqual(result["tls_events"], 1)
            self.assertEqual(result["alerts"], ["Lab STUN"])


if __name__ == "__main__":
    unittest.main()
