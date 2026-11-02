//go:build !linux || !cgo

package main

// Nothing to build here: the core is a HarmonyOS library (main_linux.go). This
// only exists so `go build ./...` and an editor work on a developer's machine.
func main() {}
