package markdown

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type projectFrontmatter struct {
	Key           string    `yaml:"key"`
	Name          string    `yaml:"name"`
	CreatedAt     time.Time `yaml:"created_at"`
	SchemaVersion int       `yaml:"schema_version"`
}

// ProjectStore persists project metadata under projects/<KEY>/project.md.
type ProjectStore struct {
	RootDir string
}

func (s ProjectStore) CreateProject(_ context.Context, project contracts.Project) error {
	if err := project.Validate(); err != nil {
		return err
	}

	projectDir := storage.ProjectDir(s.RootDir, project.Key)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project %s already exists", project.Key)
	}

	if err := os.MkdirAll(storage.TicketsDir(s.RootDir, project.Key), 0o755); err != nil {
		return fmt.Errorf("create project directories: %w", err)
	}

	fm := projectFrontmatter{Key: project.Key, Name: project.Name, CreatedAt: project.CreatedAt, SchemaVersion: contracts.CurrentSchemaVersion}
	rawFM, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal project frontmatter: %w", err)
	}

	doc := fmt.Sprintf("---\n%s---\n\n# %s\n\nProject `%s`.\n", string(rawFM), project.Name, project.Key)
	if err := os.WriteFile(storage.ProjectFile(s.RootDir, project.Key), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}

	return nil
}

func (s ProjectStore) ListProjects(_ context.Context) ([]contracts.Project, error) {
	projectsRoot := storage.ProjectsDir(s.RootDir)
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.Project{}, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	projects := make([]contracts.Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		project, err := s.GetProject(context.Background(), entry.Name())
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Key < projects[j].Key
	})
	return projects, nil
}

func (s ProjectStore) GetProject(_ context.Context, key string) (contracts.Project, error) {
	raw, err := os.ReadFile(storage.ProjectFile(s.RootDir, key))
	if err != nil {
		return contracts.Project{}, fmt.Errorf("read project %s: %w", key, err)
	}
	fmRaw, _, err := splitFrontmatter(string(raw))
	if err != nil {
		return contracts.Project{}, fmt.Errorf("parse project frontmatter: %w", err)
	}

	var fm projectFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.Project{}, fmt.Errorf("unmarshal project frontmatter: %w", err)
	}

	project := contracts.Project{
		Key:       strings.TrimSpace(fm.Key),
		Name:      strings.TrimSpace(fm.Name),
		CreatedAt: fm.CreatedAt,
	}
	if err := project.Validate(); err != nil {
		return contracts.Project{}, err
	}
	return project, nil
}
