package main

import (
	"fmt"
	"net/http"

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

	fmt.Println("🚀 Starting goplusplus High-Throughput Load Test Server on http://localhost:8080")
	fmt.Println("   • Benchmark Endpoint: GET http://localhost:8080/api/v1/bench/100")
	fmt.Println("   • Run ApacheBench (ab): ab -n 100000 -c 100 http://localhost:8080/api/v1/bench/100")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
