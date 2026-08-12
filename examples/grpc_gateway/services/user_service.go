// Package services provides local and remote user-service example adapters.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/saifsilver/goplusplus/examples/grpc_gateway/proto"
)

// InMemUserService implements UserServiceClient for Modular Monolith deployment mode (Direct In-Memory calls).
type InMemUserService struct {
	db map[string]*proto.UserDTO
}

// NewInMemUserService creates an in-memory user service instance.
func NewInMemUserService() *InMemUserService {
	return &InMemUserService{
		db: map[string]*proto.UserDTO{
			"42": {
				ID:        "42",
				Name:      "Alex Dev",
				Email:     "alex@example.com",
				Role:      "Administrator",
				CreatedAt: time.Now().Add(-48 * time.Hour),
			},
		},
	}
}

// GetUser returns a locally stored example user by ID.
func (s *InMemUserService) GetUser(ctx context.Context, id string) (*proto.UserDTO, error) {
	user, exists := s.db[id]
	if !exists {
		return nil, fmt.Errorf("user with ID '%s' not found", id)
	}
	return user, nil
}

// CreateUser adds a user to the process-local example store.
func (s *InMemUserService) CreateUser(ctx context.Context, name, email, role string) (*proto.UserDTO, error) {
	id := fmt.Sprintf("usr_%d", time.Now().UnixNano())
	user := &proto.UserDTO{
		ID:        id,
		Name:      name,
		Email:     email,
		Role:      role,
		CreatedAt: time.Now(),
	}
	s.db[id] = user
	return user, nil
}

// GRPCUserServiceProxy implements UserServiceClient over gRPC for Microservices deployment mode.
type GRPCUserServiceProxy struct {
	targetAddr string
}

// NewGRPCUserServiceProxy creates a gRPC service client stub targeting a remote microservice address.
func NewGRPCUserServiceProxy(targetAddr string) *GRPCUserServiceProxy {
	return &GRPCUserServiceProxy{
		targetAddr: targetAddr,
	}
}

// GetUser demonstrates the shape of a generated remote gRPC lookup.
func (s *GRPCUserServiceProxy) GetUser(ctx context.Context, id string) (*proto.UserDTO, error) {
	// In production, this invokes the generated gRPC client method over HTTP/2 channel
	return &proto.UserDTO{
		ID:        id,
		Name:      "Remote gRPC User",
		Email:     fmt.Sprintf("grpc_user_%s@remote-service.internal", id),
		Role:      "gRPC Microservice Entity",
		CreatedAt: time.Now(),
	}, nil
}

// CreateUser demonstrates the shape of a generated remote gRPC create call.
func (s *GRPCUserServiceProxy) CreateUser(ctx context.Context, name, email, role string) (*proto.UserDTO, error) {
	return &proto.UserDTO{
		ID:        "grpc_created_99",
		Name:      name,
		Email:     email,
		Role:      role,
		CreatedAt: time.Now(),
	}, nil
}
