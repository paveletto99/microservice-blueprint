"""Create local SPIRE bootstrap material without placing tokens in logs or arguments."""
import json
import os
from pathlib import Path
import subprocess
import sys

SPIRE = "/opt/spire/bin/spire-server"
SOCKET = "/run/spire/server/api.sock"
PARENT = "spiffe://service-auth.test/agent/compose"
OUTPUT = Path("/run/spire/bootstrap")


def cli(*args, json_output=True):
    command = [SPIRE, *args, "-socketPath", SOCKET]
    if json_output:
        command += ["-output", "json"]
    result = subprocess.run(command, capture_output=True, timeout=15)
    if result.returncode:
        # CLI output can contain bootstrap material; never propagate it into container logs.
        raise RuntimeError("SPIRE bootstrap operation failed: " + args[0])
    return json.loads(result.stdout) if json_output else result.stdout


def atomic_file(name, data):
    temporary = OUTPUT / (name + ".tmp")
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
    os.replace(temporary, OUTPUT / name)


def prepare():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    os.umask(0o077)
    if not (OUTPUT / "join-token").exists():
        token = cli("token", "generate", "-spiffeID", PARENT, "-ttl", "600")
        atomic_file("join-token", token["value"].encode())
        bundle = cli("bundle", "show", "-format", "pem", json_output=False)
        if b"BEGIN CERTIFICATE" not in bundle:
            raise RuntimeError("SPIRE bootstrap bundle is invalid")
        atomic_file("bundle.pem", bundle)
    if not (OUTPUT / "bundle.pem").exists():
        raise RuntimeError("Incomplete bootstrap state; recreate only this demo's volumes")
    for role, uid in (("service-a", "10001"), ("service-b", "10002"), ("vtgate", "10003"), ("unauthorized", "10004"), ("payment", "10005"), ("metrics-collector", "10006")):
        identity = "spiffe://service-auth.test/" + role
        existing = cli("entry", "show", "-spiffeID", identity)
        entries = existing.get("entries", [])
        if entries:
            expected = {("unix", "uid:" + uid), ("docker", "label:io.service-auth.workload:" + role), ("docker", "label:io.service-auth.deployment:spiffe-compose")}
            if len(entries) != 1:
                raise RuntimeError("Ambiguous existing workload registration")
            entry = entries[0]
            actual = {(selector["type"], selector["value"]) for selector in entry.get("selectors", [])}
            if actual != expected or entry.get("parent_id") != {"trust_domain": "service-auth.test", "path": "/agent/compose"} or entry.get("admin") or entry.get("downstream") or entry.get("x509_svid_ttl") != 60:
                raise RuntimeError("Existing workload registration does not match the least-privilege configuration")
            continue
        cli("entry", "create", "-parentID", PARENT, "-spiffeID", identity,
            "-selector", "unix:uid:" + uid,
            "-selector", "docker:label:io.service-auth.workload:" + role,
            "-selector", "docker:label:io.service-auth.deployment:spiffe-compose",
            "-x509SVIDTTL", "60")
    print("SPIRE bootstrap and workload registration prepared (no credentials printed).")


if __name__ == "__main__":
    try:
        prepare()
    except Exception:
        sys.exit("SPIRE bootstrap failed; verify server health and fresh/matching demo state.")
