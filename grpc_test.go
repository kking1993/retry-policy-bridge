package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFormatSeconds(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"whole seconds", 30 * time.Second, "30s"},
		{"zero", 0, "0s"},
		{"half second", 500 * time.Millisecond, "0.5s"},
		{"one and a half seconds", 1500 * time.Millisecond, "1.5s"},
		{"tenth of a second", 100 * time.Millisecond, "0.1s"},
		{"no trailing zeros to trim", 33 * time.Millisecond, "0.033s"},
		{"quarter second", 1250 * time.Millisecond, "1.25s"},
		{"large duration stays in seconds, not minutes", 3661 * time.Second, "3661s"},
		{"sub-millisecond rounds away to zero", time.Nanosecond, "0s"},
		{"negative duration", -1500 * time.Millisecond, "-1.5s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSeconds(tc.d)
			if got != tc.want {
				t.Errorf("formatSeconds(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

// formatSeconds is only fed durations parsed by time.ParseDuration on the
// way in, so round-tripping through it should reproduce the same duration
// for every value that survives the truncation to milliseconds.
func TestFormatSecondsRoundTrip(t *testing.T) {
	durations := []time.Duration{
		1 * time.Second,
		250 * time.Millisecond,
		2500 * time.Millisecond,
		60 * time.Second,
	}

	for _, d := range durations {
		s := formatSeconds(d)
		got, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("formatSeconds(%v) produced %q, which failed to parse: %v", d, s, err)
		}
		if got != d {
			t.Errorf("round trip of %v through %q gave %v", d, s, got)
		}
	}
}

func TestPolicyFromGRPC(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Policy
		wantErr bool
	}{
		{
			name: "bare retryPolicy object",
			in:   `{"maxAttempts":4,"initialBackoff":"0.5s","maxBackoff":"10s","backoffMultiplier":2,"retryableStatusCodes":["UNAVAILABLE","DEADLINE_EXCEEDED"]}`,
			want: Policy{
				MaxAttempts:       4,
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        10 * time.Second,
				BackoffMultiplier: 2,
				RetryOn:           []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"},
			},
		},
		{
			name: "full service config wrapper",
			in:   `{"methodConfig":[{"name":[{"service":"foo.Bar"}],"retryPolicy":{"maxAttempts":3,"initialBackoff":"1s","backoffMultiplier":1.5,"retryableStatusCodes":["UNAVAILABLE"]}}]}`,
			want: Policy{
				MaxAttempts:       3,
				InitialBackoff:    time.Second,
				BackoffMultiplier: 1.5,
				RetryOn:           []string{"UNAVAILABLE"},
			},
		},
		{
			name: "wrapper picks first methodConfig entry that has a retryPolicy",
			in:   `{"methodConfig":[{"name":[{"service":"no.Retry"}]},{"retryPolicy":{"maxAttempts":2}}]}`,
			want: Policy{MaxAttempts: 2},
		},
		{
			name: "minimal policy with no backoff or retryable codes",
			in:   `{"maxAttempts":1}`,
			want: Policy{MaxAttempts: 1},
		},
		{
			name:    "wrapper with methodConfig but no retryPolicy anywhere",
			in:      `{"methodConfig":[{"name":[{"service":"no.Retry"}]}]}`,
			wantErr: true,
		},
		{
			name:    "wrapper with empty methodConfig array",
			in:      `{"methodConfig":[]}`,
			wantErr: true,
		},
		{
			name:    "maxAttempts zero is rejected",
			in:      `{"maxAttempts":0}`,
			wantErr: true,
		},
		{
			name:    "maxAttempts missing is rejected",
			in:      `{}`,
			wantErr: true,
		},
		{
			name:    "negative maxAttempts is rejected",
			in:      `{"maxAttempts":-1}`,
			wantErr: true,
		},
		{
			name:    "invalid initialBackoff",
			in:      `{"maxAttempts":1,"initialBackoff":"soon"}`,
			wantErr: true,
		},
		{
			name:    "invalid maxBackoff",
			in:      `{"maxAttempts":1,"maxBackoff":"forever"}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			in:      `{"maxAttempts":`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := policyFromGRPC([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("policyFromGRPC(%q) succeeded, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("policyFromGRPC(%q) returned error: %v", tc.in, err)
			}
			if !policiesEqual(got, tc.want) {
				t.Errorf("policyFromGRPC(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPolicyToGRPC(t *testing.T) {
	cases := []struct {
		name string
		p    Policy
		want grpcRetryPolicy
	}{
		{
			name: "typical policy",
			p: Policy{
				MaxAttempts:       4,
				InitialBackoff:    500 * time.Millisecond,
				MaxBackoff:        10 * time.Second,
				BackoffMultiplier: 2,
				RetryOn:           []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"},
			},
			want: grpcRetryPolicy{
				MaxAttempts:          4,
				InitialBackoff:       "0.5s",
				MaxBackoff:           "10s",
				BackoffMultiplier:    2,
				RetryableStatusCodes: []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"},
			},
		},
		{
			name: "zero backoff durations are omitted, not written as 0s",
			p:    Policy{MaxAttempts: 1},
			want: grpcRetryPolicy{MaxAttempts: 1},
		},
		{
			name: "only initial backoff set",
			p:    Policy{MaxAttempts: 2, InitialBackoff: 2 * time.Second},
			want: grpcRetryPolicy{MaxAttempts: 2, InitialBackoff: "2s"},
		},
		{
			name: "only max backoff set",
			p:    Policy{MaxAttempts: 2, MaxBackoff: 30 * time.Second},
			want: grpcRetryPolicy{MaxAttempts: 2, MaxBackoff: "30s"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := policyToGRPC(tc.p, true)
			if err != nil {
				t.Fatalf("policyToGRPC(%+v) returned error: %v", tc.p, err)
			}
			var got grpcRetryPolicy
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("policyToGRPC(%+v) produced unparseable output %s: %v", tc.p, data, err)
			}
			if got.MaxAttempts != tc.want.MaxAttempts ||
				got.InitialBackoff != tc.want.InitialBackoff ||
				got.MaxBackoff != tc.want.MaxBackoff ||
				got.BackoffMultiplier != tc.want.BackoffMultiplier {
				t.Errorf("policyToGRPC(%+v) = %+v, want %+v", tc.p, got, tc.want)
			}
			if len(got.RetryableStatusCodes) != len(tc.want.RetryableStatusCodes) {
				t.Fatalf("retryableStatusCodes = %v, want %v", got.RetryableStatusCodes, tc.want.RetryableStatusCodes)
			}
			for i := range got.RetryableStatusCodes {
				if got.RetryableStatusCodes[i] != tc.want.RetryableStatusCodes[i] {
					t.Errorf("retryableStatusCodes = %v, want %v", got.RetryableStatusCodes, tc.want.RetryableStatusCodes)
				}
			}
		})
	}
}

func TestExtractGRPCRetryPolicy(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    grpcRetryPolicy
		wantErr bool
	}{
		{
			name: "bare object passes through untouched",
			in:   `{"maxAttempts":5}`,
			want: grpcRetryPolicy{MaxAttempts: 5},
		},
		{
			name: "wrapper with a single methodConfig entry",
			in:   `{"methodConfig":[{"retryPolicy":{"maxAttempts":2}}]}`,
			want: grpcRetryPolicy{MaxAttempts: 2},
		},
		{
			name: "wrapper skips entries with no retryPolicy before finding one",
			in:   `{"methodConfig":[{"retryPolicy":null},{"retryPolicy":{"maxAttempts":7}}]}`,
			want: grpcRetryPolicy{MaxAttempts: 7},
		},
		{
			name:    "wrapper with no retryPolicy anywhere",
			in:      `{"methodConfig":[{"retryPolicy":null}]}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			in:      `{"methodConfig":`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractGRPCRetryPolicy([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("extractGRPCRetryPolicy(%q) succeeded, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractGRPCRetryPolicy(%q) returned error: %v", tc.in, err)
			}
			if got.MaxAttempts != tc.want.MaxAttempts {
				t.Errorf("extractGRPCRetryPolicy(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
