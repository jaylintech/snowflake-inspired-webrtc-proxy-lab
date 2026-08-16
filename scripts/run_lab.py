#!/usr/bin/env python3
"""Convenience runner for the benign WebRTC lab on Linux."""

from __future__ import annotations

import argparse
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_STUN = "stun:stun.l.google.com:19302"


def command_available(name: str) -> bool:
    return shutil.which(name) is not None


def stun_value(args: argparse.Namespace) -> str:
    return "" if args.no_stun else args.stun


def ice_port_args(args: argparse.Namespace) -> list[str]:
    out: list[str] = []
    if args.ice_port_min != 0 or args.ice_port_max != 0:
        out.extend(
            [
                "-ice-port-min",
                str(args.ice_port_min),
                "-ice-port-max",
                str(args.ice_port_max),
            ]
        )
    if args.advertise_ip:
        out.extend(["-advertise-ip", args.advertise_ip])
    return out


def role_command(role: str, role_args: list[str]) -> list[str]:
    binary = REPO_ROOT / "bin" / role
    if binary.exists():
        return [str(binary), *role_args]

    if command_available("go"):
        return ["go", "run", f"./cmd/{role}", *role_args]

    raise SystemExit(
        f"No bin/{role} binary found and Go is not installed. "
        "Run: python3 scripts/run_lab.py build"
    )


def broker_args(args: argparse.Namespace) -> list[str]:
    return ["-listen", args.listen]


def target_args(args: argparse.Namespace) -> list[str]:
    return ["-listen", args.target_listen]


def listener_args(args: argparse.Namespace) -> list[str]:
    return [
        "-broker",
        args.broker_url,
        "-session",
        args.session,
        "-stun",
        stun_value(args),
        "-task-every",
        str(args.task_every),
        "-task-sequence",
        args.task_sequence,
        "-synthetic-bytes",
        str(args.synthetic_bytes),
        "-chunk-bytes",
        str(args.chunk_bytes),
    ] + ice_port_args(args)


def client_args(args: argparse.Namespace) -> list[str]:
    return [
        "-broker",
        args.broker_url,
        "-session",
        args.session,
        "-stun",
        stun_value(args),
        "-interval",
        args.interval,
        "-jitter",
        str(args.jitter),
        "-count",
        str(args.count),
        "-host-id",
        args.host_id,
        "-task-delay",
        args.task_delay,
        "-chunk-delay",
        args.chunk_delay,
    ] + ice_port_args(args)


def relay_args(args: argparse.Namespace) -> list[str]:
    return [
        "-broker",
        args.broker_url,
        "-session",
        args.session,
        "-stun",
        stun_value(args),
        "-target",
        args.target_url,
        "-max-body",
        str(args.max_body),
    ] + ice_port_args(args)


def webclient_args(args: argparse.Namespace) -> list[str]:
    return [
        "-broker",
        args.broker_url,
        "-session",
        args.session,
        "-stun",
        stun_value(args),
        "-paths",
        args.paths,
        "-method",
        args.method,
        "-body",
        args.body,
        "-interval",
        args.interval,
    ] + ice_port_args(args)


def browserui_args(args: argparse.Namespace) -> list[str]:
    return [
        "-listen",
        args.ui_listen,
        "-broker",
        args.broker_url,
        "-session",
        args.session,
        "-stun",
        stun_value(args),
        "-target-url",
        args.target_url,
    ]


def run_foreground(cmd: list[str]) -> int:
    print("+", " ".join(cmd), flush=True)
    return subprocess.call(cmd, cwd=REPO_ROOT)


def run_broker(args: argparse.Namespace) -> int:
    return run_foreground(role_command("broker", broker_args(args)))


def run_target(args: argparse.Namespace) -> int:
    return run_foreground(role_command("target", target_args(args)))


def run_listener(args: argparse.Namespace) -> int:
    return run_foreground(role_command("listener", listener_args(args)))


def run_client(args: argparse.Namespace) -> int:
    return run_foreground(role_command("client", client_args(args)))


def run_relay(args: argparse.Namespace) -> int:
    return run_foreground(role_command("relay", relay_args(args)))


def run_webclient(args: argparse.Namespace) -> int:
    return run_foreground(role_command("webclient", webclient_args(args)))


def run_browserui(args: argparse.Namespace) -> int:
    return run_foreground(role_command("browserui", browserui_args(args)))


def run_local(args: argparse.Namespace) -> int:
    print(f"Starting safe WebRTC lab session '{args.session}'")
    print(
        "This wrapper emits synthetic lab traffic only; it does not execute "
        "commands, collect files, or install persistence."
    )

    processes: list[subprocess.Popen[bytes]] = []
    try:
        processes.append(
            subprocess.Popen(role_command("broker", broker_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("listener", listener_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        client = subprocess.Popen(role_command("client", client_args(args)), cwd=REPO_ROOT)
        processes.append(client)

        if args.count > 0:
            return client.wait()

        while True:
            exited = [p for p in processes if p.poll() is not None]
            if exited:
                return exited[0].returncode or 0
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopping lab processes...")
        return 130
    finally:
        stop_processes(processes)


def run_relay_local(args: argparse.Namespace) -> int:
    print(f"Starting bounded WebRTC proxy lab session '{args.session}'")
    print(f"The proxy server only connects to the configured target URL: {args.target_url}")

    processes: list[subprocess.Popen[bytes]] = []
    try:
        processes.append(
            subprocess.Popen(role_command("broker", broker_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("target", target_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("relay", relay_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        webclient = subprocess.Popen(
            role_command("webclient", webclient_args(args)), cwd=REPO_ROOT
        )
        processes.append(webclient)
        return webclient.wait()
    except KeyboardInterrupt:
        print("\nStopping proxy lab processes...")
        return 130
    finally:
        stop_processes(processes)


def run_browser_local(args: argparse.Namespace) -> int:
    print(f"Starting bounded WebRTC browser viewer lab session '{args.session}'")
    print(f"The proxy server only connects to the configured target URL: {args.target_url}")

    processes: list[subprocess.Popen[bytes]] = []
    try:
        processes.append(
            subprocess.Popen(role_command("broker", broker_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("target", target_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("relay", relay_args(args)), cwd=REPO_ROOT)
        )
        time.sleep(1)
        processes.append(
            subprocess.Popen(role_command("browserui", browserui_args(args)), cwd=REPO_ROOT)
        )
        print(f"Open http://{args.ui_listen} in a browser on this machine.")
        while True:
            exited = [p for p in processes if p.poll() is not None]
            if exited:
                return exited[0].returncode or 0
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopping browser viewer lab processes...")
        return 130
    finally:
        stop_processes(processes)


def stop_processes(processes: list[subprocess.Popen[bytes]]) -> None:
    for process in reversed(processes):
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)

    deadline = time.time() + 5
    for process in reversed(processes):
        while process.poll() is None and time.time() < deadline:
            time.sleep(0.1)
        if process.poll() is None:
            process.kill()


def run_build(_: argparse.Namespace) -> int:
    bin_dir = REPO_ROOT / "bin"
    bin_dir.mkdir(exist_ok=True)

    if not command_available("go"):
        raise SystemExit(
            "Go 1.22+ is required. Install it from https://go.dev/dl/, "
            "restart your shell, then rerun this command."
        )

    commands = [
        ["go", "mod", "tidy"],
        ["go", "test", "./..."],
        ["go", "build", "-o", "bin", "./cmd/..."],
    ]
    for cmd in commands:
        code = run_foreground(cmd)
        if code != 0:
            return code
    return 0


def run_test(_: argparse.Namespace) -> int:
    if not command_available("go"):
        raise SystemExit(
            "Go 1.22+ is required. Install it from https://go.dev/dl/, "
            "restart your shell, then rerun this command."
        )

    return run_foreground(["go", "test", "./..."])


def run_teardown(args: argparse.Namespace) -> int:
    cert_dir = getattr(args, "out", "testbed/private/turn")
    result = 0
    if command_available("docker"):
        compose_file = REPO_ROOT / "testbed" / "compose.yaml"
        env_file = REPO_ROOT / "testbed" / ".env"
        if compose_file.exists() and env_file.exists():
            print("Stopping testbed containers...")
            result = run_foreground(["docker", "compose", "--env-file", str(env_file), "-f", str(compose_file), "down"])

    cmd = role_command("labcert", ["-out", cert_dir, "-clean"])
    cert_result = run_foreground(cmd)
    if result == 0:
        result = cert_result

    if result != 0:
        print("Teardown did not complete successfully; review the command output above.", file=sys.stderr)
        return result
    print("Teardown complete. If you imported ca-cert.pem into your OS trust store, ensure it is removed.")
    return 0


def run_analyze_telemetry(args: argparse.Namespace) -> int:
    script = REPO_ROOT / "scripts" / "analyze_telemetry.py"
    cmd = [sys.executable, str(script)]
    if getattr(args, "coturn_log", None):
        cmd.extend(["--coturn-log", args.coturn_log])
    if getattr(args, "zeek_dir", None):
        cmd.extend(["--zeek-dir", args.zeek_dir])
    if getattr(args, "eve_json", None):
        cmd.extend(["--eve-json", args.eve_json])
    return run_foreground(cmd)


def add_common_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--session", default="lab-demo")
    parser.add_argument("--broker-url", default="http://127.0.0.1:8080")
    parser.add_argument("--listen", default=":8080")
    parser.add_argument("--target-listen", default=":9090")
    parser.add_argument("--target-url", default="http://127.0.0.1:9090")
    parser.add_argument("--ui-listen", default="127.0.0.1:7777")
    parser.add_argument("--stun", default=DEFAULT_STUN)
    parser.add_argument("--no-stun", action="store_true")
    parser.add_argument("--ice-port-min", type=int, default=0)
    parser.add_argument("--ice-port-max", type=int, default=0)
    parser.add_argument("--advertise-ip", default="")
    parser.add_argument("--max-body", type=int, default=262144)
    parser.add_argument("--interval", default="8s")
    parser.add_argument("--jitter", type=int, default=35)
    parser.add_argument("--count", type=int, default=8)
    parser.add_argument("--host-id", default="Host_ID_8842_Active")
    parser.add_argument("--task-every", type=int, default=2)
    parser.add_argument(
        "--task-sequence", default="sleep,inventory,synthetic-upload"
    )
    parser.add_argument("--synthetic-bytes", type=int, default=8192)
    parser.add_argument("--chunk-bytes", type=int, default=1024)
    parser.add_argument("--task-delay", default="1s")
    parser.add_argument("--chunk-delay", default="250ms")
    parser.add_argument("--paths", default="/,/healthz,/article-proof?via=webrtc")
    parser.add_argument("--method", choices=["GET", "POST"], default="GET")
    parser.add_argument("--body", default="synthetic proxy lab body")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run the benign WebRTC DataChannel lab."
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    commands = {
        "local": run_local,
        "relay-local": run_relay_local,
        "proxy-local": run_relay_local,
        "browser-local": run_browser_local,
        "broker": run_broker,
        "target": run_target,
        "listener": run_listener,
        "client": run_client,
        "relay": run_relay,
        "proxy": run_relay,
        "webclient": run_webclient,
        "browserui": run_browserui,
        "build": run_build,
        "test": run_test,
        "teardown": run_teardown,
        "analyze-telemetry": run_analyze_telemetry,
    }

    for name, func in commands.items():
        subparser = subparsers.add_parser(name)
        if name in {"local", "relay-local", "proxy-local", "browser-local", "broker", "listener", "client", "target", "relay", "proxy", "webclient", "browserui"}:
            add_common_args(subparser)
        elif name == "teardown":
            subparser.add_argument("--out", default="testbed/private/turn")
        elif name == "analyze-telemetry":
            subparser.add_argument("--coturn-log", default="")
            subparser.add_argument("--zeek-dir", default="")
            subparser.add_argument("--eve-json", default="")
        subparser.set_defaults(func=func)

    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except BrokenPipeError:
        os._exit(1)
