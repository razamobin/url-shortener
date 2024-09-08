package main

import (
	"testing"
)

func TestGenerateShortCode(t *testing.T) {
	testCases := []struct {
		id       int64
		expected string
	}{
		{0, "26GwX"},
		{1, "26GwY"},
		{57, "26GxW"},
		{58, "26GxX"},
		{123456, "26ue5"},
		{1000000, "2BQCu"},
		{1000000000, "2YTY6q"},
	}

	for _, tc := range testCases {
		result := generateShortCode(tc.id)
		if result != tc.expected {
			t.Errorf("For id %d, expected %s, but got %s", tc.id, tc.expected, result)
		}
	}
}