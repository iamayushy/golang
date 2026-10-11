// Package mongostore implements shortener.Repository on MongoDB.
package mongostore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/iamayushy/golang/internal/shortener"
)

const (
	linksCollection    = "links"
	countersCollection = "counters"
	linksCounterID     = "links_created"
)

type LinkRepository struct {
	coll     *mongo.Collection
	counters *mongo.Collection
}

// NewLinkRepository returns a repository and ensures its indexes exist: a unique
// index on code, and a TTL index so MongoDB deletes links once expires_at passes.
func NewLinkRepository(ctx context.Context, db *mongo.Database) (*LinkRepository, error) {
	coll := db.Collection(linksCollection)
	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
	})
	if err != nil {
		return nil, err
	}
	return &LinkRepository{coll: coll, counters: db.Collection(countersCollection)}, nil
}

func (r *LinkRepository) Create(ctx context.Context, link shortener.Link) error {
	_, err := r.coll.InsertOne(ctx, link)
	if mongo.IsDuplicateKeyError(err) {
		return shortener.ErrCodeExists
	}
	if err != nil {
		return err
	}
	// Links are deleted when they expire, so the all-time total lives in its own
	// counter. Failing to bump it shouldn't fail a link that was already saved.
	_, _ = r.counters.UpdateByID(ctx, linksCounterID,
		bson.M{"$inc": bson.M{"count": 1}},
		options.UpdateOne().SetUpsert(true),
	)
	return nil
}

func (r *LinkRepository) Stats(ctx context.Context, now time.Time) (shortener.Stats, error) {
	var counter struct {
		Count int64 `bson:"count"`
	}
	err := r.counters.FindOne(ctx, bson.M{"_id": linksCounterID}).Decode(&counter)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return shortener.Stats{}, err
	}
	live, err := r.coll.CountDocuments(ctx, bson.M{"expires_at": bson.M{"$gt": now}})
	if err != nil {
		return shortener.Stats{}, err
	}
	return shortener.Stats{TotalCreated: counter.Count, Live: live}, nil
}

func (r *LinkRepository) Get(ctx context.Context, code string) (shortener.Link, error) {
	var link shortener.Link
	err := r.coll.FindOne(ctx, bson.M{"code": code}).Decode(&link)
	return link, mapNotFound(err)
}

func (r *LinkRepository) Resolve(ctx context.Context, code string) (shortener.Link, error) {
	var link shortener.Link
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"code": code},
		bson.M{"$inc": bson.M{"clicks": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&link)
	return link, mapNotFound(err)
}

func (r *LinkRepository) Delete(ctx context.Context, code string) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"code": code})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return shortener.ErrNotFound
	}
	return nil
}

func mapNotFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return shortener.ErrNotFound
	}
	return err
}
