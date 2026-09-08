package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func TestDoctorReportsPendingJournalThenRepairRestoresHealth(t *testing.T) {
	withTempWorkspace(t)
	seedTwoTickets(t)
	w, err := openWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	root := w.root
	ctx := context.Background()
	journal := service.MutationJournal{Root: root, Clock: defaultNow}
	err = w.withWriteLock(ctx, "simulate interrupted promotion", func(ctx context.Context) error {
		ticket, err := w.ticket.GetTicket(ctx, "APP-2")
		if err != nil {
			return err
		}
		from := ticket.Status
		ticket.Status = contracts.StatusReady
		ticket.UpdatedAt = defaultNow()
		id, err := w.actions.NextEventID(ctx, "APP")
		if err != nil {
			return err
		}
		event := contracts.Event{EventID: id, Timestamp: ticket.UpdatedAt, Actor: contracts.ActorAtlasSystem,
			Type: contracts.EventTicketMoved, Project: "APP", TicketID: ticket.ID, Reason: "interrupted promotion",
			Payload:       map[string]any{"from": from, "to": ticket.Status, "ticket": ticket},
			SchemaVersion: contracts.CurrentSchemaVersion}
		entry, err := journal.Begin("promote dependent", "ticket_snapshot", event)
		if err != nil {
			return err
		}
		if err := w.ticket.UpdateTicket(ctx, ticket); err != nil {
			return err
		}
		_, err = journal.Mark(entry, service.MutationStageCanonicalWritten, "event append failed")
		return err
	})
	w.close()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if exit := Execute([]string{"doctor", "--json"}, &stdout, &stderr); exit != 7 {
		t.Fatalf("pending journal must be repair_needed; exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed doctor wrote success output: %s", stdout.String())
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.Code != "repair_needed" {
		t.Fatalf("unexpected doctor error: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"doctor", "--repair", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("repair exited %d: %s", exit, stderr.String())
	}
	entries, err := journal.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("repair left pending journals: %#v", entries)
	}

	w, err = openWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	got, queryErr := w.projection.QueryTicket(ctx, "APP-2")
	history, historyErr := w.projection.QueryHistory(ctx, "APP-2")
	w.close()
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if got.Status != contracts.StatusReady {
		t.Fatalf("repair lost promotion: %s", got.Status)
	}
	moves := 0
	for _, event := range history {
		if event.Type == contracts.EventTicketMoved {
			moves++
		}
	}
	if moves != 1 {
		t.Fatalf("repair must replay promotion once, got %d", moves)
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Execute([]string{"doctor", "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor after repair exited %d: %s", exit, stderr.String())
	}
	var healthy struct {
		OK            bool `json:"ok"`
		RepairPending int  `json:"repair_pending"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &healthy); err != nil {
		t.Fatal(err)
	}
	if !healthy.OK || healthy.RepairPending != 0 {
		t.Fatalf("doctor not healthy after repair: %s", stdout.String())
	}
}
