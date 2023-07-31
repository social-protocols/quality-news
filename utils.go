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

func mapSlice[T, U any](ts []T, f func(T) U) []U {
	results := make([]U, len(ts))
	for i, t := range ts {
		results[i] = f(t)
	}
	return results
}
