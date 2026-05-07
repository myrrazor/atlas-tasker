#!/usr/bin/env python3
"""Offline RC validation checks for release-polish regressions."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import time
from typing import Iterable
import unicodedata


ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[ -/]*[@-~]")
MARKDOWN_LINK_RE = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
STALE_VERSION_RE = re.compile(r"v1\.(?:6|7)(?:\.\d+)?-{1,2}rc\d+")
ACTOR_REQUIRED_TRACKER_RE = re.compile(
    r"(?:^|[./\s])tracker\s+(?:"
    r"ticket\s+(?:create|move|edit|delete|assign|priority|comment|claim|release|heartbeat|request-review|approve|reject|complete|label\s+(?:add|remove)|link|unlink|policy\s+set)|"
    r"project\s+(?:policy\s+set|codeowners\s+write)|"
    r"run\s+(?:dispatch|start|launch|attach|checkpoint|evidence\s+add|handoff|complete|fail|abort|cleanup)|"
    r"agent\s+(?:create|enable|disable)|"
    r"goal\s+manifest|"
    r"gate\s+(?:approve|reject|waive)|"
    r"change\s+(?:create|sync|review-request|merge|link|import-url|unlink)|"
    r"checks\s+(?:record|sync)|"
    r"key\s+(?:generate|import-public|rotate|revoke)|"
    r"trust\s+(?:bind-key|revoke-key)|"
    r"governance\s+pack\s+(?:create|apply)|"
    r"classify\s+set|"
    r"redact\s+(?:preview|export)|"
    r"audit\s+(?:report|export)|"
    r"backup\s+(?:create|restore-apply)|"
    r"sign\s+\S+|"
    r"collaborator\s+(?:add|edit|trust|suspend|remove)|"
    r"membership\s+(?:bind|unbind)|"
    r"remote\s+(?:add|edit|remove)|"
    r"dispatch\s+(?:run|bulk)|"
    r"bulk\s+(?:move|assign|request-review|complete|claim|release)|"
    r"import\s+(?:preview|apply)|"
    r"export\s+create|"
    r"archive\s+(?:apply|restore)|"
    r"bundle\s+(?:create|import)|"
    r"conflict\s+resolve|"
    r"sync\s+(?:fetch|pull|push|run)|"
    r"sweep|"
    r"compact(?:\s+\S+)?|"
    r"views\s+save|"
    r"watch\s+(?:ticket|project|view)|"
    r"unwatch\s+(?:ticket|project|view)|"
    r"notify\s+send|"
    r"mcp\s+approve-operation|"
    r"permission-profile\s+(?:create|bind|unbind)"
    r")\b"
)
ACTOR_REQUIRED_SMOKE_COMMANDS = [
    "tracker ticket label add APP-1 bug",
    "tracker ticket link APP-1 --blocks APP-2",
    "tracker ticket policy set APP-1 --completion-mode dual_gate",
    "tracker project policy set APP --completion-mode dual_gate",
    "tracker collaborator add rev-1",
    "tracker membership bind rev-1 --scope-kind project --scope-id APP --role reviewer",
    "tracker remote add origin --kind git --location repo",
    "tracker bundle create",
    "tracker conflict resolve C-1 --resolution use_remote",
    "tracker sweep",
    "tracker bulk move ready --ticket APP-1 --yes",
    "tracker sign goal MAN-1 --signing-key KEY-1",
]
SECRET_PATTERNS = [
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    re.compile(r"\bAKIA[0-9A-Z]{16}\b"),
    re.compile(r"\bghp_[A-Za-z0-9_]{20,}\b"),
    re.compile(r"\bgithub_pat_[A-Za-z0-9_]{20,}\b"),
    re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}\b"),
    re.compile(r"\bsk-[A-Za-z0-9]{24,}\b"),
    re.compile(r"/Users/masterhit\b"),
    re.compile(r"/private/var/folders\b"),
    re.compile(r"/var/folders\b"),
    re.compile(r"/tmp/Test[A-Za-z0-9_/-]*"),
]

PERFORMANCE_BUDGETS = {
    "version_json": 2.0,
    "board_json": 3.0,
    "dashboard_json": 3.0,
    "goal_brief_md": 3.0,
}


class ValidationError(Exception):
    pass


def run_cmd(
    args: list[str],
    *,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    input_text: str | None = None,
    timeout: int = 20,
) -> subprocess.CompletedProcess[str]:
    merged_env = os.environ.copy()
    if env:
        merged_env.update(env)
    proc = subprocess.run(
        args,
        cwd=str(cwd) if cwd else None,
        env=merged_env,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
        check=False,
    )
    if proc.returncode != 0:
        joined = " ".join(args)
        raise ValidationError(
            f"command failed ({joined}) exit={proc.returncode}\nstdout={proc.stdout}\nstderr={proc.stderr}"
        )
    return proc


def public_markdown_files(repo: Path) -> list[Path]:
    roots = [
        "README.md",
        "CONTRIBUTING.md",
        "CODE_OF_CONDUCT.md",
        "SECURITY.md",
        "SUPPORT.md",
        "GOVERNANCE.md",
        "ROADMAP.md",
        "CHANGELOG.md",
    ]
    files = [repo / item for item in roots if (repo / item).exists()]
    files.extend(sorted((repo / "docs").rglob("*.md")))
    return [path for path in files if should_scan_public_doc(repo, path)]


def should_scan_public_doc(repo: Path, path: Path) -> bool:
    rel = path.relative_to(repo).as_posix()
    if rel.startswith("docs/release-proof/"):
        return False
    return True


def active_release_text_files(repo: Path) -> list[Path]:
    candidates: list[Path] = []
    for path in public_markdown_files(repo):
        rel = path.relative_to(repo).as_posix()
        if rel.startswith("docs/v1") or rel.startswith("docs/upgrade-"):
            continue
        if rel in {"docs/v1-decision-log.md", "docs/v1-implementation-plan.md", "docs/v1-ticket-pr-breakdown.md"}:
            continue
        candidates.append(path)
    candidates.extend(sorted((repo / "scripts").glob("*.sh")))
    candidates.extend(sorted((repo / "examples").rglob("*.sh")))
    return candidates


def check_docs_links(repo: Path) -> None:
    failures: list[str] = []
    for path in public_markdown_files(repo):
        text = path.read_text(encoding="utf-8")
        for match in MARKDOWN_LINK_RE.finditer(text):
            target = match.group(1).strip()
            target = target.split(" ", 1)[0].strip("<>")
            if not target or target.startswith(("#", "http://", "https://", "mailto:", "app://")):
                continue
            target_path = target.split("#", 1)[0]
            if not target_path:
                continue
            if target_path.startswith("/"):
                failures.append(f"{path.relative_to(repo)} links to absolute markdown path {target}")
                continue
            resolved = (path.parent / target_path).resolve()
            try:
                resolved.relative_to(repo.resolve())
            except ValueError:
                failures.append(f"{path.relative_to(repo)} links outside repo: {target}")
                continue
            if not resolved.exists():
                failures.append(f"{path.relative_to(repo)} missing link target {target}")
    if failures:
        raise ValidationError("docs link check failed:\n" + "\n".join(failures[:40]))
    print("docs_links=ok")


def fenced_commands(text: str) -> Iterable[str]:
    in_block = False
    lang = ""
    buffer: list[str] = []
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("```"):
            if in_block:
                if lang in {"bash", "sh", "shell", "zsh"}:
                    yield from join_continuations(buffer)
                in_block = False
                lang = ""
                buffer = []
            else:
                in_block = True
                lang = stripped[3:].strip().lower()
                buffer = []
            continue
        if in_block:
            buffer.append(line)


def join_continuations(lines: list[str]) -> Iterable[str]:
    current = ""
    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if current:
            current += " " + stripped
        else:
            current = stripped
        if current.endswith("\\"):
            current = current[:-1].rstrip()
            continue
        yield current
        current = ""
    if current:
        yield current


def check_snippets(repo: Path) -> None:
    failures: list[str] = []
    for path in public_markdown_files(repo):
        rel = path.relative_to(repo).as_posix()
        text = path.read_text(encoding="utf-8")
        for command in fenced_commands(text):
            for segment in split_shell_commands(command):
                if "tracker " not in segment:
                    continue
                if ACTOR_REQUIRED_TRACKER_RE.search(segment) and not command_has_flag(segment, "--actor"):
                    failures.append(f"{rel}: mutation example missing --actor: {segment}")
                if "tracker goal manifest" in segment and not command_has_flag(segment, "--reason"):
                    failures.append(f"{rel}: goal manifest example missing --reason: {segment}")
    if failures:
        raise ValidationError("snippet lint failed:\n" + "\n".join(failures[:40]))
    print("snippets=ok")


def check_validator_self_tests() -> None:
    misses = [command for command in ACTOR_REQUIRED_SMOKE_COMMANDS if not ACTOR_REQUIRED_TRACKER_RE.search(command)]
    if misses:
        raise ValidationError("actor-required matcher missed smoke commands:\n" + "\n".join(misses))
    if command_has_flag("tracker collaborator add rev-1 --actor-map agent:reviewer-1", "--actor"):
        raise ValidationError("actor flag matcher confused --actor-map with --actor")
    chained = split_shell_commands(
        "tracker ticket create --project APP --title X && tracker ticket move APP-1 ready --actor human:owner"
    )
    if len(chained) != 2 or command_has_flag(chained[0], "--actor"):
        raise ValidationError("snippet splitter allowed a later --actor to satisfy an earlier command")


def split_shell_commands(command: str) -> list[str]:
    parts: list[str] = []
    buf: list[str] = []
    in_single = False
    in_double = False
    escape = False
    i = 0
    while i < len(command):
        char = command[i]
        if escape:
            buf.append(char)
            escape = False
            i += 1
            continue
        if char == "\\" and not in_single:
            buf.append(char)
            escape = True
            i += 1
            continue
        if char == "'" and not in_double:
            in_single = not in_single
        elif char == '"' and not in_single:
            in_double = not in_double
        if not in_single and not in_double:
            if command.startswith(("&&", "||"), i):
                parts.append("".join(buf).strip())
                buf = []
                i += 2
                continue
            if char == ";":
                parts.append("".join(buf).strip())
                buf = []
                i += 1
                continue
        buf.append(char)
        i += 1
    parts.append("".join(buf).strip())
    return [part for part in parts if part]


def command_has_flag(command: str, flag: str) -> bool:
    try:
        tokens = shlex.split(command)
    except ValueError:
        tokens = command.split()
    return any(token == flag or token.startswith(flag + "=") for token in tokens)


def display_width(text: str) -> int:
    width = 0
    for char in text:
        if unicodedata.combining(char):
            continue
        width += 2 if unicodedata.east_asian_width(char) in {"F", "W"} else 1
    return width


def check_stale_versions(repo: Path) -> None:
    failures: list[str] = []
    for path in active_release_text_files(repo):
        rel = path.relative_to(repo).as_posix()
        text = path.read_text(encoding="utf-8")
        for match in STALE_VERSION_RE.finditer(text):
            failures.append(f"{rel}: stale release candidate {match.group(0)}")
    if failures:
        raise ValidationError("stale version scan failed:\n" + "\n".join(failures[:40]))
    print("stale_versions=ok")


def check_leakage(repo: Path) -> None:
    scan_roots = [
        repo / "README.md",
        repo / "CONTRIBUTING.md",
        repo / "SECURITY.md",
        repo / "SUPPORT.md",
        repo / "GOVERNANCE.md",
        repo / "ROADMAP.md",
        repo / "CHANGELOG.md",
        repo / "docs",
        repo / "examples",
        repo / "scripts",
    ]
    failures: list[str] = []
    for root in scan_roots:
        paths = [root] if root.is_file() else sorted(root.rglob("*"))
        for path in paths:
            if not path.is_file() or path.suffix not in {".md", ".py", ".sh", ".txt", ".json", ".toml", ".yml", ".yaml"}:
                continue
            rel = path.relative_to(repo).as_posix()
            if rel.startswith("docs/release-proof/"):
                continue
            lines = path.read_text(encoding="utf-8", errors="ignore").splitlines()
            for lineno, line in enumerate(lines, start=1):
                for pattern in SECRET_PATTERNS:
                    if pattern.search(line) and not allowed_leakage_pattern(rel, line):
                        failures.append(f"{rel}:{lineno}: matched {pattern.pattern}")
                        break
    if failures:
        raise ValidationError("leakage scan failed:\n" + "\n".join(failures[:40]))
    print("leakage=ok")


def allowed_leakage_pattern(rel: str, line: str) -> bool:
    if rel == "scripts/validate_rc.py":
        stripped = line.strip()
        if stripped.startswith(("re.compile(r", "'re.compile(r")) and stripped.endswith(("),", "',")):
            return True
        return "s#/private/var/folders/" in stripped or "s#/var/folders/" in stripped
    if rel == "examples/generate-demo-assets.sh":
        allowed_fragments = [
            "s#/private/var/folders/",
            "s#/var/folders/",
            "leak_pattern=",
        ]
        return any(fragment in line for fragment in allowed_fragments)
    return False


def seed_workspace(tracker: Path, work_dir: Path) -> Path:
    workspace = work_dir / "quickstart-workspace"
    workspace.mkdir(parents=True, exist_ok=True)
    run_cmd(["git", "init", "-b", "main"], cwd=workspace)
    run_cmd(["git", "config", "user.email", "atlas@example.invalid"], cwd=workspace)
    run_cmd(["git", "config", "user.name", "Atlas RC"], cwd=workspace)
    (workspace / "README.md").write_text("# Example App\n", encoding="utf-8")
    run_cmd(["git", "add", "README.md"], cwd=workspace)
    run_cmd(["git", "commit", "-m", "init workspace"], cwd=workspace)
    run_cmd([str(tracker), "init"], cwd=workspace)
    run_cmd([str(tracker), "project", "create", "APP", "Example App"], cwd=workspace)
    run_cmd(
        [
            str(tracker),
            "ticket",
            "create",
            "--project",
            "APP",
            "--title",
            "Ship first feature",
            "--type",
            "task",
            "--actor",
            "human:owner",
            "--reason",
            "quickstart",
        ],
        cwd=workspace,
    )
    run_cmd(
        [
            str(tracker),
            "ticket",
            "move",
            "APP-1",
            "ready",
            "--actor",
            "human:owner",
            "--reason",
            "start work",
        ],
        cwd=workspace,
    )
    run_cmd(
        [
            str(tracker),
            "ticket",
            "comment",
            "APP-1",
            "--body",
            "Wide timeline marker 表表表 with enough text to force truncation checks.",
            "--actor",
            "human:owner",
            "--reason",
            "timeline width",
        ],
        cwd=workspace,
    )
    return workspace


def check_readme_quickstart(tracker: Path, workspace: Path) -> None:
    board = run_cmd([str(tracker), "board"], cwd=workspace).stdout
    if "APP-1" not in board or "Ship first feature" not in board:
        raise ValidationError("README quickstart board did not show APP-1")
    view = json.loads(run_cmd([str(tracker), "ticket", "view", "APP-1", "--json"], cwd=workspace).stdout)
    ticket = view.get("ticket", {})
    if ticket.get("id") != "APP-1" or ticket.get("status") != "ready":
        raise ValidationError(f"README quickstart ticket JSON mismatch: {ticket}")
    print("readme_quickstart=ok")


def check_terminal_accessibility(tracker: Path, workspace: Path) -> None:
    cases = [
        ("board_narrow", ["board"], {"NO_COLOR": "1", "TERM": "dumb", "COLUMNS": "40"}, 40),
        ("dashboard_normal", ["dashboard"], {"NO_COLOR": "1", "TERM": "dumb", "COLUMNS": "80"}, 80),
        ("timeline_normal", ["timeline", "APP-1"], {"NO_COLOR": "1", "TERM": "dumb", "COLUMNS": "80"}, 80),
        ("goal_brief_md", ["goal", "brief", "APP-1", "--md"], {"NO_COLOR": "1", "TERM": "dumb", "COLUMNS": "80"}, 80),
    ]
    for name, args, env, max_width in cases:
        output = run_cmd([str(tracker), *args], cwd=workspace, env=env).stdout
        if ANSI_RE.search(output):
            raise ValidationError(f"{name} emitted ANSI escapes under NO_COLOR/TERM=dumb")
        too_wide = [display_width(line) for line in output.splitlines() if display_width(line) > max_width]
        if too_wide:
            raise ValidationError(f"{name} exceeded {max_width} columns; longest={max(too_wide)}")
    print("terminal_accessibility=ok")


def extract_first_json(text: str) -> object:
    decoder = json.JSONDecoder()
    for index, char in enumerate(text):
        if char not in "{[":
            continue
        try:
            parsed, _ = decoder.raw_decode(text[index:])
            return parsed
        except json.JSONDecodeError:
            continue
    raise ValidationError(f"no JSON object found in shell output:\n{text}")


def shell_json(tracker: Path, workspace: Path, slash_command: str) -> object:
    proc = run_cmd(
        [str(tracker), "shell"],
        cwd=workspace,
        input_text=slash_command + "\n/exit\n",
        timeout=20,
    )
    return extract_first_json(proc.stdout)


def check_cross_surface_parity(tracker: Path, workspace: Path) -> None:
    pairs = [
        ("board", ["board", "--json"], "/board --json"),
        ("dashboard", ["dashboard", "--json"], "/dashboard --json"),
        ("ticket_view", ["ticket", "view", "APP-1", "--json"], "/ticket view APP-1 --json"),
    ]
    for name, cli_args, shell_command in pairs:
        cli_json = json.loads(run_cmd([str(tracker), *cli_args], cwd=workspace).stdout)
        shell_view = shell_json(tracker, workspace, shell_command)
        if strip_volatile_json(cli_json) != strip_volatile_json(shell_view):
            raise ValidationError(f"{name} CLI/shell JSON mismatch")
    tools = json.loads(
        run_cmd([str(tracker), "mcp", "tools", "--json", "--tool-profile", "read"], cwd=workspace).stdout
    )
    names = {tool.get("name") for tool in tools.get("tools", [])}
    required = {"atlas.board", "atlas.ticket.view", "atlas.dashboard"}
    missing = sorted(required - names)
    if missing:
        raise ValidationError(f"MCP read profile missing expected read tools: {missing}")
    print("cross_surface_parity=ok")


def strip_volatile_json(value: object) -> object:
    if isinstance(value, dict):
        return {
            key: strip_volatile_json(item)
            for key, item in value.items()
            if key not in {"generated_at"}
        }
    if isinstance(value, list):
        return [strip_volatile_json(item) for item in value]
    return value


def check_performance(tracker: Path, workspace: Path) -> None:
    cases = [
        ("version_json", [str(tracker), "version", "--json"], None),
        ("board_json", [str(tracker), "board", "--json"], workspace),
        ("dashboard_json", [str(tracker), "dashboard", "--json"], workspace),
        ("goal_brief_md", [str(tracker), "goal", "brief", "APP-1", "--md"], workspace),
    ]
    for name, args, cwd in cases:
        start = time.monotonic()
        run_cmd(args, cwd=cwd, timeout=20)
        elapsed = time.monotonic() - start
        budget = PERFORMANCE_BUDGETS[name]
        if elapsed > budget:
            raise ValidationError(f"{name} exceeded {budget:.1f}s budget: {elapsed:.3f}s")
        print(f"performance_{name}_ms={int(elapsed * 1000)} budget_ms={int(budget * 1000)}")
    print("performance_budgets=ok")


def check_version_contract(tracker: Path, version: str) -> None:
    payload = json.loads(run_cmd([str(tracker), "version", "--json"]).stdout)
    required = {"format_version", "kind", "version", "commit", "build_date", "go_version", "platform"}
    missing = required - set(payload)
    if missing:
        raise ValidationError(f"version JSON missing fields: {sorted(missing)}")
    if payload["version"] != version:
        raise ValidationError(f"version mismatch: got {payload['version']}, want {version}")
    print("version_contract=ok")


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate Atlas v1.8 RC polish gates offline.")
    parser.add_argument("--repo", required=True, type=Path)
    parser.add_argument("--tracker", required=True, type=Path)
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()

    repo = args.repo.resolve()
    tracker = args.tracker.resolve()
    work_dir = args.work_dir.resolve()

    try:
        check_validator_self_tests()
        check_version_contract(tracker, args.version)
        check_docs_links(repo)
        check_snippets(repo)
        check_stale_versions(repo)
        check_leakage(repo)
        workspace = seed_workspace(tracker, work_dir)
        check_readme_quickstart(tracker, workspace)
        check_terminal_accessibility(tracker, workspace)
        check_cross_surface_parity(tracker, workspace)
        check_performance(tracker, workspace)
    except (OSError, subprocess.SubprocessError, ValidationError, json.JSONDecodeError) as exc:
        print(f"rc_validation_error={exc}", file=sys.stderr)
        return 1
    finally:
        if work_dir.exists() and os.environ.get("VALIDATE_RC_KEEP_WORK") != "1":
            shutil.rmtree(work_dir / "quickstart-workspace", ignore_errors=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
