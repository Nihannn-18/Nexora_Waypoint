package routes

import "sort"

// sortStrings sorts ids in place. Kept as a named helper so the locking order in
// Confirm reads clearly: deterministic order prevents concurrent confirmations
// from deadlocking on the row locks.
func sortStrings(s []string) { sort.Strings(s) }
