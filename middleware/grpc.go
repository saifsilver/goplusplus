package middleware

import (
	"net/http"
	"strings"

	gpp "github.com/saifsilver/goplusplus"
)

// IsGRPC checks whether an incoming request is a gRPC request based on Content-Type header and protocol.
func IsGRPC(r *http.Request) bool {
	return r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc")
}

// GRPCMultiplex returns middleware that multiplexes incoming gRPC calls to a gRPC server handler over HTTP/2.
func GRPCMultiplex(grpcHandler http.Handler) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		if IsGRPC(c.Request) {
			c.Abort()
			grpcHandler.ServeHTTP(c.Writer, c.Request)
			return nil
		}
		return c.Next()
	}
}
