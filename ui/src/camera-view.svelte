<!--
  One camera, live. An H.264 camera plays over WebRTC: Connect relays the
  camera's own stream, so it connects when this is drawn and hangs up when it
  goes, and the camera is read only while somebody looks. A camera that only
  takes stills shows its latest picture, fetched again every few seconds.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import type { PrinterCamera } from './cameras';
  import { heldHost } from './host';

  let { camera }: { camera: PrinterCamera } = $props();

  const host = heldHost();
  const t = host.t;

  /** How long the browser may take to find its own network candidates. */
  const GATHER_MS = 2000;
  /** How often a stills camera's picture is fetched again. */
  const STILLS_MS = 3000;

  let video = $state<HTMLVideoElement | undefined>(undefined);
  let playing = $state(false);
  let failed = $state(false);
  let still = $state<string | undefined>(undefined);

  const gathered = (connection: RTCPeerConnection): Promise<void> =>
    new Promise((resolve) => {
      if (connection.iceGatheringState === 'complete') {
        resolve();
        return;
      }
      const timer = setTimeout(resolve, GATHER_MS);
      connection.addEventListener('icegatheringstatechange', () => {
        if (connection.iceGatheringState === 'complete') {
          clearTimeout(timer);
          resolve();
        }
      });
    });

  const watchLive = (name: string): (() => void) => {
    const ended = new AbortController();
    const connection = new RTCPeerConnection();
    connection.addTransceiver('video', { direction: 'recvonly' });
    connection.addEventListener('track', (event) => {
      if (video)
        video.srcObject = event.streams[0] ?? new MediaStream([event.track]);
    });
    connection.addEventListener('connectionstatechange', () => {
      if (connection.connectionState === 'failed' && !ended.signal.aborted)
        failed = true;
    });
    const start = async (): Promise<void> => {
      await connection.setLocalDescription(await connection.createOffer());
      await gathered(connection);
      const answer = await host.client.watchCamera(
        {
          path: { camera: name },
          body: connection.localDescription?.sdp ?? '',
        },
        ended.signal,
      );
      await connection.setRemoteDescription({ type: 'answer', sdp: answer });
    };
    start().catch(() => {
      if (!ended.signal.aborted) failed = true;
    });

    return () => {
      ended.abort();
      connection.close();
    };
  };

  const watchStills = (name: string): (() => void) => {
    const ended = new AbortController();
    let shown: string | undefined;
    const next = async (): Promise<void> => {
      const answer = await host.client.getCameraSnapshot(
        { path: { camera: name } },
        ended.signal,
      );
      if (!answer.ok) throw new Error(String(answer.status));
      const local = URL.createObjectURL(await answer.blob());
      if (shown) URL.revokeObjectURL(shown);
      shown = local;
      still = local;
      playing = true;
    };
    const tick = (): void => {
      next().catch(() => {
        if (!ended.signal.aborted && !playing) failed = true;
      });
    };
    tick();
    const timer = setInterval(tick, STILLS_MS);

    return () => {
      ended.abort();
      clearInterval(timer);
      if (shown) URL.revokeObjectURL(shown);
    };
  };

  // The camera is fixed for the life of this view: the page draws a new one
  // for another camera.
  onMount(() =>
    camera.kind === 'live' ? watchLive(camera.name) : watchStills(camera.name),
  );
</script>

<div class="relative size-full overflow-hidden rounded-md bg-black">
  {#if camera.kind === 'live'}
    <video
      bind:this={video}
      class="size-full object-contain"
      aria-label={t('printer-canvas.camera.label', { camera: camera.name })}
      autoplay
      muted
      playsinline
      onplaying={() => {
        playing = true;
      }}
    ></video>
  {:else if still}
    <img
      src={still}
      alt={t('printer-canvas.camera.label', { camera: camera.name })}
      draggable="false"
      class="size-full object-contain"
    />
  {/if}
  {#if !playing}
    <span
      class="absolute inset-0 grid place-items-center p-4 text-center text-xs text-white/70"
    >
      {t(
        failed
          ? 'printer-canvas.camera.failed'
          : 'printer-canvas.camera.connecting',
      )}
    </span>
  {/if}
</div>
