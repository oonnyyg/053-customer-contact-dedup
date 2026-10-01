package contacts

import "sort"

// FieldScore exposes the raw values compared and the points awarded.
type FieldScore struct {
	Matched bool   `json:"matched"`
	Score   int    `json:"score"`
	Left    string `json:"left"`
	Right   string `json:"right"`
}

// Candidate is one possible duplicate pair.
type Candidate struct {
	Left    Ref        `json:"left"`
	Right   Ref        `json:"right"`
	Total   int        `json:"total"`
	Email   FieldScore `json:"email"`
	Phone   FieldScore `json:"phone"`
	Name    FieldScore `json:"name"`
	Company FieldScore `json:"company"`
}

// CandidateQuery filters and paginates candidates.
type CandidateQuery struct {
	Source   string `json:"source,omitempty"`
	MinScore int    `json:"min_score,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// CandidatePage returns one page plus the total count after filtering.
type CandidatePage struct {
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Items []Candidate `json:"items"`
}

func fieldScore(left, right string, matched bool, points int) FieldScore {
	fs := FieldScore{Left: left, Right: right}
	if matched {
		fs.Matched = true
		fs.Score = points
	}
	return fs
}

func scorePair(a, b *storedRecord) Candidate {
	c := Candidate{Left: Ref{a.Source, a.ID}, Right: Ref{b.Source, b.ID}}
	an, bn := a.Norm, b.Norm
	if an.email != "" && bn.email != "" && an.email == bn.email {
		c.Email = fieldScore(a.Email, b.Email, true, 60)
	} else {
		c.Email = fieldScore(a.Email, b.Email, false, 0)
	}
	if an.phone != "" && bn.phone != "" && an.phone == bn.phone {
		c.Phone = fieldScore(a.Phone, b.Phone, true, 60)
	} else {
		c.Phone = fieldScore(a.Phone, b.Phone, false, 0)
	}
	if an.name != "" && bn.name != "" && an.name == bn.name {
		c.Name = fieldScore(a.Name, b.Name, true, 20)
	} else {
		c.Name = fieldScore(a.Name, b.Name, false, 0)
	}
	if an.company != "" && bn.company != "" && an.company == bn.company {
		c.Company = fieldScore(a.Company, b.Company, true, 20)
	} else {
		c.Company = fieldScore(a.Company, b.Company, false, 0)
	}
	c.Total = c.Email.Score + c.Phone.Score + c.Name.Score + c.Company.Score
	return c
}

// Candidates computes duplicate candidates among records that have not yet
// been merged into an archive.
func (s *Store) Candidates(q CandidateQuery) (CandidatePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	minScore := q.MinScore
	if minScore < 40 {
		minScore = 40
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	var live []*storedRecord
	for _, ref := range s.recordOrder {
		if _, done := s.archived[ref]; done {
			continue
		}
		live = append(live, s.records[ref])
	}
	sort.Slice(live, func(i, j int) bool { return lessRef(Ref{live[i].Source, live[i].ID}, Ref{live[j].Source, live[j].ID}) })

	var all []Candidate
	for i := 0; i < len(live); i++ {
		for j := i + 1; j < len(live); j++ {
			c := scorePair(live[i], live[j])
			if c.Total < minScore {
				continue
			}
			if q.Source != "" && c.Left.Source != q.Source && c.Right.Source != q.Source {
				continue
			}
			all = append(all, c)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Total != all[j].Total {
			return all[i].Total > all[j].Total
		}
		if all[i].Left != all[j].Left {
			return lessRef(all[i].Left, all[j].Left)
		}
		return lessRef(all[i].Right, all[j].Right)
	})

	total := len(all)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := all[start:end]
	if items == nil {
		items = []Candidate{}
	}
	return CandidatePage{Total: total, Page: page, Items: items}, nil
}
