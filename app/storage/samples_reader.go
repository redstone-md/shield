package storage

import (
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"
)

type sampleReader struct {
	rows    *sqlx.Rows
	buffer  []byte
	current string
	closed  bool
}

func (r *sampleReader) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}

	if len(r.buffer) == 0 {
		if r.rows == nil || !r.rows.Next() {
			if r.rows != nil && r.rows.Err() != nil {
				return 0, fmt.Errorf("rows iteration failed: %w", r.rows.Err())
			}
			return 0, io.EOF
		}

		if err := r.rows.Scan(&r.current); err != nil {
			return 0, fmt.Errorf("scan failed: %w", err)
		}
		r.buffer = []byte(r.current + "\n")
	}

	n = copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (r *sampleReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.rows != nil {
		if err := r.rows.Close(); err != nil {
			return fmt.Errorf("failed to close rows: %w", err)
		}
		r.rows = nil
	}
	return nil
}
