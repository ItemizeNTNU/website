package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/timefmt"
)

// errStorage stands for any infrastructure failure that is not one of the
// sentinel errors the handlers branch on.
var errStorage = errors.New("the database is on fire")

// recordingRepo is a fake with injectable failures that also remembers the
// context it was handed. The context matters as much as the arguments here:
// Save deliberately detaches from the request, and nothing else can observe
// whether it actually did.
type recordingRepo struct {
	Repository

	stored map[bson.ObjectID]*Event

	byIDErr   error
	upsertErr error
	deleteErr error

	byIDCalls   int
	upsertCalls int
	deleteCalls int

	// seen describes the context of the most recent call, sampled while the
	// call is in flight. Sampling afterwards would be useless: Save's deferred
	// cancel has fired by then, so every context looks cancelled.
	seen ctxSnapshot
}

// ctxSnapshot is what a dependency could observe about the context it was
// handed, taken at the moment of the call.
type ctxSnapshot struct {
	called      bool
	err         error
	hasDeadline bool
	deadline    time.Time
}

func snapshot(ctx context.Context) ctxSnapshot {
	deadline, ok := ctx.Deadline()
	return ctxSnapshot{called: true, err: ctx.Err(), hasDeadline: ok, deadline: deadline}
}

func newRecordingRepo() *recordingRepo {
	return &recordingRepo{stored: map[bson.ObjectID]*Event{}}
}

func (r *recordingRepo) seed(e *Event) bson.ObjectID {
	if e.ID.IsZero() {
		e.ID = bson.NewObjectID()
	}
	copied := *e
	r.stored[e.ID] = &copied
	return e.ID
}

func (r *recordingRepo) ByID(ctx context.Context, id bson.ObjectID) (*Event, error) {
	r.byIDCalls++
	r.seen = snapshot(ctx)
	if r.byIDErr != nil {
		return nil, r.byIDErr
	}
	e, ok := r.stored[id]
	if !ok {
		return nil, ErrNotFound
	}
	copied := *e
	return &copied, nil
}

func (r *recordingRepo) Upsert(ctx context.Context, e *Event) (bson.ObjectID, error) {
	r.upsertCalls++
	r.seen = snapshot(ctx)
	if r.upsertErr != nil {
		return bson.ObjectID{}, r.upsertErr
	}
	id := e.ID
	if id.IsZero() {
		id = bson.NewObjectID()
		e.ID = id
	}
	copied := *e
	r.stored[id] = &copied
	return id, nil
}

func (r *recordingRepo) Delete(ctx context.Context, id bson.ObjectID) error {
	r.deleteCalls++
	r.seen = snapshot(ctx)
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.stored[id]; !ok {
		return ErrNotFound
	}
	delete(r.stored, id)
	return nil
}

// recordingDiscord separates the two failure modes — publishing and removing —
// because the service treats them differently, and remembers the identifier it
// was asked to update so the create-versus-update branch is observable.
type recordingDiscord struct {
	enabled bool

	upsertErr error
	deleteErr error
	returnID  string

	upserted []publishCall
	deleted  []string
	seen     ctxSnapshot
}

// publishCall is one publish attempt, with the existing identifier the
// service passed alongside the payload.
type publishCall struct {
	ExistingID  string
	Name        string
	Description string
	Location    string
	Start, End  time.Time
}

func (d *recordingDiscord) Enabled() bool { return d.enabled }

func (d *recordingDiscord) UpsertScheduledEvent(ctx context.Context, existingID string, e discord.ScheduledEvent) (string, error) {
	d.seen = snapshot(ctx)
	if d.upsertErr != nil {
		return "", d.upsertErr
	}
	d.upserted = append(d.upserted, publishCall{
		ExistingID: existingID, Name: e.Name, Description: e.Description,
		Location: e.Location, Start: e.Start, End: e.End,
	})
	if existingID != "" {
		return existingID, nil
	}
	return d.returnID, nil
}

func (d *recordingDiscord) DeleteScheduledEvent(ctx context.Context, id string) error {
	d.seen = snapshot(ctx)
	if d.deleteErr != nil {
		return d.deleteErr
	}
	d.deleted = append(d.deleted, id)
	return nil
}

// ── Save: which branch runs ───────────────────────────────────────────────

// Editing an event that has since been deleted must fail before anything is
// written. Falling through would insert a second copy under the submitted
// identifier, without the check-in code or the created timestamp of the
// original.
func TestSaveOfAVanishedEventTouchesNothing(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true}
	svc := newService(repo, sync)

	in, _ := FromForm(validForm())
	_, err := svc.Save(context.Background(), bson.NewObjectID(), in)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound so the handler can render a 404 rather than a server error", err)
	}
	if repo.upsertCalls != 0 {
		t.Error("the event was written anyway; an edit of a deleted event would resurrect it in a mangled form")
	}
	if len(sync.upserted) != 0 || len(sync.deleted) != 0 {
		t.Error("Discord was contacted about an event that does not exist")
	}
}

// A lookup failure that is not "missing" has to reach the caller unchanged.
// Collapsing it into ErrNotFound would show the board a 404 for an event that
// is still there, and they would go and create a duplicate.
func TestSavePropagatesLookupFailures(t *testing.T) {
	repo := newRecordingRepo()
	repo.byIDErr = errStorage
	svc := newService(repo, &recordingDiscord{})

	in, _ := FromForm(validForm())
	_, err := svc.Save(context.Background(), bson.NewObjectID(), in)

	if !errors.Is(err, errStorage) {
		t.Fatalf("got %v, want the storage failure passed through", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a storage failure was reported as a missing event; the board would be told the event is gone when it is not")
	}
}

// Creating and editing differ in exactly one respect: an edit inherits the
// fields the form is not allowed to carry. Everything else is written from the
// submission, including fields that were previously set and have now been
// cleared.
func TestSaveCreateVersusUpdate(t *testing.T) {
	t.Run("creating stamps nothing from an earlier event", func(t *testing.T) {
		repo := newRecordingRepo()
		svc := newService(repo, &recordingDiscord{})

		in, _ := FromForm(validForm())
		id, err := svc.Save(context.Background(), bson.ObjectID{}, in)
		if err != nil {
			t.Fatal(err)
		}
		if repo.byIDCalls != 0 {
			t.Error("a create read an existing event first; there is nothing to read")
		}
		stored := repo.stored[id]
		if stored.DiscordEventID != "" {
			t.Errorf("a new event was given a Discord id: %q", stored.DiscordEventID)
		}
		if !stored.HasCheckIn() {
			t.Error("a new event was saved without a check-in code, so no QR code can be printed for it")
		}
	})

	t.Run("editing inherits the fields the form may not set", func(t *testing.T) {
		repo := newRecordingRepo()
		svc := newService(repo, &recordingDiscord{})

		created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		id := repo.seed(&Event{
			Name:           "Før",
			CheckIn:        CheckIn{Code: "trykt-på-plakat", Attendances: []Attendance{{Name: "Kari", UserID: "fa-1"}}},
			DiscordEventID: "guild-77",
			Created:        created,
		})

		in, _ := FromForm(validForm())
		if _, err := svc.Save(context.Background(), id, in); err != nil {
			t.Fatal(err)
		}

		stored := repo.stored[id]
		if stored.CheckIn.Code != "trykt-på-plakat" {
			t.Errorf("check-in code = %q; every printed QR code would stop working", stored.CheckIn.Code)
		}
		if stored.Attendees() != 1 {
			t.Errorf("attendees = %d, want the register carried over — an edit must not erase who turned up", stored.Attendees())
		}
		if stored.DiscordEventID != "guild-77" {
			t.Errorf("discord id = %q; losing it orphans the guild event", stored.DiscordEventID)
		}
		if !stored.Created.Equal(created) {
			t.Errorf("created = %v, want %v — an edit is not a creation", stored.Created, created)
		}
		if stored.ID != id {
			t.Errorf("id = %v, want %v", stored.ID, id)
		}
		if stored.Name != "Pizza og CTF" {
			t.Errorf("name = %q, want the submitted value", stored.Name)
		}
	})
}

// "null" is the sentinel the previous application wrote before replacing it,
// and a handful of documents still carry it. Such an event has no usable QR
// code, so an edit is the moment to mint one.
func TestSaveReplacesTheNullCheckInSentinel(t *testing.T) {
	repo := newRecordingRepo()
	svc := newService(repo, &recordingDiscord{})

	id := repo.seed(&Event{Name: "Gammelt", CheckIn: CheckIn{Code: "null"}})

	in, _ := FromForm(validForm())
	if _, err := svc.Save(context.Background(), id, in); err != nil {
		t.Fatal(err)
	}

	got := repo.stored[id].CheckIn.Code
	if got == "null" || got == "" {
		t.Fatalf("check-in code is still %q; the check-in page would never work for this event", got)
	}
	if !uuidPattern.MatchString(got) {
		t.Errorf("check-in code %q is not a canonical lowercase UUIDv4", got)
	}
}

// ── Save: the two systems disagreeing ─────────────────────────────────────

// Discord is updated before the database so that a newly created guild event's
// identifier is part of the same write. The cost is this case: the guild event
// exists and nothing remembers it. The save still has to report the failure it
// actually had — the database one — because that is what the board must retry.
func TestSaveReportsTheDatabaseFailureEvenAfterDiscordSucceeded(t *testing.T) {
	repo := newRecordingRepo()
	repo.upsertErr = errStorage
	sync := &recordingDiscord{enabled: true, returnID: "guild-new"}
	svc := newService(repo, sync)

	form := validForm()
	form.Set("discord", "1")
	in, _ := FromForm(form)

	_, err := svc.Save(context.Background(), bson.ObjectID{}, in)

	if !errors.Is(err, errStorage) {
		t.Fatalf("got %v, want the storage failure — that is the one the board has to retry", err)
	}
	if errors.Is(err, ErrDiscordSync) {
		t.Error("a database failure was reported as a Discord sync problem, which reads as \"saved, but…\" when nothing was saved")
	}
	if len(sync.upserted) != 1 {
		t.Errorf("published %d guild events, want 1 — this is the orphan the ordering trades away", len(sync.upserted))
	}
}

// Both failing at once is the same story: the save is what the board cares
// about, so the storage error is the one that surfaces.
func TestSaveReportsTheDatabaseFailureWhenBothFail(t *testing.T) {
	repo := newRecordingRepo()
	repo.upsertErr = errStorage
	sync := &recordingDiscord{enabled: true, upsertErr: errors.New("503 from discord")}
	svc := newService(repo, sync)

	form := validForm()
	form.Set("discord", "1")
	in, _ := FromForm(form)

	if _, err := svc.Save(context.Background(), bson.ObjectID{}, in); !errors.Is(err, errStorage) {
		t.Fatalf("got %v, want the storage failure to win", err)
	}
}

// Hiding an event whose announcement cannot be retracted must keep the guild
// identifier. Clearing it would leave the announcement standing with nothing
// left pointing at it, so no later save could remove it either.
func TestFailedRetractionKeepsTheDiscordIdentifier(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true, deleteErr: errors.New("503 from discord")}
	svc := newService(repo, sync)

	id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}})

	form := validForm()
	form.Set("hidden", "1")
	in, _ := FromForm(form)

	_, err := svc.Save(context.Background(), id, in)
	if !errors.Is(err, ErrDiscordSync) {
		t.Fatalf("got %v, want ErrDiscordSync so the board is told the announcement is still up", err)
	}
	if got := repo.stored[id].DiscordEventID; got != "guild-77" {
		t.Errorf("discord id = %q, want it kept so a later save can retry the retraction", got)
	}
	if !repo.stored[id].Hidden {
		t.Error("the event was not hidden; a Discord failure must not block the change the board asked for")
	}
}

// ── Save: whether Discord is contacted at all ─────────────────────────────

func TestSaveDiscordBranches(t *testing.T) {
	tests := []struct {
		name string
		// existing is the event already stored, or nil for a create.
		existing    *Event
		hidden      bool
		announce    bool
		enabled     bool
		nilSyncer   bool
		wantUpserts int
		wantDeletes []string
		wantStoredD string
		why         string
	}{
		{
			name: "announcing a new event publishes it", announce: true, enabled: true,
			wantUpserts: 1, wantStoredD: "guild-new",
			why: "the guild identifier has to be stored or the event can never be updated again",
		},
		{
			name: "an unannounced new event is not published", enabled: true,
			why: "publishing an event nobody asked to announce cannot be undone once the message is out",
		},
		{
			name:     "an unannounced event with nothing published needs no retraction",
			existing: &Event{Name: "Stille", CheckIn: CheckIn{Code: "kode"}}, enabled: true,
			why: "a pointless delete call would be a request per save against Discord's rate limit",
		},
		{
			name:     "un-ticking the Discord box retracts the announcement",
			existing: &Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}},
			enabled:  true, wantDeletes: []string{"guild-77"},
			why: "the announcement would otherwise stay up for an event that is no longer announced",
		},
		{
			name:     "re-announcing an already published event updates it in place",
			existing: &Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}},
			announce: true, enabled: true, wantUpserts: 1, wantStoredD: "guild-77",
			why: "creating a second guild event would leave the first one stale and unreachable",
		},
		{
			name:     "hiding retracts even when the Discord box is still ticked",
			existing: &Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}},
			hidden:   true, announce: true, enabled: true, wantDeletes: []string{"guild-77"},
			why: "a draft must not stay announced in the guild",
		},
		{
			name:     "with the integration switched off nothing is touched",
			existing: &Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}},
			announce: true, enabled: false, wantStoredD: "guild-77",
			why: "a deployment without Discord credentials must still be able to save events, and must not forget the identifier",
		},
		{
			name:      "with no syncer wired in at all the save still works",
			announce:  true,
			nilSyncer: true,
			why:       "NewService documents that the syncer may be nil; a nil dereference here would take down every save",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRecordingRepo()
			sync := &recordingDiscord{enabled: tt.enabled, returnID: "guild-new"}

			var svc *Service
			if tt.nilSyncer {
				svc = newService(repo, nil)
			} else {
				svc = newService(repo, sync)
			}

			var id bson.ObjectID
			if tt.existing != nil {
				id = repo.seed(tt.existing)
			}

			form := validForm()
			if tt.hidden {
				form.Set("hidden", "1")
			}
			if tt.announce {
				form.Set("discord", "1")
			}
			in, _ := FromForm(form)

			savedID, err := svc.Save(context.Background(), id, in)
			if err != nil {
				t.Fatalf("save failed: %v — %s", err, tt.why)
			}

			if got := len(sync.upserted); got != tt.wantUpserts {
				t.Errorf("published %d times, want %d — %s", got, tt.wantUpserts, tt.why)
			}
			if len(sync.deleted) != len(tt.wantDeletes) {
				t.Errorf("retracted %v, want %v — %s", sync.deleted, tt.wantDeletes, tt.why)
			} else {
				for i, want := range tt.wantDeletes {
					if sync.deleted[i] != want {
						t.Errorf("retracted %q, want %q", sync.deleted[i], want)
					}
				}
			}
			if got := repo.stored[savedID].DiscordEventID; got != tt.wantStoredD {
				t.Errorf("stored Discord id = %q, want %q — %s", got, tt.wantStoredD, tt.why)
			}
		})
	}
}

// Discord returning an empty identifier — which the client does when it
// updated an existing event rather than creating one — must not blank the
// stored value. That would orphan the guild event on the very next save.
func TestAnEmptyDiscordIdentifierDoesNotOverwriteTheStoredOne(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true, returnID: ""}
	svc := newService(repo, sync)

	id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77", CheckIn: CheckIn{Code: "kode"}})

	form := validForm()
	form.Set("discord", "1")
	in, _ := FromForm(form)

	if _, err := svc.Save(context.Background(), id, in); err != nil {
		t.Fatal(err)
	}
	if got := repo.stored[id].DiscordEventID; got != "guild-77" {
		t.Errorf("discord id = %q, want it unchanged", got)
	}
}

// What is sent to the guild is what members read in Discord. The end time in
// particular is derived, not submitted, so it has to be computed before the
// payload is built rather than after the save.
func TestTheDiscordPayloadMatchesTheEvent(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true, returnID: "guild-new"}
	svc := newService(repo, sync)

	form := validForm()
	form.Set("discord", "1")
	form.Set("name", "Pizza og CTF")
	form.Set("location.name", "Rådssalen")
	form.Set("location.url", "https://kart.ntnu.no/radssalen")
	form.Set("register_url", "https://itemize.no/pamelding")
	form.Set("ctf.name", "Julekalender")
	form.Set("ctf.url", "https://ctf.itemize.no")
	form.Set("info", "Ta med laptop.")
	form.Set("date", "2026-09-01T17:15")
	form.Set("duration", "2.5")
	in, _ := FromForm(form)

	if _, err := svc.Save(context.Background(), bson.ObjectID{}, in); err != nil {
		t.Fatal(err)
	}
	if len(sync.upserted) != 1 {
		t.Fatalf("published %d events, want 1", len(sync.upserted))
	}
	got := sync.upserted[0]

	if got.ExistingID != "" {
		t.Errorf("existing id = %q, want empty for a new event", got.ExistingID)
	}
	if got.Name != "Pizza og CTF" {
		t.Errorf("name = %q", got.Name)
	}
	// The venue name, not its URL: Discord shows this as the location line.
	if got.Location != "Rådssalen" {
		t.Errorf("location = %q, want the venue name — the URL belongs in the description", got.Location)
	}

	wantStart := time.Date(2026, 9, 1, 17, 15, 0, 0, timefmt.Oslo)
	if !got.Start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", got.Start, wantStart)
	}
	if !got.End.Equal(wantStart.Add(150 * time.Minute)) {
		t.Errorf("end = %v, want the start plus the 2.5 hour duration — an end of zero would make the guild event nonsense", got.End)
	}

	want := strings.Join([]string{
		"Registrering: https://itemize.no/pamelding",
		"Hvor: https://kart.ntnu.no/radssalen",
		"CTF: Julekalender (https://ctf.itemize.no)",
		strings.Repeat("-", 50),
		"",
		"Ta med laptop.",
	}, "\n")
	if got.Description != want {
		t.Errorf("description =\n%q\nwant\n%q", got.Description, want)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteBranches(t *testing.T) {
	t.Run("an event that was never announced needs no retraction", func(t *testing.T) {
		repo := newRecordingRepo()
		sync := &recordingDiscord{enabled: true}
		svc := newService(repo, sync)

		id := repo.seed(&Event{Name: "Aldri publisert"})
		if err := svc.Delete(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if len(sync.deleted) != 0 {
			t.Error("Discord was called for an event that was never announced, spending a request against the rate limit for nothing")
		}
	})

	t.Run("with the integration off the record is still removed", func(t *testing.T) {
		repo := newRecordingRepo()
		sync := &recordingDiscord{enabled: false}
		svc := newService(repo, sync)

		id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77"})
		if err := svc.Delete(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if len(sync.deleted) != 0 {
			t.Error("a disabled integration was called anyway")
		}
		if _, ok := repo.stored[id]; ok {
			t.Error("the event survived the delete")
		}
	})

	t.Run("with no syncer wired in the record is still removed", func(t *testing.T) {
		repo := newRecordingRepo()
		svc := newService(repo, nil)

		id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77"})
		if err := svc.Delete(context.Background(), id); err != nil {
			t.Fatalf("delete failed with no syncer configured: %v", err)
		}
	})

	t.Run("a lookup failure stops the delete", func(t *testing.T) {
		repo := newRecordingRepo()
		repo.byIDErr = errStorage
		svc := newService(repo, &recordingDiscord{enabled: true})

		if err := svc.Delete(context.Background(), bson.NewObjectID()); !errors.Is(err, errStorage) {
			t.Fatalf("got %v, want the storage failure", err)
		}
		if repo.deleteCalls != 0 {
			t.Error("the delete ran despite not knowing what it was deleting")
		}
	})

	t.Run("a failed delete is reported after the announcement is retracted", func(t *testing.T) {
		repo := newRecordingRepo()
		sync := &recordingDiscord{enabled: true}
		svc := newService(repo, sync)

		id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77"})
		repo.deleteErr = errStorage

		if err := svc.Delete(context.Background(), id); !errors.Is(err, errStorage) {
			t.Fatalf("got %v, want the storage failure surfaced", err)
		}
		// The retraction has already happened, so the event is still listed
		// but no longer announced. Recorded because it is the state a retry
		// starts from, not because it is ideal.
		if len(sync.deleted) != 1 {
			t.Errorf("retracted %v, want the announcement removed before the failed delete", sync.deleted)
		}
	})
}

// ── Detaching from the request ────────────────────────────────────────────

// A save touches two systems that cannot be rolled back together, so it must
// not be abandoned halfway because the browser gave up. If the request context
// leaked through, a double-submitted form or a closed tab could cancel the
// operation between the Discord call and the write, leaving a guild event
// whose identifier was never stored.
func TestSaveRunsToCompletionAfterTheRequestIsCancelled(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true, returnID: "guild-new"}
	svc := newService(repo, sync)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	form := validForm()
	form.Set("discord", "1")
	in, _ := FromForm(form)

	id, err := svc.Save(ctx, bson.ObjectID{}, in)
	if err != nil {
		t.Fatalf("the save was abandoned because the visitor's request was: %v", err)
	}
	if _, ok := repo.stored[id]; !ok {
		t.Fatal("nothing was written")
	}

	for _, dep := range []struct {
		name string
		seen ctxSnapshot
	}{{"the repository", repo.seen}, {"Discord", sync.seen}} {
		if !dep.seen.called {
			t.Fatalf("%s was never called", dep.name)
		}
		if dep.seen.err != nil {
			t.Errorf("%s was handed an already-cancelled context (%v); the two systems would be left disagreeing about an event nobody can now reconcile", dep.name, dep.seen.err)
		}
		if !dep.seen.hasDeadline {
			t.Errorf("%s was handed a context with no deadline; a hung dependency would block the write forever", dep.name)
		}
	}
}

func TestDeleteRunsToCompletionAfterTheRequestIsCancelled(t *testing.T) {
	repo := newRecordingRepo()
	sync := &recordingDiscord{enabled: true}
	svc := newService(repo, sync)

	id := repo.seed(&Event{Name: "Publisert", DiscordEventID: "guild-77"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := svc.Delete(ctx, id); err != nil {
		t.Fatalf("the delete was abandoned because the request was: %v", err)
	}
	if _, ok := repo.stored[id]; ok {
		t.Error("the event survived; a cancelled request left the announcement retracted but the record in place")
	}
}

// The deadline is the service's own, not the request's, and it has to be far
// enough out that a slow Discord call plus a write both fit.
func TestTheDetachedDeadlineIsTheServicesOwn(t *testing.T) {
	repo := newRecordingRepo()
	svc := newService(repo, nil)

	// A request with a deadline in the past: detach must not inherit it.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	in, _ := FromForm(validForm())
	if _, err := svc.Save(ctx, bson.ObjectID{}, in); err != nil {
		t.Fatalf("an expired request deadline killed the save: %v", err)
	}

	if !repo.seen.hasDeadline {
		t.Fatal("no deadline was set")
	}
	if remaining := time.Until(repo.seen.deadline); remaining <= 0 || remaining > writeTimeout {
		t.Errorf("the write ran with %v left, want a fresh deadline of at most the %v write timeout — inheriting the request's expired one would abandon it immediately", remaining, writeTimeout)
	}
}

// ── Check-in codes ────────────────────────────────────────────────────────

// The codes are printed on QR codes that are already in circulation, so the
// format is fixed: canonical, lowercase, hyphenated, version 4, variant 10.
// Two events sharing a code would send one event's attendees into the other's
// register, so the generator also has to be unique in practice.
func TestUUIDv4(t *testing.T) {
	const n = 2000
	seen := make(map[string]struct{}, n)

	for range n {
		code, err := uuidV4()
		if err != nil {
			t.Fatalf("generating a check-in code: %v", err)
		}
		if !uuidPattern.MatchString(code) {
			t.Fatalf("code %q is not a canonical lowercase UUIDv4; the printed QR codes follow the old format exactly", code)
		}
		if len(code) != 36 {
			t.Fatalf("code %q is %d characters, want 36", code, len(code))
		}
		if strings.ToLower(code) != code {
			t.Fatalf("code %q is not lowercase", code)
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("generated %q twice in %d draws; a collision routes one event's check-ins into another's register", code, n)
		}
		seen[code] = struct{}{}
	}
}

// Two events created in the same request must not end up sharing a code.
func TestConcurrentCreatesGetDistinctCheckInCodes(t *testing.T) {
	repo := newRecordingRepo()
	svc := newService(repo, nil)

	codes := map[string]bool{}
	for range 20 {
		in, _ := FromForm(validForm())
		id, err := svc.Save(context.Background(), bson.ObjectID{}, in)
		if err != nil {
			t.Fatal(err)
		}
		code := repo.stored[id].CheckIn.Code
		if codes[code] {
			t.Fatalf("two events were given the check-in code %q", code)
		}
		codes[code] = true
	}
}
