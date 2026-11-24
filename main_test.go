package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

// TestMovesMergeSoNobodyOverwritesAnotherPrinter holds the one rule of the
// shared canvas: a patch moves only the printers it names.
func TestMovesMergeSoNobodyOverwritesAnotherPrinter(t *testing.T) {
	store := &canvasStore{dir: t.TempDir()}
	mux := http.NewServeMux()
	canvasRoutes(mux, func() *canvasStore { return store })

	patch := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/canvas", strings.NewReader(body)))

		return recorder
	}

	read := func() Canvas {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/canvas", nil))

		var canvas Canvas
		if err := json.Unmarshal(recorder.Body.Bytes(), &canvas); err != nil {
			t.Fatalf("the canvas is not JSON: %v: %s", err, recorder.Body)
		}

		return canvas
	}

	if got := read(); got.Revision != 0 || len(got.Tiles) != 0 {
		t.Fatalf("a fresh canvas reads %+v, want revision 0 and no tiles", got)
	}

	if got := patch(`{"tiles": {"a": {"x": 10.4, "y": 20}, "b": {"x": 0, "y": 0}}}`); got.Code != http.StatusOK {
		t.Fatalf("the first move answered %d: %s", got.Code, got.Body)
	}

	if got := patch(`{"tiles": {"b": {"x": -300, "y": 40}}}`); got.Code != http.StatusOK {
		t.Fatalf("the second move answered %d: %s", got.Code, got.Body)
	}

	got := read()
	if got.Revision != 2 || got.Tiles["a"] != (Point{10, 20}) || got.Tiles["b"] != (Point{-300, 40}) {
		t.Fatalf("after two moves the canvas reads %+v", got)
	}

	if got := patch(`{"tiles": {"a": null}}`); got.Code != http.StatusOK {
		t.Fatalf("taking a printer off answered %d: %s", got.Code, got.Body)
	}

	if got := read(); len(got.Tiles) != 1 {
		t.Fatalf("a null point left %d tiles, want 1", len(got.Tiles))
	}

	if got := patch(`{"labels": {"l1": {"x": 0, "y": -60, "text": "  Hallway shelf "}}}`); got.Code != http.StatusOK {
		t.Fatalf("adding a label answered %d: %s", got.Code, got.Body)
	}

	if got := read(); got.Labels["l1"].Text != "Hallway shelf" || len(got.Tiles) != 1 {
		t.Fatalf("after adding a label the canvas reads %+v", got)
	}

	if got := patch(`{"labels": {"l1": null}}`); got.Code != http.StatusOK || len(read().Labels) != 0 {
		t.Fatalf("removing a label answered %d: %s", got.Code, got.Body)
	}

	for _, body := range []string{
		`{}`,
		`{"tiles": []}`,
		`{"labels": {"l": {"x": 0, "y": 0, "text": "   "}}}`,
		`{"labels": {"l": {"x": 0, "y": 0, "text": "` + strings.Repeat("a", 121) + `"}}}`,
		`{"tiles": {"": {"x": 0, "y": 0}}}`,
		`{"tiles": {"a": {"x": 1e9, "y": 0}}}`,
		`{"tiles": {"` + strings.Repeat("x", 200) + `": {"x": 0, "y": 0}}}`,
		`not json`,
	} {
		if got := patch(body); got.Code != http.StatusBadRequest {
			t.Fatalf("%.60s answered %d, want 400", body, got.Code)
		}
	}
}

// TestAnUnreadableCanvasCanBeSavedOver: a file a crash left empty must not
// lock everyone out, so it reads as nothing saved and the next move repairs it.
func TestAnUnreadableCanvasCanBeSavedOver(t *testing.T) {
	store := &canvasStore{dir: t.TempDir()}
	if err := os.WriteFile(store.path(), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if canvas, err := store.load(); err != nil || canvas.Revision != 0 {
		t.Fatalf("an empty file read as revision %d, %v; want 0", canvas.Revision, err)
	}

	x := 5.0
	if _, err := store.apply(Patch{Tiles: map[string]*Point{"a": {X: x, Y: x}}}, "user:test", time.Now()); err != nil {
		t.Fatalf("a move over an empty file failed: %v", err)
	}
}
