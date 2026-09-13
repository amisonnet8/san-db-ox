package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/amisonnet8/san-db-ox/engine"
)

// stdioRequest is one line of client input (spec §7). Every op-specific
// field is optional at the JSON level (each op reads only the ones it
// needs); a field an op requires but that is missing/empty is reported
// as bad_request by that op's handler below, not by unmarshaling itself.
type stdioRequest struct {
	ID        json.RawMessage `json:"id,omitempty"`
	Op        string          `json:"op"`
	SQL       string          `json:"sql,omitempty"`
	Params    []any           `json:"params,omitempty"`
	Filename  string          `json:"filename,omitempty"`
	SQLite    bool            `json:"sqlite,omitempty"`
	Timestamp bool            `json:"timestamp,omitempty"`
	Path      string          `json:"path,omitempty"`
	Table     string          `json:"table,omitempty"`
	Pattern   string          `json:"pattern,omitempty"`
}

// stdioResp is one response line's fields beyond "ok"/"id"/"error",
// built up by each op handler and marshaled by writeLine. A plain
// map[string]any rather than one struct with every op's fields as
// optional members: the op table (spec §7) gives each op its own
// response shape, and a single struct would need as many omitempty
// fields as there are distinct op responses combined.
type stdioResp map[string]any

// runStdio serves the stdio protocol (spec §7): opens this run's Session
// (openSession, repl.go, applying --read-only's query_only pragma the
// same way REPL/batch do), emits the hello line, then reads one JSON
// request per line until stdin's EOF and responds to each with exactly
// one JSON line -- flushed immediately, since a client may be blocked
// waiting for it (the protocol's own flush-per-message contract,
// docs/usage/stdio-protocol_ja.md). stdout carries protocol responses
// only; nothing else may be written to it (spec §0).
func runStdio(db *engine.DB, self string, opts *options, in io.Reader, out, errw io.Writer) int {
	sess, err := openSession(context.Background(), db, opts)
	if err != nil {
		fmt.Fprintln(errw, "san-db-ox:", err)
		return 1
	}
	defer sess.Close()

	r := newRepl(db, sess, self, opts, false, out, errw)

	// Best-effort: a failure here (self somehow unreadable after having
	// just been exec'd from) only degrades the "inspect" op's
	// version/data_length fields to their has_data=false shape, so it is
	// not itself fatal to serving the rest of the protocol.
	info, _ := engine.Inspect(self)

	w := bufio.NewWriter(out)
	writeLine(w, map[string]any{"protocol": 1, "version": version, "product": "SanDBox"})

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var req stdioRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeLine(w, errResp(nil, "bad_request", err.Error()))
			continue
		}

		resp := r.dispatchOp(&req, info)
		writeLine(w, resp)

		if (req.Op == "close" || req.Op == "overwrite") && isOK(resp) {
			return 0
		}
	}
	return 0 // stdin EOF: process ends normally, no save (spec §4, §7)
}

// writeLine marshals v as one JSON line and flushes it immediately --
// the protocol's flush-per-message contract is this process's own
// obligation to keep (docs/usage/stdio-protocol_ja.md's "実装者への
// 注意"), not just something asked of client drivers. A marshal error
// here is a bug in this package (every v is built from JSON-safe values
// already), so it goes to stderr rather than trying to fabricate another
// response line for it.
func writeLine(w *bufio.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(w, `{"ok":false,"error":{"code":"io_error","message":"internal: failed to encode response"}}`)
		w.Flush()
		return
	}
	w.Write(b)
	w.WriteByte('\n')
	w.Flush()
}

func isOK(resp stdioResp) bool {
	ok, _ := resp["ok"].(bool)
	return ok
}

// okResp builds a successful response, echoing id back verbatim (spec
// §7: "リクエストに id フィールドを含めた場合、対応するレスポンスに
// そのまま echo back する"). fields is merged in as-is; nil is fine for
// an op with nothing beyond "ok"/"id" to report (close, overwrite).
func okResp(id json.RawMessage, fields map[string]any) stdioResp {
	resp := stdioResp{"ok": true}
	if len(id) > 0 {
		resp["id"] = id
	}
	for k, v := range fields {
		resp[k] = v
	}
	return resp
}

// errResp builds an error response (spec §7's error shape and code
// list, .claude/rules/... none needed -- the code assignment rationale
// lives in the spec itself, §7).
func errResp(id json.RawMessage, code, message string) stdioResp {
	resp := stdioResp{"ok": false, "error": map[string]any{"code": code, "message": message}}
	if len(id) > 0 {
		resp["id"] = id
	}
	return resp
}

// errFromSaveOp classifies a doSnapshot/doLoad/doOverwrite error
// (dotcmd.go) into the stdio error code spec §7 assigns it: ErrReadOnly
// specifically becomes read_only (checked first, before the ordinary
// io_error catch-all -- .claude/rules convention of a specific check
// before a general one), and everything else -- Windows sharing
// violations, missing files, ErrBusy, ErrUnsupportedFile, and so on --
// becomes io_error, per spec §7's "snapshot/load/overwrite/dumpが
// ファイルI/O・パス絡みの理由で失敗した場合はio_error".
func errFromSaveOp(id json.RawMessage, err error) stdioResp {
	if errors.Is(err, ErrReadOnly) {
		return errResp(id, "read_only", err.Error())
	}
	return errResp(id, "io_error", err.Error())
}

// dispatchOp runs one already-decoded request and returns its response.
// query/exec/tables/schema/dump/snapshot/load/overwrite reuse the exact
// same logic the REPL's dot commands do (dotcmd.go, dump.go) --
// .claude/rules/directory-structure.md's "3つの実行モードは同じドット
// コマンド実装を共有する" applied to stdio's op handlers.
func (r *repl) dispatchOp(req *stdioRequest, info *engine.FileInfo) stdioResp {
	switch req.Op {
	case "query":
		return r.opQuery(req)
	case "exec":
		return r.opExec(req)
	case "snapshot":
		return r.opSnapshot(req)
	case "load":
		return r.opLoad(req)
	case "inspect":
		return r.opInspect(req, info)
	case "tables":
		return r.opTables(req)
	case "schema":
		return r.opSchema(req)
	case "dump":
		return r.opDump(req)
	case "overwrite":
		return r.opOverwrite(req)
	case "close":
		return okResp(req.ID, nil)
	default:
		return errResp(req.ID, "unsupported_op", fmt.Sprintf("unknown op %q", req.Op))
	}
}

// paramsToArgs converts a request's already-JSON-decoded "params" into
// database/sql bind arguments via sqlValue (value.go), spec §7's JSON
// value table read in reverse.
func paramsToArgs(params []any) ([]any, error) {
	args := make([]any, len(params))
	for i, p := range params {
		v, err := sqlValue(p)
		if err != nil {
			return nil, fmt.Errorf("params[%d]: %w", i, err)
		}
		args[i] = v
	}
	return args, nil
}

// opQuery implements the "query" op (spec §7): run req.SQL on r.sess and
// return every row of the result set in one response (no cursor -- spec
// §7: "行を逐次取得するカーソル機構は持たない"). Values are converted
// via jsonValue (value.go), the same conversion ".mode json" uses
// (format.go), so the two never diverge.
func (r *repl) opQuery(req *stdioRequest) stdioResp {
	args, err := paramsToArgs(req.Params)
	if err != nil {
		return errResp(req.ID, "bad_request", err.Error())
	}

	rows, err := r.sess.Query(req.SQL, args...)
	if err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}

	outRows := [][]any{}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return errResp(req.ID, "sqlite_error", err.Error())
		}
		row := make([]any, len(cols))
		for i, v := range vals {
			jv, err := jsonValue(v)
			if err != nil {
				return errResp(req.ID, "sqlite_error", err.Error())
			}
			row[i] = jv
		}
		outRows = append(outRows, row)
	}
	if err := rows.Err(); err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}

	return okResp(req.ID, map[string]any{"columns": cols, "rows": outRows})
}

// opExec implements the "exec" op (spec §7): run req.SQL (DML/DDL/TCL --
// BEGIN/COMMIT/ROLLBACK included, spec §7: "専用の op を設けず、exec で
// そのまま送る") on r.sess and report the standard database/sql result
// fields.
func (r *repl) opExec(req *stdioRequest) stdioResp {
	args, err := paramsToArgs(req.Params)
	if err != nil {
		return errResp(req.ID, "bad_request", err.Error())
	}

	res, err := r.sess.Exec(req.SQL, args...)
	if err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}
	affected, _ := res.RowsAffected() // an error here means the driver doesn't support it; modernc.org/sqlite always does
	lastID, _ := res.LastInsertId()   // same
	return okResp(req.ID, map[string]any{"rows_affected": affected, "last_insert_id": lastID})
}

// opSnapshot implements the "snapshot" op (spec §7), reusing doSnapshot
// (dotcmd.go) -- the same logic ".snapshot" runs, including its
// --read-only check.
func (r *repl) opSnapshot(req *stdioRequest) stdioResp {
	path, err := r.doSnapshot(req.Filename, req.SQLite, req.Timestamp)
	if err != nil {
		return errFromSaveOp(req.ID, err)
	}
	return okResp(req.ID, map[string]any{"path": path})
}

// opLoad implements the "load" op (spec §7), reusing doLoad (dotcmd.go).
// The footer-Version-mismatch warning doLoad may return has no field in
// spec §7's response shape, so it is intentionally dropped here (the
// REPL's ".load" prints it to stderr instead, dotcmd.go's cmdLoad).
func (r *repl) opLoad(req *stdioRequest) stdioResp {
	if req.Path == "" {
		return errResp(req.ID, "bad_request", "missing required field: path")
	}
	if _, err := r.doLoad(req.Path); err != nil {
		return errFromSaveOp(req.ID, err)
	}
	return okResp(req.ID, nil)
}

// opOverwrite implements the "overwrite" op (spec §4, §7): reuses
// doOverwrite (dotcmd.go) rather than cmdOverwrite, since cmdOverwrite
// writes a REPL-facing confirmation line to r.out -- exactly the stdout
// stdio must never carry outside protocol responses (spec §0). On
// success, runStdio's caller ends the process after this response is
// written and flushed (spec §4: "応答を1行返してからプロセスを
// 終了する").
func (r *repl) opOverwrite(req *stdioRequest) stdioResp {
	if err := r.doOverwrite(); err != nil {
		return errFromSaveOp(req.ID, err)
	}
	return okResp(req.ID, nil)
}

// opTables implements the "tables" op (spec §7), reusing listTables
// (dotcmd.go).
func (r *repl) opTables(req *stdioRequest) stdioResp {
	names, err := r.listTables()
	if err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}
	if names == nil {
		names = []string{}
	}
	return okResp(req.ID, map[string]any{"tables": names})
}

// opSchema implements the "schema" op (spec §7), reusing schemaSQL
// (dotcmd.go). req.Table is read straight off the request (empty means
// "every object", schemaSQL's own convention).
func (r *repl) opSchema(req *stdioRequest) stdioResp {
	stmts, err := r.schemaSQL(req.Table)
	if err != nil {
		return errResp(req.ID, "sqlite_error", err.Error())
	}
	if stmts == nil {
		stmts = []string{}
	}
	return okResp(req.ID, map[string]any{"schema": stmts})
}

// opDump implements the "dump" op (spec §7, added in Phase 4: ".dump" is
// the one dot command with an op despite not appearing in naming.md's
// original table -- ".import" stays without one, spec §7's own note on
// why), reusing dumpSQL (dump.go). pattern defaults to "%" (every table)
// when omitted, matching cmdDump (dotcmd.go... dump.go).
func (r *repl) opDump(req *stdioRequest) stdioResp {
	pattern := req.Pattern
	if pattern == "" {
		pattern = "%"
	}
	sqlText, err := r.dumpSQL(pattern)
	if err != nil {
		return errResp(req.ID, "io_error", err.Error())
	}
	return okResp(req.ID, map[string]any{"sql": sqlText})
}

// opInspect implements the "inspect" op (spec §7): reports this
// process's own state, sourced from the same facts the REPL's startup
// banner shows (banner.go) -- has_data comes from db.HasData() (spec
// §7: "起動時点の事実であり、その後.loadを実行しても変化しない", the
// same invariant HasData's own doc comment states), version/data_length
// come from the one-time engine.Inspect(self) call runStdio made at
// startup (info), and are null when has_data is false (info itself may
// also be nil if that Inspect call failed -- treated the same as
// has_data=false, since there is nothing more informative to report).
func (r *repl) opInspect(req *stdioRequest, info *engine.FileInfo) stdioResp {
	hasData := r.db.HasData()
	fields := map[string]any{
		"has_data":    hasData,
		"version":     nil,
		"data_length": nil,
		"source":      filepath.Base(r.self),
		"read_only":   r.readOnly(),
	}
	if hasData && info != nil {
		fields["version"] = info.Version
		fields["data_length"] = info.DataLength
	}
	return okResp(req.ID, fields)
}
