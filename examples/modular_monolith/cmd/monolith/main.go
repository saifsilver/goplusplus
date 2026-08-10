package main

import (
	"fmt"
	"go++"
	"go++/examples/modular_monolith/modules/order"
	"go++/examples/modular_monolith/modules/user"
	"go++/middleware"
)

func main() {
	app := gpp.New()

	// Global security & system middleware
	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
	)

	// API Version Group
	api := app.Group("/api/v1")

	// Register ALL modules into a single Modular Monolith deployment!
	api.RegisterModule("/users", user.New())
	api.RegisterModule("/orders", order.New())

	fmt.Println("🏛️  Starting go++ Modular Monolith Server on http://localhost:8080")
	fmt.Println("   • User Module:  http://localhost:8080/api/v1/users/profile/42")
	fmt.Println("   • Order Module: http://localhost:8080/api/v1/orders/1001")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
