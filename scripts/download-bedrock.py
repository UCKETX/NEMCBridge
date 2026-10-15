"""Download the requested official Windows BDS version; never substitute a version."""
from pathlib import Path
from urllib.request import Request, urlopen
import hashlib
import json
import zipfile

ROOT = Path(__file__).resolve().parents[1] / "runtime"
ROOT.mkdir(parents=True, exist_ok=True)
VERSION = "1.21.124.2"
URL = f"https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-{VERSION}.zip"
ARCHIVE = ROOT / f"bedrock-server-{VERSION}.zip"
DESTINATION = ROOT / f"bedrock-{VERSION}"

if not ARCHIVE.exists():
    for attempt in range(3):
        try:
            request = Request(URL, headers={"User-Agent": "Mozilla/5.0"})
            with urlopen(request, timeout=45) as response, ARCHIVE.with_suffix(".part").open("wb") as output:
                expected = response.headers.get("Content-Length")
                count = 0
                while chunk := response.read(1024 * 1024):
                    output.write(chunk)
                    count += len(chunk)
                if expected and count != int(expected):
                    raise RuntimeError("Incomplete official server archive")
            ARCHIVE.with_suffix(".part").replace(ARCHIVE)
            break
        except Exception as error:
            print(f"Download attempt {attempt + 1}: {type(error).__name__}: {error}", flush=True)
    else:
        raise SystemExit("Official server download failed")

with zipfile.ZipFile(ARCHIVE) as archive:
    if archive.testzip() is not None:
        raise SystemExit("Server archive CRC validation failed")
    if "bedrock_server.exe" not in archive.namelist():
        raise SystemExit("Official archive does not contain bedrock_server.exe")
    if not (DESTINATION / "bedrock_server.exe").exists():
        archive.extractall(DESTINATION)

evidence = {
    "version": VERSION,
    "official_url": URL,
    "archive_bytes": ARCHIVE.stat().st_size,
    "sha256": hashlib.sha256(ARCHIVE.read_bytes()).hexdigest(),
    "destination": str(DESTINATION),
}
(ROOT / "bedrock-download.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
print(json.dumps(evidence, indent=2), flush=True)
