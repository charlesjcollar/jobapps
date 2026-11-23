package graph

import (
	"encoding/json"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date is the GraphQL Date scalar: a calendar date formatted YYYY-MM-DD.
type Date struct{ time.Time }

func (Date) ImplementsGraphQLType(name string) bool { return name == "Date" }

func (d *Date) UnmarshalGraphQL(input any) error {
	s, ok := input.(string)
	if !ok {
		return fmt.Errorf("Date must be a string, got %T", input)
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("Date must be formatted YYYY-MM-DD, got %q", s)
	}
	d.Time = t
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Format(dateLayout))
}

// NullDate is a Date input that tells an omitted value (Set false) apart from
// an explicit null (Set true, Value nil).
type NullDate struct {
	Value *Date
	Set   bool
}

func (NullDate) ImplementsGraphQLType(name string) bool { return name == "Date" }

func (n *NullDate) UnmarshalGraphQL(input any) error {
	n.Set = true
	if input == nil {
		return nil
	}
	n.Value = new(Date)
	return n.Value.UnmarshalGraphQL(input)
}

func (*NullDate) Nullable() {}

// DateTime is the GraphQL DateTime scalar: an RFC 3339 timestamp.
type DateTime struct{ time.Time }

func (DateTime) ImplementsGraphQLType(name string) bool { return name == "DateTime" }

func (d *DateTime) UnmarshalGraphQL(input any) error {
	s, ok := input.(string)
	if !ok {
		return fmt.Errorf("DateTime must be a string, got %T", input)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("DateTime must be RFC 3339, got %q", s)
	}
	d.Time = t
	return nil
}

func (d DateTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.UTC().Format(time.RFC3339))
}
