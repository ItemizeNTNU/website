// Package store opens the MongoDB connection.
package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ItemizeNTNU/website/internal/config"
)

// Connect dials MongoDB and verifies the connection.
//
// The driver connects lazily, so without an explicit ping a bad URI or an
// unreachable host would not surface until the first visitor hit the events
// page. Failing here instead means a broken deployment is obvious at rollout.
func Connect(ctx context.Context, cfg config.Mongo) (*mongo.Client, *mongo.Database, error) {
	opts := options.Client().
		ApplyURI(cfg.URI).
		SetAppName("itemize-website").
		SetServerSelectionTimeout(10 * time.Second)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to MongoDB: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("MongoDB is not reachable at %s: %w", redactURI(cfg.URI), err)
	}

	return client, client.Database(cfg.Database), nil
}

// redactURI strips any credentials from a connection string so it can go in an
// error message or a log line.
func redactURI(uri string) string {
	scheme, rest, ok := cutAfter(uri, "://")
	if !ok {
		return uri
	}
	at := lastIndexByte(rest, '@')
	if at < 0 {
		return uri
	}
	return scheme + "://****@" + rest[at+1:]
}

func cutAfter(s, sep string) (before, after string, found bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
