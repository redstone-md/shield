package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type KnowledgeData struct {
	StopPhrases   []string `json:"stop_phrases"`
	IgnoredWords  []string `json:"ignored_words"`
	SpamSamples   int      `json:"spam_samples"`
	HamSamples    int      `json:"ham_samples"`
	DictionaryVer int      `json:"dictionary_entries"`
	CreatedAt     string   `json:"created_at"`
}

type KnowledgeSnapshot struct {
	ID        int64         `db:"id" json:"id"`
	GID       string        `db:"gid" json:"-"`
	TenantID  string        `db:"tenant_id" json:"-"`
	Version   int           `db:"version" json:"version"`
	Data      KnowledgeData `db:"-" json:"data"`
	DataJSON  string        `db:"data_json" json:"-"`
	CreatedBy string        `db:"created_by" json:"-"`
	CreatedAt time.Time     `db:"created_at" json:"created_at"`
}

type KnowledgeStore interface {
	Create(ctx context.Context, snap KnowledgeSnapshot) (KnowledgeSnapshot, error)
	GetByID(ctx context.Context, id int64) (KnowledgeSnapshot, error)
	List(ctx context.Context, limit, offset int) ([]KnowledgeSnapshot, error)
}

type DictionaryReader interface {
	ReadStopPhrases(ctx context.Context) ([]string, error)
	ReadIgnoredWords(ctx context.Context) error
	CountEntries(ctx context.Context) (int, error)
}

type SampleCounter interface {
	CountSamples(ctx context.Context) (spamCount, hamCount int, err error)
}

type StopPhraseRestorer interface {
	ReadStopPhrases(ctx context.Context) ([]string, error)
	DeleteStopPhrases(ctx context.Context) error
	AddStopPhrase(ctx context.Context, phrase string) error
}

type KnowledgeService struct {
	store    KnowledgeStore
	dict     DictionaryReader
	sample   SampleCounter
	restorer StopPhraseRestorer
}

func NewKnowledgeService(store KnowledgeStore, dict DictionaryReader, sample SampleCounter, restorer StopPhraseRestorer) *KnowledgeService {
	return &KnowledgeService{store: store, dict: dict, sample: sample, restorer: restorer}
}

func (k *KnowledgeService) Snapshot(ctx context.Context, createdBy string) (KnowledgeSnapshot, error) {
	data := KnowledgeData{}

	if k.dict != nil {
		phrases, err := k.dict.ReadStopPhrases(ctx)
		if err != nil {
			log.Printf("[WARN] knowledge snapshot: failed to read stop phrases: %v", err)
		} else {
			data.StopPhrases = phrases
		}

		count, err := k.dict.CountEntries(ctx)
		if err != nil {
			log.Printf("[WARN] knowledge snapshot: failed to count dictionary entries: %v", err)
		} else {
			data.DictionaryVer = count
		}
	}

	if k.sample != nil {
		spam, ham, err := k.sample.CountSamples(ctx)
		if err != nil {
			log.Printf("[WARN] knowledge snapshot: failed to count samples: %v", err)
		} else {
			data.SpamSamples = spam
			data.HamSamples = ham
		}
	}

	data.CreatedAt = time.Now().Format(time.RFC3339)

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("marshal knowledge data: %w", err)
	}

	snap := KnowledgeSnapshot{
		Data:      data,
		DataJSON:  string(jsonBytes),
		CreatedBy: createdBy,
	}

	created, err := k.store.Create(ctx, snap)
	if err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("create snapshot: %w", err)
	}
	created.Data = data
	return created, nil
}

func (k *KnowledgeService) GetSnapshot(ctx context.Context, id int64) (KnowledgeSnapshot, error) {
	snap, err := k.store.GetByID(ctx, id)
	if err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("get snapshot: %w", err)
	}
	if err = json.Unmarshal([]byte(snap.DataJSON), &snap.Data); err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("unmarshal snapshot data: %w", err)
	}
	return snap, nil
}

func (k *KnowledgeService) ListSnapshots(ctx context.Context, limit, offset int) ([]KnowledgeSnapshot, error) {
	snaps, err := k.store.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	for i := range snaps {
		_ = json.Unmarshal([]byte(snaps[i].DataJSON), &snaps[i].Data)
	}
	return snaps, nil
}

func (k *KnowledgeService) Rollback(ctx context.Context, snapshotID int64, rolledBackBy string) (KnowledgeSnapshot, error) {
	if k.restorer == nil {
		return KnowledgeSnapshot{}, fmt.Errorf("rollback not available: no restorer configured")
	}

	snap, err := k.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("get snapshot for rollback: %w", err)
	}

	if err = k.restorer.DeleteStopPhrases(ctx); err != nil {
		return KnowledgeSnapshot{}, fmt.Errorf("delete current stop phrases: %w", err)
	}

	var added, failed int
	for _, phrase := range snap.Data.StopPhrases {
		if addErr := k.restorer.AddStopPhrase(ctx, phrase); addErr != nil {
			log.Printf("[WARN] knowledge rollback: failed to add stop phrase %q: %v", phrase, addErr)
			failed++
			continue
		}
		added++
	}

	log.Printf("[INFO] knowledge rollback to snapshot %d by %q: restored %d stop phrases (%d failed)",
		snapshotID, rolledBackBy, added, failed)

	return snap, nil
}
