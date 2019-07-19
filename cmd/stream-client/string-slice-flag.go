package main

import "fmt"

type stringSlice []string

func (ss *stringSlice) String() string {
	// The flag package may call the String method with a zero-valued receiver,
	// such as a nil pointer.
	// This lets us avoid panicking when a nil pointer is dereferenced.
	if ss == nil {
		return "[]"
	}

	return fmt.Sprintf("%v", *ss)
}

func (ss *stringSlice) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}
