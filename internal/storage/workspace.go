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
