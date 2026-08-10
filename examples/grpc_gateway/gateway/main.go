package main

import (
	"fmt"
	"net/http"
	"os"

	"go++"
	"go++/examples/grpc_gateway/proto"
	"go++/examples/grpc_gateway/services"
	"go++/middleware"
)

// GatewayServer defines the HTTP API Gateway instance wrapping a UserServiceClient instance.
type GatewayServer struct {
	userClient proto.UserServiceClient
}

func main() {
	// 1. Determine execution mode (Monolith vs. gRPC Microservice) from ENV
	mode := os.Getenv("DEPLOYMENT_MODE")

	var userClient proto.UserServiceClient
	if mode == "grpc" {
		fmt.Println("⚡ Gateway Mode: Routing HTTP -> Remote gRPC Microservice (localhost:50051)")
		userClient = services.NewGRPCUserServiceProxy("localhost:50051")
	} else {
		fmt.Println("⚡ Gateway Mode: Routing HTTP -> In-Memory Monolith (0ms Network Latency)")
		userClient = services.NewInMemUserService()
	}

	gateway := &GatewayServer{userClient: userClient}

	// 2. Initialize go++ HTTP Engine
	app := gpp.New()

	// 3. Attach OWASP security suite, CORS, and rate limiting
	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
		middleware.RateLimit(middleware.RateLimiterConfig{
			Rate:     50,
			Capacity: 100,
		}),
	)

	// 4. Register HTTP API Routes
	v1 := app.Group("/api/v1")
	v1.GET("/users/:id", gateway.handleGetUser)
	v1.POST("/users", gateway.handleCreateUser)

	fmt.Println("🌐 Starting go++ HTTP API Gateway on http://localhost:8080")
	fmt.Println("   • GET  http://localhost:8080/api/v1/users/42")
	fmt.Println("   • POST http://localhost:8080/api/v1/users")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}

func (gw *GatewayServer) handleGetUser(c *gpp.Context) error {
	id := c.Param("id")
	user, err := gw.userClient.GetUser(c.Request.Context(), id)
	if err != nil {
		return gpp.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, gpp.H{
		"gateway": "go++ HTTP Gateway",
		"data":    user,
	})
}

func (gw *GatewayServer) handleCreateUser(c *gpp.Context) error {
	type createReq struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	var req createReq
	if err := c.BindJSON(&req); err != nil {
		return gpp.NewHTTPError(http.StatusBadRequest, "Invalid request body", err.Error())
	}

	user, err := gw.userClient.CreateUser(c.Request.Context(), req.Name, req.Email, req.Role)
	if err != nil {
		return gpp.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, gpp.H{
		"gateway": "go++ HTTP Gateway",
		"data":    user,
	})
}
