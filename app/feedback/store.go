package feedback

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("not found")

type CandidateStore interface {
	Create(ctx context.Context, entry CandidateEntry) (CandidateEntry, error)
	GetByID(ctx context.Context, id int64) (CandidateEntry, error)
	List(ctx context.Context, filter CandidateFilter) ([]CandidateEntry, error)
	UpdateStatus(ctx context.Context, id int64, status CandidateStatus, reviewedBy, comment string) error
}

type LabelStore interface {
	Create(ctx context.Context, entry LabelEntry) (LabelEntry, error)
	GetByID(ctx context.Context, id int64) (LabelEntry, error)
	GetByDetectedSpamID(ctx context.Context, spamID int64) ([]LabelEntry, error)
	GetByIncidentID(ctx context.Context, incidentID int64) ([]LabelEntry, error)
	List(ctx context.Context, filter LabelFilter) ([]LabelEntry, error)
	Stats(ctx context.Context) (map[Label]int, error)
}
