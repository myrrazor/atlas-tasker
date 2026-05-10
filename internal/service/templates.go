package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

var templateNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type templateFrontmatter struct {
	Type             contracts.TicketType     `yaml:"type,omitempty"`
	Labels           []string                 `yaml:"labels,omitempty"`
	Reviewer         contracts.Actor          `yaml:"reviewer,omitempty"`
	CompletionMode   contracts.CompletionMode `yaml:"completion_mode,omitempty"`
	AllowedWorkers   []contracts.Actor        `yaml:"allowed_workers,omitempty"`
	RequiredReviewer contracts.Actor          `yaml:"required_reviewer,omitempty"`
	Blueprint        string                   `yaml:"blueprint,omitempty"`
	SkillHint        string                   `yaml:"skill_hint,omitempty"`
}

func (s *QueryService) ListTemplates(_ context.Context) ([]TemplateView, error) {
	dir := filepath.Join(storage.TrackerDir(s.Root), "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TemplateView{}, nil
		}
		return nil, err
	}
	views := make([]TemplateView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		view, err := s.Template(context.Background(), strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})
	return views, nil
}

func (s *QueryService) Template(_ context.Context, name string) (TemplateView, error) {
	name = strings.TrimSpace(name)
	if !templateNamePattern.MatchString(name) {
		return TemplateView{}, fmt.Errorf("template name must match ^[A-Za-z][A-Za-z0-9_-]{0,63}$")
	}
	path := filepath.Join(storage.TrackerDir(s.Root), "templates", name+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return TemplateView{}, err
	}
	view := TemplateView{
		Name:         name,
		Path:         path,
		TemplateBody: string(raw),
	}
	body := string(raw)
	if strings.HasPrefix(body, "---\n") {
		parts := strings.SplitN(body, "\n---\n", 2)
		if len(parts) == 2 {
			var meta templateFrontmatter
			if err := yaml.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &meta); err != nil {
				return TemplateView{}, fmt.Errorf("parse template frontmatter %s: %w", name, err)
			}
			view.Type = meta.Type
			view.Labels = meta.Labels
			view.Reviewer = meta.Reviewer
			view.Policy = contracts.TicketPolicy{
				CompletionMode:   meta.CompletionMode,
				AllowedWorkers:   meta.AllowedWorkers,
				RequiredReviewer: meta.RequiredReviewer,
			}
			view.Blueprint = strings.TrimSpace(meta.Blueprint)
			view.SkillHint = strings.TrimSpace(meta.SkillHint)
			body = parts[1]
			view.TemplateBody = body
		}
	}
	view.Description, view.Acceptance = parseTemplateBody(body)
	return view, nil
}

func parseTemplateBody(body string) (string, []string) {
	description := ""
	acceptance := make([]string, 0)
	lines := strings.Split(body, "\n")
	section := ""
	for _, line := range lines {
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "## description":
			section = "description"
			continue
		case "## acceptance criteria":
			section = "acceptance"
			continue
		case "## notes", "# summary", "":
			continue
		}
		switch section {
		case "description":
			if description == "" {
				description = strings.TrimSpace(line)
			}
		case "acceptance":
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if trimmed != "" {
				acceptance = append(acceptance, trimmed)
			}
		}
	}
	return description, acceptance
}
