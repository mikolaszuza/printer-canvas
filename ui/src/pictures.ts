// The picture inside the file a printer prints, downloaded once per print and
// shared: the card and its hover preview show the same copy. Connect asks the
// printer for the picture on every request that does not name the file's
// size and date, and a running job carries neither, so a second <img> with
// the same address would ask the printer again.

import type { heldHost } from './host';
import type { Thumbnail } from './printer';

type Client = Pick<
  ReturnType<typeof heldHost>['client'],
  'getDeviceFileThumbnail'
>;

/** How many pictures stay in memory; the oldest is let go first. */
const KEEP = 64;

const held = new Map<string, Promise<string | undefined>>();

const download = async (
  client: Client,
  thumbnail: Thumbnail,
): Promise<string | undefined> => {
  const answer = await client.getDeviceFileThumbnail({
    path: { device_id: thumbnail.deviceId },
    query: { path: thumbnail.path },
  });
  if (!answer.ok) return undefined;
  return URL.createObjectURL(await answer.blob());
};

/**
 * A local address for the picture, or undefined for a file with none. The
 * key names the print, so a reprint of a file with the same name is fetched
 * anew.
 */
export const pictureOf = (
  client: Client,
  thumbnail: Thumbnail,
): Promise<string | undefined> => {
  const key = thumbnail.key;
  const known = held.get(key);
  if (known) return known;

  const loading = download(client, thumbnail).catch(() => undefined);
  held.set(key, loading);
  // A failure is not kept, so the next card drawn for this print tries again.
  void loading.then((local) => {
    if (local === undefined) held.delete(key);
  });

  while (held.size > KEEP) {
    const [oldest, picture] = held.entries().next().value ?? [];
    if (oldest === undefined || picture === undefined) break;
    held.delete(oldest);
    void picture.then((local) => {
      if (local !== undefined) URL.revokeObjectURL(local);
    });
  }

  return loading;
};
