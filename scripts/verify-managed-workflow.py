#!/usr/bin/env python3
"""Inspect generated atlas-worker skills and exercise atlas.context / atlas.status.

Named-provider natural-language sessions (Codex, Claude, Cursor, OpenClaw, Grok)
remain a recorded remaining dependency: this VM does not host those clients.
Generic coverage is this script (a representative stdio MCP client) plus
`go test ./internal/mcp/testhost`.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import select
import subprocess
import sys
import tempfile
import time
from typing import Any


PROVIDERS = ("codex", "claude", "openclaw", "generic", "cursor", "grok")
PROVIDER_LABELS = {
    "codex": "Codex",
    "claude": "Claude Code",
    "openclaw": "OpenClaw",
    "generic": "generic agent",
    "cursor": "Cursor",
    "grok": "Grok",
}
LIFECYCLE_NEEDLES = (
    "Prefer Atlas MCP tools",
    "atlas.context",
    "atlas.status",
    "Search first",
    "Use the ticket the user named",
    "Avoid duplicate work",
    "Claim before substantial edits",
    "legal workflow edge",
    "milestone progress",
    "durable evidence",
    "Request review or complete",
    "Query Atlas before every status report",
    "Reconcile Atlas state",
    "tracking_excluded",
    "follow_workspace",
    "dependency_blocked",
    "If MCP is unavailable",
)
NL_SCENARIOS = (
    "Use Atlas Tasker to manage this project.",
    "What is the current status?",
    "Show the board for APP.",
    "Pick up the next ready ticket.",
    "Implement this feature.",
    "I'm blocked on the API decision.",
    "Give me an update.",
    "Send this for review.",
    "What should happen next?",
    "Hand this off to Claude.",
)
SKILL_PATHS = {
    "codex": Path(".codex/skills/atlas-worker/SKILL.md"),
    "claude": Path(".claude/skills/atlas-worker/SKILL.md"),
    "openclaw": Path(".agents/skills/atlas-worker/SKILL.md"),
    "generic": Path(".tracker/integrations/generic-agent-skill/SKILL.md"),
    "cursor": Path(".cursor/skills/atlas-worker/SKILL.md"),
    "grok": Path(".tracker/integrations/grok-agent-skill/SKILL.md"),
}


class Failure(RuntimeError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Failure(message)


def lifecycle_section(body: str) -> str:
    start = "<!-- atlas-managed-lifecycle -->"
    end = "<!-- /atlas-managed-lifecycle -->"
    require(start in body and end in body, "skill missing managed lifecycle markers")
    return body.split(start, 1)[1].split(end, 1)[0]


def run_cli(tracker: Path, cwd: Path, args: list[str], timeout: float) -> str:
    completed = subprocess.run(
        [str(tracker), *args],
        cwd=cwd,
        env={**os.environ, "NO_COLOR": "1", "TERM": "dumb", "LC_ALL": "C"},
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=timeout,
        check=False,
    )
    if completed.returncode != 0:
        raise Failure(
            f"CLI failed ({' '.join(args)}), exit={completed.returncode}: "
            f"{completed.stderr.strip() or completed.stdout.strip()}"
        )
    return completed.stdout


class MCPClient:
    def __init__(self, tracker: Path, workspace: Path, timeout: float) -> None:
        self.timeout = timeout
        self.request_id = 0
        self.buffer = bytearray()
        self.process = subprocess.Popen(
            [
                str(tracker),
                "mcp",
                "serve",
                "--workspace",
                str(workspace),
                "--tool-profile",
                "read",
            ],
            cwd=workspace,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env={**os.environ, "NO_COLOR": "1", "TERM": "dumb", "LC_ALL": "C"},
        )

    def close(self) -> None:
        if self.process.stdin:
            self.process.stdin.close()
        try:
            self.process.wait(timeout=self.timeout)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=self.timeout)

    def request(self, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        self.request_id += 1
        request_id = self.request_id
        message: dict[str, Any] = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            message["params"] = params
        raw = json.dumps(message, separators=(",", ":")).encode() + b"\n"
        assert self.process.stdin is not None
        self.process.stdin.write(raw)
        self.process.stdin.flush()
        deadline = time.monotonic() + self.timeout
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise Failure("MCP response timed out")
            assert self.process.stdout is not None
            ready, _, _ = select.select([self.process.stdout.fileno()], [], [], remaining)
            if not ready:
                raise Failure("MCP response timed out")
            chunk = os.read(self.process.stdout.fileno(), 65536)
            if not chunk:
                raise Failure(f"MCP server exited; stderr={self.process.stderr.read() if self.process.stderr else ''}")
            self.buffer.extend(chunk)
            while b"\n" in self.buffer:
                line, _, rest = self.buffer.partition(b"\n")
                self.buffer = bytearray(rest)
                if not line.strip():
                    continue
                response = json.loads(line)
                if response.get("id") == request_id:
                    return response

    def initialize(self) -> None:
        self.request(
            "initialize",
            {
                "protocolVersion": "2025-11-25",
                "capabilities": {},
                "clientInfo": {"name": "verify-managed-workflow", "version": "114.3"},
            },
        )
        notify = {"jsonrpc": "2.0", "method": "notifications/initialized"}
        assert self.process.stdin is not None
        self.process.stdin.write(json.dumps(notify).encode() + b"\n")
        self.process.stdin.flush()

    def list_tools(self) -> set[str]:
        response = self.request("tools/list", {})
        result = response.get("result") or {}
        tools = result.get("tools") or []
        return {item["name"] for item in tools if isinstance(item, dict) and item.get("name")}

    def call(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        response = self.request("tools/call", {"name": name, "arguments": arguments})
        if response.get("error"):
            raise Failure(f"{name} RPC error: {response['error']}")
        result = response.get("result") or {}
        if result.get("isError"):
            raise Failure(f"{name} tool error: {result}")
        structured = result.get("structuredContent")
        require(isinstance(structured, dict), f"{name} omitted structuredContent")
        return structured


def inspect_skills(workspace: Path) -> dict[str, Any]:
    cores: dict[str, str] = {}
    proof: dict[str, Any] = {"providers": {}}
    for provider in PROVIDERS:
        path = workspace / SKILL_PATHS[provider]
        body = path.read_text(encoding="utf-8")
        require("name: atlas-worker" in body, f"{provider} is not atlas-worker")
        require("atlas-manager" not in body, f"{provider} invented atlas-manager")
        core = lifecycle_section(body)
        cores[provider] = core
        for needle in LIFECYCLE_NEEDLES:
            require(needle in core, f"{provider} lifecycle missing {needle!r}")
        require(
            PROVIDER_LABELS[provider] in body,
            f"{provider} skill omitted identity {PROVIDER_LABELS[provider]!r}",
        )
        proof["providers"][provider] = {"skill": str(SKILL_PATHS[provider]), "lifecycle_bytes": len(core)}
    first = cores["codex"]
    for provider, core in cores.items():
        require(core == first, f"{provider} lifecycle core differs from Codex")
    proof["shared_lifecycle"] = True
    proof["duplicate_skill_introduced"] = False
    return proof


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tracker", required=True, type=Path)
    parser.add_argument("--timeout", type=float, default=30.0)
    args = parser.parse_args()
    tracker = args.tracker.resolve()
    require(tracker.is_file(), f"tracker binary not found: {tracker}")
    proof: dict[str, Any] = {
        "status": "passed",
        "nl_scenarios": list(NL_SCENARIOS),
        "remaining_dependencies": [
            "Codex, Claude, Cursor, OpenClaw, and Grok real-client natural-language sessions"
        ],
    }
    with tempfile.TemporaryDirectory(prefix="atlas-managed-") as tmp:
        workspace = Path(tmp)
        run_cli(tracker, workspace, ["init", "--skip-integrations"], args.timeout)
        for provider in PROVIDERS:
            run_cli(tracker, workspace, ["integrations", "install", provider], args.timeout)
        proof["skills"] = inspect_skills(workspace)
        run_cli(tracker, workspace, ["project", "create", "APP", "Managed"], args.timeout)
        tools_json = run_cli(tracker, workspace, ["mcp", "tools", "--json", "--tool-profile", "read"], args.timeout)
        require("atlas.context" in tools_json and "atlas.status" in tools_json, "mcp tools omitted context/status")
        client = MCPClient(tracker, workspace, args.timeout)
        try:
            client.initialize()
            listed = client.list_tools()
            require("atlas.context" in listed and "atlas.status" in listed, f"MCP list missing tools: {sorted(listed)}")
            context = client.call("atlas.context", {"project": "APP", "actor": "agent:builder-1"})
            status = client.call("atlas.status", {"project": "APP", "actor": "agent:builder-1"})
            require("payload" in context or "managed_mode" in json.dumps(context), "context payload missing")
            text = json.dumps(status)
            require("APP" in text, "status of APP omitted APP")
            require("NOPE" not in text, "status of APP leaked an unknown project")
            proof["generic_mcp_client"] = {
                "atlas.context": "passed",
                "atlas.status": "passed",
                "tools": sorted(listed),
            }
        finally:
            client.close()
    print(json.dumps(proof, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Failure as exc:
        print(json.dumps({"status": "failed", "error": str(exc)}), file=sys.stderr)
        raise SystemExit(1)
