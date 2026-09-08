package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/spf13/cobra"
)

func newScheduleCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "schedule", Short: "Schedule human reminders and agent wakeups"}

	set := &cobra.Command{Use: "set <ID>", Args: cobra.ExactArgs(1), Short: "Set or replace a one-time ticket schedule", RunE: runScheduleSet}
	set.Flags().String("at", "", "Run time in RFC3339 format")
	set.Flags().String("runner", "", "Human or agent actor that will own the ticket")
	addMutationFlags(set, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(set, &outputFlags{})
	_ = set.MarkFlagRequired("at")
	_ = set.MarkFlagRequired("runner")
	cmd.AddCommand(set)

	clear := &cobra.Command{Use: "clear <ID>", Args: cobra.ExactArgs(1), Short: "Remove a ticket schedule", RunE: runScheduleClear}
	addMutationFlags(clear, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(clear, &outputFlags{})
	cmd.AddCommand(clear)

	list := &cobra.Command{Use: "list", Short: "List scheduled tickets", RunE: runScheduleList}
	addScheduleQueryFlags(list)
	addReadOutputFlags(list, &outputFlags{})
	cmd.AddCommand(list)

	history := &cobra.Command{Use: "history", Short: "List ticket completion history", RunE: runScheduleHistory}
	addScheduleQueryFlags(history)
	addReadOutputFlags(history, &outputFlags{})
	cmd.AddCommand(history)

	tick := &cobra.Command{Use: "tick", Short: "Trigger every schedule due now", RunE: runScheduleTick}
	tick.Flags().String("now", "", "Override the due-time check with an RFC3339 instant")
	addMutationFlags(tick, &mutationFlags{Actor: "human:owner"})
	addReadOutputFlags(tick, &outputFlags{})
	cmd.AddCommand(tick)

	return cmd
}

func addScheduleQueryFlags(cmd *cobra.Command) {
	cmd.Flags().String("project", "", "Project filter")
	cmd.Flags().String("from", "", "Inclusive RFC3339 start")
	cmd.Flags().String("to", "", "Exclusive RFC3339 end")
}

func runScheduleSet(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	atRaw, _ := cmd.Flags().GetString("at")
	at, err := parseScheduleTime(atRaw, "at")
	if err != nil {
		return err
	}
	runnerRaw, _ := cmd.Flags().GetString("runner")
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ticket, err := w.actions.SetTicketSchedule(commandContext(cmd), args[0], at, contracts.Actor(strings.TrimSpace(runnerRaw)), normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("scheduled %s for %s with %s", ticket.ID, ticket.Schedule.At.Format(timeRFC3339), ticket.Assignee)
	md := fmt.Sprintf("## Scheduled %s\n\n- At: `%s`\n- Runner: `%s`\n", ticket.ID, ticket.Schedule.At.Format(timeRFC3339), ticket.Assignee)
	return writeCommandOutput(cmd, ticket, md, pretty)
}

func runScheduleClear(cmd *cobra.Command, args []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	ticket, err := w.actions.ClearTicketSchedule(commandContext(cmd), args[0], normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, ticket, fmt.Sprintf("## %s\n\nSchedule cleared.\n", ticket.ID), fmt.Sprintf("cleared schedule for %s", ticket.ID))
}

func runScheduleList(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	query, err := scheduleQueryFromFlags(cmd)
	if err != nil {
		return err
	}
	view, err := w.queries.Schedule(commandContext(cmd), query)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, view.Entries, scheduleEntriesMarkdown(view.Entries), scheduleEntriesPretty(view.Entries))
}

func runScheduleHistory(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	query, err := scheduleQueryFromFlags(cmd)
	if err != nil {
		return err
	}
	history, err := w.queries.CompletionHistory(commandContext(cmd), query)
	if err != nil {
		return err
	}
	return writeCommandOutput(cmd, history, completionHistoryMarkdown(history), completionHistoryPretty(history))
}

func runScheduleTick(cmd *cobra.Command, _ []string) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	defer w.close()
	nowRaw, _ := cmd.Flags().GetString("now")
	var at time.Time
	if strings.TrimSpace(nowRaw) != "" {
		at, err = parseScheduleTime(nowRaw, "now")
		if err != nil {
			return err
		}
	}
	actorRaw, _ := cmd.Flags().GetString("actor")
	reason, _ := cmd.Flags().GetString("reason")
	result, err := w.actions.TickSchedules(commandContext(cmd), at, normalizeActor(actorRaw), reason)
	if err != nil {
		return err
	}
	pretty := fmt.Sprintf("schedule tick at %s: %d handled", result.At.Format(timeRFC3339), len(result.Entries))
	md := fmt.Sprintf("## Schedule Tick\n\n- At: `%s`\n- Handled: %d\n", result.At.Format(timeRFC3339), len(result.Entries))
	for _, entry := range result.Entries {
		md += fmt.Sprintf("- `%s` %s `%s`", entry.TicketID, entry.State, entry.Runner)
		if entry.Error != "" {
			md += ": " + entry.Error
		}
		md += "\n"
		pretty += fmt.Sprintf("\n- %s %s %s", entry.TicketID, entry.State, entry.Runner)
	}
	return writeCommandOutput(cmd, result, md, pretty)
}

func scheduleQueryFromFlags(cmd *cobra.Command) (service.ScheduleQuery, error) {
	project, _ := cmd.Flags().GetString("project")
	fromRaw, _ := cmd.Flags().GetString("from")
	toRaw, _ := cmd.Flags().GetString("to")
	query := service.ScheduleQuery{Project: strings.TrimSpace(project)}
	var err error
	if strings.TrimSpace(fromRaw) != "" {
		query.From, err = parseScheduleTime(fromRaw, "from")
		if err != nil {
			return service.ScheduleQuery{}, err
		}
	}
	if strings.TrimSpace(toRaw) != "" {
		query.To, err = parseScheduleTime(toRaw, "to")
		if err != nil {
			return service.ScheduleQuery{}, err
		}
	}
	return query, nil
}

func parseScheduleTime(raw string, name string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --%s time %q; use RFC3339 with a timezone", name, raw)
	}
	return parsed.UTC(), nil
}

func scheduleEntriesPretty(entries []service.ScheduleEntry) string {
	if len(entries) == 0 {
		return "no scheduled tickets"
	}
	lines := []string{"SCHEDULE"}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("- %s %s %s %s", entry.Ticket.Schedule.At.Format(timeRFC3339), entry.Ticket.ID, entry.State, entry.Runner))
	}
	return strings.Join(lines, "\n")
}

func scheduleEntriesMarkdown(entries []service.ScheduleEntry) string {
	if len(entries) == 0 {
		return "## Schedule\n\nNo scheduled tickets.\n"
	}
	lines := []string{"## Schedule", ""}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("- `%s` **%s** — %s with `%s`", entry.Ticket.Schedule.At.Format(timeRFC3339), entry.Ticket.ID, entry.State, entry.Runner))
	}
	return strings.Join(lines, "\n") + "\n"
}

func completionHistoryPretty(history []service.CompletionEntry) string {
	if len(history) == 0 {
		return "no completed tickets"
	}
	lines := []string{"COMPLETED"}
	for _, entry := range history {
		lines = append(lines, fmt.Sprintf("- %s %s by %s", entry.CompletedAt.Format(timeRFC3339), entry.Ticket.ID, entry.CompletedBy))
	}
	return strings.Join(lines, "\n")
}

func completionHistoryMarkdown(history []service.CompletionEntry) string {
	if len(history) == 0 {
		return "## Completion History\n\nNo completed tickets.\n"
	}
	lines := []string{"## Completion History", ""}
	for _, entry := range history {
		lines = append(lines, fmt.Sprintf("- `%s` **%s** — completed by `%s`", entry.CompletedAt.Format(timeRFC3339), entry.Ticket.ID, entry.CompletedBy))
	}
	return strings.Join(lines, "\n") + "\n"
}
