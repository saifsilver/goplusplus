package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/cache"
	"github.com/saifsilver/goplusplus/middleware"
	"github.com/saifsilver/goplusplus/search"
	"github.com/saifsilver/goplusplus/storage"
)

func main() {
	ctx := context.Background()

	// Polymorphic Interface Drivers
	redisStore, err := cache.NewRedisStore(ctx, cache.RedisConfig{URL: os.Getenv("REDIS_URL")})
	if err != nil {
		panic(err)
	}
	defer redisStore.Close()
	var cacheStore cache.Store = cache.NewMultiLevelStore(cache.NewMemoryStore(), redisStore)
	idempotencyStore, err := middleware.NewRedisIdempotencyStore(
		redisStore.Client(), middleware.RedisIdempotencyConfig{},
	)
	if err != nil {
		panic(err)
	}
	elasticsearch, err := search.NewElasticsearchClient(ctx, search.ESConfig{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
		APIKey:    os.Getenv("ELASTICSEARCH_API_KEY"),
	})
	if err != nil {
		panic(err)
	}
	var searchEngine search.Engine = elasticsearch
	s3Store, err := storage.NewS3Client(ctx, storage.S3Config{Bucket: "my-bucket", Region: "us-east-1"})
	if err != nil {
		panic(err)
	}
	var storageProvider storage.Provider = s3Store

	// Initialize goplusplus App Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.Idempotency(middleware.IdempotencyConfig{Store: idempotencyStore}),
	)

	app.POST("/api/v1/user", func(c *gpp.Context) error {
		// Use polymorphic cache interface
		user, _ := cacheStore.GetOrSet(ctx, "user:101", 10*time.Minute, func() (any, error) {
			return gpp.H{"id": "usr_101", "name": "Interface User"}, nil
		})

		// Use polymorphic search interface
		_ = searchEngine.IndexDocument(ctx, "users", "usr_101", user)

		// Use polymorphic storage interface
		uploadURL, _ := storageProvider.Upload(ctx, "avatars/usr_101.png", []byte("img"), "image/png")

		return c.JSON(http.StatusOK, gpp.H{
			"status":     "success",
			"user":       user,
			"upload_url": uploadURL,
		})
	})

	fmt.Println("🚀 Starting goplusplus Interface-Driven Architecture Server on http://localhost:8080")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
