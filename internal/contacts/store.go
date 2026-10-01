package contacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const dataFileName = "contacts_data.json"

// storedRecord keeps the original raw values alongside normalized values.
type storedRecord struct {
	Record
	Norm normalized `json:"-"`
}

type persistedArchive struct {
	Archive
	Members []Record `json:"members"`
}

type persistedData struct {
	Records      []Record           `json:"records"`
	Archives     []persistedArchive `json:"archives"`
	NextArchiveN int                `json:"next_archive_n"`
}

// Store is the in-memory application state, backed by a JSON file in a
// designated local directory.
type Store struct {
	mu           sync.Mutex
	dir          string
	records      map[Ref]*storedRecord
	recordOrder  []Ref
	archived     map[Ref]string
	archives     map[string]*persistedArchive
	archiveOrder []string
	nextArchiveN int
}

// Open loads state from dir, creating it if necessary.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Store{
		dir:          dir,
		records:      map[Ref]*storedRecord{},
		archived:     map[Ref]string{},
		archives:     map[string]*persistedArchive{},
		nextArchiveN: 1,
	}
	path := filepath.Join(dir, dataFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read data: %w", err)
	}
	var p persistedData
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse data: %w", err)
	}
	s.nextArchiveN = p.NextArchiveN
	if s.nextArchiveN < 1 {
		s.nextArchiveN = 1
	}
	for _, r := range p.Records {
		ref := Ref{r.Source, r.ID}
		if _, ok := s.records[ref]; ok {
			return nil, fmt.Errorf("corrupt data: duplicate ref %s/%s", r.Source, r.ID)
		}
		s.records[ref] = &storedRecord{Record: r, Norm: normalize(r)}
		s.recordOrder = append(s.recordOrder, ref)
	}
	for i := range p.Archives {
		a := p.Archives[i]
		s.archives[a.ArchiveID] = &a
		s.archiveOrder = append(s.archiveOrder, a.ArchiveID)
		for _, m := range a.Members {
			s.archived[Ref{m.Source, m.ID}] = a.ArchiveID
		}
	}
	return s, nil
}

func (s *Store) persistLocked() error {
	p := persistedData{NextArchiveN: s.nextArchiveN}
	for _, ref := range s.recordOrder {
		p.Records = append(p.Records, s.records[ref].Record)
	}
	for _, id := range s.archiveOrder {
		p.Archives = append(p.Archives, *s.archives[id])
	}
	b, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, dataFileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, dataFileName)); err != nil {
		return fmt.Errorf("commit data: %w", err)
	}
	return nil
}

// BusinessError marks a request that is well-formed but rejected by the
// domain rules, as opposed to a malformed request.
type BusinessError struct{ Msg string }

func (e *BusinessError) Error() string { return e.Msg }

func bizError(format string, args ...any) error {
	return &BusinessError{Msg: fmt.Sprintf(format, args...)}
}

// lessRef reports whether a sorts before b (source, then id).
func lessRef(a, b Ref) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.ID < b.ID
}

func sortRefs(refs []Ref) {
	sort.Slice(refs, func(i, j int) bool { return lessRef(refs[i], refs[j]) })
}
