package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/customer-contact-dedup/internal/contact"
)

type request struct {
	Op       string                  `json:"op"`
	Records  []contact.Record        `json:"records,omitempty"`
	Filter   contact.CandidateFilter `json:"filter,omitempty"`
	RecordID *contact.RecordID       `json:"record_id,omitempty"`
	Merge    *contact.MergeRequest   `json:"merge,omitempty"`
}

type errorResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code"`
	Error     string `json:"error"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "./contacts-data", "local data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := contact.Open(*dir)
	if err != nil {
		return err
	}

	in := bufio.NewReader(stdin)
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			handleLine(store, line, out)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				writeErrorTo(out, "format_error", "read error: "+err.Error())
			}
			return nil
		}
	}
}

func handleLine(store *contact.Store, line []byte, out *bufio.Writer) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		writeErrorTo(out, "format_error", "invalid JSON: "+err.Error())
		return
	}
	switch req.Op {
	case "import":
		res, err := store.Import(req.Records)
		respond(out, "import_result", res, err)
	case "candidates":
		res, err := store.FindCandidates(req.Filter)
		respond(out, "candidates", res, err)
	case "get_record":
		if req.RecordID == nil {
			writeErrorTo(out, "format_error", "record_id is required")
			return
		}
		res, err := store.GetRecord(*req.RecordID)
		respond(out, "record", res, err)
	case "merge":
		if req.Merge == nil {
			writeErrorTo(out, "format_error", "merge is required")
			return
		}
		res, err := store.Merge(*req.Merge)
		respond(out, "profile", res, err)
	case "export":
		respond(out, "export", store.Export(), nil)
	default:
		writeErrorTo(out, "format_error", fmt.Sprintf("unknown op: %q", req.Op))
	}
}

func respond(out *bufio.Writer, kind string, payload any, err error) {
	if err != nil {
		code := "invalid_request"
		switch {
		case errors.Is(err, contact.ErrConflict):
			code = "business_rejected"
		case errors.Is(err, contact.ErrInvalid):
			code = "invalid_request"
		default:
			code = "internal_error"
		}
		writeErrorTo(out, code, err.Error())
		return
	}
	writeOK(out, kind, payload)
}

func writeOK(out *bufio.Writer, kind string, payload any) {
	b, err := json.Marshal(map[string]any{"ok": true, "type": kind, kind: payload})
	if err != nil {
		writeErrorTo(out, "internal_error", err.Error())
		return
	}
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}

func writeErrorTo(out *bufio.Writer, code, msg string) {
	b, _ := json.Marshal(errorResponse{OK: false, ErrorCode: code, Error: msg})
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}
