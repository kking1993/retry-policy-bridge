package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPolicyFromEnvoy(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Policy
		wantErr bool
	}{
		{
			name: "basic policy with backoff and mixed retry_on tokens",
			in:   `{"retry_on":"grpc-unavailable,grpc-deadline-exceeded,5xx","num_retries":3,"retry_back_off":{"base_interval":"0.5s","max_interval":"10s"}}`,
			want: Policy{
				MaxAttempts:       4,
				BackoffMultiplier: 2,
				RetryOn:           []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"},
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        10 * time.Second,
			},
		},
		{
			name: "retry_on tokens with surrounding whitespace are trimmed",
			in:   `{"retry_on":"grpc-cancelled, grpc-internal , grpc-resource-exhausted","num_retries":1}`,
			want: Policy{
				MaxAttempts:       2,
				BackoffMultiplier: 2,
				RetryOn:           []string{"CANCELLED", "INTERNAL", "RESOURCE_EXHAUSTED"},
			},
		},
		{
			name: "empty retry_on yields no codes",
			in:   `{"num_retries":1}`,
			want: Policy{MaxAttempts: 2, BackoffMultiplier: 2},
		},
		{
			name: "only non-grpc retry_on tokens yields no codes",
			in:   `{"retry_on":"5xx,reset,connect-failure","num_retries":2}`,
			want: Policy{MaxAttempts: 3, BackoffMultiplier: 2},
		},
		{
			name:    "num_retries zero is rejected",
			in:      `{"num_retries":0}`,
			wantErr: true,
		},
		{
			name:    "negative num_retries is rejected",
			in:      `{"num_retries":-1}`,
			wantErr: true,
		},
		{
			name:    "invalid base_interval",
			in:      `{"num_retries":1,"retry_back_off":{"base_interval":"soon"}}`,
			wantErr: true,
		},
		{
			name:    "invalid max_interval",
			in:      `{"num_retries":1,"retry_back_off":{"max_interval":"forever"}}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			in:      `{"num_retries":`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := policyFromEnvoy([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("policyFromEnvoy(%q) succeeded, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("policyFromEnvoy(%q) returned error: %v", tc.in, err)
			}
			if !policiesEqual(got, tc.want) {
				t.Errorf("policyFromEnvoy(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPolicyToEnvoy(t *testing.T) {
	cases := []struct {
		name           string
		p              Policy
		wantNumRetries int
		wantRetryOn    string
		wantBackOff    *envoyRetryBackOff
	}{
		{
			name:           "typical policy",
			p:              Policy{MaxAttempts: 4, RetryOn: []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"}},
			wantNumRetries: 3,
			wantRetryOn:    "grpc-unavailable,grpc-deadline-exceeded",
		},
		{
			name:           "unmapped status codes are dropped",
			p:              Policy{MaxAttempts: 4, RetryOn: []string{"UNAVAILABLE", "NOT_FOUND"}},
			wantNumRetries: 3,
			wantRetryOn:    "grpc-unavailable",
		},
		{
			name:           "maxAttempts of 1 clamps num_retries to 1, not 0",
			p:              Policy{MaxAttempts: 1},
			wantNumRetries: 1,
		},
		{
			name:           "maxAttempts of 0 also clamps to 1",
			p:              Policy{MaxAttempts: 0},
			wantNumRetries: 1,
		},
		{
			name:           "initial backoff only",
			p:              Policy{MaxAttempts: 2, InitialBackoff: 250 * time.Millisecond},
			wantNumRetries: 1,
			wantBackOff:    &envoyRetryBackOff{BaseInterval: "0.25s"},
		},
		{
			name:           "max backoff only",
			p:              Policy{MaxAttempts: 2, MaxBackoff: 5 * time.Second},
			wantNumRetries: 1,
			wantBackOff:    &envoyRetryBackOff{MaxInterval: "5s"},
		},
		{
			name:           "both backoff bounds",
			p:              Policy{MaxAttempts: 2, InitialBackoff: time.Second, MaxBackoff: 20 * time.Second},
			wantNumRetries: 1,
			wantBackOff:    &envoyRetryBackOff{BaseInterval: "1s", MaxInterval: "20s"},
		},
		{
			name:           "no backoff bounds means no retry_back_off object",
			p:              Policy{MaxAttempts: 2},
			wantNumRetries: 1,
			wantBackOff:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := policyToEnvoy(tc.p, true)
			if err != nil {
				t.Fatalf("policyToEnvoy(%+v) returned error: %v", tc.p, err)
			}
			var got envoyRetryPolicy
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("policyToEnvoy(%+v) produced unparseable output %s: %v", tc.p, data, err)
			}
			if got.NumRetries != tc.wantNumRetries {
				t.Errorf("num_retries = %d, want %d", got.NumRetries, tc.wantNumRetries)
			}
			if got.RetryOn != tc.wantRetryOn {
				t.Errorf("retry_on = %q, want %q", got.RetryOn, tc.wantRetryOn)
			}
			if (got.RetryBackOff == nil) != (tc.wantBackOff == nil) {
				t.Fatalf("retry_back_off = %+v, want %+v", got.RetryBackOff, tc.wantBackOff)
			}
			if tc.wantBackOff != nil && *got.RetryBackOff != *tc.wantBackOff {
				t.Errorf("retry_back_off = %+v, want %+v", *got.RetryBackOff, *tc.wantBackOff)
			}
		})
	}
}

// policiesEqual compares two Policy values field by field, treating a nil
// and empty RetryOn slice as equal since callers don't distinguish them.
func policiesEqual(a, b Policy) bool {
	if a.MaxAttempts != b.MaxAttempts ||
		a.InitialBackoff != b.InitialBackoff ||
		a.MaxBackoff != b.MaxBackoff ||
		a.BackoffMultiplier != b.BackoffMultiplier {
		return false
	}
	if len(a.RetryOn) != len(b.RetryOn) {
		return false
	}
	for i := range a.RetryOn {
		if a.RetryOn[i] != b.RetryOn[i] {
			return false
		}
	}
	return true
}
