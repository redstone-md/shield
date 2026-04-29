package feedback

import (
	"time"
)

type CandidateType string

const (
	CandidateStopPhrase CandidateType = "stop_phrase"
	CandidateRegex      CandidateType = "regex"
)

type CandidateStatus string

const (
	CandidatePending  CandidateStatus = "pending"
	CandidateApproved CandidateStatus = "approved"
	CandidateRejected CandidateStatus = "rejected"
)

type CandidateEntry struct {
	ID         int64           `db:"id" json:"id"`
	GID        string          `db:"gid" json:"-"`
	TenantID   string          `db:"tenant_id" json:"-"`
	Type       CandidateType   `db:"type" json:"type"`
	Value      string          `db:"value" json:"value"`
	Source     string          `db:"source" json:"source"`
	SourceID   int64           `db:"source_id" json:"source_id"`
	Score      float64         `db:"score" json:"score"`
	Status     CandidateStatus `db:"status" json:"status"`
	ReviewedBy string          `db:"reviewed_by" json:"reviewed_by"`
	Comment    string          `db:"review_comment" json:"comment"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	ReviewedAt *time.Time      `db:"reviewed_at" json:"-"`
}

type CandidateFilter struct {
	Status   CandidateStatus
	Type     CandidateType
	Source   string
	SourceID int64
	Limit    int
	Offset   int
}
