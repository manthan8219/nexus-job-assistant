package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// scanBool scans a boolean-ish column that may arrive as a native bool
// (Postgres BOOLEAN), as 0/1 ints (SQLite INTEGER), or as their string forms.
// This keeps one query portable across the SQLite and Postgres backends.
type scanBool struct{ v bool }

// Scan implements sql.Scanner.
func (b *scanBool) Scan(src any) error {
	switch x := src.(type) {
	case nil:
		b.v = false
	case bool:
		b.v = x
	case int64:
		b.v = x != 0
	case int:
		b.v = x != 0
	case float64:
		b.v = x != 0
	case []byte:
		return b.Scan(string(x))
	case string:
		t := strings.TrimSpace(x)
		switch {
		case t == "":
			b.v = false
		case strings.EqualFold(t, "false"), t == "0":
			b.v = false
		case strings.EqualFold(t, "true"), t == "1":
			b.v = true
		default:
			n, err := strconv.ParseInt(t, 10, 64)
			if err != nil {
				return fmt.Errorf("scan bool from %q", t)
			}
			b.v = n != 0
		}
	default:
		return fmt.Errorf("scan bool from %T", src)
	}
	return nil
}

// ensure *sql.Scanner is implemented (compile-time check).
var _ sql.Scanner = (*scanBool)(nil)
