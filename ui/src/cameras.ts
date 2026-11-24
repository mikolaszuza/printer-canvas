// The cameras watching each printer, as Connect lists them. A printer with
// several shows the first one that can be watched live.
import type { heldHost } from './host';

type Client = ReturnType<typeof heldHost>['client'];

/** One camera as `list-cameras` answers it. */
export type Camera = NonNullable<
  Awaited<ReturnType<Client['listCameras']>>['items']
>[number];

/** How a camera is shown: live over WebRTC, or as stills refreshed in turn. */
export type CameraKind = 'live' | 'stills';

export interface PrinterCamera {
  name: string;
  kind: CameraKind;
}

const kindOf = (camera: Camera): CameraKind | undefined => {
  if (camera.watch) return 'live';
  if (camera.snapshot) return 'stills';
  return undefined;
};

/** The camera to show for each printer, by device id. */
export const readPrinterCameras = async (
  client: Pick<Client, 'listCameras'>,
): Promise<Map<string, PrinterCamera>> => {
  const page = await client.listCameras({});
  const found = new Map<string, PrinterCamera>();
  for (const camera of page.items ?? []) {
    const kind = kindOf(camera);
    if (kind === undefined || !camera.device_id) continue;
    const known = found.get(camera.device_id);
    // A live camera wins over one that only takes stills.
    if (known === undefined || (known.kind === 'stills' && kind === 'live'))
      found.set(camera.device_id, { name: camera.name, kind });
  }

  return found;
};
