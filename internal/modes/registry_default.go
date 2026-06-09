//go:build !patched_pgx

package modes

// builtinModes returns the set of modes available without the patched-pgx
// fork. Mode 4 ('M' RequestHeaders message) is not in this set; see
// builtinModes in registry_patched.go for the build-tagged variant.
func builtinModes() []Mode {
	return []Mode{
		&mode0{},
		&mode1a{},
		&mode1b{},
		&mode2a{},
		&mode2b{},
		&mode3{},
	}
}
