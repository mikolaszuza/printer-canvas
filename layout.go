package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// maxCoord keeps a dragged tile within reach of "fit all".
	maxCoord = 1_000_000
	maxIDLen = 128
	// maxLabels and maxLabelText bound the text labels people write on the canvas.
	maxLabels    = 500
	maxLabelText = 120
)

// Point is where the top-left corner of one printer's tile sits on the
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

// Patch moves the printers it names and leaves every other one where it is.
// A null point takes the printer off the canvas.
type Patch struct {
	Tiles  map[string]*Point `json:"tiles"`
	Labels map[string]*Label `json:"labels"`
}

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

type canvasStore struct {
	mu  sync.Mutex
	dir string
}

func (s *canvasStore) path() string { return filepath.Join(s.dir, "canvas.json") }

func (s *canvasStore) load() (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.read()
}

func (s *canvasStore) read() (Canvas, error) {
	empty := Canvas{Tiles: map[string]Point{}, Labels: map[string]Label{}}

	raw, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}

	if err != nil {
		return Canvas{}, fmt.Errorf("read the canvas: %w", err)
	}

	var held Canvas
	if err = json.Unmarshal(raw, &held); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s holds no canvas, starting over: %v\n", name, s.path(), err)

		return empty, nil
	}

	if held.Tiles == nil {
		held.Tiles = map[string]Point{}
	}

	if held.Labels == nil {
		held.Labels = map[string]Label{}
	}

	return held, nil
}

// apply merges a patch into the stored canvas and writes it atomically.
func (s *canvasStore) apply(patch Patch, by string, now time.Time) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.read()
	if err != nil {
		return Canvas{}, err
	}

	for id, point := range patch.Tiles {
		if point == nil {
			delete(current.Tiles, id)

			continue
		}

		current.Tiles[id] = Point{X: math.Round(point.X), Y: math.Round(point.Y)}
	}

	for id, label := range patch.Labels {
		if label == nil {
			delete(current.Labels, id)

			continue
		}

		current.Labels[id] = Label{X: math.Round(label.X), Y: math.Round(label.Y), Text: strings.TrimSpace(label.Text)}
	}

	if len(current.Labels) > maxLabels {
		return Canvas{}, fmt.Errorf("%w: the canvas would hold %d labels, at most %d", errInvalid, len(current.Labels), maxLabels)
	}

	if len(current.Tiles) > maxTiles {
		return Canvas{}, fmt.Errorf("%w: the canvas would hold %d printers, at most %d", errInvalid, len(current.Tiles), maxTiles)
	}

	current.Revision++
	current.SavedBy = by
	current.SavedAt = &now

	raw, err := json.Marshal(current)
	if err != nil {
		return Canvas{}, fmt.Errorf("encode the canvas: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, "canvas-*.json")
	if err != nil {
		return Canvas{}, fmt.Errorf("write the canvas: %w", err)
	}

	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename; only a failed write leaves it

	if _, err = tmp.Write(raw); err == nil {
		err = tmp.Sync()
	}

	if err != nil {
		tmp.Close()

		return Canvas{}, fmt.Errorf("write the canvas: %w", err)
	}

	if err = tmp.Close(); err != nil {
		return Canvas{}, fmt.Errorf("write the canvas: %w", err)
	}

	if err = os.Rename(tmp.Name(), s.path()); err != nil {
		return Canvas{}, fmt.Errorf("write the canvas: %w", err)
	}

	if d, err := os.Open(s.dir); err == nil {
		_ = d.Sync()
		d.Close()
	}

	return current, nil
}

func canvasRoutes(mux *http.ServeMux, store func() *canvasStore) {
	mux.HandleFunc("GET /canvas", func(w http.ResponseWriter, r *http.Request) {
		held := store()
		if held == nil {
			refuse(w, r, NoDataDirectory.New("the operator granted printer-canvas no data directory"))

			return
		}

		canvas, err := held.load()
		if err != nil {
			refuse(w, r, LayoutUnreadable.New(err.Error()))

			return
		}

		answer(w, http.StatusOK, canvas)
	})

	mux.HandleFunc("PATCH /canvas", func(w http.ResponseWriter, r *http.Request) {
		held := store()
		if held == nil {
			refuse(w, r, NoDataDirectory.New("the operator granted printer-canvas no data directory"))

			return
		}

		var patch Patch

		body, err := io.ReadAll(r.Body)
		if err == nil {
			err = json.Unmarshal(body, &patch)
		}

		if err == nil {
			err = patch.validate()
		}

		if err != nil {
			refuse(w, r, LayoutInvalid.New(err.Error()))

			return
		}

		canvas, err := held.apply(patch, routesapi.PrincipalOf(r), time.Now().UTC())
		if errors.Is(err, errInvalid) {
			refuse(w, r, LayoutInvalid.New(err.Error()))

			return
		}

		if err != nil {
			refuse(w, r, LayoutUnreadable.New(err.Error()))

			return
		}

		answer(w, http.StatusOK, canvas)
	})
}
