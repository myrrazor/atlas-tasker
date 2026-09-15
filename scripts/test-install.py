#!/usr/bin/env python3
"""Exercise the release installer with a real tracker binary and a local fixture.

Local tests cover checksums, destination checks, next-step/init, receipts, and
the auth-free `gh attestation verify --bundle` argument contract. A mock `gh`
stands in for hosted Sigstore proof; published releases still need real
`gh attestation verify --bundle` against GitHub-issued bundles.
Uses only Python's standard library on the supported macOS/Linux platforms.
"""

import argparse
import contextlib
import errno
import functools
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import shutil
import pty
import select
import signal
import subprocess
import tarfile
import tempfile
import threading
import time
import unittest


MOCK_GH = r'''#!/usr/bin/env python3
import json
import os
import sys

def log_args():
    path = os.environ.get("MOCK_GH_LOG")
    if not path:
        return
    with open(path, "a", encoding="utf-8") as fh:
        json.dump(sys.argv[1:], fh)
        fh.write("\n")

log_args()
mode = os.environ.get("MOCK_GH_MODE", "ok")
if mode == "always-fail":
    sys.stderr.write("gh mock always-fail\n")
    sys.exit(1)

args = sys.argv[1:]
if len(args) < 2 or args[0] != "attestation" or args[1] != "verify":
    sys.stderr.write("expected gh attestation verify\n")
    sys.exit(1)

def flag(name):
    try:
        i = args.index(name)
    except ValueError:
        return None
    if i + 1 >= len(args):
        return None
    return args[i + 1]

bundle = flag("--bundle")
if not bundle:
    sys.stderr.write("To get started with GitHub CLI, please run:  gh auth login\n")
    sys.exit(1)
if not os.path.isfile(bundle) or os.path.getsize(bundle) == 0:
    sys.stderr.write("attestation bundle missing or empty\n")
    sys.exit(1)
with open(bundle, encoding="utf-8") as fh:
    body = fh.read()
if "BAD_ATTESTATION" in body:
    sys.stderr.write("attestation verification failed\n")
    sys.exit(1)
if "mediaType" not in body:
    sys.stderr.write("invalid attestation bundle\n")
    sys.exit(1)
if flag("--repo") != "myrrazor/atlas-tasker":
    sys.stderr.write("missing or unexpected --repo\n")
    sys.exit(1)
if flag("--signer-workflow") != "myrrazor/atlas-tasker/.github/workflows/release.yml":
    sys.stderr.write("missing or unexpected --signer-workflow\n")
    sys.exit(1)
ref = flag("--source-ref")
if not ref or not ref.startswith("refs/tags/"):
    sys.stderr.write("missing or unexpected --source-ref\n")
    sys.exit(1)
if mode == "fail-verify":
    sys.stderr.write("attestation verification failed\n")
    sys.exit(1)
sys.exit(0)
'''

INIT_STUB = """#!/bin/sh
case "$1" in
  init)
    if [ "${2:-}" = "--help" ] || [ "${2:-}" = "-h" ]; then
      echo "Usage: tracker init [--no-open]"
      exit 0
    fi
    mkdir -p .tracker
    printf '%s\\n' '{"workspace_id":"ws-install-test","created_at":"2026-09-11T00:00:00Z"}' > .tracker/workspace.json
    echo initialized
    ;;
  setup)
    echo "unexpected setup" >&2
    exit 1
    ;;
  *) echo stub ;;
esac
"""

INIT_FAIL_STUB = """#!/bin/sh
case "$1" in
  init)
    if [ "${2:-}" = "--help" ] || [ "${2:-}" = "-h" ]; then
      echo "Usage: tracker init"
      exit 0
    fi
    echo "init failed" >&2
    exit 1
    ;;
  setup)
    echo "unexpected setup" >&2
    exit 1
    ;;
  *) echo stub ;;
esac
"""

OLD_STUB = """#!/bin/sh
case "$1" in
  setup) exit 1 ;;
  init) echo 'Usage: tracker init [--integrations]' ;;
  *) echo old ;;
esac
"""


class FixtureHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        gets = getattr(self.server, "gets", None)
        if gets is not None:
            gets.append(path)
        marker = "/repos/myrrazor/atlas-tasker/attestations/sha256:"
        if path.startswith(marker):
            digest = path[len(marker):]
            table = getattr(self.server, "api_attestations", None) or {}
            body = table.get(digest)
            if body is None:
                self.send_error(404, "no attestation")
                return
            data = body.encode("utf-8") if isinstance(body, str) else body
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        super().do_GET()


class InstallerTests(unittest.TestCase):
    tracker: Path
    script = Path(__file__).resolve().with_name("install.sh")
    bundle = '{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","fixture":true}\n'

    @classmethod
    def setUpClass(cls):
        cls.storage = tempfile.TemporaryDirectory(prefix="atlas-install-test-")
        cls.root = Path(cls.storage.name)
        cls.assets = cls.root / "assets"
        cls.assets.mkdir()
        os_name = {"Darwin": "darwin", "Linux": "linux"}[platform.system()]
        arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}[platform.machine()]
        cls.os_name = os_name
        cls.arch = arch
        cls.archive = f"tracker_1.12.0-test_{os_name}_{arch}.tar.gz"
        with tarfile.open(cls.assets / cls.archive, "w:gz") as bundle:
            bundle.add(cls.tracker, arcname="tracker")
        digest = hashlib.sha256((cls.assets / cls.archive).read_bytes()).hexdigest()
        cls.archive_digest = digest
        cls.checksum = f"{digest}  {cls.archive}\n"
        (cls.assets / "checksums.txt").write_text(cls.checksum)
        (cls.assets / "attestation-bundle.jsonl").write_text(cls.bundle)
        handler = functools.partial(FixtureHandler, directory=str(cls.assets))
        cls.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        cls.server.gets = []
        cls.server.api_attestations = {}
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
        self.mock_bin = self.case / "mock-bin"
        self.mock_bin.mkdir()
        gh = self.mock_bin / "gh"
        gh.write_text(MOCK_GH)
        gh.chmod(0o755)
        self.gh_config = self.case / "gh-config"
        self.gh_config.mkdir()
        self.server.gets.clear()
        self.server.api_attestations = {}
        self.env = dict(os.environ)
        for key in (
            "CODEX_HOME", "CLAUDE_CONFIG_DIR", "TRACKER_ACTOR", "SKIP_INTEGRATIONS",
            "XDG_STATE_HOME", "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN",
            "GH_HOST", "GH_REPO", "GH_PROMPT", "ATTESTATION_BUNDLE_URL",
            "SIGNER_WORKFLOW", "SOURCE_REF",
        ):
            self.env.pop(key, None)
        port = self.server.server_port
        path_dirs = [str(self.mock_bin), "/usr/bin", "/bin", "/usr/sbin", "/sbin"]
        py3 = shutil.which("python3")
        if py3:
            py_dir = str(Path(py3).resolve().parent)
            if py_dir not in path_dirs:
                path_dirs.insert(1, py_dir)
        self.env.update({
            "HOME": str(self.agent_home),
            "PATH": ":".join(path_dirs),
            "BIN_DIR": str(self.bin_dir),
            "VERSION": "v1.12.0-test",
            "RELEASE_BASE_URL": f"http://127.0.0.1:{port}",
            "ATTESTATION_API_BASE_URL": f"http://127.0.0.1:{port}",
            "ALLOW_INSECURE_RELEASE_BASE_URL": "1",
            "TMPDIR": str(self.case),
            "GH_CONFIG_DIR": str(self.gh_config),
            "MOCK_GH_LOG": str(self.case / "gh-args.log"),
            "MOCK_GH_MODE": "ok",
        })
        self.env.pop("VERIFY_ATTESTATIONS", None)

    def receipt_path(self):
        xdg = self.env.get("XDG_STATE_HOME")
        if xdg:
            return Path(xdg) / "atlas-tasker" / "install-receipt.json"
        home = Path(self.env["HOME"])
        if platform.system() == "Darwin":
            return home / "Library" / "Application Support" / "Atlas Tasker" / "install-receipt.json"
        return home / ".local" / "state" / "atlas-tasker" / "install-receipt.json"

    def run_install(self, timeout=20):
        return subprocess.run(
            ["sh"],
            input=self.script.read_text(),
            cwd=self.workspace,
            env=self.env,
            capture_output=True,
            text=True,
            timeout=timeout,
        )

    def last_gh_args(self):
        log = Path(self.env["MOCK_GH_LOG"])
        self.assertTrue(log.is_file(), "gh was not invoked")
        lines = [line for line in log.read_text().splitlines() if line.strip()]
        self.assertTrue(lines, "gh was not invoked")
        return json.loads(lines[-1])

    def assert_bundle_verify_args(self, args=None):
        args = args if args is not None else self.last_gh_args()
        self.assertEqual(args[0], "attestation")
        self.assertEqual(args[1], "verify")
        self.assertTrue(any(part.endswith(".tar.gz") for part in args[:4]))
        self.assertIn("--bundle", args)
        self.assertIn("--repo", args)
        self.assertIn("--signer-workflow", args)
        self.assertIn("--source-ref", args)
        self.assertEqual(args[args.index("--repo") + 1], "myrrazor/atlas-tasker")
        self.assertEqual(
            args[args.index("--signer-workflow") + 1],
            "myrrazor/atlas-tasker/.github/workflows/release.yml",
        )
        self.assertEqual(
            args[args.index("--source-ref") + 1],
            f"refs/tags/{self.env['VERSION']}",
        )
        bundle = args[args.index("--bundle") + 1]
        self.assertTrue(bundle.endswith("attestation-bundle.jsonl") or "attestation" in bundle)

    def assert_no_gh(self):
        log = Path(self.env["MOCK_GH_LOG"])
        self.assertFalse(log.exists() and log.read_text().strip())

    def archive_gets(self):
        return [path for path in self.server.gets if path.endswith(".tar.gz")]

    def assert_installed(self, expected=None):
        expected = expected or self.tracker
        installed = self.bin_dir / "tracker"
        self.assertTrue(os.access(installed, os.X_OK))
        self.assertEqual(hashlib.sha256(installed.read_bytes()).digest(),
                         hashlib.sha256(expected.read_bytes()).digest())
        receipt = self.receipt_path()
        self.assertTrue(receipt.is_file(), f"missing install receipt at {receipt}")
        data = json.loads(receipt.read_text())
        self.assertEqual(data["format"], "atlas_install_receipt_v1")
        self.assertEqual(data["install_method"], "script")
        self.assertEqual(data["binary_path"], str(installed))
        self.assertEqual(data["version"], self.env["VERSION"])
        self.assertEqual(data["binary_sha256"], hashlib.sha256(installed.read_bytes()).hexdigest())
        payload = f"{data['binary_path']}\n{data['binary_sha256']}\nscript\n{self.env['VERSION']}\n"
        self.assertEqual(data["digest"], hashlib.sha256(payload.encode()).hexdigest())

    def terminal_install(self, answers, expect_success=True):
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
            decoded = output.decode(errors="replace")
            self.assertIsNotNone(status, f"installer hung:\n{decoded}")
            code = os.waitstatus_to_exitcode(status)
            if expect_success:
                self.assertEqual(code, 0, decoded)
                self.assertFalse(pending, f"expected prompt was not shown: {pending}\n{decoded}")
            return decoded, code
        finally:
            if status is None:
                os.killpg(pid, signal.SIGTERM)
                os.waitpid(pid, 0)
            os.close(terminal)

    @contextlib.contextmanager
    def extra_archive(self, version, script_text):
        ver = version[1:] if version.startswith("v") else version
        archive = f"tracker_{ver}_{self.os_name}_{self.arch}.tar.gz"
        stub_dir = self.case / f"stub-{ver}"
        stub_dir.mkdir()
        stub = stub_dir / "tracker"
        stub.write_text(script_text)
        stub.chmod(0o755)
        path = self.assets / archive
        with tarfile.open(path, "w:gz") as bundle:
            bundle.add(stub, arcname="tracker")
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        checksum_path = self.assets / "checksums.txt"
        original = checksum_path.read_text()
        checksum_path.write_text(original + f"{digest}  {archive}\n")
        previous = self.env["VERSION"]
        self.env["VERSION"] = version
        try:
            yield stub
        finally:
            self.env["VERSION"] = previous
            checksum_path.write_text(original)
            path.unlink(missing_ok=True)

    def test_unattended_install_never_prompts_or_initializes(self):
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("[y/N]", result.stdout)
        self.assertIn("Next: run tracker init", result.stdout)
        self.assertNotIn("tracker setup", result.stdout)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()
        self.assert_bundle_verify_args()
        self.assertIn("is not on PATH", result.stderr)

    def test_terminal_default_declines_init(self):
        output, _code = self.terminal_install([("[y/N]", b"\n")])
        self.assertIn(str(self.workspace), output)
        self.assertIn("Initialize an Atlas workspace", output)
        self.assertNotIn("Selection:", output)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_terminal_opt_out_does_not_prompt(self):
        self.env["SKIP_INTEGRATIONS"] = "1"
        output, _code = self.terminal_install([])
        self.assertNotIn("[y/N]", output)
        self.assertIn("Next: run tracker init", output)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_terminal_yes_keeps_binary_when_init_fails(self):
        with self.extra_archive("v0.9.0-initfail", INIT_FAIL_STUB) as stub:
            output, code = self.terminal_install([("[y/N]", b"yes\n")])
            self.assertEqual(code, 0, output)
            self.assertIn("Tracker is installed, but workspace init did not complete", output)
            self.assertFalse((self.workspace / ".tracker").exists())
            installed = self.bin_dir / "tracker"
            self.assertTrue(os.access(installed, os.X_OK))
            self.assertEqual(hashlib.sha256(installed.read_bytes()).digest(),
                             hashlib.sha256(stub.read_bytes()).digest())
            self.assertTrue(self.receipt_path().is_file())

    def test_terminal_yes_runs_init_not_setup(self):
        with self.extra_archive("v0.9.0-initok", INIT_STUB):
            output, code = self.terminal_install([("[y/N]", b"yes\n")])
            self.assertEqual(code, 0, output)
            self.assertIn("initialized", output)
            self.assertNotIn("unexpected setup", output)
            self.assertTrue((self.workspace / ".tracker" / "workspace.json").is_file())
            self.assertTrue(self.receipt_path().is_file())

    def test_already_workspace_does_not_init_or_setup(self):
        tracker_dir = self.workspace / ".tracker"
        tracker_dir.mkdir()
        (tracker_dir / "workspace.json").write_text(
            '{"workspace_id":"ws-install-test","created_at":"2026-09-11T00:00:00Z"}\n'
        )
        (tracker_dir / "config.toml").write_text("[workflow]\ncompletion_mode = \"open\"\n")
        output, _code = self.terminal_install([])
        self.assertIn("already an Atlas workspace", output)
        self.assertNotIn("[y/N]", output)
        self.assertNotIn("Setup plan", output)
        self.assertFalse((self.workspace / "AGENTS.md").exists())
        self.assert_installed()

    def test_terminal_eof_skips_init(self):
        output, _code = self.terminal_install([("[y/N]", b"\x04")])
        self.assertIn("Skipped init", output)
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_installed()

    def test_xdg_state_home_overrides_platform_layout(self):
        xdg = self.case / "xdg-state"
        xdg.mkdir()
        self.env["XDG_STATE_HOME"] = str(xdg)
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_installed()
        self.assertTrue((xdg / "atlas-tasker" / "install-receipt.json").is_file())
        if platform.system() == "Darwin":
            mac = Path(self.env["HOME"]) / "Library" / "Application Support" / "Atlas Tasker" / "install-receipt.json"
            self.assertFalse(mac.exists())

    def test_custom_bin_dir(self):
        self.assertNotEqual(str(self.bin_dir), str(Path.home() / ".local" / "bin"))
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.bin_dir / "tracker").is_file())
        self.assert_installed()

    def test_old_version_falls_back_safely(self):
        with self.extra_archive("v0.9.0-old", OLD_STUB):
            output, code = self.terminal_install([("[y/N]", b"\n")])
            self.assertEqual(code, 0, output)
            self.assertIn("tracker init", output)
            self.assertFalse((self.workspace / ".tracker").exists())
            self.assertTrue(os.access(self.bin_dir / "tracker", os.X_OK))

    def test_bad_checksum_stops_before_install_or_init(self):
        checksum_path = self.assets / "checksums.txt"
        checksum_path.write_text(f"{'0' * 64}  {self.archive}\n")
        try:
            result = self.run_install()
        finally:
            checksum_path.write_text(self.checksum)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertFalse((self.bin_dir / "tracker").exists())
        self.assertFalse((self.workspace / ".tracker").exists())
        self.assert_no_gh()

    def test_missing_gh_fails_before_download(self):
        bin_only = self.case / "no-gh-path"
        bin_only.mkdir()
        for name in ("sh", "curl", "awk", "tar", "mktemp", "install", "dirname", "grep", "sed", "shasum", "sha256sum"):
            src = shutil.which(name)
            if src:
                dest = bin_only / name
                if not dest.exists():
                    os.symlink(src, dest)
        self.env["PATH"] = str(bin_only)
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing required command: gh", result.stderr)
        self.assertIn("https://cli.github.com/", result.stderr)
        self.assertFalse((self.bin_dir / "tracker").exists())
        self.assertEqual(self.archive_gets(), [])
        self.assertFalse((self.workspace / ".tracker").exists())

    def test_unauthenticated_config_uses_bundle_without_login(self):
        self.assertEqual(list(self.gh_config.iterdir()), [])
        for key in ("GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN"):
            self.assertNotIn(key, self.env)
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_installed()
        args = self.last_gh_args()
        self.assert_bundle_verify_args(args)
        self.assertNotIn("gh auth login", result.stderr)

    def test_bad_attestation_stops_before_install(self):
        bundle_path = self.assets / "attestation-bundle.jsonl"
        original = bundle_path.read_text()
        bundle_path.write_text('{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","BAD_ATTESTATION":true}\n')
        try:
            result = self.run_install()
        finally:
            bundle_path.write_text(original)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("attestation verification failed", result.stderr)
        self.assertIn("refusing to install", result.stderr)
        self.assertFalse((self.bin_dir / "tracker").exists())
        self.assertFalse((self.workspace / ".tracker").exists())

    def test_missing_attestation_bundle_refuses_checksum_only(self):
        bundle_path = self.assets / "attestation-bundle.jsonl"
        original = bundle_path.read_text()
        bundle_path.unlink()
        try:
            result = self.run_install()
        finally:
            bundle_path.write_text(original)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("provenance could not be verified", result.stderr)
        self.assertFalse((self.bin_dir / "tracker").exists())

    def test_anonymous_api_bundle_unwrap(self):
        bundle_path = self.assets / "attestation-bundle.jsonl"
        original = bundle_path.read_text()
        bundle_path.unlink()
        self.server.api_attestations[self.archive_digest] = json.dumps({
            "attestations": [{
                "bundle": {
                    "mediaType": "application/vnd.dev.sigstore.bundle.v0.3+json",
                    "fixture": True,
                }
            }]
        })
        try:
            result = self.run_install()
        finally:
            bundle_path.write_text(original)
            self.server.api_attestations = {}
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_installed()
        self.assert_bundle_verify_args()
        api_gets = [path for path in self.server.gets if "attestations/sha256:" in path]
        self.assertTrue(api_gets)

    def test_destination_permissions_fail_before_download(self):
        parent = self.case / "locked"
        parent.mkdir()
        dest = parent / "bin"
        os.chmod(parent, 0o555)
        self.env["BIN_DIR"] = str(dest)
        try:
            result = self.run_install()
        finally:
            os.chmod(parent, 0o755)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not writable", result.stderr)
        self.assertIn("BIN_DIR=", result.stderr)
        self.assertFalse(dest.exists())
        self.assertEqual(self.archive_gets(), [])
        self.assertFalse((self.workspace / ".tracker").exists())

    def test_relative_bin_dir_rejected(self):
        self.env["BIN_DIR"] = "relative-bin"
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("absolute path", result.stderr)
        self.assertEqual(self.archive_gets(), [])

    def test_path_on_path_has_no_warning(self):
        self.env["PATH"] = f"{self.mock_bin}:{self.bin_dir}:{self.env['PATH']}"
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("is not on PATH", result.stderr)
        self.assert_installed()

    def test_verify_attestations_zero_does_not_call_gh(self):
        self.env["VERIFY_ATTESTATIONS"] = "0"
        self.env["MOCK_GH_MODE"] = "always-fail"
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_installed()
        self.assert_no_gh()

    def test_explicit_bundle_url_override(self):
        alt = self.assets / "alt-bundle.jsonl"
        alt.write_text(self.bundle)
        bad = self.assets / "attestation-bundle.jsonl"
        original = bad.read_text()
        bad.write_text('{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","BAD_ATTESTATION":true}\n')
        self.env["ATTESTATION_BUNDLE_URL"] = f"{self.env['RELEASE_BASE_URL']}/alt-bundle.jsonl"
        try:
            result = self.run_install()
        finally:
            bad.write_text(original)
            alt.unlink(missing_ok=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_installed()
        self.assert_bundle_verify_args()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tracker", type=Path, required=True, help="Already-built tracker binary")
    args = parser.parse_args()
    InstallerTests.tracker = args.tracker.resolve(strict=True)
    unittest.main(argv=[__file__], verbosity=2)
