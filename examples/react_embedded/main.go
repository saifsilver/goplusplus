package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

//go:embed all:static
var embeddedWeb embed.FS

func main() {
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// API Endpoints
	v1 := app.Group("/api/v1")
	v1.GET("/user", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"id":    "usr_100",
			"name":  "React Embedded User",
			"role":  "Developer",
		})
	})

	// SubFS for embedded React dist assets
	staticFS, err := fs.Sub(embeddedWeb, "static")
	if err != nil {
		// Fallback for development if static folder isn't populated
		staticFS = nil
	}

	if staticFS != nil {
		// Serve embedded React/Vite app on root "/" with client-side SPA routing fallback to index.html
		app.StaticFS("/", staticFS)
	}

	fmt.Println("🚀 Starting goplusplus Embedded React App on http://localhost:8080")
	fmt.Println("   • REST API Endpoint:      http://localhost:8080/api/v1/user")
	fmt.Println("   • Embedded React Frontend: http://localhost:8080/")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
