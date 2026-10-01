package contacts

import (
	"strconv"
)

// RecordDetail returns the original raw record.
type RecordDetail struct {
	Record
	ArchivedIn string `json:"archived_in,omitempty"`
}

// Record returns one original record plus evidence of its archive, if any.
func (s *Store) Record(ref Ref) (RecordDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[ref]
	if !ok {
		return RecordDetail{}, bizError("record %s/%s not found", ref.Source, ref.ID)
	}
	return RecordDetail{Record: r.Record, ArchivedIn: s.archived[ref]}, nil
}

// MergeRequest confirms an archive from at least two unarchived records.
type MergeRequest struct {
	Records []Ref                  `json:"records"`
	Choices map[string]FieldChoice `json:"choices,omitempty"`
}

var mergeFields = []struct {
	name string
	get  func(Record) string
	norm func(Record) string
	out  func(*Archive, string)
}{
	{"name", func(r Record) string { return r.Name }, func(r Record) string { return normalize(r).name }, func(a *Archive, v string) { a.Name = v }},
	{"company", func(r Record) string { return r.Company }, func(r Record) string { return normalize(r).company }, func(a *Archive, v string) { a.Company = v }},
	{"email", func(r Record) string { return r.Email }, func(r Record) string { return normalize(r).email }, func(a *Archive, v string) { a.Email = v }},
	{"phone", func(r Record) string { return r.Phone }, func(r Record) string { return normalize(r).phone }, func(a *Archive, v string) { a.Phone = v }},
}

// Merge creates one archive from the named records.
func (s *Store) Merge(req MergeRequest) (Archive, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(req.Records) < 2 {
		return Archive{}, bizError("at least two records are required to merge")
	}

	seen := map[Ref]bool{}
	var refs []Ref
	var members []Record
	for _, ref := range req.Records {
		if seen[ref] {
			return Archive{}, bizError("record %s/%s listed more than once", ref.Source, ref.ID)
		}
		seen[ref] = true
		r, ok := s.records[ref]
		if !ok {
			return Archive{}, bizError("record %s/%s not found", ref.Source, ref.ID)
		}
		if aid, done := s.archived[ref]; done {
			return Archive{}, bizError("record %s/%s already archived in %s", ref.Source, ref.ID, aid)
		}
		refs = append(refs, ref)
		members = append(members, r.Record)
	}
	sortRefs(refs)
	byRef := map[Ref]Record{}
	for _, r := range members {
		byRef[Ref{r.Source, r.ID}] = r
	}

	archive := Archive{
		ArchiveID: "A" + strconv.Itoa(s.nextArchiveN),
		Sources:   append([]Ref(nil), refs...),
	}

	for _, f := range mergeFields {
		type nv struct {
			n   string
			ref Ref
		}
		var distinct []nv
		values := map[string]Ref{}
		for _, ref := range refs {
			r := byRef[ref]
			n := f.norm(r)
			if n == "" {
				continue
			}
			if _, ok := values[n]; !ok {
				values[n] = ref
				distinct = append(distinct, nv{n, ref})
			}
		}
		if len(distinct) == 0 {
			continue
		}
		var chosen Ref
		if len(distinct) == 1 {
			chosen = refs[0]
			for _, ref := range refs {
				if f.norm(byRef[ref]) == distinct[0].n {
					chosen = ref
					break
				}
			}
		} else {
			c, provided := req.Choices[f.name]
			if !provided {
				return Archive{}, bizError("field %q has conflicting values; choice is required", f.name)
			}
			chosen = Ref{c.Source, c.ID}
			if !seen[chosen] {
				return Archive{}, bizError("choice for field %q references record outside the merge group", f.name)
			}
			if f.norm(byRef[chosen]) == "" {
				return Archive{}, bizError("choice for field %q references a record with an empty %s", f.name, f.name)
			}
		}
		f.out(&archive, f.get(byRef[chosen]))
		archive.Provenance = append(archive.Provenance, FieldSource{Field: f.name, Source: chosen.Source, ID: chosen.ID})
	}

	aid := archive.ArchiveID
	s.archives[aid] = &persistedArchive{Archive: archive, Members: members}
	s.archiveOrder = append(s.archiveOrder, aid)
	for _, ref := range refs {
		s.archived[ref] = aid
	}
	s.nextArchiveN++
	if err := s.persistLocked(); err != nil {
		return Archive{}, err
	}
	return archive, nil
}
