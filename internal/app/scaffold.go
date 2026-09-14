package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

const (
	ManagedGitignoreBegin = "# atlas-tasker:begin-local-ignore"
	ManagedGitignoreEnd   = "# atlas-tasker:end-local-ignore"
)

type ScaffoldOptions struct {
	Now     func() time.Time
	GitMode GitMode
}

type ScaffoldResult struct {
	Kind    string
	Root    string
	Created []string
}

func RefuseNestedWorkspaceInit(root string) error {
	info, err := os.Stat(storage.TrackerDir(root))
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(storage.TrackerDir(dir))
		if err == nil && info.IsDir() {
			return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("%s is inside existing Atlas workspace %s; run tracker from there", root, dir))
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
}

func ScaffoldWorkspace(root string, opts ScaffoldOptions) (ScaffoldResult, error) {
	root, err := service.CanonicalWorkspaceRoot(root)
	if err != nil {
		return ScaffoldResult{}, err
	}
	if err := RefuseNestedWorkspaceInit(root); err != nil {
		return ScaffoldResult{}, err
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	result := ScaffoldResult{Kind: "workspace_init", Root: root, Created: []string{}}
	trackerDir := storage.TrackerDir(root)
	privateDirs := map[string]struct{}{
		trackerDir:                            {},
		storage.ImportsDir(root):              {},
		storage.ExportsDir(root):              {},
		storage.ArchivesDir(root):             {},
		filepath.Join(trackerDir, "evidence"): {},
		filepath.Join(trackerDir, "runtime"):  {},
	}
	for _, dir := range []string{
		trackerDir,
		storage.EventsDir(root),
		storage.AutomationsDir(root),
		storage.ViewsDir(root),
		storage.SubscriptionsDir(root),
		storage.AgentsDir(root),
		storage.RunbooksDir(root),
		storage.RunsDir(root),
		storage.GatesDir(root),
		storage.ChangesDir(root),
		storage.ChecksDir(root),
		storage.PermissionProfilesDir(root),
		storage.HandoffsDir(root),
		storage.ImportsDir(root),
		storage.ExportsDir(root),
		storage.RetentionPoliciesDir(root),
		storage.ArchivesDir(root),
		filepath.Join(trackerDir, "evidence"),
		filepath.Join(trackerDir, "runtime"),
		filepath.Join(trackerDir, "templates"),
		storage.ProjectsDir(root),
	} {
		missing, err := missingPath(dir)
		if err != nil {
			return ScaffoldResult{}, err
		}
		mode := os.FileMode(0o755)
		if _, ok := privateDirs[dir]; ok {
			mode = 0o700
		}
		if err := os.MkdirAll(dir, mode); err != nil {
			return ScaffoldResult{}, err
		}
		if _, ok := privateDirs[dir]; ok {
			if err := os.Chmod(dir, 0o700); err != nil {
				return ScaffoldResult{}, err
			}
		}
		if missing {
			result.Created = append(result.Created, relToRoot(root, dir))
		}
	}
	identityMissing, err := missingPath(storage.WorkspaceMetadataFile(root))
	if err != nil {
		return ScaffoldResult{}, err
	}
	if _, err := service.EnsureWorkspaceIdentityForCLI(root); err != nil {
		return ScaffoldResult{}, err
	}
	if identityMissing {
		result.Created = append(result.Created, relToRoot(root, storage.WorkspaceMetadataFile(root)))
	}
	configMissing, err := missingPath(config.Path(root))
	if err != nil {
		return ScaffoldResult{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return ScaffoldResult{}, err
	}
	if err := config.Save(root, cfg); err != nil {
		return ScaffoldResult{}, err
	}
	if configMissing {
		result.Created = append(result.Created, relToRoot(root, config.Path(root)))
	}
	monthFile := filepath.Join(storage.EventsDir(root), now().Format("2006-01")+".jsonl")
	if missing, err := missingPath(monthFile); err != nil {
		return ScaffoldResult{}, err
	} else if missing {
		if err := os.WriteFile(monthFile, []byte(""), 0o644); err != nil {
			return ScaffoldResult{}, err
		}
		result.Created = append(result.Created, relToRoot(root, monthFile))
	}
	templates := defaultTicketTemplates()
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(trackerDir, "templates", name)
		missing, err := missingPath(path)
		if err != nil {
			return ScaffoldResult{}, err
		}
		if !missing {
			continue
		}
		if err := os.WriteFile(path, []byte(templates[name]), 0o644); err != nil {
			return ScaffoldResult{}, err
		}
		result.Created = append(result.Created, relToRoot(root, path))
	}
	mode := opts.GitMode.Normalized()
	if mode != GitModeUnmanaged {
		if updated, err := applyGitIgnore(root, mode); err != nil {
			return ScaffoldResult{}, err
		} else if updated {
			if mode == GitModePrivate {
				result.Created = append(result.Created, relToRoot(root, filepath.Join(root, ".git", "info", "exclude")))
			} else {
				result.Created = append(result.Created, relToRoot(root, filepath.Join(root, ".gitignore")))
			}
		}
	}
	if err := os.WriteFile(filepath.Join(storage.TrackerDir(root), "runtime", "git-mode"), []byte(string(mode)+"\n"), 0o600); err != nil {
		return ScaffoldResult{}, err
	}
	return result, nil
}

func missingPath(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, err
}

func relToRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func defaultTicketTemplates() map[string]string {
	return map[string]string{
		"epic.md": `---
type: epic
blueprint: design
---
# Summary

## Description

Shape the full slice before you break it into child work.

## Acceptance Criteria
- Scope is clear
- Child tickets can be created from this epic
`,
		"task.md": `---
type: task
blueprint: implement
---
# Summary

## Description

Implement the scoped change.

## Acceptance Criteria
- Code is merged locally
- Tests cover the new behavior
`,
		"bug.md": `---
type: bug
blueprint: qa
---
# Summary

## Description

Describe the broken behavior and the expected fix.

## Acceptance Criteria
- Repro is documented
- Fix is verified
`,
		"subtask.md": `---
type: subtask
blueprint: implement
---
# Summary

## Description

Small child task under a parent item.

## Acceptance Criteria
- Parent stays up to date
`,
		"design.md": `---
type: task
labels:
  - design
blueprint: design
skill_hint: design
---
# Summary

## Description

Capture the UX, constraints, and acceptance shape before implementation.

## Acceptance Criteria
- Design direction is written down
- Open questions are resolved or tracked
`,
		"implement.md": `---
type: task
labels:
  - implementation
blueprint: implement
skill_hint: implement
---
# Summary

## Description

Build the scoped change and keep the diff reviewable.

## Acceptance Criteria
- Behavior works locally
- Tests are updated
`,
		"review.md": `---
type: task
labels:
  - review
blueprint: review
skill_hint: review
---
# Summary

## Description

Audit the implementation for regressions and missing tests.

## Acceptance Criteria
- Findings are documented
- Blocking issues are fixed or tracked
`,
		"qa.md": `---
type: task
labels:
  - qa
blueprint: qa
skill_hint: qa
---
# Summary

## Description

Run end-to-end validation and record the results.

## Acceptance Criteria
- Happy path is verified
- Edge cases are covered
`,
		"spike.md": `---
type: task
labels:
  - spike
blueprint: spike
skill_hint: spike
---
# Summary

## Description

Time-boxed investigation with explicit follow-up output.

## Acceptance Criteria
- Findings are written down
- Next steps are clear
`,
	}
}
