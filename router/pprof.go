//go:build debug
// +build debug

package router

// This file only imports pprof if "-tags debug" is supplied during build.

import (
	_ "net/http/pprof"
)
