package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestAPathThisModuleDoesNotServeIsRefusedByCode is the refusal half. A client
// branches on the code, so the test reads the code and not the sentence: a
// reworded sentence must not break a caller, and a changed code must.
func TestAPathThisModuleDoesNotServeIsRefusedByCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nothing-here", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("an unserved path answered %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("the refusal is %s, want application/problem+json", got)
	}

	var problem struct {
		Code   string            `json:"code"`
		Title  string            `json:"title"`
		Detail string            `json:"detail"`
		Params map[string]string `json:"params"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the refusal is not JSON: %v", err)
	}

	if want := RouteNotFound.Code().String(); problem.Code != want {
		t.Fatalf("the refusal carries %q, want %q", problem.Code, want)
	}

	// The English is kept beside the code, so a log line and an untranslated
	// screen still read.
	if problem.Title == "" || problem.Detail == "" {
		t.Fatalf("the refusal lost its English: title %q, detail %q", problem.Title, problem.Detail)
	}

	if problem.Params["path"] != "/nothing-here" {
		t.Fatalf("the refusal names the path %q, want /nothing-here", problem.Params["path"])
	}
}

// TestNoSettingIsReadFromTheEnvironment keeps every setting in the manifest. A
// setting read from the environment is one an operator cannot find in the
// manifest, cannot see on the settings pane, and cannot set through the API.
func TestNoSettingIsReadFromTheEnvironment(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	if strings.Contains(string(source), "os.Getenv") {
		t.Fatal("main.go reads an environment variable. Declare the setting in module.yaml instead")
	}
}

// canvasServer serves the canvas routes over a fresh data directory, the way
// the host does once it has granted one.
func canvasServer(t *testing.T) (http.Handler, *canvasFile) {
	t.Helper()

	file := &canvasFile{dir: t.TempDir()}
	mux := http.NewServeMux()
	canvasRoutes(mux, func() *canvasFile { return file })

	return mux, file
}

func send(t *testing.T, h http.Handler, method, body string) (int, Canvas) {
	t.Helper()

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(method, "/canvas", strings.NewReader(body)))

	var c Canvas
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &c); err != nil {
			t.Fatalf("%s /canvas answered something that is not a canvas: %v: %s", method, err, recorder.Body)
		}
	}

	return recorder.Code, c
}

// TestChangesMergeInsteadOfOverwriting holds the rule of the shared canvas:
// a change touches only the printers and labels it names, in the order sent.
func TestChangesMergeInsteadOfOverwriting(t *testing.T) {
	server, _ := canvasServer(t)

	if code, c := send(t, server, http.MethodGet, ""); code != http.StatusOK || c.Revision != 0 || len(c.Tiles)+len(c.Labels) != 0 {
		t.Fatalf("a canvas nobody changed answered %d with %+v", code, c)
	}

	steps := []struct {
		change string
		check  func(Canvas) bool
	}{
		{`{"tiles": {"a": {"x": 10.4, "y": 20}, "b": {"x": 0, "y": 0}}}`,
			func(c Canvas) bool { return c.Tiles["a"] == Point{10, 20} && len(c.Tiles) == 2 }},
		{`{"tiles": {"b": {"x": -300, "y": 40}}}`,
			func(c Canvas) bool { return c.Tiles["a"] == Point{10, 20} && c.Tiles["b"] == Point{-300, 40} }},
		{`{"tiles": {"a": null}}`,
			func(c Canvas) bool { _, gone := c.Tiles["a"]; return !gone && len(c.Tiles) == 1 }},
		{`{"labels": {"shelf": {"x": 0, "y": -60, "text": "  Hallway shelf "}}}`,
			func(c Canvas) bool { return c.Labels["shelf"].Text == "Hallway shelf" && len(c.Tiles) == 1 }},
		{`{"labels": {"shelf": null}}`,
			func(c Canvas) bool { return len(c.Labels) == 0 && len(c.Tiles) == 1 }},
	}

	for i, step := range steps {
		code, c := send(t, server, http.MethodPatch, step.change)
		if code != http.StatusOK || c.Revision != i+1 || !step.check(c) {
			t.Fatalf("change %d %s answered %d with %+v", i+1, step.change, code, c)
		}

		if _, again := send(t, server, http.MethodGet, ""); again.Revision != c.Revision {
			t.Fatalf("after change %d the saved canvas is at revision %d, want %d", i+1, again.Revision, c.Revision)
		}
	}
}

// TestChangesThatCannotApplyAreRefused: each of these answers 400 with the
// module's code and leaves the canvas as it was.
func TestChangesThatCannotApplyAreRefused(t *testing.T) {
	server, _ := canvasServer(t)

	for _, change := range []string{
		`{}`,
		`{"tiles": []}`,
		`{"labels": {"l": {"x": 0, "y": 0, "text": "   "}}}`,
		`{"labels": {"l": {"x": 0, "y": 0, "text": "` + strings.Repeat("a", maxLabelText+1) + `"}}}`,
		`{"tiles": {"": {"x": 0, "y": 0}}}`,
		`{"tiles": {"a": {"x": 1e9, "y": 0}}}`,
		`{"tiles": {"` + strings.Repeat("x", maxIDLen+1) + `": {"x": 0, "y": 0}}}`,
		`not json`,
	} {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/canvas", strings.NewReader(change)))

		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), BadCanvasChange.Code().String()) {
			t.Fatalf("%.60s answered %d: %s", change, recorder.Code, recorder.Body)
		}
	}

	if _, c := send(t, server, http.MethodGet, ""); c.Revision != 0 {
		t.Fatalf("refused changes moved the canvas to revision %d", c.Revision)
	}
}

// TestACutShortFileIsRepairedByTheNextChange: a file a crash left empty must
// not lock everyone out, so it reads as an empty canvas and the next change
// saves over it.
func TestACutShortFileIsRepairedByTheNextChange(t *testing.T) {
	server, file := canvasServer(t)
	if err := os.WriteFile(file.path(), []byte(`{"revision": 4, "tiles": {"a"`), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, c := send(t, server, http.MethodGet, ""); code != http.StatusOK || c.Revision != 0 {
		t.Fatalf("a cut-short file answered %d at revision %d, want an empty canvas", code, c.Revision)
	}

	if code, c := send(t, server, http.MethodPatch, `{"tiles": {"a": {"x": 5, "y": 5}}}`); code != http.StatusOK || c.Revision != 1 {
		t.Fatalf("a change over a cut-short file answered %d at revision %d", code, c.Revision)
	}

	if _, err := os.Stat(file.path() + ".next"); !os.IsNotExist(err) {
		t.Fatalf("the save left its sibling file behind: %v", err)
	}
}

// TestWithoutADataDirectoryTheCanvasSaysSo: a module installed without the
// directory it asks for answers with its code, not a crash.
func TestWithoutADataDirectoryTheCanvasSaysSo(t *testing.T) {
	mux := http.NewServeMux()
	canvasRoutes(mux, func() *canvasFile { return nil })

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/canvas", nil))

	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), NowhereToSave.Code().String()) {
		t.Fatalf("no data directory answered %d: %s", recorder.Code, recorder.Body)
	}
}
