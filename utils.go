package main

func getKeys[K comparable, V comparable](m map[K]V) []K {
	keys := make([]K, len(m))
	var i int
	for key := range m {
		keys[i] = key
		i++
	}
	return keys
}

func getValues[K comparable, V any](m map[K]V) []V {
	values := make([]V, len(m))
	var i int
	for _, value := range m {
		values[i] = value
		i++
	}
	return values
}
