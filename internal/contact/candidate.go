package contact

import "sort"

// FieldScore 描述单个字段的匹配得分及双方原值。
type FieldScore struct {
	Field  string `json:"field"`
	Points int    `json:"points"`
	Left   string `json:"left_value"`
	Right  string `json:"right_value"`
}

// Candidate 是一对可能重复的尚未合并记录。
type Candidate struct {
	Left   RecordID     `json:"left"`
	Right  RecordID     `json:"right"`
	Total  int          `json:"total"`
	Scores []FieldScore `json:"scores"`
}

// CandidateFilter 是候选筛选与分页条件。
type CandidateFilter struct {
	Source   string `json:"source,omitempty"`
	MinScore int    `json:"min_score,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// CandidatePage 是分页后的候选结果。
type CandidatePage struct {
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Items []Candidate `json:"items"`
}

const candidateThreshold = 40

// FindCandidates 只在尚未合并的记录之间两两比较并评分。
func (s *Store) FindCandidates(f CandidateFilter) (*CandidatePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	min := f.MinScore
	if min < candidateThreshold {
		min = candidateThreshold
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	size := f.PageSize
	if size < 1 {
		size = 20
	}

	merged := make(map[RecordID]bool)
	for _, p := range s.data.Profiles {
		for _, src := range p.Sources {
			merged[src] = true
		}
	}

	rs := make([]*storedRecord, 0, len(s.data.Records))
	for _, r := range s.data.Records {
		if !merged[RecordID{r.Source, r.ID}] {
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool { return recordLess(rs[i], rs[j]) })

	var all []Candidate
	for i := 0; i < len(rs); i++ {
		for j := i + 1; j < len(rs); j++ {
			cand, ok := scorePair(rs[i], rs[j])
			if !ok || cand.Total < min {
				continue
			}
			if f.Source != "" && cand.Left.Source != f.Source && cand.Right.Source != f.Source {
				continue
			}
			all = append(all, cand)
		}
	}

	sort.Slice(all, func(a, b int) bool {
		if all[a].Total != all[b].Total {
			return all[a].Total > all[b].Total
		}
		if c := compareID(all[a].Left, all[b].Left); c != 0 {
			return c < 0
		}
		return compareID(all[a].Right, all[b].Right) < 0
	})

	total := len(all)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return &CandidatePage{Total: total, Page: page, Items: all[start:end]}, nil
}

func scorePair(a, b *storedRecord) (Candidate, bool) {
	cand := Candidate{
		Left:  RecordID{Source: a.Source, ID: a.ID},
		Right: RecordID{Source: b.Source, ID: b.ID},
	}
	// 先按来源及编号排序双方
	if compareID(cand.Left, cand.Right) > 0 {
		a, b = b, a
		cand.Left, cand.Right = RecordID{Source: a.Source, ID: a.ID}, RecordID{Source: b.Source, ID: b.ID}
	}

	type rule struct {
		field      string
		points     int
		na, nb     string
		rawA, rawB string
	}
	rules := []rule{
		{"email", 60, a.normEmail, b.normEmail, a.Email, b.Email},
		{"phone", 60, a.normPhone, b.normPhone, a.Phone, b.Phone},
		{"name", 20, a.normName, b.normName, a.Name, b.Name},
		{"company", 20, a.normCompany, b.normCompany, a.Company, b.Company},
	}
	for _, r := range rules {
		if r.na == "" || r.nb == "" {
			continue
		}
		if r.na == r.nb {
			cand.Scores = append(cand.Scores, FieldScore{
				Field: r.field, Points: r.points, Left: r.rawA, Right: r.rawB,
			})
			cand.Total += r.points
		}
	}
	return cand, cand.Total > 0
}

func compareID(a, b RecordID) int {
	if a.Source != b.Source {
		if a.Source < b.Source {
			return -1
		}
		return 1
	}
	if a.ID != b.ID {
		if a.ID < b.ID {
			return -1
		}
		return 1
	}
	return 0
}
