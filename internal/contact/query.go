package contact

import (
	"fmt"
	"sort"
)

// RecordView 是一条记录的全部原始信息及其归档状态。
type RecordView struct {
	Record    Record     `json:"record"`
	Archived  bool       `json:"archived"`
	ProfileID string     `json:"profile_id,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

// Evidence 说明该记录的某字段在合并档案中的去向（含原值）。
type Evidence struct {
	ProfileID string `json:"profile_id"`
	Field     string `json:"field,omitempty"`
	Value     string `json:"value,omitempty"`
	Used      bool   `json:"used"`
}

// GetRecord 返回原文与证据；已归档时给出档案编号及各字段是否被采用。
func (s *Store) GetRecord(id RecordID) (*RecordView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var st *storedRecord
	for _, r := range s.data.Records {
		if (RecordID{r.Source, r.ID}) == id {
			st = r
			break
		}
	}
	if st == nil {
		return nil, fmt.Errorf("%w: %s does not exist", ErrInvalid, id)
	}

	v := &RecordView{Record: st.Record}
	for _, p := range s.data.Profiles {
		if !containsID(p.Sources, id) {
			continue
		}
		v.Archived = true
		v.ProfileID = p.ProfileID
		for _, field := range mergeFields {
			raw := fieldValue(st, field)
			if raw == "" {
				continue
			}
			fs, used := p.Fields[field]
			e := Evidence{ProfileID: p.ProfileID, Field: field, Value: raw}
			if used && fs.Source == st.Source && fs.ID == st.ID {
				e.Used = true
			}
			v.Evidence = append(v.Evidence, e)
		}
	}
	return v, nil
}

func containsID(ids []RecordID, id RecordID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func fieldValue(r *storedRecord, field string) string {
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

// ExportData 是导出结果：合并档案与尚未合并联系人，均按稳定标识排序。
type ExportData struct {
	Profiles []Profile `json:"profiles"`
	Unmerged []Record  `json:"unmerged"`
}

// Export 不重复输出已合并来源。
func (s *Store) Export() *ExportData {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := make(map[RecordID]bool)
	profiles := make([]Profile, 0, len(s.data.Profiles))
	for _, p := range s.data.Profiles {
		cp := *p
		cp.Sources = append([]RecordID(nil), p.Sources...)
		sortSourceIDs(cp.Sources)
		profiles = append(profiles, cp)
		for _, src := range p.Sources {
			merged[src] = true
		}
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ProfileID < profiles[j].ProfileID })

	unmerged := make([]Record, 0)
	for _, r := range s.data.Records {
		id := RecordID{r.Source, r.ID}
		if !merged[id] {
			unmerged = append(unmerged, r.Record)
		}
	}
	sort.Slice(unmerged, func(i, j int) bool {
		if unmerged[i].Source != unmerged[j].Source {
			return unmerged[i].Source < unmerged[j].Source
		}
		return unmerged[i].ID < unmerged[j].ID
	})

	return &ExportData{Profiles: profiles, Unmerged: unmerged}
}
