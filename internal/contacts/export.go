package contacts

import "sort"

// ExportData contains confirmed archives and the remaining unmerged
// contacts, each sorted by a stable identifier.
type ExportData struct {
	Archives []Archive `json:"archives"`
	Unmerged []Record  `json:"unmerged"`
}

// Export returns archives sorted by archive id and unmerged records sorted
// by source/id. Records already consumed by an archive are never repeated.
func (s *Store) Export() ExportData {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := append([]string(nil), s.archiveOrder...)
	sort.Strings(ids)
	out := ExportData{Archives: []Archive{}, Unmerged: []Record{}}
	for _, id := range ids {
		out.Archives = append(out.Archives, s.archives[id].Archive)
	}
	var refs []Ref
	for _, ref := range s.recordOrder {
		if _, done := s.archived[ref]; !done {
			refs = append(refs, ref)
		}
	}
	sortRefs(refs)
	for _, ref := range refs {
		out.Unmerged = append(out.Unmerged, s.records[ref].Record)
	}
	return out
}
