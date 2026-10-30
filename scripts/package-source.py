#!/usr/bin/env python3
"""Build a source-only ZIP from an explicit allowlist, never secrets or build outputs."""
from pathlib import Path
import re
import zipfile

root = Path(__file__).resolve().parent.parent
files = {root / name for name in (
    "README.md", "LICENSE", "go.mod", "go.sum", "compile_meta.go", ".env.example",
    ".gitignore", ".dockerignore", "compose.yaml",
    "Dockerfile.service-a", "Dockerfile.service-b", "docs/service-auth.md",
)}
for directory in ("cmd", "internal", "pkg", "utils", "test"):
    for path in (root / directory).rglob("*"):
        if path.suffix in (".go", ".proto") and path.is_file():
            files.add(path)
files.update((root / "scripts").glob("*.sh"))
files.add(Path(__file__).resolve())
for path in files:
    if path.is_symlink() or not path.is_file():
        raise SystemExit("Packaging rejected a missing file or symlink")
    if re.search(rb"-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----", path.read_bytes()):
        raise SystemExit("Packaging rejected private signing material")
output = root / "dist" / "service-auth.zip"
output.parent.mkdir(exist_ok=True)
with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
    for path in sorted(files):
        archive.write(path, Path("service-auth") / path.relative_to(root))
print(f"Source-only archive: {output.relative_to(root)} ({len(files)} files)")
