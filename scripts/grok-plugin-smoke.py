#!/usr/bin/env python3
"""Reviewer smoke test for the Atlas Tasker Grok plugin.

Launches exactly the stdio command declared in .mcp.json, performs the MCP
initialize handshake, lists tools, and calls the read-only atlas_status tool.
No network access; standard library only.
"""
import json, subprocess, sys
cfg = json.load(open(sys.argv[1] if len(sys.argv) > 1 else ".mcp.json"))["mcpServers"]["atlas-tasker"]
p = subprocess.Popen([cfg["command"], *cfg["args"]], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
def rpc(i, method, params=None):
    msg = {"jsonrpc": "2.0", "method": method, **({"id": i} if i else {}), **({"params": params} if params is not None else {})}
    p.stdin.write(json.dumps(msg) + "\n"); p.stdin.flush()
    return json.loads(p.stdout.readline()) if i else None
init = rpc(1, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "review", "version": "0"}})
print("server:", init["result"]["serverInfo"])
rpc(None, "notifications/initialized")
tools = [t["name"] for t in rpc(2, "tools/list", {})["result"]["tools"]]
print(f"tools ({len(tools)}):", ", ".join(sorted(tools)))
res = rpc(3, "tools/call", {"name": "atlas_status", "arguments": {}})["result"]
print("atlas_status isError:", res.get("isError", False))
print(res["content"][0]["text"][:600])
p.stdin.close(); p.wait(timeout=10)
