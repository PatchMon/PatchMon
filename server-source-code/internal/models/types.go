package models

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
)

// JSON is a type alias for jsonb columns. pgx scans jsonb into []byte.
type JSON []byte

// Scan implements sql.Scanner for jsonb columns. Handles NULL.
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
		return nil
	case string:
		*j = []byte(v)
		return nil
	default:
		*j = nil
		return nil
	}
}

// Value implements driver.Valuer for jsonb columns.
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return []byte(j), nil
}

// MarshalJSON implements json.Marshaler.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *JSON) UnmarshalJSON(data []byte) error {
	*j = append((*j)[0:0], data...)
	return nil
}

// Unmarshal into v.
func (j JSON) Unmarshal(v interface{}) error {
	if len(j) == 0 {
		return nil
	}
	return json.Unmarshal(j, v)
}

// StringArray scans PostgreSQL text[] into []string.
type StringArray []string

// Scan implements sql.Scanner for text[].
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	str, ok := value.(string)
	if !ok {
		b, ok := value.([]byte)
		if !ok {
			return nil
		}
		str = string(b)
	}
	str = strings.Trim(str, "{}")
	if str == "" {
		*s = []string{}
		return nil
	}
	*s = strings.Split(str, ",")
	return nil
}

// Value implements driver.Valuer for text[].
func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}
	return "{" + strings.Join(s, ",") + "}", nil
}
