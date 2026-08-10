package main

import (
	"fmt"
	"go++"
	"go++/examples/modular_monolith/modules/order"
	"go++/middleware"
)

func main() {
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// Standalone Order Microservice - Zero code refactoring of order.Module!
	app.RegisterModule("/orders", order.New())

	fmt.Println("🛒 Starting go++ Standalone Order Microservice on http://localhost:8082")
	fmt.Println("   • Endpoints: http://localhost:8082/orders/1001")

	if err := app.Listen(":8082"); err != nil {
		panic(err)
	}
}
