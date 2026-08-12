// Package tenant extracts and installs tenant identity for request processing.
// Extraction is not authorization; applications must verify membership and
// enforce tenant isolation in every data and service boundary.
package tenant
