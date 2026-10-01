package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCLI(t *testing.T, input string) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"-dir", dir}, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	var results []map[string]any
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad output JSON %q: %v", sc.Text(), err)
		}
		results = append(results, m)
	}
	return results
}

func TestCLIProtocol(t *testing.T) {
	input := strings.Join([]string{
		`{"op":"import","records":[{"source":"a","id":"1","name":" Alice  Wang ","company":"Acme","email":"Alice@x.com","phone":"138-0000-1234"},{"source":"b","id":"2","name":"Alice","company":"Acme","email":"alice@X.COM","phone":"(138) 0000 1234"}]}`,
		`{"op":"candidates","filter":{"page":1,"page_size":10}}`,
		`{"op":"merge","merge":{"sources":[{"source":"a","id":"1"},{"source":"b","id":"2"}],"choices":{"name":{"source":"a","id":"1"}}}}`,
		`{"op":"get_record","record_id":{"source":"b","id":"2"}}`,
		`{"op":"export"}`,
		`{"op":"bogus"}`,
		`not json`,
		``,
	}, "\n")

	res := runCLI(t, input)
	if len(res) != 7 {
		t.Fatalf("got %d responses: %v", len(res), res)
	}
	if res[0]["ok"] != true || res[0]["type"] != "import_result" {
		t.Fatalf("import response: %v", res[0])
	}
	cand := res[1]["candidates"].(map[string]any)
	if cand["total"].(float64) != 1 {
		t.Fatalf("candidate total: %v", cand["total"])
	}
	if res[2]["type"] != "profile" {
		t.Fatalf("merge response: %v", res[2])
	}
	rec := res[3]["record"].(map[string]any)
	if rec["archived"] != true {
		t.Fatalf("record view: %v", rec)
	}
	ev := rec["evidence"].([]any)
	if len(ev) == 0 {
		t.Fatal("expected evidence entries")
	}
	exp := res[4]["export"].(map[string]any)
	if len(exp["profiles"].([]any)) != 1 || len(exp["unmerged"].([]any)) != 0 {
		t.Fatalf("export: %v", exp)
	}
	if res[5]["error_code"] != "format_error" {
		t.Fatalf("unknown op: %v", res[5])
	}
	if res[6]["error_code"] != "format_error" {
		t.Fatalf("bad json: %v", res[6])
	}
}

func TestCLIBusinessReject(t *testing.T) {
	input := strings.Join([]string{
		`{"op":"import","records":[{"source":"a","id":"1","name":"A","email":"x@y.com","phone":"1"},{"source":"b","id":"2","name":"B","email":"x@y.com","phone":"2"}]}`,
		`{"op":"merge","merge":{"sources":[{"source":"a","id":"1"},{"source":"b","id":"2"}]}}`,
		`{"op":"import","records":[{"source":"c","id":"3","name":""}]}`,
	}, "\n")
	res := runCLI(t, input)
	if res[1]["error_code"] != "business_rejected" {
		t.Fatalf("conflicting field values must be business_rejected: %v", res[1])
	}
	if res[2]["error_code"] != "invalid_request" {
		t.Fatalf("missing name must be invalid_request: %v", res[2])
	}
}
