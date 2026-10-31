package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReleaseChecksumsBareNames(t *testing.T) {
	parent := t.TempDir()
	dist := filepath.Join(parent, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	archiveName := "tracker_1.18.2_linux_amd64.tar.gz"
	sbomName := "sbom-v1.18.2.cdx.json"
	archiveBody := []byte("release-archive")
	sbomBody := []byte("{\"bomFormat\":\"CycloneDX\"}\n")
	if err := os.WriteFile(filepath.Join(dist, archiveName), archiveBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, sbomName), sbomBody, 0o644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join("..", "..", "scripts", "write-release-checksums.sh")
	cmd := exec.Command("sh", script, dist)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("write-release-checksums.sh: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "dist/") {
		t.Fatalf("checksums.txt still has a dist/ prefix:\n%s", raw)
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("checksum line %q must be \"<sha256>  <bare filename>\"", line)
		}
		if strings.ContainsAny(fields[1], `/\`) {
			t.Fatalf("asset name %q is not a bare filename", fields[1])
		}
		got[fields[1]] = fields[0]
	}
	want := map[string]string{
		archiveName: hex.EncodeToString(sum256(archiveBody)),
		sbomName:    hex.EncodeToString(sum256(sbomBody)),
	}
	if len(got) != len(want) {
		t.Fatalf("checksum entries = %#v, want %#v", got, want)
	}
	for name, sum := range want {
		if got[name] != sum {
			t.Fatalf("checksum for %s = %s, want %s", name, got[name], sum)
		}
	}

	checkReleaseChecksums(t, dist)
}

func TestReleaseWorkflowWritesBareChecksums(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	if !strings.Contains(text, "scripts/write-release-checksums.sh") {
		t.Fatal("release workflow must build checksums.txt with scripts/write-release-checksums.sh")
	}
	if strings.Contains(text, `shasum -a 256 "dist/`) || strings.Contains(text, "dist/*.sha256") {
		t.Fatal("release workflow still records dist/ paths in checksums.txt")
	}
	rehearsal, err := os.ReadFile(filepath.Join(root, "scripts", "release-rehearsal.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rehearsal), "write-release-checksums.sh") {
		t.Fatal("release rehearsal must use scripts/write-release-checksums.sh")
	}
}

func TestVerifyChecksumAcceptsBareAndDistPrefix(t *testing.T) {
	dir := t.TempDir()
	name := "tracker_1.18.1_linux_amd64.tar.gz"
	body := []byte("published-archive")
	archive := filepath.Join(dir, name)
	if err := os.WriteFile(archive, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := hex.EncodeToString(sum256(body))
	checksums := filepath.Join(dir, "checksums.txt")

	for _, line := range []string{
		fmt.Sprintf("%s  %s\n", sum, name),
		fmt.Sprintf("%s  dist/%s\n", sum, name),
	} {
		if err := os.WriteFile(checksums, []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := verifyChecksum(archive, name, checksums); err != nil {
			t.Fatalf("verifyChecksum(%q): %v", strings.TrimSpace(line), err)
		}
	}

	bad := fmt.Sprintf("%s  dist/%s\n", strings.Repeat("0", 64), name)
	if err := os.WriteFile(checksums, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	err := verifyChecksum(archive, name, checksums)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch for dist/ line, got %v", err)
	}
}

func TestVerifyReleaseScriptAcceptsBareAndDistPrefix(t *testing.T) {
	name := "tracker_1.18.1_linux_amd64.tar.gz"
	body := []byte("verify-release-archive")
	sum := hex.EncodeToString(sum256(body))
	for _, prefix := range []string{"", "dist/"} {
		t.Run(prefix+"name", func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, name)
			if err := os.WriteFile(archive, body, 0o644); err != nil {
				t.Fatal(err)
			}
			line := fmt.Sprintf("%s  %s%s\n", sum, prefix, name)
			mux := http.NewServeMux()
			mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(line))
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			cmd := exec.Command("sh", filepath.Join("..", "..", "scripts", "verify-release.sh"), archive)
			cmd.Env = append(os.Environ(),
				"VERSION=v1.18.1",
				"RELEASE_BASE_URL="+srv.URL,
				"VERIFY_ATTESTATIONS=0",
				"ALLOW_INSECURE_RELEASE_BASE_URL=1",
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("verify-release.sh prefix %q: %v\n%s", prefix, err, out)
			}
			if !strings.Contains(string(out), "verified "+name+" for v1.18.1") {
				t.Fatalf("unexpected verify-release output:\n%s", out)
			}
		})
	}
}

func checkReleaseChecksums(t *testing.T, dir string) {
	t.Helper()
	sha256sum, shaErr := exec.LookPath("sha256sum")
	shasum, shasumErr := exec.LookPath("shasum")
	if shaErr != nil && shasumErr != nil {
		t.Fatal("sha256sum or shasum is required")
	}
	var cmd *exec.Cmd
	if shaErr == nil {
		cmd = exec.Command(sha256sum, "-c", "checksums.txt")
	} else {
		cmd = exec.Command(shasum, "-a", "256", "-c", "checksums.txt")
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.Contains(line, ": OK") {
			t.Fatalf("checksum check line %q", line)
		}
	}
}

func sum256(body []byte) []byte {
	sum := sha256.Sum256(body)
	return sum[:]
}
