package main

import (
	"fmt"
	"net/http"
	"os"

	gpp "github.com/saifsilver/goplusplus"
)

var staticJSONResponse = []byte(`{"status":"ok","engine":"goplusplus"}`)

func main() {
	app := gpp.New()

	// Minimal endpoint for reproducible local load testing.
	app.GET("/api/v1/bench/:id", func(c *gpp.Context) error {
		return c.Data(http.StatusOK, "application/json", staticJSONResponse)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}
	addr := ":" + port

	fmt.Printf("🚀 Starting goplusplus load-test server on http://localhost:%s\n", port)
	fmt.Printf("   • Benchmark Endpoint: GET http://localhost:%s/api/v1/bench/100\n", port)
	fmt.Printf("   • Run ApacheBench (ab): ab -n 100000 -c 100 -k http://localhost:%s/api/v1/bench/100\n", port)

	if err := app.Listen(addr); err != nil {
		panic(err)
	}
}
