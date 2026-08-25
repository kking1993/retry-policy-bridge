package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// envoyRetryPolicy mirrors the RetryPolicy fields from an Envoy route
// config, as documented at
// https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto
//
// per_try_timeout has no equivalent on the gRPC side (retryPolicy has no
// per-attempt timeout), so it round-trips only when converting envoy to
// envoy and is otherwise dropped.
type envoyRetryPolicy struct {
	RetryOn       string             `json:"retry_on,omitempty"`
	NumRetries    int                `json:"num_retries,omitempty"`
	PerTryTimeout string             `json:"per_try_timeout,omitempty"`
	RetryBackOff  *envoyRetryBackOff `json:"retry_back_off,omitempty"`
}

type envoyRetryBackOff struct {
	BaseInterval string `json:"base_interval,omitempty"`
	MaxInterval  string `json:"max_interval,omitempty"`
}

func policyFromEnvoy(data []byte) (Policy, error) {
	var e envoyRetryPolicy
	if err := json.Unmarshal(data, &e); err != nil {
		return Policy{}, err
	}
	if e.NumRetries < 1 {
		return Policy{}, fmt.Errorf("num_retries must be at least 1, got %d", e.NumRetries)
	}

	p := Policy{
		// envoy counts retries after the first attempt, gRPC counts
		// total attempts including the first.
		MaxAttempts: e.NumRetries + 1,
		// envoy has no configurable multiplier; its default backoff
		// algorithm doubles the base interval each attempt.
		BackoffMultiplier: 2,
	}

	for _, tok := range strings.Split(e.RetryOn, ",") {
		tok = strings.TrimSpace(tok)
		if code, ok := envoyToGRPCRetryOn[tok]; ok {
			p.RetryOn = append(p.RetryOn, code)
		}
	}

	if e.RetryBackOff != nil {
		if e.RetryBackOff.BaseInterval != "" {
			d, err := time.ParseDuration(e.RetryBackOff.BaseInterval)
			if err != nil {
				return Policy{}, fmt.Errorf("invalid base_interval %q: %w", e.RetryBackOff.BaseInterval, err)
			}
			p.InitialBackoff = d
		}
		if e.RetryBackOff.MaxInterval != "" {
			d, err := time.ParseDuration(e.RetryBackOff.MaxInterval)
			if err != nil {
				return Policy{}, fmt.Errorf("invalid max_interval %q: %w", e.RetryBackOff.MaxInterval, err)
			}
			p.MaxBackoff = d
		}
	}

	return p, nil
}

func policyToEnvoy(p Policy) ([]byte, error) {
	numRetries := p.MaxAttempts - 1
	if numRetries < 1 {
		numRetries = 1
	}
	e := envoyRetryPolicy{NumRetries: numRetries}

	var tokens []string
	for _, code := range p.RetryOn {
		if tok, ok := grpcToEnvoyRetryOn[code]; ok {
			tokens = append(tokens, tok)
		}
	}
	e.RetryOn = strings.Join(tokens, ",")

	if p.InitialBackoff > 0 || p.MaxBackoff > 0 {
		e.RetryBackOff = &envoyRetryBackOff{}
		if p.InitialBackoff > 0 {
			e.RetryBackOff.BaseInterval = formatSeconds(p.InitialBackoff)
		}
		if p.MaxBackoff > 0 {
			e.RetryBackOff.MaxInterval = formatSeconds(p.MaxBackoff)
		}
	}

	return json.MarshalIndent(e, "", "  ")
}
