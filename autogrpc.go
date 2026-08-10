package gpp

import (
	"net/http"
	"strings"
)

// AutoGRPCMultiplex returns middleware that automatically detects gRPC requests over HTTP/2 and multiplexes them to a gRPC server.
func (engine *Engine) AutoGRPCMultiplex(grpcHandler http.Handler) HandlerFunc {
	return func(c *Context) error {
		r := c.Request
		isGRPC := r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc")
		if isGRPC && grpcHandler != nil {
			c.Abort()
			grpcHandler.ServeHTTP(c.Writer, c.Request)
			return nil
		}
		return c.Next()
	}
}
