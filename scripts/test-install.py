#!/usr/bin/env python3
"""Exercise the release installer with a real tracker binary and a local fixture.

This tests installation and terminal interaction, not hosted provenance. The
fixture explicitly disables attestations; published releases need hosted proof.
Uses only Python's standard library on the supported macOS/Linux platforms.
"""

import argparse
import errno
import functools
import hashlib
import http.server
import os
from pathlib import Path
import platform
import pty
import select
import signal
import subprocess
import tarfile
import tempfile
import threading
import time
import unittest


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


class InstallerTests(unittest.TestCase):
    tracker: Path
    script = Path(__file__).resolve().with_name("install.sh")

    @classmethod
    def setUpClass(cls):
        cls.storage = tempfile.TemporaryDirectory(prefix="atlas-install-test-")
        cls.root = Path(cls.storage.name)
        cls.assets = cls.root / "assets"
        cls.assets.mkdir()
        os_name = {"Darwin": "darwin", "Linux": "linux"}[platform.system()]
        arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}[platform.machine()]
        cls.archive = f"tracker_1.12.0-test_{os_name}_{arch}.tar.gz"
        with tarfile.open(cls.assets / cls.archive, "w:gz") as bundle:
            bundle.add(cls.tracker, arcname="tracker")
        digest = hashlib.sha256((cls.assets / cls.archive).read_bytes()).hexdigest()
        cls.checksum = f"{digest}  {cls.archive}\n"
        (cls.assets / "checksums.txt").write_text(cls.checksum)
        handler = functools.partial(QuietHandler, directory=str(cls.assets))
        cls.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()
        cls.storage.cleanup()

    def setUp(self):
        self.case = Path(tempfile.mkdtemp(dir=self.root, prefix="case-"))
        self.workspace = self.case / "workspace"
        self.workspace.mkdir()
        self.agent_home = self.case / "agent-home"
        (self.agent_home / ".codex").mkdir(parents=True)
        self.bin_dir = self.case / "bin"
        self.env = dict(os.environ)
        for key in ("CODEX_HOME", "CLAUDE_CONFIG_DIR", "TRACKER_ACTOR", "SKIP_INTEGRATIONS"):
            self.env.pop(key, None)
        self.env.update({
            "HOME": str(self.agent_home),
            "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
            "BIN_DIR": str(self.bin_dir),
            "VERSION": "v1.12.0-test",
            "RELEASE_BASE_URL": f"http://127.0.0.1:{self.server.server_port}",
            "ALLOW_INSECURE_RELEASE_BASE_URL": "1",
            "VERIFY_ATTESTATIONS": "0",
            "TMPDIR": str(self.case),
        })

    def assert_installed(self):
        installed = self.bin_dir / "tracker"
        self.assertTrue(os.access(installed, os.X_OK))
        self.assertEqual(hashlib.sha256(installed.read_bytes()).digest(),
                         hashlib.sha256(self.tracker.read_bytes()).digest())

    def terminal_install(self, answers):
        """Run the same pipe-to-shell shape as curl | sh, with a real TTY."""
        pid, terminal = pty.fork()
        if pid == 0:
            os.chdir(self.workspace)
            os.execve("/bin/sh", ["sh", "-c", 'cat "$1" | sh', "installer-test", str(self.script)], self.env)
        pending = list(answers)
        output = bytearray()
        status = None
        deadline = time.monotonic() + 20
        try:
            while status is None and time.monotonic() < deadline:
                if select.select([terminal], [], [], 0.1)[0]:
                    try:
                        chunk = os.read(terminal, 65536)
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
                        chunk = b""
                    output.extend(chunk)
                    if pending and pending[0][0].encode() in output:
                        _prompt, answer = pending.pop(0)
                        os.write(terminal, answer)
                done, value = os.waitpid(pid, os.WNOHANG)
                if done:
                    status = value
            self.assertIsNotNone(status, f"installer hung:\n{output.decode(errors='replace')}")
            self.assertEqual(os.waitstatus_to_exitcode(status), 0, output.decode(errors="replace"))
            self.assertFalse(pending, f"expected prompt was not shown: {pending}\n{output.decode(errors='replace')}")
            return output.decode(errors="replace")
        finally:
            if status is None:
                os.killpg(pid, signal.SIGTERM)
                os.waitpid(pid, 0)
            os.close(terminal)

    def test_unattended_install_never_prompts_or_initializes(self):
        result = subprocess.run(["sh"], input=self.script.read_text(), cwd=self.workspace,
                                env=self.env, capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("[y/N]", result.stdout)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_terminal_default_declines_setup(self):
        output = self.terminal_install([("[y/N]", b"\n")])
        self.assertIn(str(self.workspace), output)
        self.assertNotIn("Selection:", output)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_terminal_opt_out_does_not_prompt(self):
        self.env["SKIP_INTEGRATIONS"] = "1"
        output = self.terminal_install([])
        self.assertNotIn("[y/N]", output)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_terminal_installs_only_selected_guidance(self):
        output = self.terminal_install([("[y/N]", b"yes\n"), ("Selection:", b"codex\n")])
        self.assertIn("[x] codex", output)
        self.assertTrue((self.workspace / ".codex/skills/atlas-worker/SKILL.md").is_file())
        self.assertFalse((self.workspace / ".claude").exists())
        self.assertFalse((self.workspace / ".cursor").exists())
        self.assert_installed()

    def test_terminal_none_skips_guidance_successfully(self):
        output = self.terminal_install([("[y/N]", b"yes\n"), ("Selection:", b"none\n")])
        self.assertIn("skipped integrations", output)
        # Initializing this workspace was explicitly accepted in the first prompt.
        self.assertTrue((self.workspace / ".tracker").is_dir())
        self.assertFalse((self.workspace / ".codex").exists())
        self.assert_installed()

    def test_bad_checksum_stops_before_install_or_setup(self):
        checksum_path = self.assets / "checksums.txt"
        checksum_path.write_text(f"{'0' * 64}  {self.archive}\n")
        try:
            result = subprocess.run(["sh"], input=self.script.read_text(), cwd=self.workspace,
                                    env=self.env, capture_output=True, text=True, timeout=20)
        finally:
            checksum_path.write_text(self.checksum)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertFalse((self.bin_dir / "tracker").exists())
        self.assertFalse((self.workspace / ".tracker").exists())


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tracker", type=Path, required=True, help="Already-built tracker binary")
    args = parser.parse_args()
    InstallerTests.tracker = args.tracker.resolve(strict=True)
    unittest.main(argv=[__file__], verbosity=2)
