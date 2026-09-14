package mcp

import (
	"encoding/json"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/myrrazor/atlas-tasker/internal/render"
)

const (
	BoardAppResourceURI = "ui://atlas/board"
	BoardAppMIME        = "text/html;profile=mcp-app"
	BoardAppCSP         = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'"
	BoardAppProtocol    = "2026-01-26"
)

func boardAppUICSPMeta() mcpsdk.Meta {
	return mcpsdk.Meta{"ui": map[string]any{
		"csp": map[string]any{
			"connectDomains":  []string{},
			"resourceDomains": []string{},
			"frameDomains":    []string{},
		},
	}}
}

// boardAppHTML is the MCP Apps host document. Conforming hosts load this
// resource in a sandbox and post the structured atlas.board result. Markdown
// stays in CallToolResult.Content for hosts that cannot render Apps.
func boardAppHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="` + BoardAppCSP + `">
<title>Atlas board</title>
<style>
` + render.BoardAppCSS + `
details.diagnostics{margin:.35rem 0 .6rem;color:var(--muted);font-size:.85rem}
details.diagnostics summary{cursor:pointer}
button.board-nav{margin:.15rem 0 .55rem;padding:.28rem .7rem;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--text);font:inherit}
.empty.error{color:var(--blocked)}
</style>
</head>
<body>
<div id="root"><p class="muted lede">Waiting for host tool result…</p></div>
<script>
(function () {
  var root = document.getElementById("root");
  var pendingInit = 1;
  var alive = true;
  var hostCapabilities = {};
  function text(el, value) {
    el.textContent = value == null ? "" : String(value);
  }
  function post(msg) {
    if (!window.parent || window.parent === window) return;
    window.parent.postMessage(msg, "*");
  }
  function isRPC(msg) {
    return msg && typeof msg === "object" && msg.jsonrpc === "2.0";
  }
  function sendRequest(method, params, id) {
    post({ jsonrpc: "2.0", id: id, method: method, params: params || {} });
  }
  function sendNotify(method, params) {
    post({ jsonrpc: "2.0", method: method, params: params || {} });
  }
  function reply(id, result) {
    post({ jsonrpc: "2.0", id: id, result: result || {} });
  }
  function notifySize() {
    var doc = document.documentElement;
    var body = document.body;
    var width = 0;
    var height = 0;
    if (doc) {
      width = doc.clientWidth || doc.scrollWidth || 0;
      height = doc.scrollHeight || doc.clientHeight || 0;
    }
    if (body) {
      if (!width) width = body.scrollWidth || 0;
      if (!height) height = body.scrollHeight || 0;
    }
    sendNotify("ui/notifications/size-changed", { width: width, height: height });
  }
  function hostCanOpenLinks() {
    var caps = hostCapabilities || {};
    return !!caps.openLinks && typeof caps.openLinks === "object" && !Array.isArray(caps.openLinks);
  }
  function applyHostContext(ctx) {
    if (!ctx || typeof ctx !== "object") return;
    var scheme = ctx.theme;
    if (scheme === "dark" || scheme === "light") {
      document.documentElement.setAttribute("data-theme", scheme);
    }
    var container = ctx.containerDimensions && typeof ctx.containerDimensions === "object" ? ctx.containerDimensions : null;
    var width = container && (container.width || container.maxWidth);
    if (typeof width === "number" && isFinite(width) && width > 0) {
      document.documentElement.setAttribute("data-container-width", String(width));
      if (width <= 390) document.documentElement.setAttribute("data-container-narrow", "1");
      else document.documentElement.removeAttribute("data-container-narrow");
    }
  }
  function boardFrom(data) {
    if (!data || typeof data !== "object") return null;
    var body = data;
    if (data.structuredContent && typeof data.structuredContent === "object") body = data.structuredContent;
    if (body.payload && typeof body.payload === "object") body = body.payload;
    if (body.payload && typeof body.payload === "object" && body.payload.board) body = body.payload;
    if (body.board && typeof body.board === "object") return body.board;
    if (Array.isArray(body.columns)) return body;
    return null;
  }
  function backupLine(backup) {
    if (!backup || typeof backup !== "object") return "";
    var parts = [];
    if (backup.automatic_enabled) parts.push("automatic checkpoints");
    else if (backup.configured === false || backup.automatic_enabled === false) parts.push("local checkpoints off");
    if (backup.last_local_checkpoint_id) parts.push("local checkpoint present");
    if (backup.unbacked_event_count) parts.push(String(backup.unbacked_event_count) + " unbacked events");
    if (backup.verified_remote) parts.push("remote verified");
    if (backup.last_error_class) parts.push(String(backup.last_error_class));
    return parts.join(" · ");
  }
  function diagnosticNotes(board) {
    var notes = [];
    if (board && Array.isArray(board.notes)) notes = notes.concat(board.notes);
    if (board && board.backup && Array.isArray(board.backup.notes)) notes = notes.concat(board.backup.notes);
    return notes.filter(function (n) { return n && String(n).trim(); });
  }
  function countPhrase(board) {
    var total = board.total_cards || 0;
    var shown = board.shown_cards || 0;
    if (board.truncated && shown !== total) return "showing " + shown + " of " + total + " tickets";
    if (total === 1) return "1 ticket";
    return String(total) + " tickets";
  }
  function showMessage(msg, cls) {
    root.replaceChildren();
    var p = document.createElement("p");
    p.className = cls || "empty";
    text(p, msg);
    root.appendChild(p);
    notifySize();
  }
  function toolResultError(params) {
    if (!params || typeof params !== "object") return "";
    if (params.isError !== true && params.is_error !== true) return "";
    var content = params.content;
    if (Array.isArray(content)) {
      for (var i = 0; i < content.length; i++) {
        if (content[i] && content[i].text) return String(content[i].text);
      }
    }
    var structured = params.structuredContent;
    if (structured && structured.error) {
      var err = structured.error;
      if (typeof err === "string") return err;
      if (err && err.message) return String(err.message);
    }
    return "Tool failed";
  }
  function render(data) {
    root.replaceChildren();
    var board = boardFrom(data);
    if (!board || typeof board !== "object") {
      showMessage("No board payload", "empty");
      return;
    }
    var h1 = document.createElement("h1");
    text(h1, board.title || "Board");
    root.appendChild(h1);
    var lede = document.createElement("p");
    lede.className = "lede";
    text(lede, countPhrase(board));
    root.appendChild(lede);
    if (hostCanOpenLinks() && board.board_url) {
      var nav = document.createElement("button");
      nav.type = "button";
      nav.className = "board-nav";
      text(nav, "Open board");
      root.appendChild(nav);
    }
    if (Array.isArray(board.attention) && board.attention.length) {
      var att = document.createElement("p");
      att.className = "attention meta";
      text(att, "Attention: " + board.attention.join("; "));
      root.appendChild(att);
    }
    if (Array.isArray(board.next_actions) && board.next_actions.length) {
      var next = document.createElement("p");
      next.className = "meta";
      text(next, "Next: " + board.next_actions[0]);
      root.appendChild(next);
    }
    var backup = backupLine(board.backup);
    if (backup) {
      var b = document.createElement("p");
      b.className = "meta";
      text(b, "Backup: " + backup);
      root.appendChild(b);
    }
    var extras = diagnosticNotes(board);
    if (extras.length) {
      var details = document.createElement("details");
      details.className = "diagnostics";
      var summary = document.createElement("summary");
      text(summary, "Diagnostics");
      details.appendChild(summary);
      extras.forEach(function (note) {
        var n = document.createElement("p");
        n.className = "note";
        text(n, note);
        details.appendChild(n);
      });
      root.appendChild(details);
    }
    var lanes = document.createElement("div");
    lanes.className = "lanes";
    var columns = Array.isArray(board.columns) ? board.columns : [];
    var wrote = false;
    columns.forEach(function (col) {
      if (!col || !col.total) return;
      wrote = true;
      var section = document.createElement("section");
      section.className = "lane-" + (col.status || "");
      section.setAttribute("aria-label", col.label || col.status || "lane");
      var heading = document.createElement("h2");
      var headingText = (col.label || col.status || "Lane") + " (" + String(col.total) + ")";
      if (col.truncated) headingText += " showing " + String(col.shown) + ", truncated";
      text(heading, headingText);
      section.appendChild(heading);
      var cards = Array.isArray(col.cards) ? col.cards : [];
      cards.forEach(function (card) {
        var article = document.createElement("article");
        var title = document.createElement("span");
        title.className = "title";
        text(title, (card && card.title) ? card.title : "(untitled)");
        article.appendChild(title);
        var kicker = document.createElement("span");
        kicker.className = "kicker";
        var bits = [];
        if (card && card.id) bits.push(card.id);
        if (card && card.priority && card.priority !== "medium") bits.push(card.priority);
        if (card && card.assignee) bits.push(card.assignee);
        if (card && Array.isArray(card.labels) && card.labels.length) bits.push(card.labels.join(", "));
        text(kicker, bits.join(" · "));
        article.appendChild(kicker);
        section.appendChild(article);
      });
      lanes.appendChild(section);
    });
    if (!wrote) {
      var empty = document.createElement("p");
      empty.className = "empty";
      text(empty, "0 tickets.");
      root.appendChild(empty);
      notifySize();
      return;
    }
    root.appendChild(lanes);
    notifySize();
  }
  window.addEventListener("message", function (event) {
    if (!alive) return;
    if (event.source !== window.parent) return;
    var message = event.data;
    if (!isRPC(message)) return;
    if (pendingInit != null && message.id === pendingInit && !message.method) {
      if (message.error) {
        pendingInit = null;
        var fail = "Host initialize failed";
        if (message.error.message) fail = String(message.error.message);
        showMessage(fail, "empty error");
        return;
      }
      if (!message.result || typeof message.result !== "object") return;
      if (message.result.protocolVersion !== "` + BoardAppProtocol + `") {
        pendingInit = null;
        showMessage("Unsupported host protocol", "empty error");
        return;
      }
      hostCapabilities = message.result.hostCapabilities && typeof message.result.hostCapabilities === "object"
        ? message.result.hostCapabilities
        : {};
      if (message.result.hostContext) applyHostContext(message.result.hostContext);
      pendingInit = null;
      sendNotify("ui/notifications/initialized", { protocolVersion: "` + BoardAppProtocol + `" });
      notifySize();
      return;
    }
    if (message.method === "ping" && message.id != null) {
      reply(message.id, {});
      return;
    }
    if (message.method === "ui/resource-teardown" && message.id != null) {
      reply(message.id, {});
      alive = false;
      showMessage("App closed", "empty");
      return;
    }
    if (message.method === "ui/notifications/host-context-changed") {
      applyHostContext(message.params || {});
      notifySize();
      return;
    }
    if (message.method === "ui/notifications/tool-cancelled") {
      showMessage("Tool cancelled", "empty");
      return;
    }
    if (message.method === "ui/notifications/tool-result") {
      var params = message.params || {};
      var errText = toolResultError(params);
      if (errText) {
        showMessage(errText, "empty error");
        return;
      }
      render(params);
    }
  });
  sendRequest("ui/initialize", {
    protocolVersion: "` + BoardAppProtocol + `",
    appInfo: { name: "atlas-board", version: "1.15" },
    appCapabilities: {}
  }, pendingInit);
})();
</script>
</body>
</html>
`
}

func boardAppUsesSafeDOM(html string) bool {
	lower := strings.ToLower(html)
	if strings.Contains(lower, "innerhtml") || strings.Contains(lower, "document.write") {
		return false
	}
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "fetch(") {
		return false
	}
	return strings.Contains(html, "textContent") &&
		strings.Contains(html, "ui/initialize") &&
		strings.Contains(html, "protocolVersion") &&
		strings.Contains(html, "appInfo") &&
		strings.Contains(html, "appCapabilities") &&
		strings.Contains(html, "ui/notifications/initialized") &&
		strings.Contains(html, "ui/notifications/tool-result") &&
		strings.Contains(html, "ui/notifications/tool-cancelled") &&
		strings.Contains(html, "ui/resource-teardown") &&
		strings.Contains(html, "ui/notifications/host-context-changed") &&
		strings.Contains(html, "ui/notifications/size-changed") &&
		strings.Contains(html, "event.source") &&
		strings.Contains(html, "window.parent") &&
		!strings.Contains(html, "ui/teardown") &&
		!strings.Contains(html, "messaging")
}

func boardAppViewLines(payload map[string]any) []string {
	raw, _ := json.Marshal(payload)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	body := generic
	if nested, ok := generic["payload"].(map[string]any); ok {
		body = nested
	}
	if md, ok := body["markdown"].(string); ok && strings.TrimSpace(md) != "" {
		return strings.Split(md, "\n")
	}
	return nil
}
