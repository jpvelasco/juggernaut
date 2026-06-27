//go:build tools

// This file ensures tool dependencies are tracked in go.mod.
// See: https://github.com/golang/go/issues/25922

package main

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/securego/gosec/v2/cmd/gosec"
)
