package contact

import (
	"fmt"
)

// MergeRequest 是一次人工确认合并请求。
// Sources 至少包含两条尚未归档的记录；Choices 为多规范值字段指定来源。
type MergeRequest struct {
	Sources []RecordID          `json:"sources"`
	Choices map[string]RecordID `json:"choices,omitempty"`
}

var mergeFields = []string{"name", "company", "email", "phone"}

// Merge 校验并执行一次合并，返回新档案。
func (s *Store) Merge(req MergeRequest) (*Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(req.Sources) < 2 {
		return nil, fmt.Errorf("%w: at least two source records are required", ErrInvalid)
	}

	byID := make(map[RecordID]*storedRecord, len(s.data.Records))
	for _, r := range s.data.Records {
		byID[RecordID{r.Source, r.ID}] = r
	}
	merged := make(map[RecordID]bool)
	for _, p := range s.data.Profiles {
		for _, src := range p.Sources {
			merged[src] = true
		}
	}

	seen := make(map[RecordID]bool)
	group := make([]*storedRecord, 0, len(req.Sources))
	for _, src := range req.Sources {
		if seen[src] {
			return nil, fmt.Errorf("%w: %s listed more than once", ErrInvalid, src)
		}
		seen[src] = true
		r, ok := byID[src]
		if !ok {
			return nil, fmt.Errorf("%w: %s does not exist", ErrInvalid, src)
		}
		if merged[src] {
			return nil, fmt.Errorf("%w: %s is already archived", ErrConflict, src)
		}
		group = append(group, r)
	}

	profile := &Profile{Fields: make(map[string]FieldSource)}

	for _, field := range mergeFields {
		chosen, err := resolveField(field, group, req.Choices)
		if err != nil {
			return nil, err
		}
		if chosen == nil {
			continue // 全空字段保持为空
		}
		profile.Fields[field] = FieldSource{Source: chosen.Source, ID: chosen.ID}
		switch field {
		case "name":
			profile.Name = chosen.Name
		case "company":
			profile.Company = chosen.Company
		case "email":
			profile.Email = chosen.Email
		case "phone":
			profile.Phone = chosen.Phone
		}
	}

	profile.Sources = make([]RecordID, 0, len(group))
	for _, r := range group {
		profile.Sources = append(profile.Sources, RecordID{Source: r.Source, ID: r.ID})
	}
	sortSourceIDs(profile.Sources)

	profile.ProfileID = s.nextProfileIDLocked()
	s.data.Profiles = append(s.data.Profiles, profile)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return profile, nil
}

func resolveField(field string, group []*storedRecord, choices map[string]RecordID) (*storedRecord, error) {
	norm := func(r *storedRecord) string {
		switch field {
		case "name":
			return r.normName
		case "company":
			return r.normCompany
		case "email":
			return r.normEmail
		case "phone":
			return r.normPhone
		}
		return ""
	}
	raw := func(r *storedRecord) string {
		switch field {
		case "name":
			return r.Name
		case "company":
			return r.Company
		case "email":
			return r.Email
		case "phone":
			return r.Phone
		}
		return ""
	}

	nonEmpty := make([]*storedRecord, 0, len(group))
	for _, r := range group {
		if raw(r) != "" {
			nonEmpty = append(nonEmpty, r)
		}
	}
	if len(nonEmpty) == 0 {
		return nil, nil
	}

	distinct := make(map[string]bool)
	for _, r := range nonEmpty {
		distinct[norm(r)] = true
	}

	if len(distinct) == 1 {
		return lexSmallest(nonEmpty), nil
	}

	choice, specified := choices[field]
	if !specified {
		return nil, fmt.Errorf("%w: field %q has multiple values; choice is required", ErrConflict, field)
	}
	byID := make(map[RecordID]*storedRecord)
	for _, r := range group {
		byID[RecordID{r.Source, r.ID}] = r
	}
	chosen, ok := byID[choice]
	if !ok || raw(chosen) == "" {
		return nil, fmt.Errorf("%w: choice for %q must reference a group record whose %s is non-empty", ErrInvalid, field, field)
	}
	return chosen, nil
}

func lexSmallest(rs []*storedRecord) *storedRecord {
	best := rs[0]
	for _, r := range rs[1:] {
		if compareID(RecordID{r.Source, r.ID}, RecordID{best.Source, best.ID}) < 0 {
			best = r
		}
	}
	return best
}

func (s *Store) nextProfileIDLocked() string {
	n := s.data.NextProfileNo
	s.data.NextProfileNo++
	return fmt.Sprintf("P%04d", n)
}

func sortSourceIDs(ids []RecordID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && compareID(ids[j-1], ids[j]) > 0; j-- {
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
}
