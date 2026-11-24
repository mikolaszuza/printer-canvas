// How one printer reads on its card. Pure: no component, no host.
import {
  type Device,
  type PrinterState,
  printerStateOf,
  STATE_PATH,
  textAt,
} from '@prusa/connect-ui/domain';
import {
  type BadgeVariant,
  PRINTER_MODELS,
  type PrinterModel,
} from '@prusa/connect-ui/primitives';
import type { PrinterStateTone } from '@prusa/connect-ui/domain';

/** A disconnected machine reads OFFLINE, not the last thing it said. */
export const stateOf = (device: Device): PrinterState =>
  device.connected
    ? printerStateOf(textAt(device, STATE_PATH) ?? 'UNKNOWN')
    : 'OFFLINE';

const folded = (text: string): string =>
  text.toLowerCase().replace(/[^a-z0-9]/g, '');
const ARTWORK = new Map<string, PrinterModel>(
  PRINTER_MODELS.map((model) => [folded(model), model]),
);

/** The drawing for a model spelling, matched the way core folds it. */
export const artworkOf = (device: Device): PrinterModel =>
  ARTWORK.get(folded(device.model)) ?? 'unknown';

/** The running job's progress in whole percent, or undefined when nothing prints. */
export const progressOf = (device: Device): number | undefined => {
  const job = device.job;

  return job?.running === true && typeof job.progress === 'number'
    ? Math.max(0, Math.min(100, Math.round(job.progress)))
    : undefined;
};

/**
 * True while the print on the bed is a finished one: the printer still holds
 * the job, at 100 %, until somebody clears the bed or starts another.
 */
export const finishedOf = (device: Device): boolean =>
  stateOf(device) === 'FINISHED' &&
  device.job?.running !== true &&
  device.job?.state === 'FINISHED' &&
  device.job.path.trim() !== '';

/** Minutes left on the running job, or undefined. */
export const minutesLeftOf = (device: Device): number | undefined => {
  const ms = device.job?.time_remaining_ms;

  return device.job?.running === true && typeof ms === 'number' && ms > 0
    ? Math.round(ms / 60000)
    : undefined;
};

/** The Printers screen's badge per tint, so a state wears the same badge here. */
export const BADGE: Readonly<Record<PrinterStateTone, BadgeVariant>> = {
  active: 'stateActive',
  attention: 'stateAttention',
  done: 'stateDone',
  fault: 'stateFault',
  idle: 'stateIdle',
  quiet: 'stateQuiet',
};

/** The stripe a card carries on its left, in the state's colour. */
export const EDGE: Readonly<Record<PrinterStateTone, string>> = {
  active: 'bg-state-active-edge',
  attention: 'bg-state-attention-edge',
  done: 'bg-state-done-edge',
  fault: 'bg-state-fault-edge',
  idle: 'bg-state-idle-edge',
  quiet: 'bg-state-quiet-edge',
};

/** The border around a whole card whose state wants someone at the printer. */
export const ALERT_BORDER: Readonly<Partial<Record<PrinterStateTone, string>>> =
  {
    attention: 'border-state-attention-edge',
    fault: 'border-state-fault-edge',
  };

/** The tag on a card's top-left corner naming a printing printer's state. */
export const TAG: Readonly<Record<PrinterStateTone, string>> = {
  active: 'border-state-active-edge bg-state-active text-state-active-text',
  attention:
    'border-state-attention-edge bg-state-attention text-state-attention-text',
  done: 'border-state-done-edge bg-state-done text-state-done-text',
  fault: 'border-state-fault-edge bg-state-fault text-state-fault-text',
  idle: 'border-state-idle-edge bg-state-idle text-state-idle-text',
  quiet: 'border-state-quiet-edge bg-state-quiet text-state-quiet-text',
};

/** The picture inside the file a printer is printing, and a key naming this print. */
export interface Thumbnail {
  key: string;
  deviceId: string;
  path: string;
}

/**
 * The picture of what a printer prints, or undefined when nothing prints.
 * Connect serves the one picture the slicer embedded (380x285 in
 * PrusaSlicer's stock profiles).
 */
export const thumbnailOf = (device: Device): Thumbnail | undefined => {
  const path = device.job?.path.trim();
  if (progressOf(device) === undefined && !finishedOf(device)) return undefined;
  if (!path) return undefined;

  return {
    key: `${device.id} ${device.job?.id ?? ''} ${path}`,
    deviceId: device.id,
    path,
  };
};
