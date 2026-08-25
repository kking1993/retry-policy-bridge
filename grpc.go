package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// grpcRetryPolicy mirrors the retryPolicy object from a gRPC service
// config, as documented at
// https://github.com/grpc/grpc/blob/master/doc/service_config.md
type grpcRetryPolicy struct {
	MaxAttempts          int      `json:"maxAttempts"`
	InitialBackoff       string   `json:"initialBackoff,omitempty"`
	MaxBackoff           string   `json:"maxBackoff,omitempty"`
	BackoffMultiplier    float64  `json:"backoffMultiplier,omitempty"`
	RetryableStatusCodes []string `json:"retryableStatusCodes,omitempty"`
}

func policyFromGRPC(data []byte) (Policy, error) {
	var g grpcRetryPolicy
	if err := json.Unmarshal(data, &g); err != nil {
		return Policy{}, err
	}
	if g.MaxAttempts < 1 {
		return Policy{}, fmt.Errorf("maxAttempts must be at least 1, got %d", g.MaxAttempts)
	}

	p := Policy{
		MaxAttempts:       g.MaxAttempts,
		BackoffMultiplier: g.BackoffMultiplier,
		RetryOn:           g.RetryableStatusCodes,
	}

	if g.InitialBackoff != "" {
		d, err := time.ParseDuration(g.InitialBackoff)
		if err != nil {
			return Policy{}, fmt.Errorf("invalid initialBackoff %q: %w", g.InitialBackoff, err)
		}
		p.InitialBackoff = d
	}
	if g.MaxBackoff != "" {
		d, err := time.ParseDuration(g.MaxBackoff)
		if err != nil {
			return Policy{}, fmt.Errorf("invalid maxBackoff %q: %w", g.MaxBackoff, err)
		}
		p.MaxBackoff = d
	}

	return p, nil
}

func policyToGRPC(p Policy) ([]byte, error) {
	g := grpcRetryPolicy{
		MaxAttempts:          p.MaxAttempts,
		BackoffMultiplier:    p.BackoffMultiplier,
		RetryableStatusCodes: p.RetryOn,
	}
	if p.InitialBackoff > 0 {
		g.InitialBackoff = formatSeconds(p.InitialBackoff)
	}
	if p.MaxBackoff > 0 {
		g.MaxBackoff = formatSeconds(p.MaxBackoff)
	}
	return json.MarshalIndent(g, "", "  ")
}

// formatSeconds renders a duration the way the service config examples
// do: whole seconds as "30s", fractional seconds as "0.5s".
func formatSeconds(d time.Duration) string {
	s := fmt.Sprintf("%.3f", d.Seconds())
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + "s"
}
