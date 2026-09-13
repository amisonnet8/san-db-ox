package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// jsonValue converts one column value, as scanned via database/sql
// (int64/float64/string/[]byte/nil, matching SQLite's own INTEGER/REAL/
// TEXT/BLOB/NULL dynamic typing), into a value json.Marshal can encode
// per spec §7's value table. This is shared between ".mode json"
// (format.go) and Phase 4's stdio protocol
// (.claude/rules/cli-output.md: "実装も1箇所に集約する") -- neither may
// diverge from the other.
//
//   - NULL   -> nil                              -> `null`
//   - INTEGER (int64) -> passed through           -> a bare number
//   - REAL   (float64) -> jsonReal (below)        -> a number, always with "." or "e"
//   - TEXT   (string) -> passed through           -> a JSON string
//   - BLOB   ([]byte) -> a 1-element array holding the Base64 encoding
//
// Any other Go type is a bug (an unexpected driver value type) and is
// reported as an error rather than silently miscoded.
func jsonValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case int64:
		return x, nil
	case float64:
		return jsonReal(x), nil
	case string:
		return x, nil
	case []byte:
		return [1]string{base64.StdEncoding.EncodeToString(x)}, nil
	default:
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
}

// sqlValue is jsonValue's inverse: it converts one value already decoded
// from a stdio request's "params" array (encoding/json's own dynamic
// typing for a json.Unmarshal into `any` -- nil/bool/float64/string/
// []any/map[string]any) into a value database/sql accepts as a bind
// parameter, per spec §7's value table ("`params` でも同じ表現を受け
// 付ける"). Only jsonValue's own output shapes are accepted back:
//
//   - null           -> nil                              (NULL)
//   - a number (float64) -> passed through                (INTEGER or REAL, SQLite decides from the value)
//   - a string       -> passed through                    (TEXT)
//   - a 1-element array of a string -> base64-decoded []byte (BLOB)
//
// Anything else (an object, a bool, an array that isn't exactly one
// string) is a protocol violation and reported as bad_request by the
// caller (stdio.go), not silently coerced.
func sqlValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case float64:
		return x, nil
	case string:
		return x, nil
	case []any:
		if len(x) != 1 {
			return nil, fmt.Errorf("BLOB must be a 1-element array, got %d elements", len(x))
		}
		s, ok := x[0].(string)
		if !ok {
			return nil, fmt.Errorf("BLOB array element must be a base64 string, got %T", x[0])
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 BLOB: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unexpected param value type %T", v)
	}
}

// jsonReal renders f as a json.RawMessage guaranteed to contain "." or
// "e"/"E" -- spec §7: "REALは必ず小数点を付けて出力する（88ではなく
// 88.0）"-- since JSON's number syntax draws no distinction between
// integers and reals, and a bare "88" would read back as INTEGER on a
// consumer that infers SQLite's type from JSON's own type system.
// json.RawMessage bypasses encoding/json's own float formatting (which
// would print a whole number like 88.0 as the bare "88"), letting this
// control the literal text.
//
// NaN and +-Inf have no representation in the JSON number grammar at all
// (encoding/json itself refuses to marshal them). SanDBox follows
// sqlite3 CLI's own convention for out-of-range/infinite reals: emit a
// numeric literal (9e999 / -9e999) that overflows every IEEE754 double
// on decode, reproducing +-Inf in a consumer without relying on
// non-standard tokens (Infinity/-Infinity/NaN) many JSON parsers reject
// outright. NaN itself has no such trick (no literal decodes back to
// NaN), so it is rendered as JSON null instead -- the same choice
// spec §7's own NULL row uses, and the closest JSON has to "not a
// [representable] value" (documented here and in
// docs/spec/san-db-ox_spec_ja.md §7's known-constraints note).
func jsonReal(f float64) json.RawMessage {
	switch {
	case math.IsNaN(f):
		return json.RawMessage("null")
	case math.IsInf(f, 1):
		return json.RawMessage("9e999")
	case math.IsInf(f, -1):
		return json.RawMessage("-9e999")
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return json.RawMessage(s)
}
