package service

import (
	"context"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func (s *QueryService) EvidenceList(ctx context.Context, runID string) ([]contracts.EvidenceItem, error) {
	return s.Evidence.ListEvidence(ctx, runID)
}

func (s *QueryService) EvidenceDetail(ctx context.Context, evidenceID string) (contracts.EvidenceItem, error) {
	return s.Evidence.LoadEvidence(ctx, evidenceID)
}

func (s *QueryService) HandoffList(ctx context.Context, ticketID string) ([]contracts.HandoffPacket, error) {
	return s.Handoffs.ListHandoffs(ctx, ticketID)
}

func (s *QueryService) HandoffDetail(ctx context.Context, handoffID string) (contracts.HandoffPacket, error) {
	return s.Handoffs.LoadHandoff(ctx, handoffID)
}
