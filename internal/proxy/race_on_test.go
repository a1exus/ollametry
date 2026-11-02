//go:build race

package proxy

// raceEnabled is set when tests run with -race, which adds allocations.
const raceEnabled = true
