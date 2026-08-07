module github.com/valzkat1/vestigio

// 1.25 rather than 1.22 because of macOS, not because of a language feature.
// Binaries linked by the 1.22 toolchain are rejected by current dyld on
// darwin/arm64 with "missing LC_UUID load command" — they build, and then abort
// on launch. CI caught it on the three-platform matrix's first run, which is
// exactly why that matrix exists: the README promises a static binary that
// cross-compiles anywhere, and that promise had quietly stopped being true.
go 1.25

require modernc.org/sqlite v1.34.5

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.22.0 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
)
