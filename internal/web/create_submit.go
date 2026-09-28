package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type createSubmitRecord struct {
	ID          string `json:"id"`
	Ticket      string `json:"ticket"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// createOnce returns the ticket created for this form submission. A reload
// that retries the same submit_id does not create a second ticket. An empty
// id (tests and non-browser clients) skips the record.
func createFingerprint(ticket contracts.TicketSnapshot) string {
	raw := strings.Join([]string{
		ticket.Project,
		ticket.Title,
		string(ticket.Type),
		string(ticket.Status),
		string(ticket.Priority),
		string(ticket.Assignee),
		string(ticket.Reviewer),
		strings.Join(ticket.Labels, ","),
		ticket.Description,
		strings.Join(ticket.AcceptanceCriteria, "\n"),
	}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

func (s *Server) createOnce(submitID, fingerprint string, create func() (string, error)) (string, error) {
	submitID = strings.TrimSpace(submitID)
	if submitID == "" || len(submitID) > 80 || strings.Trim(submitID, "0123456789abcdefABCDEF") != "" || s.cfg.Root == "" {
		return create()
	}
	dir := filepath.Join(s.cfg.Root, ".tracker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	lockFile, err := os.OpenFile(filepath.Join(dir, "web-create-submits.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return "", err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	path := filepath.Join(dir, "web-create-submits.json")
	var records []createSubmitRecord
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &records)
	}
	for _, record := range records {
		if record.ID == submitID && record.Ticket != "" {
			if record.Fingerprint != "" && fingerprint != "" && record.Fingerprint != fingerprint {
				return "", apperr.New(apperr.CodeConflict, "That create was already used for a different ticket. Submit again to create a new one.")
			}
			return record.Ticket, nil
		}
	}
	id, err := create()
	if err != nil || id == "" {
		return id, err
	}
	records = append(records, createSubmitRecord{ID: submitID, Ticket: id, Fingerprint: fingerprint})
	if len(records) > 200 {
		records = records[len(records)-200:]
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return id, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return id, nil
	}
	_ = os.Rename(tmp, path)
	return id, nil
}
