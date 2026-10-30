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
    "compose.spiffe.yaml", "Dockerfile.spiffe-bootstrap", "Dockerfile.spiffe-envoy",
    "Dockerfile.spiffe-db-probe", "docs/spiffe-envoy.md", "scripts/generate-spiffe-envoy.py",
    "test/spiffe/test_envoy_config.py",
    "Dockerfile.vitess-migrate", "docs/orders-vitess.md", "scripts/orders-crud-smoke.py",
)}
for name in ("server.conf", "agent.conf", "bootstrap.py", "init-volumes.py", "vitess.sh", "probe.py"):
    files.add(root / "docker/spiffe" / name)
for role in ("service-a", "service-b", "vtgate", "unauthorized"):
    files.add(root / "docker/spiffe/envoy" / (role + ".json"))
for directory in ("cmd", "internal", "pkg", "utils", "test"):
    for path in (root / directory).rglob("*"):
        if (path.suffix in (".go", ".proto") or (directory == "internal" and path.suffix == ".sql" and "orders/migrations" in str(path))) and path.is_file():
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
