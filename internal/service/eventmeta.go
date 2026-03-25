package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type eventMetaKey struct{}

type EventMetaContext struct {
	Surface          contracts.EventSurface
	CorrelationID    string
	CausationEventID int64
	BatchID          string
	RootActor        contracts.Actor
}

func WithEventMetadata(ctx context.Context, meta EventMetaContext) context.Context {
	current, _ := ctx.Value(eventMetaKey{}).(EventMetaContext)
	if meta.Surface == "" {
		meta.Surface = current.Surface
	}
	if meta.CorrelationID == "" {
		meta.CorrelationID = current.CorrelationID
	}
	if meta.CausationEventID == 0 {
		meta.CausationEventID = current.CausationEventID
	}
	if meta.BatchID == "" {
		meta.BatchID = current.BatchID
	}
	if meta.RootActor == "" {
		meta.RootActor = current.RootActor
	}
	return context.WithValue(ctx, eventMetaKey{}, meta)
}

func eventMetadataFromContext(ctx context.Context, actor contracts.Actor) contracts.EventMetadata {
	meta, _ := ctx.Value(eventMetaKey{}).(EventMetaContext)
	if meta.Surface == "" {
		meta.Surface = contracts.EventSurfaceCLI
	}
	mutationID := randomID()
	correlationID := meta.CorrelationID
	if correlationID == "" {
		correlationID = mutationID
	}
	rootActor := meta.RootActor
	if rootActor == "" {
		rootActor = actor
	}
	return contracts.EventMetadata{
		CorrelationID:    correlationID,
		CausationEventID: meta.CausationEventID,
		MutationID:       mutationID,
		Surface:          meta.Surface,
		BatchID:          meta.BatchID,
		RootActor:        rootActor,
	}
}

func randomID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "fallback-id"
	}
	return hex.EncodeToString(raw[:])
}
