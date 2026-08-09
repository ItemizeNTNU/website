package events

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The queries below are what the site's pages are built from, and every one of
// them is written as a literal BSON document naming fields by string. Nothing
// connects those strings to the struct tags they have to match, so these tests
// pin both ends.
//
// None of this needs a server. The behavioural tests that do live in
// mongo_test.go and skip unless MONGO_TEST_URL is set.

// visible() is the single definition of "an event the public may see", used by
// both the listing and the calendar feed.
func TestVisibleFilterMatchesLegacyDocuments(t *testing.T) {
	got := visible()

	if len(got) != 1 || got[0].Key != "hidden" {
		t.Fatalf("visible() = %v, want a single condition on `hidden`", got)
	}
	cond, ok := got[0].Value.(bson.D)
	if !ok || len(cond) != 1 {
		t.Fatalf("visible() condition = %v, want a single operator", got[0].Value)
	}

	// "not true" rather than "is false" is load-bearing. Documents written
	// before the field existed have no `hidden` key at all, and Mongoose cast
	// the value 0 to false, so an equality test on false would silently drop
	// every one of them from the public listing.
	if cond[0].Key != "$ne" {
		t.Errorf("visible() uses %q; anything but $ne hides every document written before the hidden field existed", cond[0].Key)
	}
	if cond[0].Value != true {
		t.Errorf("visible() negates %v, want true", cond[0].Value)
	}
}

// A filter naming `hidden` is only worth anything if that is what the struct
// writes. Renaming a tag would leave the query reading a field nobody sets:
// no error, no failing decode, just a listing that quietly shows everything or
// nothing.
func TestQueriedFieldNamesMatchWhatIsStored(t *testing.T) {
	raw, err := bson.Marshal(Event{
		CheckIn: CheckIn{
			Code:        "kode",
			Attendances: []Attendance{{ID: bson.NewObjectID(), Name: "Kari", UserID: "fa-1"}},
		},
	})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	doc := bson.Raw(raw)

	tests := []struct {
		path []string
		why  string
	}{
		{[]string{"hidden"}, "visible() filters the public listing on it"},
		{[]string{"end"}, "the default listing keeps events whose end is inside the stale window"},
		{[]string{"date"}, "every listing sorts on it, and there is an index by that name"},
		{[]string{"check_in", "code"}, "both check-in endpoints look events up by it, and the index is named for it"},
		{[]string{"check_in", "attendances"}, "$push appends to it when somebody scans in"},
		{[]string{"discordEventId"}, "Upsert writes it back; a rename would orphan every guild event"},
		{[]string{"register_url"}, "Upsert names it in $set"},
		{[]string{"location"}, "Upsert names it in $set"},
		{[]string{"ctf"}, "Upsert names it in $set"},
		{[]string{"info"}, "Upsert names it in $set"},
		{[]string{"duration"}, "Upsert names it in $set"},
		{[]string{"discord"}, "Upsert names it in $set"},
	}

	for _, tt := range tests {
		if _, err := doc.LookupErr(tt.path...); err != nil {
			t.Errorf("%q is not written under that name, but %s", strings.Join(tt.path, "."), tt.why)
		}
	}

	// The duplicate guard in AddAttendance filters on
	// check_in.attendances.user_id. If the subdocument stored the identifier
	// under any other key the $ne would never match, and the same person could
	// check in as many times as they liked.
	arr, ok := doc.Lookup("check_in", "attendances").ArrayOK()
	if !ok {
		t.Fatal("attendances is not an array")
	}
	values, err := arr.Values()
	if err != nil || len(values) != 1 {
		t.Fatalf("attendances = %v", values)
	}
	sub, ok := values[0].DocumentOK()
	if !ok {
		t.Fatal("an attendance is not a document")
	}
	for _, key := range []string{"_id", "name", "user_id", "registered"} {
		if _, err := sub.LookupErr(key); err != nil {
			t.Errorf("an attendance has no %q; the duplicate guard and the attendance list both name it", key)
		}
	}
}

// The stale window is why an event that finished an hour ago is still on the
// page. Zero would make events vanish the moment they end, mid-event for
// anyone refreshing; a window of days would keep last week at the top.
func TestStaleWindowIsAGraceNotAHistory(t *testing.T) {
	if staleWindow <= 0 {
		t.Errorf("staleWindow = %v; an event would disappear from the listing the instant it ended", staleWindow)
	}
	if staleWindow > 24*time.Hour {
		t.Errorf("staleWindow = %v; finished events would sit at the top of the listing for a day or more", staleWindow)
	}
}

// PageSize is part of the API the previous server exposed. Anything paging
// through the listing assumes it.
func TestPageSizeMatchesThePreviousAPI(t *testing.T) {
	if PageSize != 100 {
		t.Errorf("PageSize = %d, want 100 — a client paging through the listing would skip or repeat events", PageSize)
	}
}

// An identifier from a URL is parsed before it reaches ByID. A parse failure
// has to be an error rather than a zero ObjectID, because the zero value is a
// perfectly valid filter that simply matches nothing — the difference between
// "that is not an event id" and a confusing empty page.
func TestObjectIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		wantErr bool
	}{
		{"a real identifier", bson.NewObjectID().Hex(), false},
		{"upper case hex is accepted", strings.ToUpper(bson.NewObjectID().Hex()), false},
		{"empty", "", true},
		{"too short", "5f2b0a1e0f3f", true},
		{"not hex at all", "ikke-en-objectid-abcdef1", true},
		{"a path traversal attempt", "../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := bson.ObjectIDFromHex(tt.hex)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ObjectIDFromHex(%q) err = %v, want error: %v", tt.hex, err, tt.wantErr)
			}
			if err != nil && !id.IsZero() {
				t.Errorf("a rejected identifier still produced %v", id)
			}
		})
	}

	// A zero identifier is not an error to the driver — it is a well-formed
	// filter that matches nothing — which is why the handlers must reject the
	// parse failure rather than carrying on with the zero value.
	if got := (bson.ObjectID{}).Hex(); got != strings.Repeat("0", 24) {
		t.Errorf("the zero ObjectID renders as %q", got)
	}
}

// ── Failure mapping ───────────────────────────────────────────────────────

// offlineRepo builds a repository against a client that can never reach a
// server: the address is a unix socket path that does not exist, so no network
// connection is ever attempted. Every call below passes an already-cancelled
// context, which the driver fails at server selection before it touches the
// address at all.
//
// That is enough to exercise the one thing worth exercising without a
// database: how a failure is reported.
func offlineRepo(t *testing.T) (*MongoRepo, *bytes.Buffer) {
	t.Helper()

	client, err := mongo.Connect(options.Client().
		ApplyURI("mongodb://%2Fnonexistent%2Fitemize-tests.sock/?directConnection=true").
		SetServerSelectionTimeout(time.Millisecond))
	if err != nil {
		t.Fatalf("building an offline client: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	return NewMongoRepo(client.Database("itemize_test"), log), &logged
}

func cancelled(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// ErrNotFound and ErrAlreadyCheckedIn are what the handlers branch on to
// render a 404 or the "du er allerede sjekket inn" message. An unreachable
// database must never be reported as either: the visitor would be told the
// event does not exist, or that they have already checked in when they have
// not, and nothing would be logged as an outage.
func TestInfrastructureFailuresAreNotMistakenForSentinels(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*MongoRepo, context.Context) error
		wantPrefix string
	}{
		{
			"looking an event up by id",
			func(r *MongoRepo, ctx context.Context) error {
				_, err := r.ByID(ctx, bson.NewObjectID())
				return err
			},
			"fetching event:",
		},
		{
			"looking an event up by check-in code",
			func(r *MongoRepo, ctx context.Context) error {
				_, err := r.ByCheckInCode(ctx, "kode")
				return err
			},
			"fetching event:",
		},
		{
			"listing the calendar",
			func(r *MongoRepo, ctx context.Context) error {
				_, err := r.List(ctx, Filter{})
				return err
			},
			"listing events:",
		},
		{
			"the public feed",
			func(r *MongoRepo, ctx context.Context) error {
				_, err := r.Public(ctx)
				return err
			},
			"listing events:",
		},
		{
			"saving",
			func(r *MongoRepo, ctx context.Context) error {
				_, err := r.Upsert(ctx, &Event{Name: "Test"})
				return err
			},
			"saving event:",
		},
		{
			"deleting",
			func(r *MongoRepo, ctx context.Context) error {
				return r.Delete(ctx, bson.NewObjectID())
			},
			"deleting event:",
		},
		{
			"registering attendance",
			func(r *MongoRepo, ctx context.Context) error {
				return r.AddAttendance(ctx, "kode", Attendance{Name: "Kari", UserID: "fa-1"})
			},
			"registering attendance:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := offlineRepo(t)

			err := tt.call(repo, cancelled(t))
			if err == nil {
				t.Fatal("an unreachable database was reported as success")
			}
			if errors.Is(err, ErrNotFound) {
				t.Errorf("got %v, which reads as a missing event — the visitor would see a 404 for something that is merely unreachable", err)
			}
			if errors.Is(err, ErrAlreadyCheckedIn) {
				t.Errorf("got %v, which would tell somebody standing at the door that they have already checked in", err)
			}
			if !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Errorf("error = %q, want it to say which operation failed (%q) so the log points at the right place", err, tt.wantPrefix)
			}
			// The underlying cause has to stay reachable, or an operator sees
			// only "fetching event" with no hint that the server was down.
			if !errors.Is(err, context.Canceled) {
				t.Errorf("error = %q, want the driver's cause still unwrappable", err)
			}
		})
	}
}

// Index creation is best-effort by design: a pre-existing index declared with
// different options is a conflict, not an outage, and refusing to start over
// one would take the site down for a cosmetic difference. The failure still
// has to be visible in the log, or a missing index becomes invisible and every
// QR scan quietly turns into a collection scan.
func TestEnsureIndexesNeverBlocksStartup(t *testing.T) {
	repo, logged := offlineRepo(t)

	if err := repo.EnsureIndexes(cancelled(t)); err != nil {
		t.Fatalf("EnsureIndexes returned %v; the site would refuse to start because an index could not be created", err)
	}
	out := logged.String()
	if !strings.Contains(out, "could not create every event index") {
		t.Errorf("nothing was logged about the failure (%q); a missing index would go unnoticed until check-in nights got slow", out)
	}
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the failure was not logged as a warning: %q", out)
	}
}
