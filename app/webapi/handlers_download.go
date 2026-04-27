package webapi

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha1" //nolint
	"encoding/json"
	"fmt"
	"io/fs"
	"math/big"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/rest"

	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func (s *Server) downloadDetectedSpamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	spam, err := s.DetectedSpam.Read(ctx)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get detected spam", "details": err.Error()})
		return
	}

	type jsonSpamInfo struct {
		ID        int64               `json:"id"`
		GID       string              `json:"gid"`
		Text      string              `json:"text"`
		UserID    int64               `json:"user_id"`
		UserName  string              `json:"user_name"`
		Timestamp time.Time           `json:"timestamp"`
		Added     bool                `json:"added"`
		Checks    []spamcheck.Response `json:"checks"`
	}

	// convert entries to jsonl format with lowercase fields
	lines := make([]string, 0, len(spam))
	for _, entry := range spam {
		data, err := json.Marshal(jsonSpamInfo{
			ID:        entry.ID,
			GID:       entry.GID,
			Text:      entry.Text,
			UserID:    entry.UserID,
			UserName:  entry.UserName,
			Timestamp: entry.Timestamp,
			Added:     entry.Added,
			Checks:    entry.Checks,
		})
		if err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't marshal entry", "details": err.Error()})
			return
		}
		lines = append(lines, string(data))
	}

	body := strings.Join(lines, "\n")
	w.Header().Set("Content-Type", "application/x-jsonlines")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "detected_spam.jsonl"))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// downloadBackupHandler streams a database backup as an SQL file with gzip compression
// Files are always compressed and always have .gz extension to ensure consistency
func (s *Server) downloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	if s.StorageEngine == nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "storage engine not available"})
		return
	}

	// set filename based on database type and timestamp
	dbType := "db"
	sqlEng, ok := s.StorageEngine.(*engine.SQL)
	if ok {
		dbType = string(sqlEng.Type())
	}
	timestamp := time.Now().Format("20060102-150405")

	// always use a .gz extension as the content is always compressed
	filename := fmt.Sprintf("tg-spam-backup-%s-%s.sql.gz", dbType, timestamp)

	// set headers for file download - note we're using application/octet-stream
	// instead of application/sql to prevent browsers from trying to interpret the file
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// create a gzip writer that streams to response
	gzipWriter := gzip.NewWriter(w)
	defer func() {
		if err := gzipWriter.Close(); err != nil {
			observability.Logf(r.Context(), "[ERROR] failed to close gzip writer: %v", err)
		}
	}()

	// stream backup directly to response through gzip
	if err := s.StorageEngine.Backup(r.Context(), gzipWriter); err != nil {
		observability.Logf(r.Context(), "[ERROR] failed to create backup: %v", err)
		// we've already started writing the response, so we can't send a proper error response
		return
	}

	// flush the gzip writer to ensure all data is written
	if err := gzipWriter.Flush(); err != nil {
		observability.Logf(r.Context(), "[ERROR] failed to flush gzip writer: %v", err)
	}
}

// downloadExportToPostgresHandler streams a PostgreSQL-compatible export from a SQLite database
func (s *Server) downloadExportToPostgresHandler(w http.ResponseWriter, r *http.Request) {
	if s.StorageEngine == nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "storage engine not available"})
		return
	}

	// check if the database is SQLite
	if s.StorageEngine.Type() != engine.Sqlite {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "source database must be SQLite"})
		return
	}

	// set filename based on timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("tg-spam-sqlite-to-postgres-%s.sql.gz", timestamp)

	// set headers for file download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// create a gzip writer that streams to response
	gzipWriter := gzip.NewWriter(w)
	defer func() {
		if err := gzipWriter.Close(); err != nil {
			observability.Logf(r.Context(), "[ERROR] failed to close gzip writer: %v", err)
		}
	}()

	// stream export directly to response through gzip
	if err := s.StorageEngine.BackupSqliteAsPostgres(r.Context(), gzipWriter); err != nil {
		observability.Logf(r.Context(), "[ERROR] failed to create export: %v", err)
		// we've already started writing the response, so we can't send a proper error response
		return
	}

	// flush the gzip writer to ensure all data is written
	if err := gzipWriter.Flush(); err != nil {
		observability.Logf(r.Context(), "[ERROR] failed to flush gzip writer: %v", err)
	}
}

func (s *Server) renderSamples(w http.ResponseWriter, tmplName string) {
	spam, ham, err := s.SpamFilter.DynamicSamples()
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't fetch samples", "details": err.Error()})
		return
	}

	spam, ham = s.reverseSamples(spam, ham)

	type smpleWithID struct {
		ID     string
		Sample string
	}

	makeID := func(s string) string {
		hash := sha1.New() //nolint
		if _, err := hash.Write([]byte(s)); err != nil {
			return fmt.Sprintf("%x", s)
		}
		return fmt.Sprintf("%x", hash.Sum(nil))
	}

	tmplData := struct {
		SpamSamples      []smpleWithID
		HamSamples       []smpleWithID
		TotalHamSamples  int
		TotalSpamSamples int
	}{
		TotalHamSamples:  len(ham),
		TotalSpamSamples: len(spam),
	}
	for _, s := range spam {
		tmplData.SpamSamples = append(tmplData.SpamSamples, smpleWithID{ID: makeID(s), Sample: s})
	}
	for _, h := range ham {
		tmplData.HamSamples = append(tmplData.HamSamples, smpleWithID{ID: makeID(h), Sample: h})
	}

	if err := tmpl.ExecuteTemplate(w, tmplName, tmplData); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't execute template", "details": err.Error()})
		return
	}
}

// reverseSamples returns reversed lists of spam and ham samples
func (s *Server) reverseSamples(spam, ham []string) (revSpam, revHam []string) {
	revSpam = make([]string, len(spam))
	revHam = make([]string, len(ham))

	for i, j := 0, len(spam)-1; i < len(spam); i, j = i+1, j-1 {
		revSpam[i] = spam[j]
	}
	for i, j := 0, len(ham)-1; i < len(ham); i, j = i+1, j-1 {
		revHam[i] = ham[j]
	}
	return revSpam, revHam
}

// renderDictionary renders dictionary entries for HTMX or full page request
func (s *Server) renderDictionary(ctx context.Context, w http.ResponseWriter, tmplName string) {
	dict := s.dictionary()
	stopPhrases, err := dict.ReadWithIDs(ctx, storage.DictionaryTypeStopPhrase)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't fetch stop phrases", "details": err.Error()})
		return
	}

	ignoredWords, err := dict.ReadWithIDs(ctx, storage.DictionaryTypeIgnoredWord)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't fetch ignored words", "details": err.Error()})
		return
	}

	tmplData := struct {
		StopPhrases      []storage.DictionaryEntry
		IgnoredWords     []storage.DictionaryEntry
		TotalStopPhrases int
		TotalIgnoredWords int
	}{
		StopPhrases:       stopPhrases,
		IgnoredWords:      ignoredWords,
		TotalStopPhrases:  len(stopPhrases),
		TotalIgnoredWords: len(ignoredWords),
	}

	if err := tmpl.ExecuteTemplate(w, tmplName, tmplData); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't execute template", "details": err.Error()})
		return
	}
}

// staticFS is a filtered filesystem that only exposes specific static files
type staticFS struct {
	fs        fs.FS
	urlToPath map[string]string
}

// staticFileMapping defines a mapping between URL path and filesystem path
type staticFileMapping struct {
	urlPath    string
	filesysPath string
}

func newStaticFS(fsys fs.FS, files ...staticFileMapping) *staticFS {
	urlToPath := make(map[string]string)
	for _, f := range files {
		urlToPath[f.urlPath] = f.filesysPath
	}

	return &staticFS{
		fs:        fsys,
		urlToPath: urlToPath,
	}
}

func (sfs *staticFS) Open(name string) (fs.File, error) {
	cleanName := path.Clean("/" + name)[1:]

	fsPath, ok := sfs.urlToPath[cleanName]
	if !ok {
		return nil, fs.ErrNotExist
	}

	file, err := sfs.fs.Open(fsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open static file %s: %w", fsPath, err)
	}
	return file, nil
}

// GenerateRandomPassword generates a random password of a given length
func GenerateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+"
	const charsetLen = int64(len(charset))

	result := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, big.NewInt(charsetLen))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}
