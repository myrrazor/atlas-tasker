package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type AuditReportStore struct {
	Root string
}

type AuditPacketStore struct {
	Root string
}

func (s AuditReportStore) SaveAuditReport(_ context.Context, report contracts.AuditReport) error {
	report = normalizeAuditReport(report)
	if err := report.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.AuditReportsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create audit reports dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode audit report %s: %w", report.AuditReportID, err)
	}
	return os.WriteFile(auditReportPath(s.Root, report.AuditReportID), append(raw, '\n'), 0o644)
}

func (s AuditReportStore) LoadAuditReport(_ context.Context, reportID string) (contracts.AuditReport, error) {
	raw, err := os.ReadFile(auditReportPath(s.Root, reportID))
	if err != nil {
		return contracts.AuditReport{}, fmt.Errorf("read audit report %s: %w", reportID, err)
	}
	return decodeAuditReport(raw, reportID)
}

func (s AuditReportStore) ListAuditReports(ctx context.Context) ([]contracts.AuditReport, error) {
	entries, err := os.ReadDir(storage.AuditReportsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.AuditReport{}, nil
		}
		return nil, fmt.Errorf("read audit reports dir: %w", err)
	}
	items := make([]contracts.AuditReport, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		report, err := s.LoadAuditReport(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, report)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].AuditReportID < items[j].AuditReportID
		}
		return items[i].GeneratedAt.Before(items[j].GeneratedAt)
	})
	return items, nil
}

func (s AuditPacketStore) SaveAuditPacket(_ context.Context, packet contracts.AuditPacket) error {
	packet = normalizeAuditPacket(packet)
	if err := packet.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.AuditPacketsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create audit packets dir: %w", err)
	}
	raw, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return fmt.Errorf("encode audit packet %s: %w", packet.PacketID, err)
	}
	return os.WriteFile(auditPacketPath(s.Root, packet.PacketID), append(raw, '\n'), 0o644)
}

func (s AuditPacketStore) LoadAuditPacket(_ context.Context, packetID string) (contracts.AuditPacket, error) {
	raw, err := os.ReadFile(auditPacketPath(s.Root, packetID))
	if err != nil {
		return contracts.AuditPacket{}, fmt.Errorf("read audit packet %s: %w", packetID, err)
	}
	return decodeAuditPacket(raw, packetID)
}

func (s AuditPacketStore) ListAuditPackets(ctx context.Context) ([]contracts.AuditPacket, error) {
	entries, err := os.ReadDir(storage.AuditPacketsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.AuditPacket{}, nil
		}
		return nil, fmt.Errorf("read audit packets dir: %w", err)
	}
	items := make([]contracts.AuditPacket, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		packet, err := s.LoadAuditPacket(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, packet)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Report.GeneratedAt.Equal(items[j].Report.GeneratedAt) {
			return items[i].PacketID < items[j].PacketID
		}
		return items[i].Report.GeneratedAt.Before(items[j].Report.GeneratedAt)
	})
	return items, nil
}

func decodeAuditReport(raw []byte, hint string) (contracts.AuditReport, error) {
	var report contracts.AuditReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return contracts.AuditReport{}, fmt.Errorf("decode audit report %s: %w", hint, err)
	}
	report = normalizeAuditReport(report)
	return report, report.Validate()
}

func decodeAuditPacket(raw []byte, hint string) (contracts.AuditPacket, error) {
	var packet contracts.AuditPacket
	if err := json.Unmarshal(raw, &packet); err != nil {
		return contracts.AuditPacket{}, fmt.Errorf("decode audit packet %s: %w", hint, err)
	}
	packet = normalizeAuditPacket(packet)
	return packet, packet.Validate()
}

func normalizeAuditReport(report contracts.AuditReport) contracts.AuditReport {
	report.AuditReportID = sanitizeSecurityID(report.AuditReportID)
	if report.SchemaVersion == 0 {
		report.SchemaVersion = contracts.CurrentSchemaVersion
	}
	sort.Slice(report.IncludedArtifactHashes, func(i, j int) bool {
		if report.IncludedArtifactHashes[i].Kind != report.IncludedArtifactHashes[j].Kind {
			return report.IncludedArtifactHashes[i].Kind < report.IncludedArtifactHashes[j].Kind
		}
		return report.IncludedArtifactHashes[i].UID < report.IncludedArtifactHashes[j].UID
	})
	sort.Slice(report.Findings, func(i, j int) bool { return report.Findings[i].FindingID < report.Findings[j].FindingID })
	return report
}

func normalizeAuditPacket(packet contracts.AuditPacket) contracts.AuditPacket {
	packet.PacketID = sanitizeSecurityID(packet.PacketID)
	packet.Report = normalizeAuditReport(packet.Report)
	packet.Report.SignatureEnvelopes = nil
	if packet.Canonicalization == "" {
		packet.Canonicalization = contracts.CanonicalizationAtlasV1
	}
	if packet.SchemaVersion == 0 {
		packet.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return packet
}

func auditReportPath(root string, reportID string) string {
	return filepath.Join(storage.AuditReportsDir(root), sanitizeSecurityID(reportID)+".json")
}

func auditPacketPath(root string, packetID string) string {
	return filepath.Join(storage.AuditPacketsDir(root), sanitizeSecurityID(packetID)+".json")
}
