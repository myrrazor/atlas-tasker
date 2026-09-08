package web

import (
	"os/exec"
	"testing"
)

func TestBoardRefreshJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required for the browser JavaScript regression tests")
	}
	out, err := exec.Command(node, "--test", "testdata/refresh_board.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("board refresh regression tests: %v\n%s", err, out)
	}
}
