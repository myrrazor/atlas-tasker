#!/usr/bin/env python3
"""Black-box Atlas MCP workflow smoke test using only the Python standard library."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import select
import subprocess
import tempfile
import threading
import time
from typing import Any


PROTOCOL_VERSION = "2025-11-25"

READ_TOOLS = set(
    """
    atlas.queue atlas.next atlas.agent.available atlas.agent.pending
    atlas.agent.list atlas.agent.view atlas.agent.wakeup.list atlas.agent.wakeup.view
    atlas.team.list atlas.team.show atlas.goal.brief atlas.search atlas.board
    atlas.ticket.view atlas.ticket.history atlas.ticket.inspect
    atlas.schedule.list atlas.schedule.history atlas.dashboard atlas.timeline
    atlas.run.view atlas.evidence.list atlas.evidence.view atlas.handoff.view
    atlas.approvals atlas.inbox atlas.change.status atlas.checks.list atlas.sync.status
    atlas.conflict.list atlas.conflict.view atlas.archive.plan atlas.dispatch.suggest
    atlas.dispatch.plan atlas.change.merge_plan atlas.sync.pull_plan
    atlas.bundle.import_plan atlas.archive.apply_plan atlas.import.apply_plan
    atlas.compact_plan atlas.worktree.cleanup_plan
    """.split()
)

WORKFLOW_TOOLS = set(
    """
    atlas.ticket.comment atlas.ticket.claim atlas.ticket.release atlas.ticket.move
    atlas.ticket.create atlas.ticket.assign atlas.ticket.link atlas.ticket.unlink
    atlas.ticket.approve atlas.ticket.reject atlas.ticket.complete
    atlas.agent.create atlas.agent.edit atlas.agent.enable atlas.agent.disable
    atlas.agent.wakeup.ack atlas.team.apply atlas.schedule.set atlas.schedule.clear
    atlas.ticket.request_review atlas.gate.approve atlas.gate.reject
    atlas.run.checkpoint atlas.evidence.add atlas.handoff.create atlas.import.preview
    """.split()
)

DELIVERY_TOOLS = {
    "atlas.dispatch.run",
    "atlas.change.create",
    "atlas.change.sync",
    "atlas.checks.sync",
}

DELIVERY_HIGH_IMPACT = {
    "atlas.change.review_request",
    "atlas.change.merge",
}

ADMIN_HIGH_IMPACT = {
    "atlas.gate.waive",
    "atlas.sync.pull",
    "atlas.sync.push",
    "atlas.bundle.import",
    "atlas.import.apply",
    "atlas.archive.apply",
    "atlas.archive.restore",
    "atlas.compact",
    "atlas.worktree.cleanup",
}


class SmokeFailure(RuntimeError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SmokeFailure(message)


def json_text(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def inventory_digest(names: set[str]) -> str:
    return hashlib.sha256(("\n".join(sorted(names)) + "\n").encode()).hexdigest()


def deep_dicts(value: Any):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from deep_dicts(child)
    elif isinstance(value, list):
        for child in value:
            yield from deep_dicts(child)


def contains(value: Any, needle: str) -> bool:
    return needle in json_text(value)


def tool_error(response: dict[str, Any]) -> str | None:
    if "error" in response:
        return json_text(response["error"])
    result = response.get("result")
    if not isinstance(result, dict):
        return "missing JSON-RPC result"
    if result.get("isError"):
        structured = result.get("structuredContent")
        return json_text(structured if structured is not None else result.get("content", result))
    return None


def structured(response: dict[str, Any], tool_name: str) -> dict[str, Any]:
    error = tool_error(response)
    require(error is None, f"{tool_name} failed: {error}")
    result = response.get("result", {})
    payload = result.get("structuredContent")
    require(isinstance(payload, dict), f"{tool_name} returned no structuredContent")
    require(payload.get("format_version") == "v1", f"{tool_name} returned unexpected format_version")
    return payload


def sanitize(text: str, replacements: list[tuple[str, str]]) -> str:
    result = text
    for raw, replacement in sorted(replacements, key=lambda item: len(item[0]), reverse=True):
        if raw:
            result = result.replace(raw, replacement)
    return result


class MCPProcess:
    def __init__(
        self,
        tracker: Path,
        workspace: Path,
        client_cwd: Path,
        profile: str,
        framing: str,
        timeout: float,
        allow_high_impact: bool = False,
    ) -> None:
        self.framing = framing
        self.timeout = timeout
        self.buffer = bytearray()
        self.request_id = 0
        self.tools: dict[str, dict[str, Any]] = {}
        self.stderr = bytearray()
        command = [
            str(tracker),
            "mcp",
            "serve",
            "--workspace",
            str(workspace),
            "--tool-profile",
            profile,
        ]
        if allow_high_impact:
            command.append("--dangerously-allow-high-impact-tools")
        environment = os.environ.copy()
        environment.update({"NO_COLOR": "1", "TERM": "dumb", "LC_ALL": "C"})
        self.process = subprocess.Popen(
            command,
            cwd=client_cwd,
            env=environment,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=0,
        )
        require(self.process.stdin is not None, "MCP stdin pipe unavailable")
        require(self.process.stdout is not None, "MCP stdout pipe unavailable")
        require(self.process.stderr is not None, "MCP stderr pipe unavailable")
        self.stderr_thread = threading.Thread(target=self._drain_stderr, daemon=True)
        self.stderr_thread.start()

    def _drain_stderr(self) -> None:
        assert self.process.stderr is not None
        while True:
            chunk = self.process.stderr.read(4096)
            if not chunk:
                return
            if len(self.stderr) < 65536:
                remaining = 65536 - len(self.stderr)
                self.stderr.extend(chunk[:remaining])

    def _write(self, message: dict[str, Any]) -> None:
        assert self.process.stdin is not None
        body = json_text(message).encode()
        if self.framing == "content-length":
            wire = f"Content-Length: {len(body)}\r\n\r\n".encode() + body
        else:
            wire = body + b"\n"
        try:
            self.process.stdin.write(wire)
            self.process.stdin.flush()
        except (BrokenPipeError, OSError) as exc:
            raise SmokeFailure(f"MCP server closed its input: {exc}; stderr={self.stderr_text()}") from exc

    def _read_more(self, deadline: float) -> None:
        assert self.process.stdout is not None
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise SmokeFailure(f"MCP response timed out; stderr={self.stderr_text()}")
        ready, _, _ = select.select([self.process.stdout.fileno()], [], [], remaining)
        if not ready:
            raise SmokeFailure(f"MCP response timed out; stderr={self.stderr_text()}")
        chunk = os.read(self.process.stdout.fileno(), 65536)
        if not chunk:
            raise SmokeFailure(
                f"MCP server exited before replying (exit={self.process.poll()}); stderr={self.stderr_text()}"
            )
        self.buffer.extend(chunk)

    def _read_ndjson(self, deadline: float) -> dict[str, Any]:
        while True:
            newline = self.buffer.find(b"\n")
            if newline >= 0:
                raw = bytes(self.buffer[:newline]).strip()
                del self.buffer[: newline + 1]
                if not raw:
                    continue
                try:
                    return json.loads(raw)
                except json.JSONDecodeError as exc:
                    raise SmokeFailure(f"invalid NDJSON MCP response: {raw[:300]!r}") from exc
            self._read_more(deadline)

    def _read_content_length(self, deadline: float) -> dict[str, Any]:
        separator = b"\r\n\r\n"
        while separator not in self.buffer:
            self._read_more(deadline)
        header_raw, body_start = bytes(self.buffer).split(separator, 1)
        length = None
        for line in header_raw.decode("ascii", errors="strict").split("\r\n"):
            name, marker, value = line.partition(":")
            if marker and name.strip().lower() == "content-length":
                length = int(value.strip())
        require(length is not None and length >= 0, "Content-Length response omitted a valid length")
        header_size = len(header_raw) + len(separator)
        while len(self.buffer) - header_size < length:
            self._read_more(deadline)
        raw = bytes(self.buffer[header_size : header_size + length])
        del self.buffer[: header_size + length]
        try:
            return json.loads(raw)
        except json.JSONDecodeError as exc:
            raise SmokeFailure(f"invalid Content-Length MCP response: {raw[:300]!r}") from exc

    def _read(self, deadline: float) -> dict[str, Any]:
        if self.framing == "content-length":
            return self._read_content_length(deadline)
        return self._read_ndjson(deadline)

    def request(self, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        self.request_id += 1
        request_id = self.request_id
        request: dict[str, Any] = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            request["params"] = params
        self._write(request)
        deadline = time.monotonic() + self.timeout
        for _ in range(10):
            response = self._read(deadline)
            if response.get("id") == request_id:
                return response
        raise SmokeFailure(f"MCP did not return response id {request_id}")

    def notify(self, method: str, params: dict[str, Any] | None = None) -> None:
        notification: dict[str, Any] = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            notification["params"] = params
        self._write(notification)

    def initialize(self) -> dict[str, Any]:
        response = self.request(
            "initialize",
            {
                "protocolVersion": PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {"name": "atlas-mcp-smoke", "version": "v1"},
            },
        )
        error = tool_error(response)
        require(error is None, f"initialize failed: {error}")
        result = response.get("result", {})
        require(result.get("protocolVersion") == PROTOCOL_VERSION, "unexpected negotiated MCP protocol")
        server_info = result.get("serverInfo", {})
        require(server_info.get("name") == "atlas-tasker", "unexpected MCP server name")
        self.notify("notifications/initialized")
        return result

    def list_tools(self) -> set[str]:
        response = self.request("tools/list", {})
        error = tool_error(response)
        require(error is None, f"tools/list failed: {error}")
        result = response.get("result", {})
        tools = result.get("tools")
        require(isinstance(tools, list), "tools/list returned no tools array")
        self.tools = {item["name"]: item for item in tools if isinstance(item, dict) and "name" in item}
        require(len(self.tools) == len(tools), "tools/list returned duplicate or unnamed tools")
        return set(self.tools)

    def call(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        require(name in self.tools, f"{name} is not visible in this profile")
        schema = self.tools[name].get("inputSchema", {})
        required = set(schema.get("required") or []) if isinstance(schema, dict) else set()
        missing = required - set(arguments)
        require(not missing, f"{name} smoke arguments omit schema-required fields: {sorted(missing)}")
        return self.request("tools/call", {"name": name, "arguments": arguments})

    def call_unlisted(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        require(name not in self.tools, f"denial probe requires {name} to be hidden")
        return self.request("tools/call", {"name": name, "arguments": arguments})

    def stderr_text(self) -> str:
        return self.stderr.decode("utf-8", errors="replace").strip()[-4000:]

    def close(self) -> None:
        if self.process.stdin is not None:
            try:
                self.process.stdin.close()
            except OSError:
                pass
        forced_shutdown = False
        try:
            self.process.wait(timeout=self.timeout)
        except subprocess.TimeoutExpired:
            forced_shutdown = True
            self.process.terminate()
            try:
                self.process.wait(timeout=self.timeout)
            except subprocess.TimeoutExpired:
                self.process.kill()
                try:
                    self.process.wait(timeout=self.timeout)
                except subprocess.TimeoutExpired as exc:
                    raise SmokeFailure(
                        f"MCP server remained alive after forced shutdown; stderr={self.stderr_text()}"
                    ) from exc
        self.stderr_thread.join(timeout=self.timeout)
        if forced_shutdown:
            raise SmokeFailure(
                f"MCP server did not exit after stdin closed; forced shutdown "
                f"(exit={self.process.returncode}); stderr={self.stderr_text()}"
            )
        if self.process.returncode != 0:
            raise SmokeFailure(
                f"MCP server exited abnormally during shutdown "
                f"(exit={self.process.returncode}); stderr={self.stderr_text()}"
            )

    def __enter__(self) -> "MCPProcess":
        return self

    def __exit__(self, exc_type, exc, _tb) -> None:
        try:
            self.close()
        except Exception as close_error:
            if exc_type is None:
                raise
            if exc is not None and hasattr(exc, "add_note"):
                exc.add_note(f"MCP cleanup also failed: {close_error}")


def run_cli(tracker: Path, cwd: Path, args: list[str], timeout: float) -> str:
    command = [str(tracker), *args]
    completed = subprocess.run(
        command,
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
        raise SmokeFailure(
            f"CLI setup failed ({' '.join(args)}), exit={completed.returncode}: "
            f"{completed.stderr.strip() or completed.stdout.strip()}"
        )
    return completed.stdout


def assert_inventory(label: str, actual: set[str], expected: set[str]) -> dict[str, Any]:
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    require(not missing and not extra, f"{label} inventory drift: missing={missing} extra={extra}")
    return {"count": len(actual), "sha256": inventory_digest(actual)}


def open_session(
    tracker: Path,
    workspace: Path,
    client_cwd: Path,
    profile: str,
    framing: str,
    timeout: float,
    allow_high_impact: bool = False,
) -> MCPProcess:
    return MCPProcess(
        tracker,
        workspace,
        client_cwd,
        profile,
        framing,
        timeout,
        allow_high_impact=allow_high_impact,
    )


def verify_transport_and_profiles(
    proof: dict[str, Any], tracker: Path, workspace: Path, client_cwd: Path, timeout: float
) -> None:
    base = READ_TOOLS | WORKFLOW_TOOLS | DELIVERY_TOOLS

    with open_session(tracker, workspace, client_cwd, "read", "ndjson", timeout) as session:
        session.initialize()
        read_names = session.list_tools()
        proof["profiles"]["read"] = assert_inventory("read", read_names, READ_TOOLS)
        structured(session.call("atlas.team.list", {}), "atlas.team.list")
        denial = session.call_unlisted(
            "atlas.ticket.create",
            {
                "project": "APP",
                "title": "must not be created",
                "type": "task",
                "actor": "human:owner",
                "reason": "read profile denial probe",
            },
        )
        denial_text = tool_error(denial)
        require(denial_text is not None, "read profile unexpectedly executed atlas.ticket.create")
        proof["transport"]["ndjson"] = {
            "initialize": "passed",
            "tools_list": "passed",
            "tools_call": "passed",
            "read_write_denial": "passed",
        }

    with open_session(tracker, workspace, client_cwd, "read", "content-length", timeout) as session:
        session.initialize()
        content_names = session.list_tools()
        assert_inventory("content-length read", content_names, READ_TOOLS)
        structured(session.call("atlas.board", {}), "atlas.board")
        proof["transport"]["content_length"] = {
            "initialize": "passed",
            "tools_list": "passed",
            "tools_call": "passed",
        }

    with open_session(tracker, workspace, client_cwd, "delivery", "ndjson", timeout) as session:
        session.initialize()
        names = session.list_tools()
        proof["profiles"]["delivery"] = assert_inventory("delivery", names, base)

    with open_session(tracker, workspace, client_cwd, "delivery", "ndjson", timeout, True) as session:
        session.initialize()
        names = session.list_tools()
        proof["profiles"]["delivery_high_impact_enabled"] = assert_inventory(
            "delivery with high-impact exposure", names, base | DELIVERY_HIGH_IMPACT
        )

    with open_session(tracker, workspace, client_cwd, "admin", "ndjson", timeout) as session:
        session.initialize()
        names = session.list_tools()
        proof["profiles"]["admin"] = assert_inventory("admin", names, base)

    with open_session(tracker, workspace, client_cwd, "admin", "ndjson", timeout, True) as session:
        session.initialize()
        names = session.list_tools()
        proof["profiles"]["admin_high_impact_enabled"] = assert_inventory(
            "admin with high-impact exposure",
            names,
            base | DELIVERY_HIGH_IMPACT | ADMIN_HIGH_IMPACT,
        )
    proof["safety"] = {
        "read_profile_write_denied": "passed",
        "high_impact_hidden_without_flag": "passed",
        "delivery_flag_exposes_only_delivery_high_impact": "passed",
        "admin_flag_exposes_all_high_impact": "passed",
        "high_impact_operations_executed": False,
    }


def ticket_id_from(payload: dict[str, Any]) -> str:
    for item in deep_dicts(payload):
        value = item.get("id")
        if isinstance(value, str) and re.fullmatch(r"[A-Z][A-Z0-9]*-[0-9]+", value):
            return value
    raise SmokeFailure("ticket create result did not contain a ticket ID")


def verify_workflow(
    proof: dict[str, Any], tracker: Path, workspace: Path, client_cwd: Path, timeout: float
) -> None:
    expected = READ_TOOLS | WORKFLOW_TOOLS
    with open_session(tracker, workspace, client_cwd, "workflow", "ndjson", timeout) as session:
        session.initialize()
        names = session.list_tools()
        proof["profiles"]["workflow"] = assert_inventory("workflow", names, expected)

        team_list = structured(session.call("atlas.team.list", {}), "atlas.team.list")
        require(contains(team_list, "crossfire"), "team list did not expose crossfire")
        team_show = structured(
            session.call("atlas.team.show", {"preset": "crossfire"}), "atlas.team.show"
        )
        require(contains(team_show, "reviewer-1"), "team show did not expose reviewer-1")
        team_plan = structured(
            session.call(
                "atlas.team.apply",
                {
                    "preset": "solo",
                    "dry_run": True,
                    "actor": "human:owner",
                    "reason": "MCP smoke dry run",
                },
            ),
            "atlas.team.apply",
        )
        require(contains(team_plan, "solo"), "team dry-run result omitted preset")

        structured(
            session.call(
                "atlas.agent.create",
                {
                    "agent_id": "builder-1",
                    "name": "MCP Builder",
                    "provider": "codex",
                    "role": ["worker"],
                    "actor": "human:owner",
                    "reason": "MCP smoke agent",
                },
            ),
            "atlas.agent.create",
        )
        structured(
            session.call(
                "atlas.agent.create",
                {
                    "agent_id": "reviewer-1",
                    "name": "MCP Reviewer",
                    "provider": "claude",
                    "role": ["reviewer"],
                    "actor": "human:owner",
                    "reason": "MCP smoke reviewer",
                },
            ),
            "atlas.agent.create",
        )
        agent_list = structured(session.call("atlas.agent.list", {}), "atlas.agent.list")
        require(
            contains(agent_list, "builder-1") and contains(agent_list, "reviewer-1"),
            "agent list omitted created agents",
        )
        agent_view = structured(
            session.call("atlas.agent.view", {"agent_id": "builder-1"}), "atlas.agent.view"
        )
        require(contains(agent_view, "MCP Builder"), "agent view omitted builder profile")

        blocker = structured(
            session.call(
                "atlas.ticket.create",
                {
                    "project": "APP",
                    "title": "MCP blocker",
                    "type": "task",
                    "status": "ready",
                    "reviewer": "agent:reviewer-1",
                    "actor": "human:owner",
                    "reason": "MCP smoke blocker",
                },
            ),
            "atlas.ticket.create",
        )
        blocker_id = ticket_id_from(blocker)
        dependent = structured(
            session.call(
                "atlas.ticket.create",
                {
                    "project": "APP",
                    "title": "MCP dependent",
                    "type": "task",
                    "status": "ready",
                    "reviewer": "agent:reviewer-1",
                    "actor": "human:owner",
                    "reason": "MCP smoke dependent",
                },
            ),
            "atlas.ticket.create",
        )
        dependent_id = ticket_id_from(dependent)

        for ticket_id in (blocker_id, dependent_id):
            structured(
                session.call(
                    "atlas.ticket.assign",
                    {
                        "ticket_id": ticket_id,
                        "assignee": "agent:builder-1",
                        "actor": "human:owner",
                        "reason": "MCP smoke assignment",
                    },
                ),
                "atlas.ticket.assign",
            )
        structured(
            session.call(
                "atlas.ticket.link",
                {
                    "ticket_id": dependent_id,
                    "other_id": blocker_id,
                    "kind": "blocked_by",
                    "actor": "human:owner",
                    "reason": "MCP smoke dependency",
                },
            ),
            "atlas.ticket.link",
        )

        available_before = structured(
            session.call("atlas.agent.available", {"agent_id": "builder-1"}),
            "atlas.agent.available",
        )
        pending_before = structured(
            session.call("atlas.agent.pending", {"agent_id": "builder-1"}),
            "atlas.agent.pending",
        )
        queue_before = structured(
            session.call("atlas.queue", {"actor": "agent:builder-1"}), "atlas.queue"
        )
        require(contains(available_before, blocker_id), "available queue omitted ready blocker")
        require(contains(pending_before, dependent_id), "pending queue omitted blocked dependent")
        require(contains(queue_before, blocker_id), "actor queue omitted ready blocker")

        brief = structured(
            session.call("atlas.goal.brief", {"target": blocker_id}), "atlas.goal.brief"
        )
        require(contains(brief, blocker_id), "goal brief omitted target ticket")

        structured(
            session.call(
                "atlas.ticket.claim",
                {
                    "ticket_id": blocker_id,
                    "actor": "agent:builder-1",
                    "reason": "MCP smoke claim",
                },
            ),
            "atlas.ticket.claim",
        )
        structured(
            session.call(
                "atlas.ticket.move",
                {
                    "ticket_id": blocker_id,
                    "status": "in_progress",
                    "actor": "agent:builder-1",
                    "reason": "MCP smoke start",
                },
            ),
            "atlas.ticket.move",
        )
        requested = structured(
            session.call(
                "atlas.ticket.request_review",
                {
                    "ticket_id": blocker_id,
                    "reviewer": "agent:reviewer-1",
                    "actor": "agent:builder-1",
                    "reason": "MCP smoke ready for review",
                },
            ),
            "atlas.ticket.request_review",
        )
        require(
            contains(requested, "in_review") and contains(requested, "agent:reviewer-1"),
            "request review did not persist status and reviewer",
        )
        approved = structured(
            session.call(
                "atlas.ticket.approve",
                {
                    "ticket_id": blocker_id,
                    "actor": "agent:reviewer-1",
                    "reason": "MCP smoke reviewed",
                },
            ),
            "atlas.ticket.approve",
        )
        require(contains(approved, "approved"), "review approval state was not recorded")
        completed = structured(
            session.call(
                "atlas.ticket.complete",
                {
                    "ticket_id": blocker_id,
                    "actor": "human:owner",
                    "reason": "MCP smoke owner completion",
                },
            ),
            "atlas.ticket.complete",
        )
        require(contains(completed, '"status":"done"'), "owner completion did not reach done")

        final_view = structured(
            session.call("atlas.ticket.view", {"ticket_id": blocker_id}), "atlas.ticket.view"
        )
        require(contains(final_view, '"status":"done"'), "final ticket view was not done")

        wakeups = structured(
            session.call("atlas.agent.wakeup.list", {"agent_id": "builder-1"}),
            "atlas.agent.wakeup.list",
        )
        wakeup = next(
            (
                item
                for item in deep_dicts(wakeups)
                if item.get("ticket_id") == dependent_id
                and item.get("blocker_ticket_id") == blocker_id
                and item.get("state") == "pending"
            ),
            None,
        )
        require(wakeup is not None, "blocker completion did not create the dependent wake-up")
        wakeup_id = wakeup.get("wakeup_id")
        require(isinstance(wakeup_id, str) and wakeup_id, "dependent wake-up omitted wakeup_id")
        acked = structured(
            session.call(
                "atlas.agent.wakeup.ack",
                {
                    "wakeup_id": wakeup_id,
                    "actor": "agent:builder-1",
                    "reason": "MCP smoke acknowledged",
                },
            ),
            "atlas.agent.wakeup.ack",
        )
        require(contains(acked, '"state":"acked"'), "wake-up acknowledgement was not recorded")
        available_after = structured(
            session.call("atlas.agent.available", {"agent_id": "builder-1"}),
            "atlas.agent.available",
        )
        require(contains(available_after, dependent_id), "unblocked dependent was not available")

        proof["workflow"] = {
            "team_visibility": "passed",
            "agent_create_list_view": "passed",
            "ticket_create_assign_link": "passed",
            "available_pending_queue": "passed",
            "goal_brief": "passed",
            "actor_sequence": ["agent:builder-1", "agent:reviewer-1", "human:owner"],
            "request_review_approve_complete": "passed",
            "dependency_wakeup": "passed",
            "wakeup_ack": "passed",
            "synthetic_ticket_ids": [blocker_id, dependent_id],
            "synthetic_wakeup_state": "acked",
        }


def write_outputs(proof: dict[str, Any], json_path: Path | None, report_path: Path | None) -> None:
    if json_path is not None:
        json_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = json_path.with_name(json_path.name + ".tmp")
        temporary.write_text(json.dumps(proof, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        os.replace(temporary, json_path)
    if report_path is not None:
        report_path.parent.mkdir(parents=True, exist_ok=True)
        profile_lines = []
        for name, result in proof.get("profiles", {}).items():
            profile_lines.append(f"- `{name}`: {result.get('count', '?')} tools")
        workflow = proof.get("workflow", {})
        if proof.get("status") == "passed":
            outcome = "PASS"
            detail = (
                "Both stdio framings, exact profile inventories, read-profile denial, and the "
                "actor-separated MCP workflow completed successfully."
            )
        else:
            outcome = "FAIL"
            detail = proof.get("error", {}).get("message", "unknown failure")
        report = (
            "# Atlas MCP workflow smoke proof\n\n"
            f"**Result: {outcome}**\n\n{detail}\n\n"
            "## Profile inventories\n\n"
            + ("\n".join(profile_lines) if profile_lines else "- Not completed")
            + "\n\n## Workflow\n\n"
            + f"- Ticket create/assign/link: {workflow.get('ticket_create_assign_link', 'not completed')}\n"
            + f"- Queue and goal visibility: {workflow.get('available_pending_queue', 'not completed')} / {workflow.get('goal_brief', 'not completed')}\n"
            + f"- Review/approval/completion: {workflow.get('request_review_approve_complete', 'not completed')}\n"
            + f"- Dependency wake-up and acknowledgement: {workflow.get('dependency_wakeup', 'not completed')} / {workflow.get('wakeup_ack', 'not completed')}\n"
            + "\nThe smoke uses a synthetic temporary workspace, an unrelated synthetic client working directory, "
            "and an explicit `--workspace` argument. It performs no provider or high-impact operation.\n"
        )
        temporary = report_path.with_name(report_path.name + ".tmp")
        temporary.write_text(report, encoding="utf-8")
        os.replace(temporary, report_path)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Verify Atlas MCP profiles, framing, and workflow.")
    parser.add_argument("--tracker", required=True, type=Path, help="Candidate tracker binary")
    parser.add_argument("--proof-json", type=Path, help="Optional machine-readable proof path")
    parser.add_argument("--report", type=Path, help="Optional Markdown proof report path")
    parser.add_argument("--timeout", type=float, default=8.0, help="Per-request timeout in seconds")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    tracker = args.tracker.expanduser().resolve()
    proof: dict[str, Any] = {
        "format_version": "v1",
        "kind": "atlas_mcp_workflow_smoke",
        "status": "running",
        "workspace": "synthetic_temporary",
        "client_cwd": "distinct_synthetic_temporary",
        "workspace_argument": "explicit",
        "profiles": {},
        "safety": {},
        "transport": {},
        "workflow": {},
        "cleanup": {"synthetic_directories_removed": False},
    }
    replacements = [(str(tracker), "<tracker>")]
    workspace_path: Path | None = None
    client_cwd_path: Path | None = None
    exit_code = 0
    try:
        require(args.timeout > 0, "--timeout must be positive")
        require(tracker.is_file(), "--tracker is not a regular file")
        require(os.access(tracker, os.X_OK), "--tracker is not executable")
        version_raw = run_cli(tracker, Path(tempfile.gettempdir()), ["version", "--json"], args.timeout)
        version = json.loads(version_raw)
        proof["tracker"] = {
            key: version.get(key)
            for key in ("version", "commit", "build_date", "go_version", "platform")
            if key in version
        }

        with tempfile.TemporaryDirectory(prefix="atlas-mcp-workspace-") as workspace_raw, tempfile.TemporaryDirectory(
            prefix="atlas-mcp-client-cwd-"
        ) as client_cwd_raw:
            workspace_path = Path(workspace_raw).resolve()
            client_cwd_path = Path(client_cwd_raw).resolve()
            replacements.extend(
                [
                    (str(workspace_path), "<synthetic-workspace>"),
                    (str(client_cwd_path), "<synthetic-client-cwd>"),
                ]
            )
            require(workspace_path != client_cwd_path, "synthetic workspace and client cwd must differ")
            run_cli(tracker, workspace_path, ["init", "--skip-integrations", "--json"], args.timeout)
            run_cli(tracker, workspace_path, ["project", "create", "APP", "MCP Smoke", "--json"], args.timeout)
            run_cli(
                tracker,
                workspace_path,
                [
                    "project",
                    "policy",
                    "set",
                    "APP",
                    "--completion-mode",
                    "dual_gate",
                    "--required-reviewer",
                    "agent:reviewer-1",
                    "--allowed-workers",
                    "agent:builder-1",
                    "--actor",
                    "human:owner",
                    "--reason",
                    "MCP smoke policy",
                    "--json",
                ],
                args.timeout,
            )
            verify_transport_and_profiles(proof, tracker, workspace_path, client_cwd_path, args.timeout)
            verify_workflow(proof, tracker, workspace_path, client_cwd_path, args.timeout)
            proof["status"] = "passed"
        proof["cleanup"]["synthetic_directories_removed"] = bool(
            workspace_path is not None
            and client_cwd_path is not None
            and not workspace_path.exists()
            and not client_cwd_path.exists()
        )
        require(proof["cleanup"]["synthetic_directories_removed"], "temporary directory cleanup failed")
    except Exception as exc:  # Emit bounded, sanitized proof for deterministic CI diagnostics.
        exit_code = 1
        if workspace_path is not None and client_cwd_path is not None:
            proof["cleanup"]["synthetic_directories_removed"] = bool(
                not workspace_path.exists() and not client_cwd_path.exists()
            )
        proof["status"] = "failed"
        proof["error"] = {
            "type": type(exc).__name__,
            "message": sanitize(str(exc), replacements)[:8000],
        }

    write_outputs(proof, args.proof_json, args.report)
    print(json.dumps(proof, sort_keys=True))
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
