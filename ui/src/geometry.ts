// Pure canvas maths: no component, no host.
import type { Point } from './canvas-api';

/** One printer card, in canvas units (pixels at 100 %). */
export const TILE_W = 220;
export const TILE_H = 132;
/** Cards snap to this grid when dropped, and the dots are drawn on it. */
export const GRID = 20;
export const GAP = 20;

export const MIN_ZOOM = 0.2;
export const MAX_ZOOM = 2.5;

export interface View {
  /** Where canvas (0, 0) is on the screen, relative to the stage. */
  x: number;
  y: number;
  zoom: number;
}

export const snap = (v: number): number => Math.round(v / GRID) * GRID;

export const clampZoom = (zoom: number): number =>
  Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));

/** Zooms to `zoom` while keeping the canvas point under (sx, sy) where it is. */
export const zoomAt = (
  view: View,
  zoom: number,
  sx: number,
  sy: number,
): View => {
  const next = clampZoom(zoom);
  const cx = (sx - view.x) / view.zoom;
  const cy = (sy - view.y) / view.zoom;

  return { x: sx - cx * next, y: sy - cy * next, zoom: next };
};

/** The view that shows every point, centred in a stage of the given size. */
export const fitView = (
  points: readonly Point[],
  width: number,
  height: number,
): View => {
  if (points.length === 0) return { x: 40, y: 40, zoom: 1 };
  const minX = Math.min(...points.map((p) => p.x));
  const minY = Math.min(...points.map((p) => p.y));
  const maxX = Math.max(...points.map((p) => p.x)) + TILE_W;
  const maxY = Math.max(...points.map((p) => p.y)) + TILE_H;
  const pad = 48;
  const zoom = clampZoom(
    Math.min(
      1,
      (width - pad * 2) / (maxX - minX),
      (height - pad * 2) / (maxY - minY),
    ),
  );

  return {
    x: (width - (maxX - minX) * zoom) / 2 - minX * zoom,
    y: (height - (maxY - minY) * zoom) / 2 - minY * zoom,
    zoom,
  };
};

const overlaps = (a: Point, b: Point): boolean =>
  a.x < b.x + TILE_W + GAP / 2 &&
  b.x < a.x + TILE_W + GAP / 2 &&
  a.y < b.y + TILE_H + GAP / 2 &&
  b.y < a.y + TILE_H + GAP / 2;

/**
 * Spots for printers nobody has placed yet: a grid of rows under everything
 * already on the canvas, six cards wide, never on top of a placed card.
 */
export const placeNew = (
  ids: readonly string[],
  placed: readonly Point[],
): Record<string, Point> => {
  const out: Record<string, Point> = {};
  if (ids.length === 0) return out;
  const startX = placed.length ? snap(Math.min(...placed.map((p) => p.x))) : 0;
  const startY = placed.length
    ? snap(Math.max(...placed.map((p) => p.y)) + TILE_H + GAP * 2)
    : 0;
  const taken = [...placed];
  const columns = 6;
  let i = 0;
  for (const id of ids) {
    let spot: Point;
    do {
      spot = {
        x: startX + (i % columns) * (TILE_W + GAP),
        y: startY + Math.floor(i / columns) * (TILE_H + GAP),
      };
      i += 1;
    } while (taken.some((p) => overlaps(p, spot)));
    taken.push(spot);
    out[id] = spot;
  }

  return out;
};
