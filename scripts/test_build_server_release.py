"""Exercise release packaging without compiling the gateway."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


class ServerReleaseTest(unittest.TestCase):
    def test_packages_both_linux_architectures_with_checksums(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "scripts").mkdir()
            (root / "server").mkdir()
            (root / "tools").mkdir()
            script = Path(__file__).with_name("build_server_release.sh")
            shutil.copy(script, root / "scripts" / script.name)
            for name in ("LICENSE", "NOTICE", "scripts/server_release_README.md",
                         "server/server_config.example.json"):
                (root / name).write_text("fixture\n")
            go = root / "tools/go"
            go.write_text('''#!/usr/bin/env python3
import os, pathlib, sys
if sys.argv[1] == "build":
    target = pathlib.Path(sys.argv[sys.argv.index("-o") + 1])
    target.write_text(os.environ["GOOS"] + "/" + os.environ["GOARCH"])
else:
    print("go1.27.3")
''')
            go.chmod(0o755)
            env = dict(os.environ, PATH=f"{root / 'tools'}:{os.environ['PATH']}")
            subprocess.run(["bash", str(root / "scripts" / script.name), "0.1.0"],
                           env=env, check=True, capture_output=True)
            output = root / "dist/server-v0.1.0"
            checksums = (output / "SHA256SUMS").read_text().splitlines()
            self.assertEqual(len(checksums), 2)
            for line in checksums:
                digest, name = line.split()
                self.assertEqual(hashlib.sha256((output / name).read_bytes()).hexdigest(),
                                 digest)
                prefix = name.removesuffix(".tar.gz")
                arch = prefix.rsplit("_", 1)[1]
                with tarfile.open(output / name) as archive:
                    for binary in ("sourceaec-server", "sourceaec-runtime"):
                        content = archive.extractfile(f"{prefix}/bin/{binary}").read()
                        self.assertEqual(content.decode(), f"linux/{arch}")
                        self.assertIn(f"{prefix}/bin/{binary}.buildinfo.txt",
                                      archive.getnames())
                    for entry in ("LICENSE", "NOTICE", "README.md",
                                  "server/server_config.example.json"):
                        self.assertIn(f"{prefix}/{entry}", archive.getnames())

    def test_rejects_invalid_version(self):
        script = Path(__file__).with_name("build_server_release.sh")
        result = subprocess.run(["bash", str(script), "../invalid"],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("VERSION must be", result.stderr)


if __name__ == "__main__":
    unittest.main()
