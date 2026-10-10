package database

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// TimestampLayout is how timestamps are stored in SQLite TEXT columns: UTC,
// microsecond precision, fixed width, so text order is time order.
// PostgreSQL parses the same text into timestamptz.
const TimestampLayout = "2006-01-02T15:04:05.000000Z"

// Timestamp is a time.Time that reads and writes the same way on both
// dialects. Use it for every timestamp column parameter and scan target.
// The zero Timestamp is SQL NULL, so it also serves nullable columns; writing
// it to a NOT NULL column fails loudly.
type Timestamp struct{ time.Time }

// Now returns the current time truncated to the stored precision.
func Now() Timestamp { return Timestamp{time.Now().UTC().Truncate(time.Microsecond)} }

// Value implements driver.Valuer.
func (t Timestamp) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.UTC().Format(TimestampLayout), nil
}

// Scan implements sql.Scanner.
func (t *Timestamp) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case time.Time:
		t.Time = v.UTC()
		return nil
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	default:
		return fmt.Errorf("scan timestamp: unsupported type %T", src)
	}
}

func (t *Timestamp) parse(s string) error {
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("scan timestamp %q: %w", s, err)
	}
	t.Time = v.UTC()
	return nil
}
