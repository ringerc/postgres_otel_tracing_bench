//go:build patched_pgx

package modes

// builtinModes returns the set of modes available with the patched-pgx
// fork (github.com/ringerc/pgx_patches) wired in via a go.mod replace.
// Mode 4 ('M' RequestHeaders message) joins the set.
func builtinModes() []Mode {
	return []Mode{
		&mode0{},
		&mode1a{},
		&mode1b{},
		&mode2a{},
		&mode2b{},
		&mode3{},
		&mode4{},
	}
}
