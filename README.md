# retryconv

We run gRPC services with client-side retry policies set in the service
config, and those same services sit behind Envoy, which has its own retry
policy on the route. The two configs need to agree, or a client and its
sidecar end up retrying on different conditions with different backoffs.
Keeping them in sync by hand means re-deriving one from the other every
time someone tunes a value, which is how they drift apart.

retryconv converts a retry policy between the two shapes:

- the `retryPolicy` object from a gRPC service config
- the `retry_policy` object from an Envoy route

It reads from a file or from stdin, and writes to a file or stdout.

## Usage

Convert a gRPC retry policy to its Envoy equivalent:

```
$ cat grpc-policy.json
{
  "maxAttempts": 4,
  "initialBackoff": "0.5s",
  "maxBackoff": "10s",
  "backoffMultiplier": 2,
  "retryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED"]
}

$ retryconv -from grpc -to envoy -in grpc-policy.json
{
  "retry_on": "grpc-unavailable,grpc-deadline-exceeded",
  "num_retries": 3,
  "retry_back_off": {
    "base_interval": "0.5s",
    "max_interval": "10s"
  }
}
```

Or pipe it through stdin, which works the same way:

```
$ cat grpc-policy.json | retryconv -from grpc -to envoy > envoy-policy.json
```

The reverse direction works too:

```
$ retryconv -from envoy -to grpc -in envoy-policy.json -out grpc-policy.json
```

If `-in` is omitted, or given as `-`, retryconv reads stdin. The same goes
for `-out` and stdout. That makes it easy to drop into a build pipeline
that already pipes config through other tools:

```
$ generate-envoy-config | retryconv -from envoy -to grpc | apply-to-service-config
```

## What doesn't round-trip

The two formats don't line up one-to-one:

- Envoy only defines `grpc-*` retry_on tokens for five gRPC status codes
  (`CANCELLED`, `DEADLINE_EXCEEDED`, `INTERNAL`, `RESOURCE_EXHAUSTED`,
  `UNAVAILABLE`). Any other status code in `retryableStatusCodes` is
  dropped when converting to Envoy, and any non-grpc token in `retry_on`
  (`5xx`, `reset`, `connect-failure`, ...) is dropped when converting to
  gRPC, since gRPC's retry policy has no matching concept.
- gRPC's `backoffMultiplier` has no Envoy equivalent; Envoy always doubles
  its base interval. Converting envoy to grpc fills in `2` as the
  multiplier, which is a reasonable default but not something read back
  out of the input.
- Envoy's `per_try_timeout` has no gRPC equivalent and is dropped when
  converting to gRPC.
- `maxAttempts` (gRPC, counts the first try) and `num_retries` (Envoy,
  counts only the retries) differ by exactly one; the conversion accounts
  for that but it's worth knowing about if you're comparing the numbers
  by eye.

## Building

Standard library only, no dependencies to fetch:

```
$ go build -o retryconv .
```
