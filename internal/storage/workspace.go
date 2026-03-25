package storage

import "path/filepath"

const (
	TrackerDirName  = ".tracker"
	ProjectsDirName = "projects"
)

func TrackerDir(root string) string {
	return filepath.Join(root, TrackerDirName)
}

func ProjectsDir(root string) string {
	return filepath.Join(root, ProjectsDirName)
}

func EventsDir(root string) string {
	return filepath.Join(TrackerDir(root), "events")
}

func MutationsDir(root string) string {
	return filepath.Join(TrackerDir(root), "mutations")
}

func AutomationsDir(root string) string {
	return filepath.Join(TrackerDir(root), "automations")
}

func ViewsDir(root string) string {
	return filepath.Join(TrackerDir(root), "views")
}

func SubscriptionsDir(root string) string {
	return filepath.Join(TrackerDir(root), "subscriptions")
}

func AgentsDir(root string) string {
	return filepath.Join(TrackerDir(root), "agents")
}

func AgentFile(root string, agentID string) string {
	return filepath.Join(AgentsDir(root), agentID+".toml")
}

func RunbooksDir(root string) string {
	return filepath.Join(TrackerDir(root), "runbooks")
}

func RunbookFile(root string, name string) string {
	return filepath.Join(RunbooksDir(root), name+".toml")
}

func RunsDir(root string) string {
	return filepath.Join(TrackerDir(root), "runs")
}

func RunFile(root string, runID string) string {
	return filepath.Join(RunsDir(root), runID+".md")
}

func GatesDir(root string) string {
	return filepath.Join(TrackerDir(root), "gates")
}

func GateFile(root string, gateID string) string {
	return filepath.Join(GatesDir(root), gateID+".md")
}

func HandoffsDir(root string) string {
	return filepath.Join(TrackerDir(root), "handoffs")
}

func HandoffFile(root string, handoffID string) string {
	return filepath.Join(HandoffsDir(root), handoffID+".md")
}

func EvidenceDir(root string, runID string) string {
	return filepath.Join(TrackerDir(root), "evidence", runID)
}

func EvidenceFile(root string, runID string, evidenceID string) string {
	return filepath.Join(EvidenceDir(root, runID), evidenceID+".md")
}

func RuntimeDir(root string, runID string) string {
	return filepath.Join(TrackerDir(root), "runtime", runID)
}

func ProjectDir(root string, key string) string {
	return filepath.Join(ProjectsDir(root), key)
}

func ProjectFile(root string, key string) string {
	return filepath.Join(ProjectDir(root, key), "project.md")
}

func TicketsDir(root string, key string) string {
	return filepath.Join(ProjectDir(root, key), "tickets")
}

func TicketFile(root string, project string, id string) string {
	return filepath.Join(TicketsDir(root, project), id+".md")
}
