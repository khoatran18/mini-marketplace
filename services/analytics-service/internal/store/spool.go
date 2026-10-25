package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const spoolFile = "spool.ndjson"

// spoolRecord is one line of the spool file: the target table and the row exactly as it would be sent to
// ClickHouse (JSONEachRow), so replaying needs no knowledge of the Go row types.
type spoolRecord struct {
	Table string          `json:"t"`
	Row   json.RawMessage `json:"r"`
}

// Spool is a tiny durable, append-only file of rows that could not be inserted into ClickHouse (outage longer than
// the RAM buffer, or the service is shutting down). Rows are idempotent in ClickHouse, so replaying twice is harmless.
// When the file would exceed maxBytes new rows are refused (the caller counts them as dropped).
type Spool struct {
	path     string
	maxBytes int64

	mu   sync.Mutex
	size int64
}

// NewSpool opens (creating the directory if needed) the spool in dir. maxBytes <= 0 means 256 MiB.
func NewSpool(dir string, maxBytes int64) (*Spool, error) {
	if maxBytes <= 0 {
		maxBytes = 256 << 20
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	s := &Spool{path: filepath.Join(dir, spoolFile), maxBytes: maxBytes}
	if st, err := os.Stat(s.path); err == nil {
		s.size = st.Size()
	}
	return s, nil
}

// Size is the current file size in bytes.
func (s *Spool) Size() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size
}

// Append writes the rows and fsyncs. It returns how many rows were stored; the rest did not fit (size cap) or could
// not be encoded, and err is set for I/O failures.
func (s *Spool) Append(rows []pending) (stored int, err error) {
	if len(rows) == 0 {
		return 0, nil
	}
	var buf bytes.Buffer
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.maxBytes - s.size
	for _, p := range rows {
		raw, merr := json.Marshal(p.row)
		if merr != nil {
			continue
		}
		line, merr := json.Marshal(spoolRecord{Table: p.table, Row: raw})
		if merr != nil {
			continue
		}
		line = append(line, '\n')
		if int64(buf.Len()+len(line)) > room {
			break
		}
		buf.Write(line)
		stored++
	}
	if stored == 0 {
		return 0, nil
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, err
	}
	n, werr := f.Write(buf.Bytes())
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	s.size += int64(n)
	if werr != nil {
		return 0, werr
	}
	return stored, nil
}

// readAll returns the stored records; corrupt lines (a torn write after a crash) are skipped and counted.
func (s *Spool) readAll() (recs []spoolRecord, corrupt int, err error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var r spoolRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Table == "" || len(r.Row) == 0 {
			corrupt++
			continue
		}
		recs = append(recs, r)
	}
	return recs, corrupt, sc.Err()
}

// rewrite atomically replaces the file with the given records (removing it when empty).
func (s *Spool) rewrite(recs []spoolRecord) error {
	if len(recs) == 0 {
		err := os.Remove(s.path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			s.size = 0
			return nil
		}
		return err
	}
	var buf bytes.Buffer
	for _, r := range recs {
		line, _ := json.Marshal(r)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.size = int64(buf.Len())
	return nil
}
