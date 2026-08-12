// Package proto declares the transport-neutral user contract for the gRPC example.
package proto

import (
	"context"
	"time"
)

// UserDTO represents the data transfer object for User domain data.
type UserDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// UserServiceClient is the domain service contract implemented by both In-Memory (Monolith) and gRPC (Microservice) providers.
type UserServiceClient interface {
	GetUser(ctx context.Context, id string) (*UserDTO, error)
	CreateUser(ctx context.Context, name, email, role string) (*UserDTO, error)
}
