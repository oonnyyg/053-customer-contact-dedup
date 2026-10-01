package contact

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Record 是一条渠道联系人原始记录。
type Record struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Company string `json:"company,omitempty"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
}

// RecordID 是来源与来源内编号组成的稳定标识。
type RecordID struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

func (r RecordID) String() string { return r.Source + "/" + r.ID }

// FieldSource 记录输出字段采用了哪条源记录的原文。
type FieldSource struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// Profile 是人工确认后生成的合并档案。
type Profile struct {
	ProfileID string                 `json:"profile_id"`
	Name      string                 `json:"name,omitempty"`
	Company   string                 `json:"company,omitempty"`
	Email     string                 `json:"email,omitempty"`
	Phone     string                 `json:"phone,omitempty"`
	Fields    map[string]FieldSource `json:"fields"`
	Sources   []RecordID             `json:"sources"`
}

type storedRecord struct {
	Record
	normName    string
	normCompany string
	normEmail   string
	normPhone   string
}

type dbData struct {
	Records       []*storedRecord `json:"records"`
	Profiles      []*Profile      `json:"profiles"`
	NextProfileNo int             `json:"next_profile_no"`
}

// Store 是本地通讯录数据，所有修改在内存进行并落盘到目录。
type Store struct {
	mu   sync.Mutex
	dir  string
	data dbData
}

var ErrConflict = errors.New("conflict")
var ErrInvalid = errors.New("invalid")

const dbFileName = "contacts-db.json"

// Open 打开（必要时创建）目录下的数据文件。
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: data directory is required", ErrInvalid)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	path := filepath.Join(dir, dbFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = dbData{NextProfileNo: 1}
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("corrupt data file %s: %w", path, err)
	}
	s.recomputeNorms()
	if s.data.NextProfileNo < 1 {
		s.data.NextProfileNo = 1
	}
	return s, nil
}

// ImportResult 是一次批量导入的结果。
type ImportResult struct {
	Received int `json:"received"`
	Added    int `json:"added"`
	Skipped  int `json:"skipped"`
}

// Import 先完整校验整批，再执行去重导入；任一条冲突则整批拒绝。
func (s *Store) Import(records []Record) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(records) == 0 {
		return nil, fmt.Errorf("%w: records must be a non-empty array", ErrInvalid)
	}

	existing := make(map[RecordID]*storedRecord, len(s.data.Records))
	for _, r := range s.data.Records {
		existing[RecordID{r.Source, r.ID}] = r
	}

	type pending struct {
		rec  *storedRecord
		skip bool
	}
	pendingByID := make(map[RecordID]*pending, len(records))
	ordered := make([]RecordID, 0, len(records))
	result := &ImportResult{Received: len(records)}

	for i, r := range records {
		if err := validateRaw(r); err != nil {
			return nil, fmt.Errorf("record[%d] %w: %v", i, ErrInvalid, err)
		}
		key := RecordID{Source: r.Source, ID: r.ID}
		if p, ok := pendingByID[key]; ok {
			if p.skip || rawEqual(p.rec.Record, r) {
				result.Skipped++
				continue
			}
			return nil, fmt.Errorf("%w: record[%d] duplicates %s in the same batch with different content", ErrConflict, i, key)
		}
		st := newStoredRecord(r)
		p := &pending{rec: st}
		if ex, ok := existing[key]; ok {
			if rawEqual(ex.Record, r) {
				p.skip = true
				result.Skipped++
			} else {
				return nil, fmt.Errorf("%w: record[%d] %s already exists with different content", ErrConflict, i, key)
			}
		} else {
			result.Added++
		}
		pendingByID[key] = p
		ordered = append(ordered, key)
	}

	for _, key := range ordered {
		p := pendingByID[key]
		if !p.skip {
			s.data.Records = append(s.data.Records, p.rec)
		}
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return result, nil
}

func validateRaw(r Record) error {
	if r.Source == "" {
		return errors.New("source is required")
	}
	if r.ID == "" {
		return errors.New("id is required")
	}
	if r.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func newStoredRecord(r Record) *storedRecord {
	return &storedRecord{
		Record:      r,
		normName:    NormalizeName(r.Name),
		normCompany: NormalizeCompany(r.Company),
		normEmail:   NormalizeEmail(r.Email),
		normPhone:   NormalizePhone(r.Phone),
	}
}

// rawEqual 比较原始字段（原文，不做规范化），全部一致才算重复导入。
func rawEqual(a, b Record) bool {
	return a.Source == b.Source && a.ID == b.ID && a.Name == b.Name &&
		a.Company == b.Company && a.Email == b.Email && a.Phone == b.Phone
}

func (s *Store) recomputeNorms() {
	for _, r := range s.data.Records {
		r.normName = NormalizeName(r.Name)
		r.normCompany = NormalizeCompany(r.Company)
		r.normEmail = NormalizeEmail(r.Email)
		r.normPhone = NormalizePhone(r.Phone)
	}
}

func (s *Store) saveLocked() error {
	sortRecords(s.data.Records)
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, dbFileName)
	tmp, err := os.CreateTemp(s.dir, ".contacts-db-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func recordLess(a, b *storedRecord) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.ID < b.ID
}

func sortRecords(rs []*storedRecord) {
	sort.Slice(rs, func(i, j int) bool { return recordLess(rs[i], rs[j]) })
}
