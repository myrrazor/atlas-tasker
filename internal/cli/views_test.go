package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSavedViewCommandsAndFlags(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("config", "set", "actor.default", "agent:builder-1")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Ready", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "claim", "APP-1")

	must("views", "save", "ready-board", "--kind", "board", "--project", "APP", "--column", "ready")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=ready")
	must("views", "save", "my-queue", "--kind", "queue", "--title", "Sprint Queue", "--actor", "agent:builder-1", "--queue-category", "claimed_by_me")

	listOut := must("views", "list", "--pretty")
	if !strings.Contains(listOut, "ready-board") || !strings.Contains(listOut, "ready-search") || !strings.Contains(listOut, "my-queue") {
		t.Fatalf("unexpected views list output: %s", listOut)
	}

	boardOut := must("board", "--view", "ready-board", "--md")
	if !strings.Contains(boardOut, "### ready") || strings.Contains(boardOut, "### backlog") {
		t.Fatalf("expected board view columns to be filtered: %s", boardOut)
	}

	searchOut := must("search", "--view", "ready-search", "--pretty")
	if !strings.Contains(searchOut, "APP-1") {
		t.Fatalf("expected search view output to include APP-1: %s", searchOut)
	}

	queueOut := must("queue", "--view", "my-queue", "--pretty")
	if !strings.Contains(queueOut, "sprint queue for agent:builder-1") || !strings.Contains(queueOut, "claimed_by_me") || strings.Contains(queueOut, "ready_for_me") {
		t.Fatalf("expected queue view output to be filtered: %s", queueOut)
	}

	runOut := must("views", "run", "ready-search", "--pretty")
	if !strings.Contains(runOut, "APP-1") {
		t.Fatalf("expected views run to execute search view: %s", runOut)
	}

	if out, err := runCLI(t, "board", "--view", "ready-search"); err == nil || !strings.Contains(err.Error(), "is not a board view") {
		t.Fatalf("expected wrong-kind board view failure, got err=%v out=%s", err, out)
	}
	if out, err := runCLI(t, "search", "--view", "missing-view"); err == nil || !strings.Contains(err.Error(), "read saved view") {
		t.Fatalf("expected missing saved view failure, got err=%v out=%s", err, out)
	}

	must("views", "delete", "ready-board")
	listOut = must("views", "list", "--pretty")
	if strings.Contains(listOut, "ready-board") {
		t.Fatalf("expected ready-board to be removed: %s", listOut)
	}
}

func TestSavedViewsAndWatchersSurviveReindex(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("config", "set", "actor.default", "agent:builder-1")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Ready", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("views", "save", "ready-search", "--kind", "search", "--query", "status=ready")
	must("watch", "ticket", "APP-1")
	must("watch", "view", "ready-search", "--actor", "human:owner")

	runBefore := must("views", "run", "ready-search", "--json")
	watchBefore := must("watch", "list", "--json")
	must("reindex")
	runAfter := must("views", "run", "ready-search", "--json")
	watchAfter := must("watch", "list", "--json")

	var beforeRun struct {
		FormatVersion string `json:"format_version"`
		Tickets       []struct {
			ID string `json:"id"`
		} `json:"tickets"`
	}
	var afterRun struct {
		FormatVersion string `json:"format_version"`
		Tickets       []struct {
			ID string `json:"id"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(runBefore), &beforeRun); err != nil {
		t.Fatalf("parse pre-reindex view: %v\nraw=%s", err, runBefore)
	}
	if err := json.Unmarshal([]byte(runAfter), &afterRun); err != nil {
		t.Fatalf("parse post-reindex view: %v\nraw=%s", err, runAfter)
	}
	if beforeRun.FormatVersion != jsonFormatVersion || afterRun.FormatVersion != jsonFormatVersion {
		t.Fatalf("unexpected format versions: before=%s after=%s", beforeRun.FormatVersion, afterRun.FormatVersion)
	}
	if !reflect.DeepEqual(beforeRun.Tickets, afterRun.Tickets) {
		t.Fatalf("expected saved view output to survive reindex, before=%#v after=%#v", beforeRun.Tickets, afterRun.Tickets)
	}

	beforeWatch := decodeJSONList[map[string]any](t, watchBefore)
	afterWatch := decodeJSONList[map[string]any](t, watchAfter)
	if !reflect.DeepEqual(beforeWatch, afterWatch) {
		t.Fatalf("expected watcher list to survive reindex, before=%#v after=%#v", beforeWatch, afterWatch)
	}
}
