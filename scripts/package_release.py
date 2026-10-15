#!/usr/bin/env python3
"""Package an explicit build with documentation and dependency licenses only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile


def modules():
    raw = subprocess.check_output(["go", "list", "-m", "-json", "all"], encoding="utf-8")
    decoder = json.JSONDecoder()
    result = []
    while raw.strip():
        item, end = decoder.raw_decode(raw.lstrip())
        raw = raw.lstrip()[end:]
        result.append(item)
    return result


def copy_licenses(root, target):
    records = []
    for item in modules():
        if item.get("Main"):
            continue
        replacement = item.get("Replace", {})
        directory = replacement.get("Dir") or item.get("Dir")
        records.append(f"{item['Path']} {item.get('Version', '(local)')}")
        if not directory:
            raise RuntimeError(f"Missing downloaded dependency: {item['Path']}")
        source = Path(directory)
        destination = target / item["Path"]
        # The vendored replacement includes several projects' licenses.
        candidates = source.rglob("*") if replacement else source.iterdir()
        for path in candidates:
            if path.is_file() and re.match(r"^(LICENSE|COPYING|NOTICE|PATENTS)([.\-_]|$)", path.name, re.I):
                output = destination / path.relative_to(source)
                output.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(path, output)
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], encoding="utf-8").strip())
    for name in ("LICENSE", "PATENTS"):
        if (goroot / name).is_file():
            output = target / "go" / name
            output.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(goroot / name, output)
    (target.parent / "dependencies.txt").write_text("\n".join(sorted(records)) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--os", choices=("linux", "windows", "darwin"), required=True)
    parser.add_argument("--arch", choices=("amd64", "arm64"), required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("dist"))
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.\-]*", args.version):
        parser.error("version must be a plain version label")
    root = Path(__file__).resolve().parents[1]
    os.chdir(root)
    binary = args.binary.resolve(strict=True)
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    platform = "macos" if args.os == "darwin" else args.os
    name = f"NEMCBridge_{args.version}_{platform}_{args.arch}"
    suffix = ".zip" if args.os == "windows" else ".tar.gz"
    archive = output / (name + suffix)
    with tempfile.TemporaryDirectory(prefix=".package-", dir=output) as temporary:
        staging = Path(temporary).resolve()
        if not staging.is_relative_to(output):
            raise RuntimeError("staging directory is outside output")
        package = staging / name
        package.mkdir()
        executable = package / ("NEMCBridge.exe" if args.os == "windows" else "NEMCBridge")
        shutil.copyfile(binary, executable)
        executable.chmod(0o755)
        for source, destination in (
            ("docs/RELEASE_README.md", "README.md"),
            ("config.example.json", "config.example.json"),
            ("THIRD_PARTY_NOTICES.md", "THIRD_PARTY_NOTICES.md"),
            ("LICENSE", "LICENSE"),
            ("docs/protocol.md", "docs/protocol.md"),
            ("docs/setup.md", "docs/setup.md"),
            ("docs/compatibility.md", "docs/compatibility.md"),
            ("CONTRIBUTING.md", "CONTRIBUTING.md"),
        ):
            (package / destination).parent.mkdir(parents=True, exist_ok=True)
            if source == "docs/RELEASE_README.md":
                # The release README moves from docs/ to the archive root.
                text = (root / source).read_text(encoding="utf-8")
                for before, after in (
                    ("](setup.md)", "](docs/setup.md)"),
                    ("](compatibility.md)", "](docs/compatibility.md)"),
                    ("](../LICENSE)", "](LICENSE)"),
                    ("](../THIRD_PARTY_NOTICES.md)", "](THIRD_PARTY_NOTICES.md)"),
                ):
                    text = text.replace(before, after)
                (package / destination).write_text(text, encoding="utf-8")
            else:
                shutil.copyfile(root / source, package / destination)
        copy_licenses(root, package / "licenses")
        if args.os == "windows":
            with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as target:
                for path in sorted(package.rglob("*")):
                    if path.is_file():
                        target.write(path, path.relative_to(staging).as_posix())
        else:
            def unix_permissions(info):
                info.mode = 0o755 if info.isdir() or info.name.endswith("/NEMCBridge") else 0o644
                return info

            with tarfile.open(archive, "w:gz") as target:
                target.add(package, arcname=name, filter=unix_permissions)
    with archive.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    archive.with_name(archive.name + ".sha256").write_text(f"{digest}  {archive.name}\n", encoding="ascii")
    print(archive.name)


if __name__ == "__main__":
    main()
