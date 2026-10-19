// Command openapi-bundle merges api/openapi/_base.yaml with every module
// fragment api/openapi/<module>.yaml (sorted) into api/openapi.yaml.
// Fragments may contain only `paths` and `components`; duplicate paths or
// component names are errors, so modules never edit each other's specs.
//
//	go run ./tools/openapi-bundle            # write openapi.yaml
//	go run ./tools/openapi-bundle -check     # fail if openapi.yaml is stale
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
)

func main() {
	dir := flag.String("dir", "openapi", "fragment directory")
	out := flag.String("out", "openapi.yaml", "bundled output")
	check := flag.Bool("check", false, "verify the output is up to date")
	flag.Parse()
	b, err := Bundle(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi-bundle:", err)
		os.Exit(1)
	}
	if *check {
		cur, _ := os.ReadFile(*out)
		if !bytes.Equal(cur, b) {
			fmt.Fprintln(os.Stderr, "openapi-bundle: openapi.yaml is stale; run `make openapi`")
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil { //nolint:gosec // G306: committed source file
		fmt.Fprintln(os.Stderr, "openapi-bundle:", err)
		os.Exit(1)
	}
}
