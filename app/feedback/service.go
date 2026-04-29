package feedback

import (
	"context"
	"fmt"
	"log"
)

type SampleAdder interface {
	AddSpamSample(ctx context.Context, text string) error
	AddHamSample(ctx context.Context, text string) error
}

type SpamTextProvider interface {
	GetSpamText(ctx context.Context, spamID int64) (string, error)
}

type Service struct {
	store   LabelStore
	samples SampleAdder
	spamTxt SpamTextProvider
}

func NewService(store LabelStore, samples SampleAdder, spamTxt SpamTextProvider) *Service {
	return &Service{store: store, samples: samples, spamTxt: spamTxt}
}

func (s *Service) Label(ctx context.Context, entry LabelEntry) (LabelEntry, error) {
	created, err := s.store.Create(ctx, entry)
	if err != nil {
		return LabelEntry{}, fmt.Errorf("create label: %w", err)
	}

	s.applySideEffects(ctx, created)
	return created, nil
}

func (s *Service) GetByDetectedSpamID(ctx context.Context, spamID int64) ([]LabelEntry, error) {
	return s.store.GetByDetectedSpamID(ctx, spamID)
}

func (s *Service) GetByIncidentID(ctx context.Context, incidentID int64) ([]LabelEntry, error) {
	return s.store.GetByIncidentID(ctx, incidentID)
}

func (s *Service) List(ctx context.Context, filter LabelFilter) ([]LabelEntry, error) {
	return s.store.List(ctx, filter)
}

func (s *Service) Stats(ctx context.Context) (LabelStats, error) {
	raw, err := s.store.Stats(ctx)
	if err != nil {
		return LabelStats{}, fmt.Errorf("get label stats: %w", err)
	}
	stats := LabelStats{
		Confirmed:     raw[LabelConfirmedSpam],
		FalsePositive: raw[LabelFalsePositive],
		Missed:        raw[LabelMissedSpam],
		Override:      raw[LabelPolicyOverride],
	}
	for _, v := range raw {
		stats.Total += v
	}
	return stats, nil
}

func (s *Service) AutoLabel(ctx context.Context, incidentID int64, label string) error {
	_, err := s.Label(ctx, LabelEntry{
		IncidentID: incidentID,
		Label:      Label(label),
		LabeledBy:  "appeal_system",
	})
	return err
}

func (s *Service) applySideEffects(ctx context.Context, entry LabelEntry) {
	if s.samples == nil || s.spamTxt == nil {
		return
	}
	if entry.DetectedSpamID <= 0 {
		return
	}

	text, err := s.spamTxt.GetSpamText(ctx, entry.DetectedSpamID)
	if err != nil || text == "" {
		return
	}

	switch entry.Label {
	case LabelConfirmedSpam:
		if addErr := s.samples.AddSpamSample(ctx, text); addErr != nil {
			log.Printf("[WARN] failed to add spam sample on label: %v", addErr)
		}
	case LabelFalsePositive:
		if addErr := s.samples.AddHamSample(ctx, text); addErr != nil {
			log.Printf("[WARN] failed to add ham sample on label: %v", addErr)
		}
	}
}
