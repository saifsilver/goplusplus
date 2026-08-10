package main

import (
	"context"
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/pubsub"
	"github.com/saifsilver/goplusplus/queue"
	"github.com/saifsilver/goplusplus/search"
	"github.com/saifsilver/goplusplus/storage"
)

func main() {
	ctx := context.Background()

	// 1. Initialize Infrastructure Adapters
	sqliteDB, _ := dbcore.NewSQLiteClient("app.db")
	s3Client := storage.NewS3Client(storage.S3Config{Bucket: "my-app-assets", Region: "us-east-1"})
	esClient := search.NewElasticsearchClient(search.ESConfig{})
	kafkaWorker := queue.NewKafkaWorker([]string{"localhost:9092"}, "user-events")
	rabbitBus := pubsub.NewRabbitMQBus("amqp://localhost:5672")

	// 2. Initialize goplusplus Application Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	app.POST("/api/v1/upload-avatar", func(c *gpp.Context) error {
		// 1. Upload to S3 & generate CloudFront URL
		s3URL, _ := s3Client.Upload(ctx, "avatars/user_101.jpg", []byte("fake_image_data"), "image/jpeg")
		cdnURL := storage.GenerateCloudFrontURL("cdn.myapp.com", "avatars/user_101.jpg")

		// 2. Store metadata in SQLite
		_ = sqliteDB.Exec(ctx, "INSERT INTO users (avatar_url) VALUES (?)", cdnURL)

		// 3. Index user in Elasticsearch
		_ = esClient.IndexDocument(ctx, "users", "usr_101", map[string]any{"avatar": cdnURL})

		// 4. Stream event to Kafka & RabbitMQ
		_ = kafkaWorker.PublishMessage(ctx, "usr_101", []byte(`{"event":"avatar_updated"}`))
		_ = rabbitBus.Publish(ctx, "user_events", "avatar.update", []byte(`{"user_id":"usr_101"}`))

		return c.JSON(http.StatusOK, gpp.H{
			"status":         "success",
			"s3_url":         s3URL,
			"cloudfront_url": cdnURL,
			"infrastructure": "SQLite, S3, CloudFront, Elasticsearch, Kafka, RabbitMQ operational!",
		})
	})

	fmt.Println("🚀 Starting goplusplus Cloud Infrastructure Server on http://localhost:8080")
	fmt.Println("   • Upload Endpoint: POST http://localhost:8080/api/v1/upload-avatar")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
