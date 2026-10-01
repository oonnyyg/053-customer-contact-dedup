package contact

import (
	"errors"
	"testing"
)

func rec(source, id, name, company, email, phone string) Record {
	return Record{Source: source, ID: id, Name: name, Company: company, Email: email, Phone: phone}
}

func openTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func mustImport(t *testing.T, s *Store, rs []Record) *ImportResult {
	t.Helper()
	res, err := s.Import(rs)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return res
}

func TestImportValidationDedupAndConflict(t *testing.T) {
	s := openTempStore(t)

	// 缺少必填字段：整批拒绝
	_, err := s.Import([]Record{rec("a", "1", "Alice", "", "", ""), rec("b", "2", "", "", "", "")})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}

	// 首次导入
	res := mustImport(t, s, []Record{
		rec("a", "1", "  Alice   Wang ", " Acme ", "Alice@X.com ", "+86 138-0000-1234"),
		rec("b", "2", "Bob", "", "", ""),
	})
	if res.Added != 2 || res.Skipped != 0 {
		t.Fatalf("unexpected import result %+v", res)
	}

	// 原文完全相同：跳过不新增；大小写不同的邮箱视为冲突
	res = mustImport(t, s, []Record{
		rec("a", "1", "  Alice   Wang ", " Acme ", "Alice@X.com ", "+86 138-0000-1234"),
	})
	if res.Added != 0 || res.Skipped != 1 {
		t.Fatalf("identical import should skip, got %+v", res)
	}
	_, err = s.Import([]Record{rec("a", "1", "  Alice   Wang ", " Acme ", "alice@x.com", "+86 138-0000-1234")})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("raw-content conflict should reject, got %v", err)
	}
	// 整批拒绝不落地：此前的新记录也不应入库
	_, err = s.Import([]Record{rec("c", "3", "Carol", "", "", ""), rec("a", "1", "Different", "", "", "")})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	if v, _ := s.GetRecord(RecordID{Source: "c", ID: "3"}); v != nil {
		t.Fatal("rejected batch must not persist any record")
	}
}

func TestCandidateScoringSortingPaging(t *testing.T) {
	s := openTempStore(t)
	mustImport(t, s, []Record{
		// 邮箱+电话 120
		rec("a", "1", "Alice", "Acme", "alice@x.com", "138-0000-1234"),
		rec("z", "9", "A L I C E", "Other", "ALICE@x.com", "(138) 0000 1234"),
		// 电话 60
		rec("m", "5", "Nobody", "", "", "13800001234"),
		// 仅姓名 20，不能成候选
		rec("n", "6", "alice", "", "", ""),
		// 公司+姓名 40
		rec("a", "2", "ALICE", "acme", "", ""),
	})

	page, err := s.FindCandidates(CandidateFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 {
		t.Fatalf("total = %d, want 4", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("page size = %d", len(page.Items))
	}
	if page.Items[0].Total != 120 || page.Items[1].Total != 60 {
		t.Fatalf("unexpected order: %+v %+v", page.Items[0], page.Items[1])
	}
	if page.Items[0].Left.Source != "a" || page.Items[0].Left.ID != "1" {
		t.Fatalf("left should be lex smaller, got %+v", page.Items[0].Left)
	}
	// 第二页
	page2, _ := s.FindCandidates(CandidateFilter{Page: 2, PageSize: 2})
	if len(page2.Items) != 2 || page2.Items[0].Total != 60 || page2.Items[1].Total != 40 {
		t.Fatalf("page2 unexpected: %+v", page2.Items)
	}
	if page2.Items[0].Left.Source != "m" || page2.Items[0].Left.ID != "5" {
		t.Fatalf("tie-break ordering wrong: %+v", page2.Items[0])
	}
	page3, _ := s.FindCandidates(CandidateFilter{Page: 3, PageSize: 2})
	if len(page3.Items) != 0 {
		t.Fatalf("page3 should be empty: %+v", page3.Items)
	}

	// 按一侧来源筛选
	f, _ := s.FindCandidates(CandidateFilter{Source: "m"})
	if f.Total != 2 {
		t.Fatalf("source filter total = %d", f.Total)
	}
	// 最低分 120
	h, _ := s.FindCandidates(CandidateFilter{MinScore: 120})
	if h.Total != 1 {
		t.Fatalf("min score total = %d", h.Total)
	}
	// 40 以下视为 40
	low, _ := s.FindCandidates(CandidateFilter{MinScore: 1})
	if low.Total != 4 {
		t.Fatalf("low threshold total = %d", low.Total)
	}
}

func TestMergeChoicesAndSourceTracking(t *testing.T) {
	s := openTempStore(t)
	mustImport(t, s, []Record{
		rec("a", "1", "Alice Wang", "Acme", "alice@x.com", "13800001234"),
		rec("b", "2", "Alice", "Acme Corp", "alice@x.com", "13900000000"),
	})

	// 缺选择（姓名、公司、电话冲突）应拒绝
	_, err := s.Merge(MergeRequest{Sources: []RecordID{{Source: "a", ID: "1"}, {Source: "b", ID: "2"}}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict for missing choice, got %v", err)
	}

	// 选择了组外记录应拒绝
	mustImport(t, s, []Record{rec("c", "3", "Third", "", "", "")})
	_, err = s.Merge(MergeRequest{
		Sources: []RecordID{{Source: "a", ID: "1"}, {Source: "b", ID: "2"}},
		Choices: map[string]RecordID{
			"name":    {Source: "c", ID: "3"},
			"company": {Source: "c", ID: "3"},
			"phone":   {Source: "c", ID: "3"},
		},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid for out-of-group choice, got %v", err)
	}

	// 正确合并；邮箱只有一种规范值，自动取字典序最小的原文
	p, err := s.Merge(MergeRequest{
		Sources: []RecordID{{Source: "b", ID: "2"}, {Source: "a", ID: "1"}},
		Choices: map[string]RecordID{
			"name":    {Source: "b", ID: "2"},
			"company": {Source: "a", ID: "1"},
			"phone":   {Source: "b", ID: "2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfileID != "P0001" {
		t.Fatalf("profile id = %s", p.ProfileID)
	}
	if p.Name != "Alice" || p.Company != "Acme" || p.Email != "alice@x.com" || p.Phone != "13900000000" {
		t.Fatalf("unexpected profile values %+v", p)
	}
	if p.Fields["email"].Source != "a" || p.Fields["name"].Source != "b" {
		t.Fatalf("field sources wrong: %+v", p.Fields)
	}

	// 源记录不得归入多个档案
	_, err = s.Merge(MergeRequest{Sources: []RecordID{{Source: "a", ID: "1"}, {Source: "c", ID: "3"}}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("re-merge should be rejected, got %v", err)
	}

	// 证据：已采用字段 used=true，未采用的原文仍可查
	v, err := s.GetRecord(RecordID{Source: "a", ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Archived || v.ProfileID != "P0001" {
		t.Fatalf("view wrong: %+v", v)
	}
	usedPhone := false
	for _, e := range v.Evidence {
		if e.Field == "phone" && e.Used {
			usedPhone = true
		}
	}
	if usedPhone {
		t.Fatal("a/1 phone should be recorded but not used")
	}
}

func TestReopenAndExport(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustImport(t, s, []Record{
		rec("a", "1", "Alice", "", "alice@x.com", ""),
		rec("b", "2", "Alice", "", "alice@x.com", ""),
		rec("c", "3", "Carol", "", "", ""),
	})
	p, err := s.Merge(MergeRequest{Sources: []RecordID{{Source: "b", ID: "2"}, {Source: "a", ID: "1"}}})
	if err != nil {
		t.Fatal(err)
	}

	// 正常退出后重开
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ex := s2.Export()
	if len(ex.Profiles) != 1 || ex.Profiles[0].ProfileID != p.ProfileID {
		t.Fatalf("profiles after reopen: %+v", ex.Profiles)
	}
	if ex.Profiles[0].Sources[0].Source != "a" {
		t.Fatalf("sources should be sorted: %+v", ex.Profiles[0].Sources)
	}
	if len(ex.Unmerged) != 1 || ex.Unmerged[0].Source != "c" {
		t.Fatalf("unmerged after reopen: %+v", ex.Unmerged)
	}

	// 重开后新档案编号继续递增；单条记录合并应拒绝
	if _, err := s2.Merge(MergeRequest{Sources: []RecordID{{Source: "c", ID: "3"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("single-record merge should be invalid, got %v", err)
	}
	mustImport(t, s2, []Record{rec("d", "4", "Dave", "", "dave@x.com", "")})
	p2, err := s2.Merge(MergeRequest{
		Sources: []RecordID{{Source: "c", ID: "3"}, {Source: "d", ID: "4"}},
		Choices: map[string]RecordID{"name": {Source: "d", ID: "4"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p2.ProfileID != "P0002" {
		t.Fatalf("second profile id = %s", p2.ProfileID)
	}
}
