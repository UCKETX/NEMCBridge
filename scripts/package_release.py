#!/usr/bin/env python3
"""Package an explicit build with user documentation and a configuration example."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile



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
            ("docs/protocol.md", "docs/protocol.md"),
            ("docs/setup.md", "docs/setup.md"),
            ("docs/compatibility.md", "docs/compatibility.md"),
            ("docs/resource-packs.md", "docs/resource-packs.md"),
            ("CONTRIBUTING.md", "CONTRIBUTING.md"),
        ):
            (package / destination).parent.mkdir(parents=True, exist_ok=True)
            if source == "docs/RELEASE_README.md":
                # The release README moves from docs/ to the archive root.
                text = (root / source).read_text(encoding="utf-8")
                for before, after in (
                    ("](setup.md)", "](docs/setup.md)"),
                    ("](compatibility.md)", "](docs/compatibility.md)"),
                    ("](resource-packs.md)", "](docs/resource-packs.md)"),
                    ("](../LICENSE)", "](https://github.com/UCKETX/NEMCBridge/blob/main/LICENSE)"),
                    ("](../THIRD_PARTY_NOTICES.md)", "](https://github.com/UCKETX/NEMCBridge/blob/main/THIRD_PARTY_NOTICES.md)"),
                ):
                    text = text.replace(before, after)
                (package / destination).write_text(text, encoding="utf-8")
            else:
                shutil.copyfile(root / source, package / destination)
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
