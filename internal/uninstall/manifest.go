package uninstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

var (
	atlasServerNameRe = regexp.MustCompile(`^atlas-[0-9a-f]{12}$`)
	unitIDRe          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

var allowedMarkdownMarkers = [][2]string{
	{"<!-- atlas-tasker:begin -->", "<!-- atlas-tasker:end -->"},
	{"<!-- atlas-tasker:openclaw:begin -->", "<!-- atlas-tasker:openclaw:end -->"},
	{"<!-- atlas-tasker:generic:begin -->", "<!-- atlas-tasker:generic:end -->"},
	{"<!-- atlas-tasker:cursor:begin -->", "<!-- atlas-tasker:cursor:end -->"},
	{"<!-- atlas-tasker:grok:begin -->", "<!-- atlas-tasker:grok:end -->"},
}

// NewManifest is the type-safe producer Core calls after it has created
// daemon units or managed client entries. Recovery validates allowlists,
// binds the digest to the current receipt, and writes nothing until
// WriteManifest.
func NewManifest(receipt Receipt, actions []ManifestAction, now time.Time) (Manifest, error) {
	if err := receipt.Validate(); err != nil {
		return Manifest{}, err
	}
	cleaned := make([]ManifestAction, 0, len(actions))
	for _, action := range actions {
		if err := validateManifestAction(action, receipt); err != nil {
			return Manifest{}, err
		}
		cleaned = append(cleaned, action)
	}
	sort.SliceStable(cleaned, func(i, j int) bool { return cleaned[i].ID < cleaned[j].ID })
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m := Manifest{
		Format:        ManifestFormat,
		GeneratedAt:   now.UTC(),
		ReceiptDigest: receipt.Digest,
		Actions:       cleaned,
	}
	m.Digest = ManifestDigest(m)
	return m, nil
}

func WriteManifest(stateDir string, manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	path := ManifestPath(stateDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadManifest(stateDir string) (Manifest, error) {
	raw, err := os.ReadFile(ManifestPath(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, nil
		}
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, apperr.New(apperr.CodeConflict, "uninstall manifest is not valid JSON")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.Format == "" && len(m.Actions) == 0 && m.Digest == "" {
		return nil
	}
	if m.Format != ManifestFormat {
		return apperr.New(apperr.CodeConflict, "unsupported uninstall manifest format")
	}
	if m.Digest != ManifestDigest(m) {
		return apperr.New(apperr.CodeConflict, "uninstall manifest digest does not match")
	}
	seen := map[string]struct{}{}
	for _, action := range m.Actions {
		if strings.TrimSpace(action.ID) == "" {
			return apperr.New(apperr.CodeConflict, "uninstall manifest action is missing an id")
		}
		if _, dup := seen[action.ID]; dup {
			return apperr.New(apperr.CodeConflict, "uninstall manifest has a duplicate action id")
		}
		seen[action.ID] = struct{}{}
	}
	return nil
}

func ManifestDigest(m Manifest) string {
	type row struct {
		ID, Kind, Unit, Path, Marker, Entry, Format, Binary, SHA, Pair string
		Args                                                           []string
	}
	rows := make([]row, 0, len(m.Actions))
	for _, action := range m.Actions {
		args := append([]string(nil), action.Args...)
		rows = append(rows, row{
			ID: action.ID, Kind: action.Kind, Unit: action.UnitName, Path: action.Path,
			Marker: action.Marker, Entry: action.EntryKey, Format: action.ConfigFormat,
			Binary: action.BinaryPath, SHA: action.BinarySHA256, Pair: action.PairPath, Args: args,
		})
	}
	raw, _ := json.Marshal(struct {
		Format, Receipt string
		Actions         []row
	}{Format: ManifestFormat, Receipt: m.ReceiptDigest, Actions: rows})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateManifestAction(action ManifestAction, receipt Receipt) error {
	if strings.TrimSpace(action.ID) == "" {
		return apperr.New(apperr.CodeInvalidInput, "uninstall action id is required")
	}
	switch action.Kind {
	case KindStopService, KindRemoveFile:
		if action.Kind == KindStopService && !allowedUnitName(action.UnitName) {
			return apperr.New(apperr.CodeConflict, "service unit name is not on the Atlas allowlist")
		}
		if action.Path != "" {
			if err := requireLexicalPath(action.Path); err != nil {
				return err
			}
			if action.Path != receipt.BinaryPath {
				base := filepath.Base(action.Path)
				if !allowedUnitName(base) && !allowedUnitName(strings.TrimSuffix(base, ".plist")) {
					return apperr.New(apperr.CodeConflict, "remove_file is limited to the receipt binary or an allowlisted Atlas unit")
				}
			}
		}
		if action.BinaryPath != "" && action.BinaryPath != receipt.BinaryPath {
			return apperr.New(apperr.CodeConflict, "manifest binary path does not match the install receipt")
		}
	case KindRemoveManagedBlock:
		if err := requireLexicalPath(action.Path); err != nil {
			return err
		}
		if !allowedMarkerPair(action.Marker, "") && markerBegin(action.Marker) == "" {
			return apperr.New(apperr.CodeConflict, "managed block marker is not an Atlas marker")
		}
	case KindRemoveConfigEntry:
		if err := requireLexicalPath(action.Path); err != nil {
			return err
		}
		if action.ConfigFormat != "json" && action.ConfigFormat != "toml" {
			return apperr.New(apperr.CodeConflict, "config entry format must be json or toml")
		}
		if !allowedServerKey(action.EntryKey) {
			return apperr.New(apperr.CodeConflict, "config entry key is not an Atlas MCP server name")
		}
	case KindPackageManager:
		// instruction only
	default:
		return apperr.New(apperr.CodeConflict, "unknown uninstall action kind")
	}
	return nil
}

func requireLexicalPath(path string) error {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) || strings.Contains(path, "\x00") {
		return apperr.New(apperr.CodeConflict, "uninstall path must be a clean absolute path")
	}
	if path == "/" || strings.HasSuffix(path, "/") {
		return apperr.New(apperr.CodeConflict, "refusing to operate on a directory root")
	}
	return nil
}

func requireOwnedRegularPath(path string) error {
	if err := requireLexicalPath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return apperr.New(apperr.CodeConflict, "refusing to operate on a symlink")
	}
	if info.IsDir() {
		return apperr.New(apperr.CodeConflict, "refusing to operate on a directory")
	}
	return nil
}

func allowedServerKey(key string) bool {
	return key == canonicalGlobalKey || atlasServerNameRe.MatchString(key)
}

// RebindManifest regenerates a verified manifest against a new receipt so an
// installer upgrade cannot block uninstall forever.
func RebindManifest(old Manifest, receipt Receipt, now time.Time) (Manifest, error) {
	if old.Format != ManifestFormat {
		return Manifest{}, apperr.New(apperr.CodeConflict, "uninstall manifest is not reboundable")
	}
	if old.Digest != ManifestDigest(old) {
		return Manifest{}, apperr.New(apperr.CodeConflict, "uninstall manifest is tampered")
	}
	var kept []ManifestAction
	for _, action := range old.Actions {
		if action.BinaryPath != "" && action.BinaryPath != receipt.BinaryPath {
			continue
		}
		action.BinaryPath = receipt.BinaryPath
		action.BinarySHA256 = receipt.BinarySHA256
		if err := validateManifestAction(action, receipt); err != nil {
			continue
		}
		kept = append(kept, action)
	}
	return NewManifest(receipt, kept, now)
}

func markerBegin(marker string) string {
	marker = strings.TrimSpace(marker)
	for _, pair := range allowedMarkdownMarkers {
		if marker == pair[0] || marker == pair[1] {
			return pair[0]
		}
	}
	return ""
}

func markerEnd(begin string) string {
	for _, pair := range allowedMarkdownMarkers {
		if begin == pair[0] {
			return pair[1]
		}
	}
	return ""
}

func allowedMarkerPair(begin, end string) bool {
	begin = strings.TrimSpace(begin)
	if begin == "" {
		return false
	}
	if end == "" {
		end = markerEnd(markerBegin(begin))
	}
	for _, pair := range allowedMarkdownMarkers {
		if begin == pair[0] && end == pair[1] {
			return true
		}
	}
	return false
}

// NewHomeServiceAction is a producer helper for Core's loopback Home unit.
func NewHomeServiceAction(home, goos, binaryPath, binarySHA string) (ManifestAction, error) {
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		return ManifestAction{}, apperr.New(apperr.CodeInvalidInput, "home must be absolute")
	}
	action := ManifestAction{
		ID: "stop-home-service", Kind: KindStopService, Label: "Atlas Home loopback service",
		BinaryPath: binaryPath, BinarySHA256: binarySHA, Marker: homeServiceMarker,
	}
	if goos == "darwin" {
		action.UnitName = homeLaunchdLabel
		action.Path = filepath.Join(home, "Library", "LaunchAgents", homeLaunchdLabel+".plist")
	} else {
		action.UnitName = homeSystemdService
		action.Path = filepath.Join(home, ".config", "systemd", "user", homeSystemdService)
	}
	return action, nil
}

func NewManagedJSONAction(id, path, entryKey, binaryPath string, args []string) ManifestAction {
	return ManifestAction{
		ID: id, Kind: KindRemoveConfigEntry, Path: path, EntryKey: entryKey,
		ConfigFormat: "json", BinaryPath: binaryPath, Args: append([]string(nil), args...),
	}
}

func NewManagedTOMLAction(id, path, entryKey, binaryPath string, args []string) ManifestAction {
	return ManifestAction{
		ID: id, Kind: KindRemoveConfigEntry, Path: path, EntryKey: entryKey,
		ConfigFormat: "toml", BinaryPath: binaryPath, Args: append([]string(nil), args...),
	}
}

func NewManagedBlockAction(id, path, beginMarker string) ManifestAction {
	return ManifestAction{ID: id, Kind: KindRemoveManagedBlock, Path: path, Marker: beginMarker, ConfigFormat: "markdown"}
}
