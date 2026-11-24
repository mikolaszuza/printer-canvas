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

	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// faultDomain is the first half of every code below. It is this module's
// namespace, which the host derives from the Go module path in go.mod
// (github.com/mikolaszuza/printer-canvas), so no other author can register a
// code under it. The sdk refuses the core namespace, which is what keeps this
// module's codes apart from the server's.
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

	// BadCanvasChange is a change to the canvas that cannot be applied as sent.
	BadCanvasChange = fault.Register(faultDomain, "BAD_CANVAS_CHANGE", http.StatusBadRequest, fault.Document(
		"That change to the canvas cannot be applied",
		"Send {\"tiles\": {\"<printer id>\": {\"x\": 0, \"y\": 0} or null}, \"labels\": {\"<label id>\": {\"x\": 0, \"y\": 0, \"text\": \"Shelf\"} or null}}. Coordinates stay within one million of the origin, a label holds 1 to 120 characters, and the canvas holds at most 5000 printers and 500 labels.",
		fault.RetryNever,
		fault.ShowDetail,
	))

	// CanvasFileFailed is a canvas file the process could not read or replace.
	CanvasFileFailed = fault.Register(faultDomain, "CANVAS_FILE_FAILED", http.StatusInternalServerError, fault.Document(
		"The canvas could not be read or saved",
		"Look at the server log for the reason, then at the module's data directory: its permissions and free space.",
		fault.RetryAllowed,
		fault.RedactDetail,
	))

	// NowhereToSave is a module installed without the data directory it asks for.
	NowhereToSave = fault.Register(faultDomain, "NOWHERE_TO_SAVE", http.StatusServiceUnavailable, fault.Document(
		"Printer Canvas has no place to keep the canvas",
		"Install the module again and approve the data directory it asks for.",
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
	problem := raised.ProblemJSON(r.URL.Path)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)

	_ = json.NewEncoder(w).Encode(problem)
}
