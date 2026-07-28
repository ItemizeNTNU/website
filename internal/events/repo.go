package events

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrNotFound is returned when no event matches.
var ErrNotFound = errors.New("event not found")

// ErrAlreadyCheckedIn is returned when someone scans a code twice.
var ErrAlreadyCheckedIn = errors.New("already checked in")

// PageSize is how many events one page of the listing holds. Matches the
// previous API so anything paging through it behaves the same.
const PageSize = 100

// Filter selects which events to list.
type Filter struct {
	// IncludeHidden includes drafts. Only ever true for the board.
	IncludeHidden bool
	// IncludeOld includes events that have already finished.
	IncludeOld bool
	// Page is zero-based.
	Page int
}

// Repository is the storage behind the calendar.
//
// It exists as an interface so the handlers can be exercised against an
// in-memory fake; there is deliberately only one production implementation.
type Repository interface {
	List(ctx context.Context, f Filter) ([]Event, error)
	// Public returns every visible event regardless of date, for the calendar
	// feed — a subscriber wants the history too.
	Public(ctx context.Context) ([]Event, error)
	ByID(ctx context.Context, id bson.ObjectID) (*Event, error)
	ByCheckInCode(ctx context.Context, code string) (*Event, error)
	Upsert(ctx context.Context, e *Event) (bson.ObjectID, error)
	Delete(ctx context.Context, id bson.ObjectID) error
	AddAttendance(ctx context.Context, code string, a Attendance) error
	EnsureIndexes(ctx context.Context) error
}
