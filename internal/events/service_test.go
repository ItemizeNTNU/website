package events

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/discord"
)

// ── Validation ────────────────────────────────────────────────────────────

func validForm() url.Values {
	return url.Values{
		"name":          {"Pizza og CTF"},
		"location.name": {"Savannen"},
		"location.url":  {""},
		"register_url":  {""},
		"date":          {"2026-09-01T17:15"},
		"duration":      {"3"},
		"ctf.name":      {""},
		"ctf.url":       {""},
		"info":          {"Ta med laptop."},
	}
}

func TestFromFormAcceptsValidInput(t *testing.T) {
	e, verr := FromForm(validForm())
	if verr.Any() {
		t.Fatalf("valid form rejected: %v", verr)
	}
	if e.Name != "Pizza og CTF" || e.Duration != 3 {
		t.Errorf("unexpected event: %+v", e)
	}
	if e.Date.IsZero() {
		t.Error("date was not parsed")
	}
}

func TestFromFormCollectsEveryError(t *testing.T) {
	form := validForm()
	form.Set("name", "x")             // too short
	form.Set("location.name", "")     // required
	form.Set("info", "")              // required
	form.Set("duration", "ikke tall") // not a number

	_, verr := FromForm(form)
	// The previous server stopped at the first problem, so the board fixed one
	// field per round trip. Every field should be annotated at once.
	for _, field := range []string{"name", "location.name", "info", "duration"} {
		if _, ok := verr[field]; !ok {
			t.Errorf("no error reported for %q; got %v", field, verr)
		}
	}
}

func TestURLValidation(t *testing.T) {
	tests := []struct {
		in      string
		wantErr string
	}{
		{"", ""},
		{"https://example.no/a?b=1", ""},
		{"  https://example.no  ", ""}, // trimmed before checking, as before
		{"http://example.no", "the URL needs to start with https://"},
		{"example.no", "the URL needs to start with https://"},
		{"javascript:alert(1)", "the URL needs to start with https://"},
		{"https://exa mple.no", "the URL can not include whitespace"},
		{"https://example.no/a\tb", "the URL can not include whitespace"},
		// A non-breaking space really does arrive via copy-paste, and JS's \s
		// caught it, so the port has to as well.
		{"https://example.no/a b", "the URL can not include whitespace"},
	}

	for _, tt := range tests {
		form := validForm()
		form.Set("location.url", tt.in)
		_, verr := FromForm(form)

		got := verr["location.url"]
		if got != tt.wantErr {
			t.Errorf("url %q: got %q, want %q", tt.in, got, tt.wantErr)
		}
	}
}

// Hidden is the stronger statement: there is nothing to announce.
func TestHiddenForcesDiscordOff(t *testing.T) {
	form := validForm()
	form.Set("hidden", "1")
	form.Set("discord", "1")

	e, verr := FromForm(form)
	if verr.Any() {
		t.Fatalf("unexpected errors: %v", verr)
	}
	if !e.Hidden || e.Discord {
		t.Errorf("hidden=%v discord=%v, want hidden with discord off", e.Hidden, e.Discord)
	}
}

// ── Service ───────────────────────────────────────────────────────────────

type fakeRepo struct {
	Repository
	stored  map[bson.ObjectID]*Event
	deleted []bson.ObjectID
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{stored: map[bson.ObjectID]*Event{}}
}

func (f *fakeRepo) ByID(_ context.Context, id bson.ObjectID) (*Event, error) {
	e, ok := f.stored[id]
	if !ok {
		return nil, ErrNotFound
	}
	copied := *e
	return &copied, nil
}

func (f *fakeRepo) Upsert(_ context.Context, e *Event) (bson.ObjectID, error) {
	id := e.ID
	if id.IsZero() {
		id = bson.NewObjectID()
		e.ID = id
	}
	copied := *e
	f.stored[id] = &copied
	return id, nil
}

func (f *fakeRepo) Delete(_ context.Context, id bson.ObjectID) error {
	if _, ok := f.stored[id]; !ok {
		return ErrNotFound
	}
	delete(f.stored, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeDiscord struct {
	enabled  bool
	upserted []discord.ScheduledEvent
	deleted  []string
	nextID   string
	err      error
}

func (f *fakeDiscord) Enabled() bool { return f.enabled }

func (f *fakeDiscord) UpsertScheduledEvent(_ context.Context, existingID string, e discord.ScheduledEvent) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.upserted = append(f.upserted, e)
	if existingID != "" {
		return existingID, nil
	}
	return f.nextID, nil
}

func (f *fakeDiscord) DeleteScheduledEvent(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func newService(repo Repository, d Syncer) *Service {
	return NewService(repo, d, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var uuidPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Check-in codes are printed on QR codes, so the format has to match what the
// previous implementation produced exactly.
func TestSaveAssignsCanonicalCheckInCode(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &fakeDiscord{})

	e, _ := FromForm(validForm())
	id, err := svc.Save(context.Background(), bson.ObjectID{}, e)
	if err != nil {
		t.Fatal(err)
	}

	stored := repo.stored[id]
	if !uuidPattern.MatchString(stored.CheckIn.Code) {
		t.Errorf("check-in code %q is not a canonical lowercase UUIDv4", stored.CheckIn.Code)
	}
}

// Reassigning a check-in code would invalidate every QR code already handed
// out, so an edit must never touch it — even if the submission tries.
func TestSavePreservesCheckInCodeAndDiscordID(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &fakeDiscord{})

	id := bson.NewObjectID()
	created := time.Now().Add(-72 * time.Hour)
	repo.stored[id] = &Event{
		ID:             id,
		Name:           "Eksisterende",
		CheckIn:        CheckIn{Code: "printed-on-a-poster"},
		DiscordEventID: "999",
		Created:        created,
	}

	e, _ := FromForm(validForm())
	// A crafted submission trying to take over the code and the Discord event.
	e.CheckIn.Code = "attacker-chosen"
	e.DiscordEventID = "someone-elses-event"

	if _, err := svc.Save(context.Background(), id, e); err != nil {
		t.Fatal(err)
	}

	stored := repo.stored[id]
	if stored.CheckIn.Code != "printed-on-a-poster" {
		t.Errorf("check-in code became %q", stored.CheckIn.Code)
	}
	if !stored.Created.Equal(created) {
		t.Errorf("created was overwritten: %v", stored.Created)
	}
}

func TestSaveComputesEnd(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &fakeDiscord{})

	form := validForm()
	form.Set("duration", "2.5")
	e, _ := FromForm(form)

	id, err := svc.Save(context.Background(), bson.ObjectID{}, e)
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.stored[id]
	if got := stored.End.Sub(stored.Date); got != 150*time.Minute {
		t.Errorf("end - date = %v, want 2h30m", got)
	}
}

func TestSavePublishesToDiscord(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: true, nextID: "discord-123"}
	svc := newService(repo, fake)

	form := validForm()
	form.Set("discord", "1")
	e, _ := FromForm(form)

	id, err := svc.Save(context.Background(), bson.ObjectID{}, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.upserted) != 1 {
		t.Fatalf("published %d events, want 1", len(fake.upserted))
	}
	// The returned id must be persisted, or the event is orphaned: there is
	// then no handle to update or delete it by.
	if repo.stored[id].DiscordEventID != "discord-123" {
		t.Errorf("Discord event id was not stored: %q", repo.stored[id].DiscordEventID)
	}
}

// Hiding a published event should retract the announcement.
func TestHidingRemovesTheDiscordEvent(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: true}
	svc := newService(repo, fake)

	id := bson.NewObjectID()
	repo.stored[id] = &Event{
		ID: id, Name: "Publisert", DiscordEventID: "discord-123",
		CheckIn: CheckIn{Code: "kode"},
	}

	form := validForm()
	form.Set("hidden", "1")
	e, _ := FromForm(form)

	if _, err := svc.Save(context.Background(), id, e); err != nil {
		t.Fatal(err)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "discord-123" {
		t.Errorf("deleted = %v, want the Discord event removed", fake.deleted)
	}
	if repo.stored[id].DiscordEventID != "" {
		t.Errorf("a stale Discord id was kept: %q", repo.stored[id].DiscordEventID)
	}
}

// Discord being unreachable must not lose the board's work. The event is
// saved and the failure is reported distinguishably.
func TestDiscordFailureStillSavesTheEvent(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: true, err: errors.New("503 from discord")}
	svc := newService(repo, fake)

	form := validForm()
	form.Set("discord", "1")
	e, _ := FromForm(form)

	id, err := svc.Save(context.Background(), bson.ObjectID{}, e)
	if !errors.Is(err, ErrDiscordSync) {
		t.Fatalf("got %v, want ErrDiscordSync", err)
	}
	if _, ok := repo.stored[id]; !ok {
		t.Error("the event was not saved when Discord failed")
	}
}

func TestSaveWithoutDiscordConfigured(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: false}
	svc := newService(repo, fake)

	form := validForm()
	form.Set("discord", "1")
	e, _ := FromForm(form)

	if _, err := svc.Save(context.Background(), bson.ObjectID{}, e); err != nil {
		t.Fatalf("saving without Discord configured failed: %v", err)
	}
	if len(fake.upserted) != 0 {
		t.Error("published to Discord despite the integration being off")
	}
}

func TestDeleteRemovesBoth(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: true}
	svc := newService(repo, fake)

	id := bson.NewObjectID()
	repo.stored[id] = &Event{ID: id, Name: "Slettes", DiscordEventID: "discord-9"}

	if err := svc.Delete(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(repo.deleted) != 1 {
		t.Error("the event was not deleted")
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "discord-9" {
		t.Errorf("the Discord event was not removed: %v", fake.deleted)
	}
}

// A guild event we cannot remove is untidy; refusing to delete the record
// because Discord is unreachable is worse.
func TestDeleteProceedsWhenDiscordFails(t *testing.T) {
	repo := newFakeRepo()
	fake := &fakeDiscord{enabled: true, err: errors.New("unreachable")}
	svc := newService(repo, fake)

	id := bson.NewObjectID()
	repo.stored[id] = &Event{ID: id, Name: "Slettes", DiscordEventID: "discord-9"}

	if err := svc.Delete(context.Background(), id); err != nil {
		t.Fatalf("delete failed because Discord did: %v", err)
	}
	if _, ok := repo.stored[id]; ok {
		t.Error("the event survived the delete")
	}
}

func TestDeleteUnknownEvent(t *testing.T) {
	svc := newService(newFakeRepo(), &fakeDiscord{})
	if err := svc.Delete(context.Background(), bson.NewObjectID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}
