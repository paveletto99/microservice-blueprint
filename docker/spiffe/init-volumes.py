"""Set ownership only on this Compose project's SPIRE server volumes; never read key contents."""
import os
from pathlib import Path

for root, mode in ((Path("/server-data"), 0o700), (Path("/server-api"), 0o750)):
    root.mkdir(parents=True, exist_ok=True)
    if root.stat().st_uid != 1000:
        for directory, directories, files in os.walk(root):
            os.chown(directory, 1000, 1000)
            for name in files:
                os.chown(Path(directory) / name, 1000, 1000, follow_symlinks=False)
    os.chown(root, 1000, 1000)
    os.chmod(root, mode)
print("SPIRE server volume permissions initialized.")
