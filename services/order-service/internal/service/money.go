package service

import "math"

// Prices travel as float64 on the wire (protobuf double) and are stored as NUMERIC(18,2).
// All arithmetic is done in integer minor units (cents) to avoid floating point drift.

func toMinor(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func fromMinor(minor int64) float64 {
	return float64(minor) / 100
}
