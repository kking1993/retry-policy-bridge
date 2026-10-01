package main

import "testing"

func TestDiffRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		input    string
		want     string
	}{
		{
			name: "grpc fully representable",
			from: "grpc", to: "envoy",
			input: `{"maxAttempts":4,"initialBackoff":"0.5s","maxBackoff":"10s","backoffMultiplier":2,"retryableStatusCodes":["UNAVAILABLE","DEADLINE_EXCEEDED"]}`,
			want:  "no differences",
		},
		{
			name: "grpc code without envoy token",
			from: "grpc", to: "envoy",
			input: `{"maxAttempts":3,"retryableStatusCodes":["UNAVAILABLE","ABORTED"]}`,
			want:  "dropped: retryableStatusCodes[ABORTED]",
		},
		{
			name: "grpc multiplier is not kept",
			from: "grpc", to: "envoy",
			input: `{"maxAttempts":3,"backoffMultiplier":1.5,"retryableStatusCodes":["UNAVAILABLE"]}`,
			want:  "changed: backoffMultiplier: 1.5 -> 2",
		},
		{
			name: "grpc single attempt becomes one retry",
			from: "grpc", to: "envoy",
			input: `{"maxAttempts":1,"retryableStatusCodes":["UNAVAILABLE"]}`,
			want:  "changed: maxAttempts: 1 -> 2",
		},
		{
			name: "grpc duration spelling is not a change",
			from: "grpc", to: "envoy",
			input: `{"maxAttempts":2,"initialBackoff":"500ms","retryableStatusCodes":["UNAVAILABLE"]}`,
			want:  "no differences",
		},
		{
			name: "grpc service config wrapper",
			from: "grpc", to: "envoy",
			input: `{"methodConfig":[{"name":[{"service":"a.B"}],"retryPolicy":{"maxAttempts":2,"retryableStatusCodes":["INTERNAL","ABORTED"]}}]}`,
			want:  "dropped: retryableStatusCodes[ABORTED]",
		},
		{
			name: "envoy tokens and per try timeout",
			from: "envoy", to: "grpc",
			input: `{"retry_on":"5xx,grpc-unavailable,reset","num_retries":2,"per_try_timeout":"2s"}`,
			want: "dropped: per_try_timeout = 2s\n" +
				"dropped: retry_on[5xx]\n" +
				"dropped: retry_on[reset]",
		},
		{
			name: "envoy fully representable",
			from: "envoy", to: "grpc",
			input: `{"retry_on":"grpc-cancelled,grpc-internal","num_retries":3,"retry_back_off":{"base_interval":"0.25s","max_interval":"5s"}}`,
			want:  "no differences",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := diffRoundTrip(tt.from, tt.to, []byte(tt.input))
			if err != nil {
				t.Fatalf("diffRoundTrip: %v", err)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestDiffRoundTripErrors(t *testing.T) {
	if _, err := diffRoundTrip("grpc", "envoy", []byte(`{"maxAttempts":0}`)); err == nil {
		t.Error("expected an error for maxAttempts 0")
	}
	if _, err := diffRoundTrip("grpc", "aws", []byte(`{"maxAttempts":2}`)); err == nil {
		t.Error("expected an error for an unknown target format")
	}
}
