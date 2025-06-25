package main

import (
	"fmt"
	"hash/fnv"
)

// Global storage for collision-free mapping
var (
	stringToInt = make(map[string]int32)
	intToString = make(map[int32]string)
	nextID      int32 = 0
)

// StringToInt32 converts string to int32 using hash with collision handling
func StringToInt32(s string) int32 {
	// Return existing mapping if already converted
	if id, exists := stringToInt[s]; exists {
		return id
	}

	// Try hash first for better distribution
	h := fnv.New32a()
	h.Write([]byte(s))
	hash := int32(h.Sum32())
	
	// If hash is available, use it
	if _, exists := intToString[hash]; !exists {
		stringToInt[s] = hash
		intToString[hash] = s
		return hash
	}

	// Hash collision detected, use sequential ID
	stringToInt[s] = nextID
	intToString[nextID] = s
	result := nextID
	nextID++
	return result
}

// Int32ToString converts int32 back to original string
func Int32ToString(n int32) (string, bool) {
	str, exists := intToString[n]
	return str, exists
}
