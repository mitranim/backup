package main

import (
	"math"
	"testing"

	"github.com/mitranim/gg/gtest"
)

// TODO: actual tests.

func TestIndex(t *testing.T) {
	defer gtest.Catch(t)

	gtest.Eq(Index(0).Width(), 1)
	gtest.Eq(Index(1).Width(), 1)
	gtest.Eq(Index(9).Width(), 1)
	gtest.Eq(Index(10).Width(), 2)
	gtest.Eq(Index(19).Width(), 2)
	gtest.Eq(Index(99).Width(), 2)
	gtest.Eq(Index(100).Width(), 3)
	gtest.Eq(Index(math.MaxUint64).Width(), 20)

	gtest.Eq(Index(0).String(), `00000000000000000000`)
	gtest.Eq(Index(1).String(), `00000000000000000001`)
	gtest.Eq(Index(2).String(), `00000000000000000002`)
	gtest.Eq(Index(3).String(), `00000000000000000003`)
	gtest.Eq(Index(9).String(), `00000000000000000009`)
	gtest.Eq(Index(10).String(), `00000000000000000010`)
	gtest.Eq(Index(19).String(), `00000000000000000019`)
	gtest.Eq(Index(100).String(), `00000000000000000100`)
	gtest.Eq(Index(199).String(), `00000000000000000199`)
	gtest.Eq(Index(math.MaxUint64).String(), `18446744073709551615`)
}
