# Build output

Use `go build -o bin/universebuild.exe .` for application builds. Keep one
canonical executable; do not create prefixed, suffixed, or feature-specific
binaries. If the executable is running and Windows prevents replacement, report
that the application must be closed before rebuilding instead of creating an
alternate executable.
