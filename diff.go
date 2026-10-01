package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// durationKeys are the fields whose string values are durations. They are
// compared as durations so "500ms" and "0.5s" don't show up as a change.
var durationKeys = map[string]bool{
	"initialBackoff":  true,
	"maxBackoff":      true,
	"base_interval":   true,
	"max_interval":    true,
	"per_try_timeout": true,
}

// canonicalJSON re-encodes the input as the bare policy object of the given
// format, so a full service config and a bare retryPolicy compare the same
// way against the round-tripped output.
func canonicalJSON(format string, data []byte) ([]byte, error) {
	switch format {
	case "grpc":
		g, err := extractGRPCRetryPolicy(data)
		if err != nil {
			return nil, err
		}
		return json.Marshal(g)
	case "envoy":
		var e envoyRetryPolicy
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return json.Marshal(e)
	default:
		return nil, fmt.Errorf("unknown format %q (want grpc or envoy)", format)
	}
}

// roundTrip converts data from one format to the other and back again,
// returning the result in the original format.
func roundTrip(from, to string, data []byte) ([]byte, error) {
	p, err := decode(from, data)
	if err != nil {
		return nil, fmt.Errorf("reading %s input: %w", from, err)
	}
	mid, err := encode(to, p, false)
	if err != nil {
		return nil, fmt.Errorf("writing %s output: %w", to, err)
	}
	back, err := decode(to, mid)
	if err != nil {
		return nil, fmt.Errorf("reading converted %s output: %w", to, err)
	}
	out, err := encode(from, back, false)
	if err != nil {
		return nil, fmt.Errorf("writing %s output: %w", from, err)
	}
	return out, nil
}

// diffRoundTrip reports which parts of the input did not survive a trip to
// the other format and back. Each line starts with dropped, changed or
// added.
func diffRoundTrip(from, to string, data []byte) (string, error) {
	before, err := canonicalJSON(from, data)
	if err != nil {
		return "", fmt.Errorf("reading %s input: %w", from, err)
	}
	rt, err := roundTrip(from, to, data)
	if err != nil {
		return "", err
	}

	a, err := flatten(before)
	if err != nil {
		return "", err
	}
	b, err := flatten(rt)
	if err != nil {
		return "", err
	}

	var lines []string
	for k, av := range a {
		bv, ok := b[k]
		switch {
		case !ok:
			lines = append(lines, fmt.Sprintf("dropped: %s", describe(k, av)))
		case av != bv:
			lines = append(lines, fmt.Sprintf("changed: %s: %s -> %s", k, av, bv))
		}
	}
	for k, bv := range b {
		if _, ok := a[k]; !ok {
			lines = append(lines, fmt.Sprintf("added: %s", describe(k, bv)))
		}
	}
	if len(lines) == 0 {
		return "no differences", nil
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

// describe prints a flattened entry. List members carry no value of their
// own, so they print as just the key.
func describe(key, value string) string {
	if value == "" {
		return key
	}
	return key + " = " + value
}

// flatten turns a JSON object into path -> value pairs. The list-like
// fields (a JSON array of strings, and envoy's comma-separated retry_on)
// become one entry per member, so a single dropped status code is reported
// on its own instead of shifting every index after it.
func flatten(data []byte) (map[string]string, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	out := make(map[string]string)
	flattenInto(out, "", root)
	return out, nil
}

func flattenInto(out map[string]string, prefix string, m map[string]any) {
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		switch t := v.(type) {
		case map[string]any:
			flattenInto(out, path, t)
		case []any:
			for _, item := range t {
				out[fmt.Sprintf("%s[%v]", path, item)] = ""
			}
		case string:
			switch {
			case k == "retry_on":
				for _, tok := range strings.Split(t, ",") {
					if tok = strings.TrimSpace(tok); tok != "" {
						out[fmt.Sprintf("%s[%s]", path, tok)] = ""
					}
				}
			case durationKeys[k]:
				if d, err := time.ParseDuration(t); err == nil {
					t = formatSeconds(d)
				}
				out[path] = t
			default:
				out[path] = t
			}
		default:
			out[path] = fmt.Sprint(t)
		}
	}
}
