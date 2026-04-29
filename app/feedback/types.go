package feedback

import "time"

type Label string

const (
	LabelConfirmedSpam  Label = "confirmed_spam"
	LabelFalsePositive  Label = "false_positive"
	LabelMissedSpam     Label = "missed_spam"
	LabelPolicyOverride Label = "policy_override"
)

type LabelEntry struct {
	ID             int64     `db:"id" json:"id"`
	GID            string    `db:"gid" json:"-"`
	TenantID       string    `db:"tenant_id" json:"-"`
	DetectedSpamID int64     `db:"detected_spam_id" json:"detected_spam_id"`
	IncidentID     int64     `db:"incident_id" json:"incident_id"`
	Label          Label     `db:"label" json:"label"`
	LabeledBy      string    `db:"labeled_by" json:"labeled_by"`
	Comment        string    `db:"comment" json:"comment"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}

type LabelFilter struct {
	Label     Label
	SpamID    int64
	LabeledBy string
	Limit     int
	Offset    int
}

type LabelStats struct {
	Confirmed     int `json:"confirmed"`
	FalsePositive int `json:"false_positive"`
	Missed        int `json:"missed"`
	Override      int `json:"override"`
	Total         int `json:"total"`
}
