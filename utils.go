package main

import ()

func getKeys[K comparable, V comparable](m map[K]V) []K {
	keys := make([]K, len(m))
	var i int
	for key := range m {
		keys[i] = key
		i++
	}
	return keys
}
