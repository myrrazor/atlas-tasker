package contracts

import "testing"

func TestValidateBackupTargetURLRejectsCredentials(t *testing.T) {
	t.Parallel()
	bad := []string{
		"https://user:token@github.com/org/repo.git",
		"https://user@github.com/org/repo.git",
		"ssh://git:secret@host/repo.git",
		"https://example.com",
		"ftp://host/repo.git",
		"origin",
	}
	for _, url := range bad {
		if err := ValidateBackupTargetURL(url); err == nil {
			t.Fatalf("expected rejection for %q", url)
		}
	}
	good := []string{
		"https://github.com/org/private.git",
		"ssh://git@github.com/org/private.git",
		"git@github.com:org/private.git",
		"file:///tmp/disposable.git",
	}
	for _, url := range good {
		if err := ValidateBackupTargetURL(url); err != nil {
			t.Fatalf("accepted URL rejected: %q: %v", url, err)
		}
	}
}

func TestPublicGitHubRequiresOverride(t *testing.T) {
	t.Parallel()
	target := BackupTarget{
		Format:                BackupTargetFormat,
		TargetID:              "t1",
		Type:                  BackupTargetTypeGit,
		URL:                   "https://github.com/org/repo.git",
		DataBoundaryAck:       true,
		VisibilityAttestation: BackupVisibilityPub,
	}
	if err := target.Validate(); err == nil {
		t.Fatal("public GitHub without override must fail")
	}
	target.PublicOverrideApproved = true
	if err := target.Validate(); err != nil {
		t.Fatalf("approved public override should pass: %v", err)
	}
	private := target
	private.VisibilityAttestation = BackupVisibilityPriv
	private.PublicOverrideApproved = false
	if err := private.Validate(); err != nil {
		t.Fatalf("private attestation should pass: %v", err)
	}
}

func TestRedactBackupURLOmitsPath(t *testing.T) {
	t.Parallel()
	if got := RedactBackupURL("https://github.com/org/secret.git"); got != "https://github.com" {
		t.Fatalf("redacted https: %q", got)
	}
	if got := RedactBackupURL("git@github.com:org/secret.git"); got != "ssh://github.com" {
		t.Fatalf("redacted scp: %q", got)
	}
	if got := RedactBackupURL("file:///home/user/secret.git"); got != "file://local" {
		t.Fatalf("redacted file: %q", got)
	}
}
