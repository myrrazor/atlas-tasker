package uninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func nowOf(opts Options) time.Time {
	if opts.Now != nil {
		return opts.Now().UTC()
	}
	return time.Now().UTC()
}

func homeOf(opts Options) string {
	home := strings.TrimSpace(opts.Home)
	if home == "" {
		home = strings.TrimSpace(getenvFn(opts)("HOME"))
	}
	return home
}

// PlanUninstall previews exact Atlas-owned software removals. It never
// mutates. Missing receipt → preview + refusal.
func PlanUninstall(ctx context.Context, opts Options) (Plan, error) {
	_ = ctx
	stateDir, err := resolveStateDir(opts)
	if err != nil {
		return Plan{Kind: KindResult, Status: StatusRefused, Refusal: err.Error()}, err
	}
	plan := Plan{Kind: KindResult, Status: StatusPreview, Preserved: preservedList(stateDir)}
	receipt, err := LoadReceipt(stateDir)
	if err != nil {
		plan.Status = StatusRefused
		plan.Refusal = err.Error()
		plan.Notes = []string{"no verifiable install receipt; refusing to delete uncertain files"}
		return plan, nil
	}
	if err := verifyExecutable(receipt.BinaryPath, receipt.BinarySHA256); err != nil && !os.IsNotExist(err) {
		plan.Status = StatusRefused
		plan.Refusal = err.Error()
		return plan, nil
	}

	manifest, err := LoadManifest(stateDir)
	if err != nil {
		plan.Notes = append(plan.Notes, "ignored uninstall manifest: "+err.Error())
		manifest = Manifest{}
	} else if manifest.Format != "" && manifest.ReceiptDigest != receipt.Digest {
		rebound, rebindErr := RebindManifest(manifest, receipt, nowOf(opts))
		if rebindErr != nil {
			plan.Notes = append(plan.Notes, "stale uninstall manifest ignored; using discovery")
			manifest = Manifest{}
		} else {
			// preview keeps the rebound in memory; disk is updated on apply or installer write
			manifest = rebound
			plan.Notes = append(plan.Notes, "rebound uninstall manifest to current receipt")
		}
	}

	home := homeOf(opts)
	goos := goosOf(opts)
	actions := mergeActions(manifest.Actions, discoverUnitActions(home, goos, receipt.BinaryPath), discoverClientActions(home, stateDir, receipt.BinaryPath))
	for _, action := range actions {
		if err := validateManifestAction(action, receipt); err != nil {
			plan.Notes = append(plan.Notes, "skipped "+action.ID+": "+err.Error())
			continue
		}
		if err := verifyActionLive(action, receipt); err != nil {
			plan.Notes = append(plan.Notes, "skipped "+action.ID+": "+err.Error())
			continue
		}
		plan.Actions = append(plan.Actions, toPlanAction(action))
	}

	if instr := receipt.managerInstruction(); instr != "" {
		plan.Actions = append(plan.Actions, PlanAction{
			ID: "package-manager", Kind: KindPackageManager, Detail: instr, Verified: true,
		})
		plan.Notes = append(plan.Notes, instr)
	} else {
		plan.Actions = append(plan.Actions, PlanAction{
			ID: "remove-executable", Kind: KindRemoveFile, Path: receipt.BinaryPath,
			Detail: "receipt-bound tracker executable", Verified: true,
		})
	}

	plan.PlanID = "uninstall-" + receipt.Digest[:12]
	plan.Digest = planDigest(plan)
	plan.CanApply = plan.Status == StatusPreview && len(verifiedActions(plan)) > 0
	if !plan.CanApply && plan.Refusal == "" {
		plan.Status = StatusRefused
		plan.Refusal = "nothing verified to remove"
	}
	return plan, nil
}

// Apply executes a digest-bound plan. yes/apply is required. Idempotent.
func Apply(ctx context.Context, opts Options, yes bool) (Result, error) {
	plan, err := PlanUninstall(ctx, opts)
	if err != nil {
		return Result{Kind: KindResult, Status: StatusRefused}, err
	}
	result := Result{Kind: KindResult, PlanID: plan.PlanID, Digest: plan.Digest, Preserved: plan.Preserved, Notes: plan.Notes}
	if !yes {
		return Result{}, apperr.New(apperr.CodeInvalidInput, "uninstall apply requires --yes")
	}
	if !plan.CanApply {
		result.Status = StatusRefused
		result.Notes = append(result.Notes, plan.Refusal)
		return result, apperr.New(apperr.CodeConflict, plan.Refusal)
	}
	if plan.Digest != planDigest(plan) {
		return result, apperr.New(apperr.CodeConflict, "uninstall plan digest mismatch")
	}

	stateDir, err := resolveStateDir(opts)
	if err != nil {
		return result, err
	}
	receipt, err := LoadReceipt(stateDir)
	if err != nil {
		return result, err
	}
	if err := persistReboundManifest(stateDir, receipt, nowOf(opts)); err != nil {
		result.Notes = append(result.Notes, "could not persist rebound uninstall manifest")
	}
	run := runnerFor(opts)

	var stops, rest []PlanAction
	for _, action := range plan.Actions {
		if !action.Verified {
			result.Skipped = append(result.Skipped, action.ID+": not verified")
			continue
		}
		if action.Kind == KindStopService {
			stops = append(stops, action)
			continue
		}
		rest = append(rest, action)
	}
	for _, action := range stops {
		if err := stopOwnedService(ctx, opts, run, receipt, action); err != nil {
			result.Failed = append(result.Failed, action.ID+": "+err.Error())
			result.Status = StatusRefused
			result.Notes = append(result.Notes, "service stop failed; leaving binary and unit in place for retry")
			return result, apperr.New(apperr.CodeConflict, "uninstall apply failed: "+err.Error())
		}
		result.Applied = append(result.Applied, action.ID+":stopped")
	}
	for _, action := range stops {
		removed, missing, err := applyRemoveFile(action.Path)
		if err != nil {
			result.Failed = append(result.Failed, action.ID+": "+err.Error())
			result.Status = StatusRefused
			result.Notes = append(result.Notes, "unit remove failed; leaving binary in place for retry")
			return result, err
		}
		if action.PairPath != "" {
			if _, _, pairErr := applyRemoveFile(action.PairPath); pairErr != nil {
				result.Failed = append(result.Failed, action.ID+": "+pairErr.Error())
				result.Status = StatusRefused
				return result, pairErr
			}
		}
		if missing {
			result.Skipped = append(result.Skipped, action.ID+": already removed")
		} else if removed {
			result.Applied = append(result.Applied, action.ID)
		}
	}
	for _, action := range rest {
		applied, skip, applyErr := applyPlanAction(ctx, opts, run, receipt, action)
		switch {
		case applyErr != nil:
			result.Failed = append(result.Failed, action.ID+": "+applyErr.Error())
			result.Status = StatusRefused
			return result, apperr.New(apperr.CodeConflict, "uninstall apply failed: "+applyErr.Error())
		case skip:
			result.Skipped = append(result.Skipped, action.ID+": already removed")
		case applied:
			result.Applied = append(result.Applied, action.ID)
		default:
			result.Skipped = append(result.Skipped, action.ID)
		}
	}
	if len(result.Failed) > 0 {
		result.Status = StatusRefused
		return result, apperr.New(apperr.CodeConflict, "uninstall apply failed: "+result.Failed[0])
	}
	if receipt.packageManager() == MethodHomebrew {
		if _, err := os.Lstat(receipt.BinaryPath); err == nil {
			result.Status = StatusSoftwareStillInstalled
			result.Notes = append(result.Notes, "software still installed")
			return result, nil
		}
	}
	result.Status = StatusApplied
	if len(result.Applied) == 0 {
		result.Status = StatusAlreadyUninstalled
	}
	return result, nil
}

func applyPlanAction(ctx context.Context, opts Options, run CommandRunner, receipt Receipt, action PlanAction) (bool, bool, error) {
	_ = ctx
	_ = opts
	_ = run
	switch action.Kind {
	case KindPackageManager:
		return false, true, nil
	case KindRemoveFile:
		if action.ID == "remove-executable" {
			if err := verifyExecutable(action.Path, receipt.BinarySHA256); err != nil {
				if os.IsNotExist(err) {
					return false, true, nil
				}
				return false, false, err
			}
		}
		return applyRemoveFile(action.Path)
	case KindRemoveManagedBlock:
		marker := action.Marker
		if marker == "" {
			marker = action.Detail
		}
		changed, err := removeMarkdownBlock(action.Path, marker)
		return changed, !changed && err == nil, err
	case KindRemoveConfigEntry:
		return applyConfigEntry(action, receipt)
	default:
		return false, false, apperr.New(apperr.CodeConflict, "unknown plan action")
	}
}

func stopOwnedService(ctx context.Context, opts Options, run CommandRunner, receipt Receipt, action PlanAction) error {
	if err := requireOwnedRegularPath(action.Path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	binary := receipt.BinaryPath
	if strings.HasSuffix(action.Path, ".timer") {
		service := pairedServicePath(action.Path)
		if !atlasOwnedUnit(service, binary) || !timerHasMarker(action.Path) {
			return apperr.New(apperr.CodeConflict, "timer is not paired with a verified Atlas service")
		}
	} else if !atlasOwnedUnit(action.Path, binary) {
		return apperr.New(apperr.CodeConflict, "unit file is not Atlas-owned or does not name the receipt binary")
	}
	goos := goosOf(opts)
	if goos == "darwin" {
		name, args, err := launchctlBootoutArgv(action.Path)
		if err != nil {
			return err
		}
		return run.Run(ctx, name, args...)
	}
	if action.PairPath != "" {
		if !timerHasMarker(action.PairPath) {
			return apperr.New(apperr.CodeConflict, "paired timer is not Atlas-owned")
		}
		if err := runSystemd(ctx, run, "stop", filepath.Base(action.PairPath)); err != nil {
			return err
		}
		if err := runSystemd(ctx, run, "disable", filepath.Base(action.PairPath)); err != nil {
			return err
		}
	}
	unit := filepath.Base(action.Path)
	if err := runSystemd(ctx, run, "stop", unit); err != nil {
		return err
	}
	return runSystemd(ctx, run, "disable", unit)
}

func runSystemd(ctx context.Context, run CommandRunner, action, unit string) error {
	name, args, err := systemdArgv(action, unit)
	if err != nil {
		return err
	}
	return run.Run(ctx, name, args...)
}

func applyRemoveFile(path string) (bool, bool, error) {
	if path == "" {
		return false, true, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, true, nil
		}
		return false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return false, false, apperr.New(apperr.CodeConflict, "refusing to remove a symlink or directory")
	}
	if err := os.Remove(path); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func applyConfigEntry(action PlanAction, receipt Receipt) (bool, bool, error) {
	key := action.EntryKey
	format := action.ConfigFormat
	if key == "" || format == "" {
		key, format = splitConfigDetail(action)
	}
	switch format {
	case "json":
		changed, err := removeJSONEntry(action.Path, key, receipt.BinaryPath, action.Args)
		return changed, !changed && err == nil, err
	case "toml":
		changed, err := removeTOMLEntry(action.Path, key, receipt.BinaryPath, action.Args)
		return changed, !changed && err == nil, err
	default:
		return false, false, apperr.New(apperr.CodeConflict, "unknown config format")
	}
}

func verifyActionLive(action ManifestAction, receipt Receipt) error {
	switch action.Kind {
	case KindStopService, KindRemoveFile:
		if action.Kind == KindRemoveFile && action.ID == "remove-executable" {
			return nil
		}
		if action.Path == "" {
			return apperr.New(apperr.CodeNotFound, "unit file is not present")
		}
		if requireOwnedRegularPath(action.Path) != nil || !atlasOwnedUnit(action.Path, receipt.BinaryPath) {
			return apperr.New(apperr.CodeConflict, "unit file is not Atlas-owned or does not name the receipt binary")
		}
	case KindRemoveManagedBlock:
		if err := requireOwnedRegularPath(action.Path); err != nil {
			return err
		}
		raw, err := os.ReadFile(action.Path)
		if err != nil {
			return err
		}
		begin := markerBegin(action.Marker)
		if !strings.Contains(string(raw), begin) {
			return apperr.New(apperr.CodeNotFound, "managed block is not present")
		}
	case KindRemoveConfigEntry:
		if err := requireOwnedRegularPath(action.Path); err != nil {
			return err
		}
		raw, err := os.ReadFile(action.Path)
		if err != nil {
			return err
		}
		switch action.ConfigFormat {
		case "json":
			owned, err := atlasJSONEntry(raw, action.EntryKey, receipt.BinaryPath, action.Args)
			if err != nil {
				return err
			}
			if !owned {
				return apperr.New(apperr.CodeNotFound, "Atlas JSON entry is not present")
			}
		case "toml":
			owned, err := atlasTOMLEntry(raw, action.EntryKey, receipt.BinaryPath, action.Args)
			if err != nil {
				return err
			}
			if !owned {
				return apperr.New(apperr.CodeNotFound, "Atlas TOML entry is not present")
			}
		}
	}
	return nil
}

func persistReboundManifest(stateDir string, receipt Receipt, now time.Time) error {
	manifest, err := LoadManifest(stateDir)
	if err != nil || manifest.Format == "" || manifest.ReceiptDigest == receipt.Digest {
		return nil
	}
	rebound, err := RebindManifest(manifest, receipt, now)
	if err != nil {
		return nil
	}
	return WriteManifest(stateDir, rebound)
}

func mergeActions(groups ...[]ManifestAction) []ManifestAction {
	seen := map[string]struct{}{}
	var out []ManifestAction
	for _, group := range groups {
		for _, action := range group {
			key := action.Kind + "|" + action.Path + "|" + action.UnitName + "|" + action.EntryKey + "|" + action.Marker
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, action)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func toPlanAction(action ManifestAction) PlanAction {
	return PlanAction{
		ID: action.ID, Kind: action.Kind, Path: action.Path, PairPath: action.PairPath,
		Detail: detailFor(action), Verified: true, UnitName: action.UnitName, Marker: action.Marker,
		EntryKey: action.EntryKey, ConfigFormat: action.ConfigFormat, BinaryPath: action.BinaryPath,
		Args: append([]string(nil), action.Args...),
	}
}

func detailFor(action ManifestAction) string {
	switch action.Kind {
	case KindRemoveConfigEntry:
		return action.ConfigFormat + ":" + action.EntryKey
	case KindRemoveManagedBlock:
		return action.Marker
	case KindStopService:
		return action.UnitName
	default:
		return action.Label
	}
}

func markerFromDetail(action PlanAction) string {
	if action.Kind == KindRemoveManagedBlock {
		return action.Detail
	}
	return ""
}

func splitConfigDetail(action PlanAction) (key, format string) {
	parts := strings.SplitN(action.Detail, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[1], parts[0]
}

func verifiedActions(plan Plan) []PlanAction {
	var out []PlanAction
	for _, action := range plan.Actions {
		if action.Verified {
			out = append(out, action)
		}
	}
	return out
}

func planDigest(plan Plan) string {
	type row struct {
		ID, Kind, Path, Pair, Detail, Unit, Marker, Entry, Format, Binary string
		Args                                                              []string
	}
	rows := make([]row, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		if !action.Verified {
			continue
		}
		rows = append(rows, row{
			ID: action.ID, Kind: action.Kind, Path: action.Path, Pair: action.PairPath,
			Detail: action.Detail, Unit: action.UnitName, Marker: action.Marker,
			Entry: action.EntryKey, Format: action.ConfigFormat, Binary: action.BinaryPath,
			Args: append([]string(nil), action.Args...),
		})
	}
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func preservedList(stateDir string) []string {
	return []string{
		filepath.Join(stateDir, "registry.json"),
		filepath.Join(stateDir, "settings.json"),
		filepath.Join(stateDir, "backups"),
		filepath.Join(stateDir, "workspaces"),
		"workspace projects/.tracker tickets events templates",
		"unrelated client config entries",
	}
}

func Pretty(plan Plan) string {
	if plan.Refusal != "" && !plan.CanApply {
		return "uninstall preview: refused — " + plan.Refusal
	}
	var b strings.Builder
	b.WriteString("uninstall preview")
	if plan.CanApply {
		b.WriteString(" (pass --yes to apply)")
	}
	b.WriteByte('\n')
	for _, action := range plan.Actions {
		b.WriteString("- ")
		b.WriteString(action.Kind)
		if action.Path != "" {
			b.WriteString(" ")
			b.WriteString(action.Path)
		}
		if action.Detail != "" {
			b.WriteString(" [")
			b.WriteString(action.Detail)
			b.WriteString("]")
		}
		b.WriteByte('\n')
	}
	b.WriteString("preserved: registry, backups, workspaces, unrelated client config\n")
	return strings.TrimRight(b.String(), "\n")
}

func PrettyResult(result Result) string {
	return "uninstall " + result.Status + " applied=" + strconv.Itoa(len(result.Applied)) + " skipped=" + strconv.Itoa(len(result.Skipped))
}
