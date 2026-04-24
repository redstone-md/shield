package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/fileutils"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
)

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Close() error { return nil }

// makeSpamLogger creates spam logger to keep reports about spam messages
// it writes json lines to the provided writer
func makeSpamLogger(ctx context.Context, gid string, wr io.Writer, dataDB *engine.SQL) (events.SpamLogger, error) {
	// make store and load approved users
	detectedSpamStore, auErr := storage.NewDetectedSpam(ctx, dataDB)
	if auErr != nil {
		return nil, fmt.Errorf("can't make approved users store, %w", auErr)
	}

	return auditSpamLogger{ctx: ctx, gid: gid, wr: wr, store: detectedSpamStore}, nil
}

type auditSpamLogger struct {
	ctx   context.Context
	gid   string
	wr    io.Writer
	store *storage.DetectedSpam
}

func (l auditSpamLogger) Save(msg *bot.Message, response *bot.Response) {
	entry := l.baseEntry(msg)
	if err := l.writeLog(entry, msg.From.DisplayName); err != nil {
		log.Printf("[WARN] %v", err)
		return
	}
	if err := l.store.Write(l.ctx, entry, response.CheckResults); err != nil {
		log.Printf("[WARN] can't write to db, %v", err)
	}
}

func (l auditSpamLogger) SaveAudit(ctx context.Context, record events.AuditRecord) error {
	entry := l.baseEntry(record.Message)
	entry.SignalSource = events.SignalSource(record.Response.CheckResults)
	entry.Score = record.Decision.Score
	entry.MatchedRules = events.MatchedRules(record.Response.CheckResults)
	entry.RuleSetVersion = record.RuleSetVersion
	entry.IdempotencyKey = record.Event.IdempotencyKey
	if err := l.writeLog(entry, record.Message.From.DisplayName); err != nil {
		return err
	}
	if err := l.store.Write(ctx, entry, record.Response.CheckResults); err != nil {
		return fmt.Errorf("can't write to db: %w", err)
	}
	return nil
}

func (l auditSpamLogger) baseEntry(msg *bot.Message) storage.DetectedSpamInfo {
	userID := msg.From.ID
	userName := msg.From.Username
	if msg.SenderChat.ID != 0 {
		userID = msg.SenderChat.ID
		userName = msg.SenderChat.UserName
	}
	if userName == "" {
		userName = msg.From.DisplayName
	}
	text := strings.ReplaceAll(msg.Text, "\n", " ")
	text = strings.TrimSpace(text)
	log.Printf("[DEBUG] spam detected from %v, text: %s", msg.From, text)
	return storage.DetectedSpamInfo{
		Text:      text,
		UserID:    userID,
		UserName:  userName,
		Timestamp: time.Now().In(time.Local),
		GID:       l.gid,
	}
}

func (l auditSpamLogger) writeLog(entry storage.DetectedSpamInfo, displayName string) error {
	m := struct {
		TimeStamp  string `json:"ts"`
		DisplayName string `json:"display_name"`
		UserName   string `json:"user_name"`
		UserID     int64  `json:"user_id"`
		Text       string `json:"text"`
	}{
		TimeStamp:  time.Now().In(time.Local).Format(time.RFC3339),
		DisplayName: displayName,
		UserName:   entry.UserName,
		UserID:     entry.UserID,
		Text:       entry.Text,
	}
	line, err := json.Marshal(&m)
	if err != nil {
		return fmt.Errorf("can't marshal json: %w", err)
	}
	if _, err := l.wr.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("can't write to log: %w", err)
	}
	return nil
}

// makeSpamLogWriter creates spam log writer to keep reports about spam messages
// it parses options and makes lumberjack logger with rotation
func makeSpamLogWriter(opts options) (accessLog io.WriteCloser, err error) {
	if !opts.Logger.Enabled {
		return nopWriteCloser{io.Discard}, nil
	}

	sizeParse := func(inp string) (uint64, error) {
		if inp == "" {
			return 0, errors.New("empty value")
		}
		for i, sfx := range []string{"k", "m", "g", "t"} {
			if strings.HasSuffix(inp, strings.ToUpper(sfx)) || strings.HasSuffix(inp, strings.ToLower(sfx)) {
				val, err := strconv.Atoi(inp[:len(inp)-1])
				if err != nil {
					return 0, fmt.Errorf("can't parse %s: %w", inp, err)
				}
				return uint64(float64(val) * math.Pow(float64(1024), float64(i+1))), nil
			}
		}
		return strconv.ParseUint(inp, 10, 64)
	}

	maxSize, perr := sizeParse(opts.Logger.MaxSize)
	if perr != nil {
		return nil, fmt.Errorf("can't parse logger MaxSize: %w", perr)
	}

	maxSize /= 1048576

	log.Printf("[INFO] logger enabled for %s, max size %dM", opts.Logger.FileName, maxSize)
	return &lumberjack.Logger{
		Filename:   opts.Logger.FileName,
		MaxSize:    int(maxSize), //nolint:gosec // size in MB not that big to cause overflow
		MaxBackups: opts.Logger.MaxBackups,
		Compress:   true,
		LocalTime:  true,
	}, nil
}

// migrateSamples runs migrations from legacy text files samples to db, if such files found
func migrateSamples(ctx context.Context, opts options, samplesDB *storage.Samples) error {
	if opts.Convert == "disabled" {
		log.Print("[DEBUG] samples migration disabled")
		return nil
	}
	migrateSamples := func(file string, sampleType storage.SampleType, origin storage.SampleOrigin) (*storage.SamplesStats, error) {
		if _, err := os.Stat(file); err != nil {
			log.Printf("[DEBUG] samples file %s not found, skip", file)
			return &storage.SamplesStats{}, nil
		}
		fh, err := os.Open(file) //nolint:gosec // file path is controlled by the app
		if err != nil {
			return nil, fmt.Errorf("can't open samples file, %w", err)
		}
		defer fh.Close()
		stats, err := samplesDB.Import(ctx, sampleType, origin, fh, true) // clean records before import
		if err != nil {
			return nil, fmt.Errorf("can't load samples, %w", err)
		}
		if err := fh.Close(); err != nil {
			return nil, fmt.Errorf("can't close samples file, %w", err)
		}
		if err := os.Rename(file, file+".loaded"); err != nil {
			return nil, fmt.Errorf("can't rename samples file, %w", err)
		}
		return stats, nil
	}

	if samplesDB == nil {
		return errors.New("samples db is nil")
	}

	// migrate preset spam samples if files exist
	spamPresetFile := filepath.Join(opts.Files.SamplesDataPath, samplesSpamFile)
	s, err := migrateSamples(spamPresetFile, storage.SampleTypeSpam, storage.SampleOriginPreset)
	if err != nil {
		return fmt.Errorf("can't migrate spam preset samples, %w", err)
	}
	if s.PresetHam > 0 {
		log.Printf("[DEBUG] spam preset samples loaded: %s", s)
	}

	// migrate preset ham samples if files exist
	hamPresetFile := filepath.Join(opts.Files.SamplesDataPath, samplesHamFile)
	s, err = migrateSamples(hamPresetFile, storage.SampleTypeHam, storage.SampleOriginPreset)
	if err != nil {
		return fmt.Errorf("can't migrate ham preset samples, %w", err)
	}
	if s.PresetHam > 0 {
		log.Printf("[DEBUG] ham preset samples loaded: %s", s)
	}

	// migrate dynamic spam samples if files exist
	dynSpamFile := filepath.Join(opts.Files.DynamicDataPath, dynamicSpamFile)
	s, err = migrateSamples(dynSpamFile, storage.SampleTypeSpam, storage.SampleOriginUser)
	if err != nil {
		return fmt.Errorf("can't migrate spam dynamic samples, %w", err)
	}
	if s.UserSpam > 0 {
		log.Printf("[DEBUG] spam dynamic samples loaded: %s", s)
	}

	// migrate dynamic ham samples if files exist
	dynHamFile := filepath.Join(opts.Files.DynamicDataPath, dynamicHamFile)
	s, err = migrateSamples(dynHamFile, storage.SampleTypeHam, storage.SampleOriginUser)
	if err != nil {
		return fmt.Errorf("can't migrate ham dynamic samples, %w", err)
	}
	if s.UserHam > 0 {
		log.Printf("[DEBUG] ham dynamic samples loaded: %s", s)
	}

	if s.TotalHam > 0 || s.TotalSpam > 0 {
		log.Printf("[INFO] samples migration done: %s", s)
	}
	return nil
}

// migrateDicts runs migrations from legacy dictionary text files to db, if needed
func migrateDicts(ctx context.Context, opts options, dictDB *storage.Dictionary) error {
	if opts.Convert == "disabled" {
		log.Print("[DEBUG] dictionary migration disabled")
		return nil
	}

	migrateDict := func(file string, dictType storage.DictionaryType) (*storage.DictionaryStats, error) {
		if _, err := os.Stat(file); err != nil {
			log.Printf("[DEBUG] dictionary file %s not found, skip", file)
			return &storage.DictionaryStats{}, nil
		}
		fh, err := os.Open(file) //nolint:gosec // file path is controlled by the app
		if err != nil {
			return nil, fmt.Errorf("can't open dictionary file, %w", err)
		}
		defer fh.Close()
		stats, err := dictDB.Import(ctx, dictType, fh, true) // clean records before import
		if err != nil {
			return nil, fmt.Errorf("can't load dictionary, %w", err)
		}
		if err := fh.Close(); err != nil {
			return nil, fmt.Errorf("can't close dictionary file, %w", err)
		}
		if err := os.Rename(file, file+".loaded"); err != nil {
			return nil, fmt.Errorf("can't rename dictionary file, %w", err)
		}
		return stats, nil
	}

	if dictDB == nil {
		return errors.New("dictionary db is nil")
	}

	// migrate stop-words if files exist
	stopWordsFile := filepath.Join(opts.Files.SamplesDataPath, stopWordsFile)
	s, err := migrateDict(stopWordsFile, storage.DictionaryTypeStopPhrase)
	if err != nil {
		return fmt.Errorf("can't migrate stop words, %w", err)
	}
	if s.TotalStopPhrases > 0 {
		log.Printf("[INFO] stop words loaded: %s", s)
	}

	// migrate excluded tokens if files exist
	excludeTokensFile := filepath.Join(opts.Files.SamplesDataPath, excludeTokensFile)
	s, err = migrateDict(excludeTokensFile, storage.DictionaryTypeIgnoredWord)
	if err != nil {
		return fmt.Errorf("can't migrate excluded tokens, %w", err)
	}
	if s.TotalIgnoredWords > 0 {
		log.Printf("[INFO] excluded tokens loaded: %s", s)
	}

	if s.TotalIgnoredWords > 0 || s.TotalStopPhrases > 0 {
		log.Printf("[DEBUG] dictionaries migration done: %s", s)
	}
	return nil
}

// backupDB creates a backup of the db file if the version has changed. It copies the db file to a new db file
// named as the original file with a version suffix, e.g., tg-spam.db.master-77e0bfd-20250107T23:17:34.
// The file is created only if the version has changed and a backup file with the name tg-spam.db.<version> does not exist.
// It keeps up to maxBackups files; if maxBackups is 0, no backups are made.
// Files are removed based on the final part of the version, i.e., 20250107T23:17:34, with the oldest backups removed first.
// If the backup file extension suffix with the timestamp is not found, the modification time of the file is used instead.
func backupDB(dbFile, version string, maxBackups int) error {
	if maxBackups == 0 {
		return nil
	}
	backupFile := dbFile + "." + strings.ReplaceAll(version, ".", "_") // replace dots with underscores for file name
	if _, err := os.Stat(backupFile); err == nil {
		// backup file for the version already exists, no need to make it again
		return nil
	}
	if _, err := os.Stat(dbFile); err != nil {
		// db file not found, no need to backup. This is legit if the db is not created yet on the first run
		log.Printf("[WARN] db file not found: %s, skip backup", dbFile)
		return nil
	}

	log.Printf("[DEBUG] db backup: %s -> %s", dbFile, backupFile)
	// copy current db to the backup file
	if err := fileutils.CopyFile(dbFile, backupFile); err != nil {
		return fmt.Errorf("failed to copy db file: %w", err)
	}
	log.Printf("[INFO] db backup created: %s", backupFile)

	// cleanup old backups if needed
	files, err := filepath.Glob(dbFile + ".*")
	if err != nil {
		return fmt.Errorf("failed to list backup files: %w", err)
	}

	if len(files) <= maxBackups {
		return nil
	}

	// sort files by timestamp in version suffix or mod time if suffix not formatted as timestamp
	sort.Slice(files, func(i, j int) bool {
		getTime := func(f string) time.Time {
			base := filepath.Base(f) // file name like this: tg-spam.db.master-77e0bfd-20250107T23:17:34
			// try to get timestamp from version suffix first
			parts := strings.Split(base, "-")
			if len(parts) >= 3 {
				suffix := parts[len(parts)-1]
				if t, err := time.ParseInLocation("20060102T15:04:05", suffix, time.Local); err == nil {
					return t
				}
			}
			// fallback to modification time for non-versioned files
			fi, err := os.Stat(f)
			if err != nil {
				log.Printf("[WARN] can't stat file %s: %v", f, err)
				return time.Now().Local() // treat errored files as newest to avoid deleting them
			}
			return fi.ModTime().Local() // convert to local for consistent comparison
		}
		return getTime(files[i]).Before(getTime(files[j]))
	})

	// remove oldest files
	for i := 0; i < len(files)-maxBackups; i++ {
		if err := os.Remove(files[i]); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", files[i], err)
		}
		log.Printf("[DEBUG] db backup removed: %s", files[i])
	}
	return nil
}
