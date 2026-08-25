package main

import "time"

// Policy is the in-memory representation both formats convert through.
// Neither format maps onto it perfectly, so each side drops what it
// doesn't understand rather than guessing.
type Policy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	RetryOn           []string // canonical gRPC status code names, e.g. "UNAVAILABLE"
}

// grpcToEnvoyRetryOn maps gRPC status codes to the envoy retry_on tokens
// that fire on the matching grpc-status trailer. Envoy only defines
// grpc-* tokens for these five codes, so anything else has no equivalent
// and is dropped during conversion in either direction.
var grpcToEnvoyRetryOn = map[string]string{
	"CANCELLED":           "grpc-cancelled",
	"DEADLINE_EXCEEDED":   "grpc-deadline-exceeded",
	"INTERNAL":            "grpc-internal",
	"RESOURCE_EXHAUSTED":  "grpc-resource-exhausted",
	"UNAVAILABLE":         "grpc-unavailable",
}

var envoyToGRPCRetryOn = reverseMap(grpcToEnvoyRetryOn)

func reverseMap(m map[string]string) map[string]string {
	r := make(map[string]string, len(m))
	for k, v := range m {
		r[v] = k
	}
	return r
}
