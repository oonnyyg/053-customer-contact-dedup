package contacts

import "testing"

func TestNormalization(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"name", normalizeName("  Alice   Mary  Smith "), "alice mary smith"},
		{"company", normalizeCompany("\tAcme\tCorp\n"), "acme corp"},
		{"email", normalizeEmail("  Bob@Example.COM "), "bob@example.com"},
		{"phone", normalizePhone("+86 138-0000-(1234)"), "+8613800001234"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestImportDedupAndConflict(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Import([]Record{
		{Source: "a", ID: "1", Name: "Alice", Email: "x@y.com"},
		{Source: "a", ID: "1", Name: "Alice", Email: "x@y.com"},
	})
	if err != nil || res.Imported != 1 || res.Duplicate != 1 {
		t.Fatalf("dedup import: %+v %v", res, err)
	}

	// Whitespace-only differences in normalized name/company still compare raw.
	_, err = s.Import([]Record{{Source: "a", ID: "1", Name: "Bob"}})
	if _, ok := err.(*BusinessError); !ok {
		t.Fatalf("expected business conflict, got %v", err)
	}

	// Missing name fails whole batch validation.
	_, err = s.Import([]Record{{Source: "b", ID: "1", Name: ""}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCandidateScoringSortingPaging(t *testing.T) {
	s, _ := Open(t.TempDir())
	_, _ = s.Import([]Record{
		{Source: "a", ID: "1", Name: "Alice Smith", Company: "Acme", Email: "a@x.com", Phone: "111"},
		{Source: "b", ID: "1", Name: "ALICE  smith", Company: " acme ", Email: "A@X.COM", Phone: "(111)"},
		{Source: "c", ID: "1", Name: "Alice Smith", Company: "Other"},
		{Source: "d", ID: "1", Name: "Dan", Phone: "222"},
		{Source: "d", ID: "2", Name: "Eve", Phone: "222"},
	})
	page, err := s.Candidates(CandidateQuery{Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2", page.Total)
	}
	top := page.Items[0]
	if !(top.Left == (Ref{"a", "1"}) && top.Right == (Ref{"b", "1"})) {
		t.Fatalf("unexpected top pair: %+v", top)
	}
	if top.Total != 160 {
		t.Fatalf("top total = %d want 160", top.Total)
	}
	page2, _ := s.Candidates(CandidateQuery{Page: 2, PageSize: 1})
	if page2.Items[0].Total != 60 || page2.Items[0].Left.Source != "d" {
		t.Fatalf("unexpected page 2: %+v", page2.Items)
	}

	// Name-only pair (a/1 vs c/1 scores 20) must not appear.
	full, _ := s.Candidates(CandidateQuery{PageSize: 50})
	for _, c := range full.Items {
		if (c.Left == (Ref{"a", "1"}) && c.Right == (Ref{"c", "1"})) ||
			(c.Left == (Ref{"c", "1"}) && c.Right == (Ref{"a", "1"})) {
			t.Fatal("name-only pair listed as candidate")
		}
	}

	// Source filter on either side.
	f, _ := s.Candidates(CandidateQuery{Source: "c", PageSize: 50})
	if f.Total != 0 {
		t.Fatalf("c only shares name; expected 0 got %d", f.Total)
	}
	f2, _ := s.Candidates(CandidateQuery{Source: "e", MinScore: 40, PageSize: 50})
	if f2.Total != 0 {
		t.Fatalf("unknown source should filter all, got %d", f2.Total)
	}
}

func TestMergeChoicesAndRejections(t *testing.T) {
	s, _ := Open(t.TempDir())
	_, _ = s.Import([]Record{
		{Source: "a", ID: "1", Name: "Alice", Company: "Acme", Email: "a@x.com"},
		{Source: "b", ID: "1", Name: "Alice", Company: "Beta", Email: "b@x.com"},
		{Source: "c", ID: "1", Name: "Carol"},
		{Source: "c", ID: "2", Name: "Dave"},
	})

	// Single record rejected.
	if _, err := s.Merge(MergeRequest{Records: []Ref{{"c", "1"}}}); err == nil {
		t.Fatal("single-record merge should be rejected")
	}
	// Conflicts without choices rejected.
	if _, err := s.Merge(MergeRequest{Records: []Ref{{"a", "1"}, {"b", "1"}}}); err == nil {
		t.Fatal("merge without conflict choices should be rejected")
	}
	// Choice referencing an outsider rejected.
	_, err := s.Merge(MergeRequest{
		Records: []Ref{{"a", "1"}, {"b", "1"}},
		Choices: map[string]FieldChoice{
			"company": {Source: "c", ID: "1"},
			"email":   {Source: "b", ID: "1"},
		},
	})
	if err == nil {
		t.Fatal("out-of-group choice should be rejected")
	}

	arch, err := s.Merge(MergeRequest{
		Records: []Ref{{"b", "1"}, {"a", "1"}},
		Choices: map[string]FieldChoice{
			"company": {Source: "a", ID: "1"},
			"email":   {Source: "b", ID: "1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if arch.ArchiveID != "A1" || arch.Company != "Acme" || arch.Email != "b@x.com" {
		t.Fatalf("bad archive: %+v", arch)
	}
	prov := map[string]FieldSource{}
	for _, p := range arch.Provenance {
		prov[p.Field] = p
	}
	if prov["company"].Source != "a" || prov["email"].Source != "b" || prov["name"].Source != "a" {
		t.Fatalf("bad provenance: %+v", arch.Provenance)
	}

	// Re-merging an archived record is rejected.
	_, err = s.Merge(MergeRequest{Records: []Ref{{"a", "1"}, {"c", "1"}}})
	if err == nil {
		t.Fatal("archived record must not join another archive")
	}

	// All-empty optional fields stay empty.
	_, _ = s.Import([]Record{
		{Source: "d", ID: "1", Name: "Erin"},
		{Source: "d", ID: "2", Name: "Erin"},
	})
	arch2, err := s.Merge(MergeRequest{Records: []Ref{{"d", "1"}, {"d", "2"}}})
	if err != nil || arch2.Company != "" || arch2.Email != "" || arch2.Phone != "" {
		t.Fatalf("empty-field merge: %+v %v", arch2, err)
	}

	// Originals still queryable.
	d, err := s.Record(Ref{"a", "1"})
	if err != nil || d.ArchivedIn != "A1" || d.Email != "a@x.com" {
		t.Fatalf("record lookup: %+v %v", d, err)
	}
}

func TestExportAndReopen(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_, _ = s.Import([]Record{
		{Source: "a", ID: "1", Name: "Alice", Email: "a@x.com"},
		{Source: "a", ID: "2", Name: "Alice", Email: "a@x.com"},
		{Source: "b", ID: "9", Name: "Solo"},
	})
	_, err := s.Merge(MergeRequest{Records: []Ref{{"a", "2"}, {"a", "1"}}})
	if err != nil {
		t.Fatal(err)
	}

	out := s.Export()
	if len(out.Archives) != 1 || len(out.Unmerged) != 1 || out.Unmerged[0].ID != "9" {
		t.Fatalf("bad export: %+v", out)
	}

	// Reopen and continue.
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	out2 := s2.Export()
	if len(out2.Archives) != 1 || len(out2.Unmerged) != 1 {
		t.Fatalf("bad export after reopen: %+v", out2)
	}
	// New archive gets a fresh unique id.
	_, _ = s2.Import([]Record{{Source: "c", ID: "1", Name: "Solo"}, {Source: "b", ID: "9", Name: "Solo"}})
	arch, err := s2.Merge(MergeRequest{Records: []Ref{{"b", "9"}, {"c", "1"}}})
	if err != nil || arch.ArchiveID != "A2" {
		t.Fatalf("expected A2 after reopen, got %+v %v", arch, err)
	}
}
