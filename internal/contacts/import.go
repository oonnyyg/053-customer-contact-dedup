package contacts

import "fmt"

// ImportResult reports how a batch import was applied.
type ImportResult struct {
	Received  int `json:"received"`
	Imported  int `json:"imported"`
	Duplicate int `json:"duplicate"`
}

func validateRecord(r Record) error {
	if r.Source == "" {
		return fmt.Errorf("source is required")
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func sameRaw(a, b Record) bool {
	return a.Name == b.Name && a.Company == b.Company && a.Email == b.Email && a.Phone == b.Phone
}

// Import validates the whole batch before writing anything. Records sharing
// an identity with an existing record (or another record in the batch) are
// skipped only when every raw field is identical; any conflict rejects the
// entire batch.
func (s *Store) Import(records []Record) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(records) == 0 {
		return ImportResult{}, fmt.Errorf("records must contain at least one entry")
	}

	batch := map[Ref]Record{}
	var batchOrder []Ref
	batchDup := 0
	for i, r := range records {
		if err := validateRecord(r); err != nil {
			return ImportResult{}, fmt.Errorf("record %d: %w", i, err)
		}
		ref := Ref{r.Source, r.ID}
		if prior, ok := batch[ref]; ok {
			if !sameRaw(prior, r) {
				return ImportResult{}, bizError("conflicting records for %s/%s within batch", ref.Source, ref.ID)
			}
			batchDup++
			continue
		}
		batch[ref] = r
		batchOrder = append(batchOrder, ref)
	}

	for _, ref := range batchOrder {
		r := batch[ref]
		if existing, ok := s.records[ref]; ok {
			if !sameRaw(existing.Record, r) {
				return ImportResult{}, bizError("conflicting records for existing %s/%s", ref.Source, ref.ID)
			}
		}
	}

	res := ImportResult{Received: len(records)}
	res.Duplicate = batchDup
	var added bool
	for _, ref := range batchOrder {
		r := batch[ref]
		if _, ok := s.records[ref]; ok {
			res.Duplicate++
			continue
		}
		s.records[ref] = &storedRecord{Record: r, Norm: normalize(r)}
		s.recordOrder = append(s.recordOrder, ref)
		res.Imported++
		added = true
	}
	if added {
		if err := s.persistLocked(); err != nil {
			return ImportResult{}, err
		}
	}
	return res, nil
}
