package tmux

import (
	"fmt"
)

// properties is a helper function to make implementing property lookups for
// different tmux entities a little easier.
// keys are all the properties being fetched.
// fn is a function that takes a slice of strings and fetches the property
// values for those keys.
func properties[T ~string](keys []T, fn func([]string) ([]string, error)) (map[T]string, error) {
	keyStrings := make([]string, len(keys))
	for i, k := range keys {
		keyStrings[i] = string(k)
	}

	props, err := fn(keyStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch properties %s: %w", keyStrings, err)
	}
	res := make(map[T]string, len(keys))
	for i, prop := range props {
		res[keys[i]] = prop
	}
	return res, nil
}
