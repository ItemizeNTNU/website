package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// staleWindow is how long a finished event stays in the default listing, so
// something that ended an hour ago has not already vanished from the page.
const staleWindow = 6 * time.Hour

// MongoRepo is the MongoDB-backed Repository.
type MongoRepo struct {
	coll *mongo.Collection
	log  *slog.Logger
}

// NewMongoRepo wraps a database handle.
func NewMongoRepo(db *mongo.Database, log *slog.Logger) *MongoRepo {
	return &MongoRepo{coll: db.Collection(Collection), log: log}
}

var _ Repository = (*MongoRepo)(nil)

// visible matches events the public may see.
//
// It has to be "not true" rather than "is false": the previous application
// wrote this field through Mongoose, which cast the query value 0 to false,
// and documents predating the field have no `hidden` key at all. Querying for
// a literal false would quietly drop every one of those from the page.
func visible() bson.D {
	return bson.D{{Key: "hidden", Value: bson.D{{Key: "$ne", Value: true}}}}
}

func (r *MongoRepo) List(ctx context.Context, f Filter) ([]Event, error) {
	filter := bson.D{}
	if !f.IncludeHidden {
		filter = append(filter, visible()...)
	}
	if !f.IncludeOld {
		filter = append(filter, bson.E{
			Key:   "end",
			Value: bson.D{{Key: "$gt", Value: time.Now().Add(-staleWindow)}},
		})
	}

	page := max(f.Page, 0)
	opts := options.Find().
		SetSort(bson.D{{Key: "date", Value: 1}}).
		SetSkip(int64(page * PageSize)).
		SetLimit(PageSize)

	return r.find(ctx, filter, opts)
}

func (r *MongoRepo) Public(ctx context.Context) ([]Event, error) {
	return r.find(ctx, visible(), options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
}

func (r *MongoRepo) find(ctx context.Context, filter any, opts *options.FindOptionsBuilder) ([]Event, error) {
	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	var out []Event
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decoding events: %w", err)
	}
	return out, nil
}

func (r *MongoRepo) ByID(ctx context.Context, id bson.ObjectID) (*Event, error) {
	return r.one(ctx, bson.D{{Key: "_id", Value: id}})
}

func (r *MongoRepo) ByCheckInCode(ctx context.Context, code string) (*Event, error) {
	return r.one(ctx, bson.D{{Key: "check_in.code", Value: code}})
}

func (r *MongoRepo) one(ctx context.Context, filter bson.D) (*Event, error) {
	var e Event
	err := r.coll.FindOne(ctx, filter).Decode(&e)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("fetching event: %w", err)
	}
	return &e, nil
}

// Upsert creates or updates an event and returns its identifier.
//
// It uses $set rather than replacing the document. Mongoose's updateOne
// compiled to $set of the named paths and left everything else alone, so a
// replacement would silently drop __v and anything else this struct does not
// model. `created` is only ever written on insert, which also repairs a bug in
// the previous version where create and update stamped the wrong field.
func (r *MongoRepo) Upsert(ctx context.Context, e *Event) (bson.ObjectID, error) {
	now := time.Now()
	e.End = e.ComputeEnd()

	set := bson.D{
		{Key: "name", Value: e.Name},
		{Key: "location", Value: e.Location},
		{Key: "register_url", Value: e.RegisterURL},
		{Key: "date", Value: e.Date},
		{Key: "duration", Value: e.Duration},
		{Key: "end", Value: e.End},
		{Key: "ctf", Value: e.CTF},
		{Key: "info", Value: e.Info},
		{Key: "hidden", Value: e.Hidden},
		{Key: "discord", Value: e.Discord},
		{Key: "discordEventId", Value: e.DiscordEventID},
		{Key: "check_in.code", Value: e.CheckIn.Code},
		{Key: "edited", Value: now},
	}

	id := e.ID
	if id.IsZero() {
		id = bson.NewObjectID()
	}

	_, err := r.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{
			{Key: "$set", Value: set},
			// $set and $setOnInsert must not name the same field, so `created`
			// appears only here.
			{Key: "$setOnInsert", Value: bson.D{{Key: "created", Value: now}}},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("saving event: %w", err)
	}
	e.ID = id
	return id, nil
}

func (r *MongoRepo) Delete(ctx context.Context, id bson.ObjectID) error {
	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}
	// The previous version had this test inverted and reported success when it
	// had deleted nothing.
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// AddAttendance records one person checking in.
//
// The push is guarded by a $ne on the user id so the duplicate check and the
// write are a single atomic operation. The previous version read the document,
// appended in memory and wrote it back, which let two people scanning at the
// same moment both succeed — or worse, let one overwrite the other.
func (r *MongoRepo) AddAttendance(ctx context.Context, code string, a Attendance) error {
	if a.ID.IsZero() {
		// Match the shape Mongoose produced, so old and new records are
		// indistinguishable.
		a.ID = bson.NewObjectID()
	}
	if a.Registered.IsZero() {
		a.Registered = time.Now()
	}

	res, err := r.coll.UpdateOne(ctx,
		bson.D{
			{Key: "check_in.code", Value: code},
			{Key: "check_in.attendances.user_id", Value: bson.D{{Key: "$ne", Value: a.UserID}}},
		},
		bson.D{{Key: "$push", Value: bson.D{{Key: "check_in.attendances", Value: a}}}},
	)
	if err != nil {
		return fmt.Errorf("registering attendance: %w", err)
	}
	if res.MatchedCount > 0 {
		return nil
	}

	// Nothing matched, which means either the code is wrong or this person is
	// already on the list. One extra read tells the two apart so the message
	// can be accurate.
	if _, err := r.ByCheckInCode(ctx, code); err != nil {
		return err
	}
	return ErrAlreadyCheckedIn
}

// EnsureIndexes creates the indexes the queries rely on. Safe to call on every
// start; an index that already exists is left alone.
func (r *MongoRepo) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		// Matches the index the Mongoose schema declared.
		{Keys: bson.D{{Key: "date", Value: 1}}, Options: options.Index().SetName("date_1")},
		// The default listing filters on end.
		{Keys: bson.D{{Key: "end", Value: 1}}, Options: options.Index().SetName("end_1")},
		// New. Both check-in endpoints query this and there was no index, so
		// every QR scan was a full collection scan. Not unique: legacy
		// documents share the sentinel "null".
		{
			Keys:    bson.D{{Key: "check_in.code", Value: 1}},
			Options: options.Index().SetName("check_in_code_1").SetSparse(true),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		// A pre-existing index with different options is a conflict, not an
		// outage. Log it and carry on rather than refusing to start.
		r.log.Warn("could not create every event index", "err", err)
	}
	return nil
}
