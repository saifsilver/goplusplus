package main

import (
	"fmt"
	"go++"
	"go++/examples/modular_monolith/modules/user"
	"go++/middleware"
)

func main() {
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

	// Standalone User Microservice - Zero code refactoring of user.Module!
	app.RegisterModule("/users", user.New())

	fmt.Println("👤 Starting go++ Standalone User Microservice on http://localhost:8081")
	fmt.Println("   • Endpoints: http://localhost:8081/users/profile/42")

	if err := app.Listen(":8081"); err != nil {
		panic(err)
	}
}
