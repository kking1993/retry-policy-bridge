// Command retryconv converts a retry policy between the JSON shape used
// in a gRPC service config's retryPolicy field and the JSON shape used
// in an Envoy route's retry_policy field.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	from := flag.String("from", "", "source format: grpc or envoy")
	to := flag.String("to", "", "target format: grpc or envoy")
	in := flag.String("in", "-", "input file, or - for stdin")
	out := flag.String("out", "-", "output file, or - for stdout")
	pretty := flag.Bool("pretty", true, "indent the output JSON; -pretty=false writes it compact, one line")
	flag.Parse()

	if *from == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "retryconv: -from and -to are required (grpc or envoy)")
		flag.Usage()
		os.Exit(2)
	}

	data, err := readInput(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retryconv: %v\n", err)
		os.Exit(1)
	}

	policy, err := decode(*from, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retryconv: reading %s input: %v\n", *from, err)
		os.Exit(1)
	}

	output, err := encode(*to, policy, *pretty)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retryconv: writing %s output: %v\n", *to, err)
		os.Exit(1)
	}
	output = append(output, '\n')

	if err := writeOutput(*out, output); err != nil {
		fmt.Fprintf(os.Stderr, "retryconv: %v\n", err)
		os.Exit(1)
	}
}

func readInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func writeOutput(path string, data []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func decode(format string, data []byte) (Policy, error) {
	switch format {
	case "grpc":
		return policyFromGRPC(data)
	case "envoy":
		return policyFromEnvoy(data)
	default:
		return Policy{}, fmt.Errorf("unknown format %q (want grpc or envoy)", format)
	}
}

func encode(format string, p Policy, pretty bool) ([]byte, error) {
	switch format {
	case "grpc":
		return policyToGRPC(p, pretty)
	case "envoy":
		return policyToEnvoy(p, pretty)
	default:
		return nil, fmt.Errorf("unknown format %q (want grpc or envoy)", format)
	}
}
