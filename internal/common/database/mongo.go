package database

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"time"

	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoClient struct {
	Client *mongo.Client
	DB     *mongo.Database
	cfg    *config.MongoConfig
}

func NewMongoClient(ctx context.Context, cfg *config.MongoConfig) (*MongoClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err = client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	fmt.Println("Connected to MongoDB")
	db := client.Database(cfg.DBName)

	return &MongoClient{
		Client: client,
		DB:     db,
		cfg:    cfg,
	}, nil
}

func (m *MongoClient) AddWebPage(page *models.WebPage) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll := m.DB.Collection(m.cfg.CrawlerColl)
	now := primitive.NewDateTimeFromTime(time.Now())

	if page.ID.IsZero() {
		page.CreatedAt = now
	}
	page.UpdatedAt = now

	res, err := coll.InsertOne(ctx, page)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("failed to insert webpage: %w", err)
	}

	oid, ok := res.InsertedID.(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("failed to cast InsertedID to ObjectID: got type %T", res.InsertedID)
	}

	return oid, nil
}

func (m *MongoClient) AddBatchWebPage(pages []*models.WebPage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	coll := m.DB.Collection(m.cfg.CrawlerColl)

	docs := make([]interface{}, len(pages))
	now := primitive.NewDateTimeFromTime(time.Now())
	for i, page := range pages {
		if page.ID.IsZero() {
			page.CreatedAt = now
		}
		page.UpdatedAt = now
		docs[i] = page
	}

	_, err := coll.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert webpages: %w", err)
	}
	return nil
}

func (m *MongoClient) GetBatchWebPage(
	batchSize int,
	lastID *primitive.ObjectID,
	unprocessedOnly bool,
) ([]models.WebPage, *primitive.ObjectID, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	collection := m.DB.Collection(m.cfg.CrawlerColl)
	filter := bson.M{}

	if lastID != nil {
		filter["_id"] = bson.M{"$gt": *lastID}
	}

	if unprocessedOnly {
		filter["indexed"] = bson.M{"$ne": true}
	}

	findOptions := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetLimit(int64(batchSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find documents: %w", err)
	}
	defer cursor.Close(ctx)

	var webPages []models.WebPage
	if err = cursor.All(ctx, &webPages); err != nil {
		return nil, nil, fmt.Errorf("failed to decode documents: %w", err)
	}

	var newLastID *primitive.ObjectID
	if len(webPages) > 0 {
		lastDocID := webPages[len(webPages)-1].ID
		newLastID = &lastDocID
	}

	return webPages, newLastID, nil
}

func (m *MongoClient) Disconnect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	fmt.Println("MongoDB connection closed")
	return nil
}
