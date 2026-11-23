// Package graph serves the GraphQL API defined in schema.graphqls.
package graph

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	graphql "github.com/graph-gophers/graphql-go"

	"jobapps/bff/scan"
	"jobapps/bff/store"
)

//go:embed schema.graphqls
var schemaSDL string

const maxLimit = 500

// NewSchema parses the schema and binds it to resolvers backed by s and scans.
func NewSchema(s *store.Store, scans *scan.Manager) (*graphql.Schema, error) {
	return graphql.ParseSchema(schemaSDL, &Resolver{store: s, scans: scans}, graphql.UseFieldResolvers())
}

// Resolver resolves every Query, Mutation, and Subscription field.
type Resolver struct {
	store *store.Store
	scans *scan.Manager
}

// job is the GraphQL Job type. Fields are matched to the schema by name.
type job struct {
	ID            graphql.ID
	CompanyName   string
	JobTitle      string
	PostingID     string
	Link          string
	PostedDate    *Date
	RetrievedDate DateTime
	Status        string
	LastUpdated   DateTime
	Notes         *string
	ContactName   *string
	ContactEmail  *string
	ContactPhone  *string
	SalaryRange   *string
	Location      *string
}

func toJob(j store.Job) *job {
	out := &job{
		ID:            graphql.ID(strconv.FormatInt(j.ID, 10)),
		CompanyName:   j.CompanyName,
		JobTitle:      j.JobTitle,
		PostingID:     j.PostingID,
		Link:          j.Link,
		RetrievedDate: DateTime{j.RetrievedDate},
		Status:        j.Status,
		LastUpdated:   DateTime{j.LastUpdated},
		Notes:         j.Notes,
		ContactName:   j.ContactName,
		ContactEmail:  j.ContactEmail,
		ContactPhone:  j.ContactPhone,
		SalaryRange:   j.SalaryRange,
		Location:      j.Location,
	}
	if j.PostedDate != nil {
		out.PostedDate = &Date{*j.PostedDate}
	}
	return out
}

func toJobs(js []store.Job) []*job {
	out := make([]*job, len(js))
	for i, j := range js {
		out[i] = toJob(j)
	}
	return out
}

func parseID(id graphql.ID) (int64, error) {
	n, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", id)
	}
	return n, nil
}

// --- Query ---

func (r *Resolver) Job(ctx context.Context, args struct{ ID graphql.ID }) (*job, error) {
	id, err := parseID(args.ID)
	if err != nil {
		return nil, err
	}
	j, err := r.store.Get(ctx, id)
	if err != nil || j == nil {
		return nil, err
	}
	return toJob(*j), nil
}

type jobFilter struct {
	Statuses         *[]string
	ExcludedStatuses *[]string
	CompanyName      *string
	JobTitle         *string
}

type jobSort struct {
	Field     string
	Direction string
}

type jobPage struct {
	Items      []*job
	TotalCount int32
}

func (r *Resolver) Jobs(ctx context.Context, args struct {
	Filter *jobFilter
	Sort   *jobSort
	Limit  int32 // schema defaults: 50 and 0
	Offset int32
}) (*jobPage, error) {
	p := store.ListParams{SortField: store.SortRetrievedDate, Desc: true}

	if args.Limit < 1 || args.Limit > maxLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", maxLimit)
	}
	if args.Offset < 0 {
		return nil, errors.New("offset can't be negative")
	}
	p.Limit = int(args.Limit)
	p.Offset = int(args.Offset)
	if args.Sort != nil {
		p.SortField = args.Sort.Field
		p.Desc = args.Sort.Direction == "DESC"
	}
	if f := args.Filter; f != nil {
		if f.Statuses != nil {
			p.Statuses = *f.Statuses
		}
		if f.ExcludedStatuses != nil {
			p.ExcludedStatuses = *f.ExcludedStatuses
		}
		if f.CompanyName != nil {
			p.CompanyName = strings.TrimSpace(*f.CompanyName)
		}
		if f.JobTitle != nil {
			p.JobTitle = strings.TrimSpace(*f.JobTitle)
		}
	}

	jobs, total, err := r.store.List(ctx, p)
	if err != nil {
		return nil, err
	}
	return &jobPage{Items: toJobs(jobs), TotalCount: int32(total)}, nil
}

// --- Mutation ---

type updateJobInput struct {
	CompanyName  *string
	JobTitle     *string
	PostingID    *string
	Link         *string
	PostedDate   NullDate
	Status       *string
	Notes        graphql.NullString
	ContactName  graphql.NullString
	ContactEmail graphql.NullString
	ContactPhone graphql.NullString
	SalaryRange  graphql.NullString
	Location     graphql.NullString
}

func (r *Resolver) UpdateJob(ctx context.Context, args struct {
	ID    graphql.ID
	Input updateJobInput
}) (*job, error) {
	id, err := parseID(args.ID)
	if err != nil {
		return nil, err
	}

	in := args.Input
	u := store.Update{Status: in.Status}
	for _, f := range []struct {
		name string
		in   *string
		out  **string
	}{
		{"companyName", in.CompanyName, &u.CompanyName},
		{"jobTitle", in.JobTitle, &u.JobTitle},
		{"postingId", in.PostingID, &u.PostingID},
		{"link", in.Link, &u.Link},
	} {
		if f.in == nil {
			continue
		}
		v, err := required(f.name, *f.in)
		if err != nil {
			return nil, err
		}
		*f.out = &v
	}
	if in.PostedDate.Set {
		u.SetPostedDate = true
		u.PostedDate = dateToTime(in.PostedDate.Value)
	}
	u.Notes = nullable(in.Notes)
	u.ContactName = nullable(in.ContactName)
	u.ContactEmail = nullable(in.ContactEmail)
	u.ContactPhone = nullable(in.ContactPhone)
	u.SalaryRange = nullable(in.SalaryRange)
	u.Location = nullable(in.Location)

	j, err := r.store.UpdateJob(ctx, id, u)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, fmt.Errorf("job %s not found", args.ID)
	}
	return toJob(*j), nil
}

// required trims s and rejects it if it's empty.
func required(name, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s can't be empty", name)
	}
	return s, nil
}

// nullable converts an optional GraphQL string into a store update: nil when
// omitted, otherwise the value to write (nil value = NULL). Blank strings become
// NULL; other values are stored as given.
func nullable(n graphql.NullString) *store.Nullable {
	if !n.Set {
		return nil
	}
	if n.Value == nil || strings.TrimSpace(*n.Value) == "" {
		return &store.Nullable{}
	}
	return &store.Nullable{Value: n.Value}
}

func dateToTime(d *Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}
