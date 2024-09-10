//go:build windows

package main

func fmtPath(src string) string { return `"` + src + `"` }
