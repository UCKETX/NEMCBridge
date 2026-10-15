"""Run the real BDS permission regression in an isolated test world."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bds-dir", type=Path, required=True)
    parser.add_argument("--scenario", choices=("permissions", "identities"), default="permissions")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    source = args.bds_dir.resolve()
    target = root / "runtime" / (args.scenario + "-bds-test")
    identity_test = args.scenario == "identities"
    port = 19144 if identity_test else 19142
    uid_prefix = str(time.time_ns() // 1000)
    if source == target.resolve():
        parser.error("--bds-dir must point to the original BDS distribution")
    executable = "bedrock_server.exe" if os.name == "nt" else "bedrock_server"
    if not (source / executable).is_file():
        parser.error(f"Missing {executable} in --bds-dir")
    target.mkdir(parents=True, exist_ok=True)
    for name in ("resource_packs", "behavior_packs", "definitions", "config"):
        if not (target / name).exists():
            shutil.copytree(source / name, target / name)
    for path in source.iterdir():
        if path.is_file() and (path.name == executable or path.suffix in (".dll", ".so")
                              or path.name in ("packetlimitconfig.json", "profanity_filter.wlist")):
            shutil.copy2(path, target / path.name)
    for name in ("worlds", "development_behavior_packs", "development_resource_packs",
                 "development_skin_packs", "world_templates"):
        (target / name).mkdir(exist_ok=True)
    (target / "allowlist.json").write_text("[]\n", encoding="utf-8")
    digest = hashlib.sha256(b"NEMCBridge/netease/xuid/v1\0uid:1261597877").digest()
    xuid = str((int.from_bytes(digest[:8], "big") & 0x00ffffffffffffff) | 0x4e00000000000000)
    permissions = [{"permission": "operator", "xuid": xuid}]
    if identity_test:
        permissions = []
        for suffix in ("1", "2"):
            digest = hashlib.sha256(("NEMCBridge/netease/xuid/v1\0uid:" + uid_prefix + suffix).encode()).digest()
            account_xuid = str((int.from_bytes(digest[:8], "big") & 0x00ffffffffffffff) | 0x4e00000000000000)
            permissions.append({"permission": "operator", "xuid": account_xuid})
    (target / "permissions.json").write_text(json.dumps(permissions), encoding="utf-8")
    properties = (source / "server.properties").read_text(encoding="utf-8")
    for key, value in {
        "server-port": str(port), "server-portv6": str(port+1), "enable-lan-visibility": "false",
        "level-name": "NEMCBridge-" + args.scenario + "-Test", "online-mode": "false", "allow-cheats": "true",
        "default-player-permission-level": "member", "gamemode": "creative",
        "view-distance": "5", "tick-distance": "4", "block-network-ids-are-hashes": "true",
    }.items():
        properties, count = re.subn(r"(?m)^" + re.escape(key) + r"=.*$", key + "=" + value, properties)
        if count == 0:
            properties += "\n" + key + "=" + value + "\n"
    (target / "server.properties").write_text(properties, encoding="utf-8")
    log = target / "server-test.log"
    result_log = target / "integration-output.log"
    control = target / "console-command.txt"
    control.unlink(missing_ok=True)
    env = dict(os.environ, LOBBYKEEPER_BDS_TEST=f"127.0.0.1:{port}", NEMCBRIDGE_BDS_CONTROL_FILE=str(control))
    if identity_test:
        env["NEMCBRIDGE_BDS_IDENTITY_UID_PREFIX"] = uid_prefix
    env["LD_LIBRARY_PATH"] = str(target) + os.pathsep + env.get("LD_LIBRARY_PATH", "")
    with log.open("w", encoding="utf-8") as output:
        server = subprocess.Popen([str(target / executable)], cwd=target, env=env, stdin=subprocess.PIPE,
                                  stdout=output, stderr=subprocess.STDOUT,
                                  creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        test = None
        try:
            for _ in range(60):
                if server.poll() is not None:
                    raise RuntimeError(f"BDS exited; see {log}")
                if "Server started" in log.read_text(encoding="utf-8", errors="replace"):
                    break
                time.sleep(1)
            else:
                raise RuntimeError(f"BDS startup timed out; see {log}")
            with result_log.open("w", encoding="utf-8") as test_output:
                test = subprocess.Popen(["go", "test", "./internal/integration", "-run",
                                         "^TestBDSAccountSaveIsolation$" if identity_test else "^TestBDSOperatorDemotion$", "-count=1", "-timeout=90s", "-v"],
                                        cwd=root, env=env, stdout=test_output, stderr=subprocess.STDOUT)
                while test.poll() is None:
                    if control.exists() and "Player Spawned:" in log.read_text(encoding="utf-8", errors="replace"):
                        server.stdin.write(control.read_bytes())
                        server.stdin.flush()
                        control.unlink()
                    time.sleep(.1)
            if test.returncode:
                raise RuntimeError(f"Permission regression failed; see {result_log}")
            print("PASS: " + ("different accounts have separate saves; reconnect/rename preserves inventory and position." if identity_test else "BDS operator -> member; build/mine enabled; no loading-screen restrictions."))
            print(f"Logs: {target}")
        finally:
            if test is not None and test.poll() is None:
                test.terminate()
                test.wait()
            if server.poll() is None:
                server.stdin.write(b"stop\n")
                server.stdin.flush()
                try:
                    server.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.wait()
            control.unlink(missing_ok=True)


if __name__ == "__main__":
    main()
