// The codes this module refuses a caller with.
//
// A refusal a person or a client can see is a registered code, never a bare
// English sentence. The code is what a browser half switches on to choose a
// translation, and the English sentence travels beside it so a log line and an
// untranslated screen still read.
//
// Registering a code takes its documentation with it: what went wrong, what the
// caller should do, whether sending the same request again can work, and the
// parameters the sentence names. The sdk refuses a registration without them.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// faultDomain is the first half of every code below. It is this module's
// namespace, which the host derived from the Go module path github.com/mikolaszuza/printer-canvas (change both when the module moves), so no
// other author can register a code under it. The sdk refuses the core
// namespace, which is what keeps this module's codes apart from the server's.
//
//nolint:gochecknoglobals // a domain built once at start-up; a const cannot hold one.
var faultDomain = mustDomain("com.github.aux.mikolaszuza.printer-canvas")

// The codes this module raises. Add a code here, and nowhere else, so the list
// a client branches on is one file.
//
//nolint:gochecknoglobals // definitions built once at start-up.
var (
	// RouteNotFound is a path this module does not serve.
	RouteNotFound = fault.Register(faultDomain, "ROUTE_NOT_FOUND", http.StatusNotFound, fault.Document(
		"This module does not serve that path",
		"Read openapi.json. Every path in it is relative to the mount the operator gave this module.",
		fault.RetryNever,
		fault.ShowDetail,
		"path",
	))

	// LayoutChanged is a save that named a version someone else has saved over.

	// LayoutInvalid is a save whose body is not a layout.
	LayoutInvalid = fault.Register(faultDomain, "LAYOUT_INVALID", http.StatusBadRequest, fault.Document(
		"The body is not a layout",
		"Send {\"tiles\": {\"<printer id>\": {\"x\": 0, \"y\": 0} or null}}, with at most 5000 printers and whole-number coordinates within one million of the origin.",
		fault.RetryNever,
		fault.ShowDetail,
	))

	// LayoutUnreadable is a layout file this process cannot read or write.
	LayoutUnreadable = fault.Register(faultDomain, "LAYOUT_UNREADABLE", http.StatusInternalServerError, fault.Document(
		"The layout file could not be read or written",
		"Check the module's data directory: its permissions, free space, and that canvas.json there is JSON.",
		fault.RetryAllowed,
		fault.RedactDetail,
	))

	// NoDataDirectory is a module the operator granted no data directory.
	NoDataDirectory = fault.Register(faultDomain, "NO_DATA_DIRECTORY", http.StatusServiceUnavailable, fault.Document(
		"Printer Canvas has nowhere to keep the layout",
		"Install the module again and approve its data directory.",
		fault.RetryNever,
		fault.ShowDetail,
	))
)

// mustDomain builds this module's fault domain. A name the sdk refuses is a
// mistake in this file, so it stops the process at start-up rather than
// answering a caller with an empty code.
func mustDomain(name string) fault.Domain {
	domain, err := fault.NewDomain(name)
	if err != nil {
		panic(fmt.Sprintf("printer-canvas: %s is not a fault domain: %v", name, err))
	}

	return domain
}

// refuse writes one refusal as RFC 9457 frames it, which is the shape the
// generated client reads and the shape describeFault reads in the browser half:
// the code, the English sentence, the values that sentence names, and where the
// code is documented.
func refuse(w http.ResponseWriter, r *http.Request, raised *fault.Fault) {
	// A redacted detail leaves the answer, so the log is the only place it is read.
	if raised.Redacted() {
		fmt.Fprintf(os.Stderr, "%s: %s: %s\n", name, raised.Code(), raised.Message())
	}

	problem := raised.ProblemJSON(r.URL.Path)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)

	_ = json.NewEncoder(w).Encode(problem)
}
