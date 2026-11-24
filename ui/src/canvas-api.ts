// The shared canvas, read from and merged into this module's own process.
import type { Operation } from '@prusa/connect';
import type { ModuleHost } from '@prusa/connect-ui/slots';

export interface Point {
  x: number;
  y: number;
}

/** A piece of text on the canvas, such as "Hallway shelf". */
export interface Label extends Point {
  text: string;
}

/** What the process answers: every placed printer, every label, and how often it was saved. */
export interface Canvas {
  revision: number;
  tiles: Record<string, Point>;
  labels: Record<string, Label>;
  saved_by?: string;
}

/** A change: only what it names moves; a null takes that thing off the canvas. */
export interface CanvasPatch {
  tiles?: Record<string, Point | null>;
  labels?: Record<string, Label | null>;
}

export interface CanvasApi {
  read(): Promise<Canvas>;
  patch(change: CanvasPatch): Promise<Canvas>;
}

/**
 * The mount of this module's routes. A module is not told its namespace, but
 * the address of its own page carries it: `/m/<namespace>/<path>`.
 */
const mountOf = (host: ModuleHost): string => {
  const namespace = host.pageHref('canvas').split('/')[2] ?? '';

  return `/api/v1/modules/${namespace}`;
};

export const canvasApi = (host: ModuleHost): CanvasApi => {
  const mount = mountOf(host);
  const read: Operation = {
    id: 'read-canvas',
    method: 'GET',
    path: `${mount}/canvas`,
    response: 'json',
    anonymous: false,
  };
  const patch: Operation = {
    id: 'move-printers',
    method: 'PATCH',
    path: `${mount}/canvas`,
    body: 'application/json',
    response: 'json',
    anonymous: false,
  };

  return {
    read: async () =>
      await host.client.transport.call<Canvas>(read, {}, host.signal),
    patch: async (change) =>
      await host.client.transport.call<Canvas>(
        patch,
        { body: change },
        host.signal,
      ),
  };
};

/** Ids with a change on its way to the server. Plain on purpose: nothing draws from it. */
export const pendingSet = (): Set<string> => new Set<string>();

/** A fresh label id. */
export const newLabelId = (): string =>
  `l-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
