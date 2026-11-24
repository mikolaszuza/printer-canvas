// The canvas this module keeps: where every printer's card sits and the text
// labels people put beside them. It is one JSON file in the data directory the
// operator grants, shared by everyone who opens the page.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prusa3d/prusa-connect/sdk/routesapi"
)

const (
	// maxTiles bounds the file well under the body limit the host applies.
	maxTiles = 5000
	// maxCoord keeps a dragged card within reach of "Fit all".
	maxCoord = 1_000_000
	maxIDLen = 128
	// maxLabels and maxLabelText bound the text labels people write on the canvas.
	maxLabels    = 500
	maxLabelText = 120
	// maxChangeBytes is the most a single change may weigh: every printer and
	// every label at once, with room to spare.
	maxChangeBytes = 1 << 20
)

// Point is where the top-left corner of one printer's card sits on the
// canvas, in canvas units (pixels at zoom 1).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Label is a piece of text on the canvas, such as "Hallway shelf".
type Label struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
}

// Canvas is what every viewer of this server shares.
type Canvas struct {
	// Revision counts the saves, 0 before the first one. A client uses it to
	// tell whether it has to redraw, not to lock: every move is a merge.
	Revision int              `json:"revision"`
	Tiles    map[string]Point `json:"tiles"`
	Labels   map[string]Label `json:"labels"`
	SavedBy  string           `json:"saved_by,omitempty"`
	SavedAt  *time.Time       `json:"saved_at,omitempty"`
}

// Patch moves the printers and labels it names and leaves every other one
// where it is. A null entry takes that printer or label off the canvas.
type Patch struct {
	Tiles  map[string]*Point `json:"tiles"`
	Labels map[string]*Label `json:"labels"`
}

// errInvalid marks a change that cannot be applied as sent, as opposed to a
// file that could not be read or written.
var errInvalid = errors.New("invalid")

func (p Patch) validate() error {
	if p.Tiles == nil && p.Labels == nil {
		return fmt.Errorf("%w: the body has neither a tiles nor a labels object", errInvalid)
	}

	if len(p.Labels) > maxLabels {
		return fmt.Errorf("%w: %d labels in one patch, at most %d", errInvalid, len(p.Labels), maxLabels)
	}

	for id, label := range p.Labels {
		if id == "" || len(id) > maxIDLen {
			return fmt.Errorf("%w: a label id is empty or longer than %d bytes", errInvalid, maxIDLen)
		}

		if label == nil {
			continue
		}

		if text := strings.TrimSpace(label.Text); text == "" || utf8.RuneCountInString(text) > maxLabelText {
			return fmt.Errorf("%w: label %q needs 1 to %d characters of text", errInvalid, id, maxLabelText)
		}

		if !inRange(label.X) || !inRange(label.Y) {
			return fmt.Errorf("%w: label %q has a coordinate outside ±%d", errInvalid, id, maxCoord)
		}
	}

	if len(p.Tiles) > maxTiles {
		return fmt.Errorf("%w: %d printers in one patch, at most %d", errInvalid, len(p.Tiles), maxTiles)
	}

	for id, point := range p.Tiles {
		if id == "" || len(id) > maxIDLen {
			return fmt.Errorf("%w: a printer id is empty or longer than %d bytes", errInvalid, maxIDLen)
		}

		if point == nil {
			continue
		}

		if !inRange(point.X) || !inRange(point.Y) {
			return fmt.Errorf("%w: printer %q has a coordinate outside ±%d", errInvalid, id, maxCoord)
		}
	}

	return nil
}

func inRange(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= maxCoord
}

// applyTo merges the patch into c: named entries move or go, the rest stay.
// Coordinates land on whole canvas units and label text loses outer spaces.
func (p Patch) applyTo(c *Canvas) error {
	for id, point := range p.Tiles {
		if point == nil {
			delete(c.Tiles, id)
		} else {
			c.Tiles[id] = Point{X: math.Round(point.X), Y: math.Round(point.Y)}
		}
	}

	for id, label := range p.Labels {
		if label == nil {
			delete(c.Labels, id)
		} else {
			c.Labels[id] = Label{X: math.Round(label.X), Y: math.Round(label.Y), Text: strings.TrimSpace(label.Text)}
		}
	}

	switch {
	case len(c.Tiles) > maxTiles:
		return fmt.Errorf("%w: the canvas would hold %d printers, at most %d", errInvalid, len(c.Tiles), maxTiles)
	case len(c.Labels) > maxLabels:
		return fmt.Errorf("%w: the canvas would hold %d labels, at most %d", errInvalid, len(c.Labels), maxLabels)
	}

	return nil
}

// canvasFile is the file the canvas lives in. Each read and each change holds
// the lock for the whole read-merge-write, so two people moving printers at the
// same moment both land: the second change is merged into the first.
type canvasFile struct {
	lock sync.Mutex
	dir  string
}

func (f *canvasFile) path() string { return filepath.Join(f.dir, "canvas.json") }

// get returns the canvas as it is saved now.
func (f *canvasFile) get() (Canvas, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.read()
}

// change merges a patch into the saved canvas and saves the result.
func (f *canvasFile) change(p Patch, by string, at time.Time) (Canvas, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	c, err := f.read()
	if err != nil {
		return Canvas{}, err
	}

	if err = p.applyTo(&c); err != nil {
		return Canvas{}, err
	}

	c.Revision++
	c.SavedBy, c.SavedAt = by, &at

	if err = replaceFile(f.path(), c); err != nil {
		return Canvas{}, err
	}

	return c, nil
}

// read loads the file. A missing file is a canvas nobody has changed yet. A
// file that is not a canvas (one a crash cut short) reads the same way, so the
// next change repairs it instead of locking everyone out.
func (f *canvasFile) read() (Canvas, error) {
	c := Canvas{Tiles: map[string]Point{}, Labels: map[string]Label{}}

	raw, err := os.ReadFile(f.path())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return c, nil
	case err != nil:
		return Canvas{}, fmt.Errorf("read %s: %w", f.path(), err)
	}

	if err = json.Unmarshal(raw, &c); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s is not a canvas, it reads as empty until the next change: %v\n", name, f.path(), err)

		return Canvas{Tiles: map[string]Point{}, Labels: map[string]Label{}}, nil
	}

	if c.Tiles == nil {
		c.Tiles = map[string]Point{}
	}

	if c.Labels == nil {
		c.Labels = map[string]Label{}
	}

	return c, nil
}

// replaceFile writes value as JSON to target in one step. The JSON goes to a
// sibling file first, is flushed to the disk, and then takes target's place by
// a rename, so a crash leaves either the old file or the new one.
func replaceFile(target string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", target, err)
	}

	next := target + ".next"

	out, err := os.OpenFile(next, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", next, err)
	}

	_, err = out.Write(raw)
	if err == nil {
		err = out.Sync()
	}

	if closeErr := out.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(next, target)
	}

	if err != nil {
		_ = os.Remove(next)

		return fmt.Errorf("save %s: %w", target, err)
	}

	return nil
}

// canvasRoutes serves the canvas: GET reads it, PATCH changes it. file is nil
// until the host has granted a data directory.
func canvasRoutes(mux *http.ServeMux, file func() *canvasFile) {
	mux.HandleFunc("GET /canvas", onCanvas(file, func(w http.ResponseWriter, r *http.Request, f *canvasFile) {
		c, err := f.get()
		if err != nil {
			fileFailed(w, r, err)

			return
		}

		answer(w, http.StatusOK, c)
	}))

	mux.HandleFunc("PATCH /canvas", onCanvas(file, func(w http.ResponseWriter, r *http.Request, f *canvasFile) {
		var p Patch

		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChangeBytes))
		if err := decoder.Decode(&p); err != nil {
			refuse(w, r, BadCanvasChange.New(fmt.Sprintf("the body is not a canvas change: %v", err)))

			return
		}

		if err := p.validate(); err != nil {
			refuse(w, r, BadCanvasChange.New(err.Error()))

			return
		}

		c, err := f.change(p, routesapi.PrincipalOf(r), time.Now().UTC())

		switch {
		case errors.Is(err, errInvalid):
			refuse(w, r, BadCanvasChange.New(err.Error()))
		case err != nil:
			fileFailed(w, r, err)
		default:
			answer(w, http.StatusOK, c)
		}
	}))
}

// onCanvas runs handle with the canvas file, or refuses when the module was
// installed without a data directory to keep it in.
func onCanvas(file func() *canvasFile, handle func(http.ResponseWriter, *http.Request, *canvasFile)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := file()
		if f == nil {
			refuse(w, r, NowhereToSave.New("no data directory was granted to printer-canvas"))

			return
		}

		handle(w, r, f)
	}
}

// fileFailed refuses a request the canvas file could not serve. The reason is
// kept out of the answer, which can reach any viewer, so it goes to the log.
func fileFailed(w http.ResponseWriter, r *http.Request, err error) {
	fmt.Fprintf(os.Stderr, "%s: %s %s: %v\n", name, r.Method, r.URL.Path, err)
	refuse(w, r, CanvasFileFailed.New(err.Error()))
}
