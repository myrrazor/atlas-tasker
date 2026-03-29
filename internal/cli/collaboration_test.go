package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestV16ReadStubsUseContractShape(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	statusOut, err := runCLI(t, "sync", "status", "--json")
	if err != nil {
		t.Fatalf("sync status failed: %v", err)
	}
	var status struct {
		FormatVersion string         `json:"format_version"`
		Kind          string         `json:"kind"`
		Payload       map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("parse sync status json: %v\nraw=%s", err, statusOut)
	}
	if status.FormatVersion != jsonFormatVersion || status.Kind != "sync_status" || status.Payload == nil {
		t.Fatalf("unexpected sync status stub payload: %#v", status)
	}

	mentionOut, err := runCLI(t, "mentions", "view", "mention_1", "--json")
	if err != nil {
		t.Fatalf("mentions view failed: %v", err)
	}
	var mention struct {
		FormatVersion string         `json:"format_version"`
		Kind          string         `json:"kind"`
		Payload       map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(mentionOut), &mention); err != nil {
		t.Fatalf("parse mention detail json: %v\nraw=%s", err, mentionOut)
	}
	if mention.FormatVersion != jsonFormatVersion || mention.Kind != "mention_detail" || mention.Payload == nil {
		t.Fatalf("unexpected mention detail stub payload: %#v", mention)
	}
}

func TestV16MutationStubsReturnJSONErrorExit(t *testing.T) {
	withTempWorkspace(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute([]string{"collaborator", "add", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected collaborator add stub to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout for json error path, got %s", stdout.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
			Exit int    `json:"exit"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("parse json error envelope: %v\nraw=%s", err, stderr.String())
	}
	if envelope.OK || envelope.Error.Exit == 0 {
		t.Fatalf("unexpected json error envelope: %#v", envelope)
	}
}
