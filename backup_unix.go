//go:build !windows

package main

import "strconv"

func fmtPath(src string) string { return strconv.Quote(src) }
