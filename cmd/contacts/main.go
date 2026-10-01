package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/customer-contact-dedup/internal/contacts"
)

type envelope struct {
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload"`
}

func writeResp(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

func ok(data any) map[string]any {
	return map[string]any{"status": "ok", "data": data}
}

func errResp(kind, msg string) map[string]any {
	return map[string]any{"status": "error", "error": kind, "message": msg}
}

type runner func(store *contacts.Store, payload json.RawMessage) (any, error)

func main() {
	dir := flag.String("dir", "", "local data directory (required)")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "missing required -dir")
		os.Exit(2)
	}

	store, err := contacts.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dispatch := map[string]runner{
		"import":     runImport,
		"candidates": runCandidates,
		"record":     runRecord,
		"merge":      runMerge,
		"export":     runExport,
	}

	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			handleLine(store, dispatch, line, out)
			_ = out.Flush()
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func handleLine(store *contacts.Store, dispatch map[string]runner, line []byte, out io.Writer) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		writeResp(out, errResp("format_error", "request is not valid JSON: "+err.Error()))
		return
	}
	if env.Op == "" {
		writeResp(out, errResp("format_error", "missing op"))
		return
	}
	run, known := dispatch[env.Op]
	if !known {
		writeResp(out, errResp("unknown_op", fmt.Sprintf("unknown op %q", env.Op)))
		return
	}
	data, err := run(store, env.Payload)
	if err != nil {
		if _, isBiz := err.(*contacts.BusinessError); isBiz {
			writeResp(out, errResp("rejected", err.Error()))
		} else {
			writeResp(out, errResp("format_error", err.Error()))
		}
		return
	}
	writeResp(out, ok(data))
}

func runImport(store *contacts.Store, payload json.RawMessage) (any, error) {
	var p struct {
		Records []contacts.Record `json:"records"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return store.Import(p.Records)
}

func runCandidates(store *contacts.Store, payload json.RawMessage) (any, error) {
	var q contacts.CandidateQuery
	if err := json.Unmarshal(payload, &q); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return store.Candidates(q)
}

func runRecord(store *contacts.Store, payload json.RawMessage) (any, error) {
	var ref contacts.Ref
	if err := json.Unmarshal(payload, &ref); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return store.Record(ref)
}

func runMerge(store *contacts.Store, payload json.RawMessage) (any, error) {
	var req contacts.MergeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	return store.Merge(req)
}

func runExport(store *contacts.Store, payload json.RawMessage) (any, error) {
	return store.Export(), nil
}
