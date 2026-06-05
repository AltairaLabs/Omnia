// Package main is a standalone code-generation tool.
// Run with: go run gen.go
//
// gen.go carries a //go:build ignore constraint and is excluded from normal
// builds. This file provides the package declaration and a stub main so that
// go build ./... and go vet do not error with "build constraints exclude all
// Go files" or "function main is undeclared".
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "usage: go run gen.go")
	os.Exit(1)
}
