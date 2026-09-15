#!/usr/bin/env python3
"""Hermetic proof for v1.16 native skill roots and client detection.

Creates isolated HOME/workspace fixtures. Does not touch the operator's real
client config, does not log into anything, and does not start an LLM session.
This harness is installed-guidance and detection/argv only. It does not prove
native listing, MCP initialize, or a model workflow.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile

from atlas_test_env import isolated_atlas_environment


NATIVE_SKILLS = {
    "codex": Path(".agents/skills/atlas-worker/SKILL.md"),
    "claude": Path(".claude/skills/atlas-worker/SKILL.md"),
    "openclaw": Path(".agents/skills/atlas-worker/SKILL.md"),
    "generic": Path(".tracker/integrations/generic-agent-skill/SKILL.md"),
    "cursor": Path(".cursor/skills/atlas-worker/SKILL.md"),
    "grok": Path(".grok/skills/atlas-worker/SKILL.md"),
}
LEGACY_SKILLS = {
    "codex": Path(".codex/skills/atlas-worker/SKILL.md"),
    "grok": Path(".tracker/integrations/grok-agent-skill/SKILL.md"),
}
INIT_ARGV = ["mcp", "serve", "--global", "--tool-profile", "workflow"]
GROK_INIT_ARGV = INIT_ARGV + ["--tool-name-style", "portable"]


class Failure(RuntimeError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Failure(message)


def run(tracker: Path, cwd: Path, args: list[str], env: dict[str, str], timeout: float) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(tracker), *args],
        cwd=cwd,
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=timeout,
        check=False,
    )


def run_ok(tracker: Path, cwd: Path, args: list[str], env: dict[str, str], timeout: float) -> str:
    completed = run(tracker, cwd, args, env, timeout)
    if completed.returncode != 0:
        raise Failure(
            f"{' '.join(args)} exit={completed.returncode} "
            f"stderr={completed.stderr.strip() or completed.stdout.strip()}"
        )
    return completed.stdout


def write_stub(path: Path, name: str) -> None:
    path.write_text(f"#!/bin/sh\necho {name} 0.0.0-fixture\nexit 0\n", encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IEXEC)


def atlas_env(home: Path, extra_path: Path | None = None) -> dict[str, str]:
    env = {
        **os.environ,
        "HOME": str(home),
        "USERPROFILE": str(home),
        "XDG_STATE_HOME": str(home / "state"),
        "XDG_CONFIG_HOME": str(home / "config"),
        "NO_COLOR": "1",
        "TERM": "dumb",
        "LC_ALL": "C",
    }
    if extra_path is not None:
        # Prepend fixture bins for init/install. Detection negatives replace
        # PATH entirely so a real openclaw/claude/codex on the host cannot leak in.
        env["PATH"] = str(extra_path) + os.pathsep + env.get("PATH", "")
    return env


def fixture_only_path_env(env: dict[str, str], stub_dir: Path) -> dict[str, str]:
    isolated = dict(env)
    isolated["PATH"] = str(stub_dir)
    return isolated


def require_service_disabled(home: Path) -> None:
    settings = home / "state" / "atlas-tasker" / "settings.json"
    require(settings.is_file(), f"missing isolated settings: {settings}")
    doc = json.loads(settings.read_text(encoding="utf-8"))
    service = doc.get("service") or {}
    require(service.get("enabled") is False, f"service.enabled must stay false: {service}")
    require(service.get("auto_start") is False, f"service.auto_start must stay false: {service}")


def assert_native_install(workspace: Path) -> None:
    for provider, rel in NATIVE_SKILLS.items():
        path = workspace / rel
        require(path.is_file(), f"missing native {provider} skill at {rel}")
        body = path.read_text(encoding="utf-8")
        require("name: atlas-worker" in body, f"{provider} is not atlas-worker")
        require("<!-- atlas-managed-lifecycle -->" in body, f"{provider} missing lifecycle markers")
    require(not (workspace / LEGACY_SKILLS["codex"]).exists(), "fresh Codex install wrote legacy .codex/skills")
    require(not (workspace / LEGACY_SKILLS["grok"]).exists(), "fresh Grok install wrote legacy grok-agent-skill")
    shared = (workspace / NATIVE_SKILLS["codex"]).read_text(encoding="utf-8")
    require(shared == (workspace / NATIVE_SKILLS["openclaw"]).read_text(encoding="utf-8"), "Codex/OpenClaw SKILL.md diverged")
    require("from coding-agent sessions that load project .agents/skills" in shared, "shared skill missing identity")


def migrate_fixture(tracker: Path, env: dict[str, str], timeout: float) -> None:
    with tempfile.TemporaryDirectory(prefix="atlas-migrate-") as tmp, tempfile.TemporaryDirectory(prefix="atlas-donor-") as donor_tmp:
        workspace = Path(tmp)
        donor = Path(donor_tmp)
        run_ok(tracker, workspace, ["init", "--skip-integrations"], env, timeout)
        run_ok(tracker, donor, ["init", "--skip-integrations"], env, timeout)
        run_ok(tracker, donor, ["integrations", "install", "codex"], env, timeout)
        run_ok(tracker, donor, ["integrations", "install", "grok"], env, timeout)
        (workspace / LEGACY_SKILLS["codex"]).parent.mkdir(parents=True, exist_ok=True)
        (workspace / LEGACY_SKILLS["grok"]).parent.mkdir(parents=True, exist_ok=True)
        (workspace / LEGACY_SKILLS["codex"]).write_text(
            (donor / NATIVE_SKILLS["codex"]).read_text(encoding="utf-8"), encoding="utf-8"
        )
        (workspace / LEGACY_SKILLS["grok"]).write_text(
            (donor / NATIVE_SKILLS["grok"]).read_text(encoding="utf-8"), encoding="utf-8"
        )
        run_ok(tracker, workspace, ["integrations", "install", "codex"], env, timeout)
        run_ok(tracker, workspace, ["integrations", "install", "grok"], env, timeout)
        require((workspace / NATIVE_SKILLS["codex"]).is_file(), "migration did not write Codex native skill")
        require((workspace / NATIVE_SKILLS["grok"]).is_file(), "migration did not write Grok native skill")
        require(not (workspace / LEGACY_SKILLS["codex"]).exists(), "Atlas-managed legacy Codex skill remained")
        require(not (workspace / LEGACY_SKILLS["grok"]).exists(), "Atlas-managed legacy Grok skill remained")


def collision_fixture(tracker: Path, env: dict[str, str], timeout: float) -> None:
    with tempfile.TemporaryDirectory(prefix="atlas-collision-") as tmp:
        workspace = Path(tmp)
        run_ok(tracker, workspace, ["init", "--skip-integrations"], env, timeout)
        custom = workspace / NATIVE_SKILLS["cursor"]
        custom.parent.mkdir(parents=True, exist_ok=True)
        custom.write_text("user owned atlas-worker, do not clobber\n", encoding="utf-8")
        out = run_ok(tracker, workspace, ["integrations", "install", "cursor", "--json"], env, timeout)
        require("user owned atlas-worker" in custom.read_text(encoding="utf-8"), "collision overwrote user skill")
        require("collision" in out.lower() or custom.read_text(encoding="utf-8").startswith("user owned"), "collision was not reported")


def detect_cursor_agent(tracker: Path, env: dict[str, str], timeout: float, stub_dir: Path) -> None:
    # Host PATH commonly has openclaw and other clients. This negative must
    # see only the fixture executables; product detection is unchanged.
    isolated = fixture_only_path_env(env, stub_dir)
    with tempfile.TemporaryDirectory(prefix="atlas-detect-") as tmp:
        workspace = Path(tmp)
        out = run_ok(tracker, workspace, ["integrations", "detect", "--json"], isolated, timeout)
        payload = json.loads(out)
        inner = payload.get("payload") if isinstance(payload.get("payload"), dict) else payload
        items = inner.get("found") or inner.get("detections") or []
        cursor = None
        openclaw = None
        for item in items:
            if not isinstance(item, dict):
                continue
            if item.get("target") == "cursor":
                cursor = item
            if item.get("target") == "openclaw":
                openclaw = item
        require(cursor is not None and cursor.get("found", True), f"cursor-agent not detected: {out}")
        reasons = " ".join(cursor.get("reasons") or [])
        require("cursor-agent" in reasons, f"cursor detection missing cursor-agent reason: {cursor}")
        require(not (openclaw and openclaw.get("found")), f"OpenClaw false-positive: {openclaw}")


def enable_auto_install(home: Path) -> None:
    settings = home / "state" / "atlas-tasker" / "settings.json"
    if not settings.is_file():
        return
    doc = json.loads(settings.read_text(encoding="utf-8"))
    agents = doc.setdefault("agents", {})
    agents["auto_install"] = True
    service = doc.setdefault("service", {})
    service["enabled"] = False
    service["auto_start"] = False
    settings.write_text(json.dumps(doc), encoding="utf-8")
    require_service_disabled(home)


def init_cursor_argv(tracker: Path, env: dict[str, str], timeout: float) -> None:
    enable_auto_install(Path(env["HOME"]))
    with tempfile.TemporaryDirectory(prefix="atlas-init-") as tmp:
        workspace = Path(tmp)
        completed = run(tracker, workspace, ["init", "--json"], env, timeout)
        mcp = Path(env["HOME"]) / ".cursor" / "mcp.json"
        require(mcp.is_file(), f"cursor-agent init did not write ~/.cursor/mcp.json (exit={completed.returncode} stderr={completed.stderr.strip()})")
        doc = json.loads(mcp.read_text(encoding="utf-8"))
        entry = (doc.get("mcpServers") or {}).get("atlas-tasker") or {}
        require(entry.get("args") == INIT_ARGV, f"cursor init argv {entry.get('args')} want {INIT_ARGV}")
        blob = completed.stdout.lower()
        require('"status": "connected"' not in blob, "init JSON claimed a live connected client")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tracker", required=True, type=Path)
    parser.add_argument("--timeout", type=float, default=30.0)
    args = parser.parse_args()
    tracker = args.tracker.resolve()
    require(tracker.is_file(), f"tracker binary not found: {tracker}")
    proof: dict[str, object] = {
        "status": "passed",
        "version_claim": "v1.16 candidate",
        "evidence_categories": ["installed_guidance", "hermetic_detect", "argv_contract"],
        "not_evidence": ["native_listing", "mcp_initialize", "model_workflow"],
        "not_run": [
            "live MCP initialize against a real client",
            "LLM session / skill-in-prompt",
            "cloud Cursor skill travel",
            "operator global ~/.codex ~/.cursor ~/.grok mutation",
        ],
        "client_owned_prerequisites": [
            "restart or new session after MCP/skill writes",
            "Codex workspace trust before project .codex/config.toml loads",
            "Cursor may need a restart; cloud agents do not get ~/.cursor automatically",
            "configured file match is not a live connection",
        ],
        "init_argv": INIT_ARGV,
        "grok_init_argv": GROK_INIT_ARGV,
        "native_skills": {name: str(path) for name, path in NATIVE_SKILLS.items()},
    }
    with isolated_atlas_environment():
        home = Path(os.environ["HOME"])
        require_service_disabled(home)
        stub_dir = home / "bin"
        stub_dir.mkdir(parents=True, exist_ok=True)
        write_stub(stub_dir / "cursor-agent", "cursor-agent")
        env = atlas_env(home, stub_dir)
        with tempfile.TemporaryDirectory(prefix="atlas-native-") as tmp:
            workspace = Path(tmp)
            run_ok(tracker, workspace, ["init", "--skip-integrations"], env, args.timeout)
            for provider in NATIVE_SKILLS:
                run_ok(tracker, workspace, ["integrations", "install", provider], env, args.timeout)
            assert_native_install(workspace)
            proof["fresh_install"] = "native roots only"
        migrate_fixture(tracker, env, args.timeout)
        proof["migration"] = "atlas-managed legacy removed"
        collision_fixture(tracker, env, args.timeout)
        proof["collision"] = "unmanaged skill preserved"
        detect_cursor_agent(tracker, env, args.timeout, stub_dir)
        proof["cursor_agent_detect"] = True
        init_cursor_argv(tracker, env, args.timeout)
        require_service_disabled(home)
        proof["matrix"] = {
            "claude": {"skill": "hermetic_fs", "mcp": "user CLI", "live": "not_this_harness"},
            "codex": {"skill": "hermetic_fs .agents/skills", "mcp": "config.toml", "live": "not_this_harness", "trust": "workspace_trust"},
            "cursor": {"skill": "hermetic_fs .cursor/skills", "mcp": "mcp.json", "live": "not_this_harness", "cli": "cursor-agent"},
            "openclaw": {"skill": "hermetic_fs shared .agents/skills", "mcp": "gateway CLI", "live": "not_this_harness"},
            "grok": {"skill": "hermetic_fs .grok/skills", "mcp": "portable names", "live": "not_this_harness"},
            "generic": {"skill": "hermetic_fs", "mcp": "portable descriptor", "live": "conformance_host"},
        }
    json.dump(proof, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Failure as err:
        print(f"FAILED: {err}", file=sys.stderr)
        raise SystemExit(1)
