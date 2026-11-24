module github.com/mikolaszuza/printer-canvas

go 1.26.11

require github.com/prusa3d/prusa-connect/sdk v0.0.0

// Until the SDK is published as a Go module, it is built from the pinned
// Prusa Connect checkout in .core/ (see CORE_REF in the Makefile). Delete
// these lines once `go get` can fetch it.
replace github.com/prusa3d/prusa-connect/sdk => ./.core/sdk
