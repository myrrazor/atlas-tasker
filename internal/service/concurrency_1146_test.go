package service

import (
	"sync"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestConcurrentMutationDuringCheckpoint(t *testing.T) {
	ctx, actions := newCheckpointHarness(t)
	remote := initBareRemote(t)
	if _, err := actions.AddBackupTarget(ctx, BackupTargetAddOptions{
		TargetID: "conc", URL: "file://" + remote, Enabled: true,
		AcknowledgeBoundary: true, AttestPrivate: true, AllowLocalFile: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.EnableAutoBackup(ctx, "conc"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := actions.MoveTicket(ctx, "APP-1", contracts.StatusInProgress, "human:owner", "concurrent move")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := actions.BackupTick(ctx, true)
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := actions.MoveTicket(ctx, "APP-1", contracts.StatusReady, "human:owner", "still available"); err != nil {
		t.Fatalf("mutation after concurrent checkpoint: %v", err)
	}
}
