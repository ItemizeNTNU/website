package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/discord"
)

// Syncer publishes events to Discord. The client satisfies it; tests supply a
// fake, and a nil value means the integration is switched off.
type Syncer interface {
	Enabled() bool
	UpsertScheduledEvent(ctx context.Context, existingID string, e discord.ScheduledEvent) (string, error)
	DeleteScheduledEvent(ctx context.Context, id string) error
}

// Service owns the write path: validation happens before it, storage and
// Discord are sequenced here.
type Service struct {
	repo    Repository
	discord Syncer
	log     *slog.Logger
}

// NewService builds the service. discord may be nil.
func NewService(repo Repository, syncer Syncer, log *slog.Logger) *Service {
	return &Service{repo: repo, discord: syncer, log: log}
}

// ErrDiscordSync reports that the event was saved but Discord was not updated.
// It is deliberately distinguishable: the save succeeded and the board should
// be told what did not, rather than being shown a generic failure for an
// operation that mostly worked.
var ErrDiscordSync = errors.New("event saved, but Discord was not updated")

// Save creates or updates an event.
//
// Fields the form must not control are carried over from the stored event
// rather than trusted from the submission — the check-in code in particular,
// because it is printed on QR codes and reassigning it would invalidate every
// one already handed out.
func (s *Service) Save(ctx context.Context, id bson.ObjectID, in *Event) (bson.ObjectID, error) {
	var existing *Event
	if !id.IsZero() {
		var err error
		existing, err = s.repo.ByID(ctx, id)
		if err != nil {
			return bson.ObjectID{}, err
		}
		in.ID = existing.ID
		in.CheckIn = existing.CheckIn
		in.DiscordEventID = existing.DiscordEventID
		in.Created = existing.Created
	}

	// A check-in code is assigned once and never regenerated. The previous
	// version seeded new events with the literal string "null" before
	// replacing it, so that value appears in the data and means "unassigned".
	if !in.HasCheckIn() {
		code, err := uuidV4()
		if err != nil {
			return bson.ObjectID{}, err
		}
		in.CheckIn.Code = code
	}

	in.End = in.ComputeEnd()

	syncErr := s.syncDiscord(ctx, in)

	newID, err := s.repo.Upsert(ctx, in)
	if err != nil {
		return bson.ObjectID{}, err
	}
	return newID, syncErr
}

// syncDiscord brings the guild event in line with the stored one.
//
// Ordering matters: this runs before the save so that a newly created Discord
// event's id is persisted along with everything else. A failure here is
// reported but does not block the save — losing an event because Discord was
// briefly unavailable would be the worse outcome.
func (s *Service) syncDiscord(ctx context.Context, e *Event) error {
	if s.discord == nil || !s.discord.Enabled() {
		return nil
	}

	// Hidden or unannounced: remove any event that exists.
	if e.Hidden || !e.Discord {
		if e.DiscordEventID == "" {
			return nil
		}
		if err := s.discord.DeleteScheduledEvent(ctx, e.DiscordEventID); err != nil {
			s.log.Error("removing the Discord event failed", "id", e.DiscordEventID, "err", err)
			return fmt.Errorf("%w: %v", ErrDiscordSync, err)
		}
		e.DiscordEventID = ""
		return nil
	}

	id, err := s.discord.UpsertScheduledEvent(ctx, e.DiscordEventID, discord.ScheduledEvent{
		Name: e.Name,
		Description: discord.BuildDescription(
			e.Info, e.Location.URL, e.RegisterURL, e.CTF.Name, e.CTF.URL),
		Location: e.Location.Name,
		Start:    e.Date,
		End:      e.End,
	})
	if err != nil {
		s.log.Error("publishing to Discord failed", "event", e.Name, "err", err)
		return fmt.Errorf("%w: %v", ErrDiscordSync, err)
	}
	if id != "" {
		e.DiscordEventID = id
	}
	return nil
}

// Delete removes an event and its Discord counterpart.
func (s *Service) Delete(ctx context.Context, id bson.ObjectID) error {
	existing, err := s.repo.ByID(ctx, id)
	if err != nil {
		return err
	}

	if existing.DiscordEventID != "" && s.discord != nil && s.discord.Enabled() {
		if err := s.discord.DeleteScheduledEvent(ctx, existing.DiscordEventID); err != nil {
			// Leaving a stale guild event behind is untidy; refusing to delete
			// the record because Discord is unreachable is worse.
			s.log.Error("removing the Discord event failed during delete",
				"id", existing.DiscordEventID, "err", err)
		}
	}

	return s.repo.Delete(ctx, id)
}

// uuidV4 generates a random identifier in the canonical hyphenated form.
//
// Hand-rolled rather than a dependency for a single call site. The format has
// to match exactly: existing check-in codes are lowercase and hyphenated, and
// they are printed on QR codes that are already in circulation.
func uuidV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}
