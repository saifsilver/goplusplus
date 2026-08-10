package main

import (
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/bloom"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	// 1. Bloom Filter for Cache Penetration Defense
	bloomFilter := bloom.NewFilter(10000, 0.01)
	bloomFilter.Add("usr_101")
	bloomFilter.Add("usr_102")

	// 2. HyperLogLog for Unique Daily Active Visitors
	hll := bloom.NewHyperLogLog()
	hll.Add("ip_192_168_1_1")
	hll.Add("ip_192_168_1_2")

	// 3. Count-Min Sketch for Top-K Trending Items
	cms := bloom.NewCountMinSketch()
	cms.Add("golang_framework", 1500)

	// 4. Initialize goplusplus App Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	app.GET("/api/v1/user/:id", func(c *gpp.Context) error {
		id := c.Param("id")

		// Defense against Cache Penetration Attack:
		if !bloomFilter.MayContain(id) {
			return gpp.ErrNotFound(fmt.Sprintf("Bloom Filter: User ID '%s' 100%% does not exist in DB", id))
		}

		return c.JSON(http.StatusOK, gpp.H{
			"status":            "found",
			"id":                id,
			"estimated_daus":    hll.EstimateCardinality(),
			"trending_frequency": cms.EstimateFrequency("golang_framework"),
		})
	})

	fmt.Println("🚀 Starting goplusplus Probabilistic Data Structures Server on http://localhost:8080")
	fmt.Println("   • Valid User:   GET http://localhost:8080/api/v1/user/usr_101")
	fmt.Println("   • Invalid User: GET http://localhost:8080/api/v1/user/usr_999 (Blocked by Bloom Filter)")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
