package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestCorruptIndexRecoveryDoesNotResetBeforeAcquiringLock(t *testing.T) {
	for _, args := range [][]string{{"reindex"}, {"doctor", "--repair"}} {
		t.Run(args[0], func(t *testing.T) {
			withTempWorkspace(t)
			seedTwoTickets(t)
			path := indexPath(t)
			corrupt := []byte("corrupt derived index: preserve until repair owns the lock")
			if err := os.WriteFile(path, corrupt, 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			unlock, err := (service.FileLockManager{Root: root}).Acquire(context.Background(), "another writer")
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cmd := NewRootCommand()
			cmd.SetContext(ctx)
			cmd.SetArgs(args)
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			if err := cmd.Execute(); err == nil {
				t.Fatal("recovery proceeded without the lock")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, corrupt) {
				t.Fatal("recovery reset index before acquiring the write lock")
			}
		})
	}
}
