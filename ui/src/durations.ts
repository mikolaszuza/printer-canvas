// How long a finished print took. The printer's own job carries no times, so
// they come from the printer's job history, which Connect writes as the
// printer reports. That record can trail the printer by a few seconds, so an
// answer without an end is not kept and the card asks again.
import type { heldHost } from './host';

type Client = Pick<ReturnType<typeof heldHost>['client'], 'getDeviceJobs'>;

const known = new Map<string, number>();

/** The minutes a finished job took, or undefined while its record has no end yet. */
export const minutesTakenOf = async (
  client: Client,
  deviceId: string,
  jobId: string,
): Promise<number | undefined> => {
  const held = known.get(jobId);
  if (held !== undefined) return held;

  const page = await client.getDeviceJobs({ path: { device_id: deviceId } });
  const record = [page.current, page.last].find((job) => job?.id === jobId);
  const started = Date.parse(record?.started ?? '');
  const ended = Date.parse(record?.ended ?? '');
  if (!Number.isFinite(started) || !Number.isFinite(ended)) return undefined;

  const minutes = Math.max(1, Math.round((ended - started) / 60_000));
  known.set(jobId, minutes);

  return minutes;
};
