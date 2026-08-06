package events_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ItemizeNTNU/website/internal/events"
)

// testRepo connects to the database named by MONGO_TEST_URL, skipping the test
// when it is not set. CI provides a service container; locally these stay out
// of the way until you ask for them.
func testRepo(t *testing.T) (*events.MongoRepo, *mongo.Collection) {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URL")
	if uri == "" {
		t.Skip("set MONGO_TEST_URL to run the MongoDB integration tests")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	db := client.Database("itemize_test")
	coll := db.Collection(events.Collection)
	if err := coll.Drop(context.Background()); err != nil {
		t.Fatalf("clearing the collection: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return events.NewMongoRepo(db, log), coll
}

// The previous application wrote these documents through Mongoose, which adds
// a version key and stamps an _id onto every subdocument in an array. Neither
// is used by this program, but dropping them on write would quietly rewrite
// every record into a shape the old data no longer matches — and would make a
// rollback or a diff against a backup impossible to reason about.
func TestUpsertPreservesMongooseFields(t *testing.T) {
	repo, coll := testRepo(t)
	ctx := context.Background()

	id := bson.NewObjectID()
	attendanceID := bson.NewObjectID()
	created := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)

	// Seed a document exactly as Mongoose would have left it.
	_, err := coll.InsertOne(ctx, bson.M{
		"_id":            id,
		"name":           "Eksisterende arrangement",
		"location":       bson.M{"name": "Savannen", "url": ""},
		"register_url":   "",
		"date":           created,
		"duration":       2.0,
		"end":            created.Add(2 * time.Hour),
		"ctf":            bson.M{"name": "", "url": ""},
		"info":           "Opprinnelig tekst",
		"hidden":         false,
		"discord":        true,
		"discordEventId": "999888777",
		"created":        created,
		"check_in": bson.M{
			"code": "abc-123",
			"attendances": []bson.M{{
				"_id": attendanceID, "name": "Kari", "user_id": "fa-1",
				"registered": created,
			}},
		},
		"__v": 0,
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	existing, err := repo.ByID(ctx, id)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	existing.Info = "Oppdatert tekst"
	if _, err := repo.Upsert(ctx, existing); err != nil {
		t.Fatalf("saving: %v", err)
	}

	// Inspect the stored bytes rather than a decoded struct: the point is what
	// survives on disk, and decoding through our own types would hide exactly
	// the kind of loss this is guarding against.
	raw, err := coll.FindOne(ctx, bson.M{"_id": id}).Raw()
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}

	if _, err := raw.LookupErr("__v"); err != nil {
		t.Error("__v was dropped; a replacement was used where an update was needed")
	}
	if got, ok := raw.Lookup("info").StringValueOK(); !ok || got != "Oppdatert tekst" {
		t.Errorf("info = %v, want the updated text", raw.Lookup("info"))
	}
	if got, ok := raw.Lookup("discordEventId").StringValueOK(); !ok || got != "999888777" {
		t.Errorf("discordEventId = %v; losing it orphans the Discord event",
			raw.Lookup("discordEventId"))
	}
	if got, ok := raw.Lookup("created").TimeOK(); !ok || !got.UTC().Equal(created) {
		t.Errorf("created = %v, want it untouched at %v", raw.Lookup("created"), created)
	}
	if _, err := raw.LookupErr("edited"); err != nil {
		t.Error("edited was not stamped on update")
	}

	if got, ok := raw.Lookup("check_in", "code").StringValueOK(); !ok || got != "abc-123" {
		t.Errorf("check_in.code = %v; changing it invalidates printed QR codes",
			raw.Lookup("check_in", "code"))
	}

	attendances, ok := raw.Lookup("check_in", "attendances").ArrayOK()
	if !ok {
		t.Fatal("check_in.attendances was dropped entirely")
	}
	elems, err := attendances.Values()
	if err != nil || len(elems) != 1 {
		t.Fatalf("attendances = %d, want the existing one preserved", len(elems))
	}
	first, ok := elems[0].DocumentOK()
	if !ok {
		t.Fatal("the attendance entry is not a document")
	}
	if got, ok := first.Lookup("_id").ObjectIDOK(); !ok || got != attendanceID {
		t.Errorf("the attendance subdocument _id was lost: %v", first.Lookup("_id"))
	}
}

// Two people scanning at the same moment must not both be recorded, and the
// second must be told why. The previous implementation read, appended in
// memory and wrote the whole document back, which lost one of the two.
func TestAddAttendanceRejectsDuplicates(t *testing.T) {
	repo, _ := testRepo(t)
	ctx := context.Background()

	e := &events.Event{
		Name:     "Innsjekk-test",
		Location: events.Place{Name: "Savannen"},
		Date:     time.Now(),
		Duration: 2,
		CheckIn:  events.CheckIn{Code: "kode-1"},
	}
	if _, err := repo.Upsert(ctx, e); err != nil {
		t.Fatalf("creating: %v", err)
	}

	first := events.Attendance{Name: "Kari", UserID: "fa-1"}
	if err := repo.AddAttendance(ctx, "kode-1", first); err != nil {
		t.Fatalf("first check-in: %v", err)
	}
	if err := repo.AddAttendance(ctx, "kode-1", first); err != events.ErrAlreadyCheckedIn {
		t.Errorf("second check-in returned %v, want ErrAlreadyCheckedIn", err)
	}

	// A different person is still welcome.
	if err := repo.AddAttendance(ctx, "kode-1", events.Attendance{Name: "Ola", UserID: "fa-2"}); err != nil {
		t.Errorf("a second person could not check in: %v", err)
	}

	got, err := repo.ByCheckInCode(ctx, "kode-1")
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if got.Attendees() != 2 {
		t.Errorf("attendees = %d, want 2", got.Attendees())
	}
	for _, a := range got.CheckIn.Attendances {
		if a.ID.IsZero() {
			t.Error("a new attendance was written without an _id, unlike every existing record")
		}
		if a.Registered.IsZero() {
			t.Error("a new attendance was written without a timestamp")
		}
	}
}

func TestAddAttendanceUnknownCode(t *testing.T) {
	repo, _ := testRepo(t)

	err := repo.AddAttendance(context.Background(), "finnes-ikke",
		events.Attendance{Name: "Kari", UserID: "fa-1"})
	if err != events.ErrNotFound {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// Documents written before the hidden field existed have no such key. Querying
// for a literal false would drop every one of them from the public listing.
func TestListShowsDocumentsWithoutHiddenField(t *testing.T) {
	repo, coll := testRepo(t)
	ctx := context.Background()

	future := time.Now().Add(48 * time.Hour)
	_, err := coll.InsertOne(ctx, bson.M{
		"_id":      bson.NewObjectID(),
		"name":     "Uten hidden-felt",
		"location": bson.M{"name": "Sahara", "url": ""},
		"date":     future,
		"duration": 2.0,
		"end":      future.Add(2 * time.Hour),
		"ctf":      bson.M{"name": "", "url": ""},
		"info":     "Gammelt dokument",
		"created":  time.Now(),
		// no "hidden" key at all
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	list, err := repo.List(ctx, events.Filter{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d events, want the legacy document to be visible", len(list))
	}
}

func TestListExcludesHiddenAndPast(t *testing.T) {
	repo, _ := testRepo(t)
	ctx := context.Background()

	now := time.Now()
	for _, e := range []*events.Event{
		{Name: "Kommende", Date: now.Add(48 * time.Hour), Duration: 2},
		{Name: "Skjult", Date: now.Add(48 * time.Hour), Duration: 2, Hidden: true},
		{Name: "Ferdig", Date: now.Add(-48 * time.Hour), Duration: 2},
	} {
		if _, err := repo.Upsert(ctx, e); err != nil {
			t.Fatalf("creating %s: %v", e.Name, err)
		}
	}

	public, err := repo.List(ctx, events.Filter{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(public) != 1 || public[0].Name != "Kommende" {
		t.Errorf("public listing = %v, want only the upcoming event", names(public))
	}

	board, err := repo.List(ctx, events.Filter{IncludeHidden: true, IncludeOld: true})
	if err != nil {
		t.Fatalf("listing for the board: %v", err)
	}
	if len(board) != 3 {
		t.Errorf("board listing = %v, want all three", names(board))
	}
}

func names(list []events.Event) []string {
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = e.Name
	}
	return out
}
