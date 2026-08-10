package main

import (
	"fmt"
	"net/http"
	"os"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	app := gpp.New()

	app.Use(
		middleware.Recovery(),
		middleware.Security(),
	)

	// High-Throughput Endpoint for Load Benchmarking
	app.GET("/api/v1/bench/:id", func(c *gpp.Context) error {
		id := c.Param("id")
		return c.JSON(http.StatusOK, gpp.H{
			"status": "ok",
			"id":     id,
			"engine": "goplusplus",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}
	addr := ":" + port

	fmt.Printf("🚀 Starting goplusplus High-Throughput Load Test Server on http://localhost:%s\n", port)
	fmt.Printf("   • Benchmark Endpoint: GET http://localhost:%s/api/v1/bench/100\n", port)
	fmt.Printf("   • Run ApacheBench (ab): ab -n 100000 -c 100 http://localhost:%s/api/v1/bench/100\n", port)

	if err := app.Listen(addr); err != nil {
		panic(err)
	}
}
