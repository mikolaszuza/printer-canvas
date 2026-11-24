<!--
  One printer on the canvas. Two layouts:
  - printing: the name, a small picture of the printer, a thumbnail of the
    model being printed, the percentage, the time left and the file, and the
    progress bar. The bar says "printing", so the state shows only when it is
    worth a word, such as PAUSED: on a tag over the top-left corner.
    A finished print keeps this layout at 100 %, with how long it took in
    place of the time left, until the printer starts something else.
  - anything else: the name, the state badge, and a large picture of the
    printer filling the rest of the card.
  A state that wants someone at the printer (attention, fault) also draws a
  border in its colour around the whole card.
-->
<script lang="ts">
  import {
    type Device,
    PRINTER_STATE_LABEL_KEY,
    PRINTER_STATE_TONE,
  } from '@prusa/connect-ui/domain';
  import { Badge, PrinterIcon, Progress } from '@prusa/connect-ui/primitives';

  import { minutesTakenOf } from './durations';
  import { heldHost } from './host';
  import {
    ALERT_BORDER,
    artworkOf,
    BADGE,
    EDGE,
    finishedOf,
    minutesLeftOf,
    progressOf,
    stateOf,
    TAG,
    thumbnailOf,
  } from './printer';
  import { pictureOf } from './pictures';

  let { device, lifted = false }: { device: Device; lifted?: boolean } =
    $props();

  const host = heldHost();
  const t = host.t;

  const printerState = $derived(stateOf(device));
  const deviceId = $derived(device.id);
  const tone = $derived(PRINTER_STATE_TONE[printerState]);
  const finished = $derived(finishedOf(device));
  const progress = $derived(progressOf(device) ?? (finished ? 100 : undefined));
  const printing = $derived(progress !== undefined);
  // A print that has stopped to wait (PAUSED, ATTENTION…) names its state on a
  // tag over the card's corner, so the lines inside keep the time left.
  const tagged = $derived(printing && printerState !== 'PRINTING');
  const alert = $derived(ALERT_BORDER[tone]);
  const frame = $derived.by(() => {
    if (lifted) return 'border border-primary shadow-xl';
    if (alert) return ['border-2 shadow-md', alert];
    return 'border border-border shadow-sm';
  });
  /** "15 min", "2 h", "1 h 12 min". */
  const spell = (minutes: number): string => {
    if (minutes < 60) return t('printer-canvas.duration.minutes', { minutes });
    if (minutes % 60 === 0)
      return t('printer-canvas.duration.hoursOnly', { hours: minutes / 60 });

    return t('printer-canvas.duration.hours', {
      hours: Math.floor(minutes / 60),
      minutes: minutes % 60,
    });
  };
  const minutes = $derived(minutesLeftOf(device));

  // How long a finished print took. Read by plain strings, like the picture
  // below, and asked again while the job history has no end for it yet.
  const finishedJobId = $derived(finished ? device.job?.id : undefined);
  let taken = $state<number | undefined>(undefined);
  $effect(() => {
    const jobId = finishedJobId;
    const id = deviceId;
    taken = undefined;
    if (jobId === undefined) return;
    let live = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const ask = (): void => {
      minutesTakenOf(host.client, id, jobId)
        .then((found) => {
          if (!live) return;
          if (found === undefined) timer = setTimeout(ask, 10_000);
          else taken = found;
        })
        .catch(() => {
          if (live) timer = setTimeout(ask, 30_000);
        });
    };
    ask();

    return () => {
      live = false;
      clearTimeout(timer);
    };
  });

  /** Under the percentage: the time left, or how long a finished print took. */
  const timing = $derived.by(() => {
    if (finished)
      return taken === undefined
        ? undefined
        : t('printer-canvas.card.took', { duration: spell(taken) });

    return minutes === undefined
      ? undefined
      : t('printer-canvas.card.left', { duration: spell(minutes) });
  });

  /** A text, or undefined when it is missing or blank. */
  const filled = (text: string | undefined): string | undefined =>
    text?.trim() ? text : undefined;
  // A printer re-sends its job's name only when a print starts, so after a
  // server restart a running job can have an empty name: the model reads instead.
  const line = $derived(
    filled(device.issues?.[0]?.detail) ??
      (printing ? filled(device.job?.display_name) : undefined) ??
      device.model,
  );

  // The picture the printer reads out of the file it is printing. A file with
  // no picture (or a person who may not ask the printer for it) answers an
  // error, and the printer's own picture stands in.
  // The device is a new object with every report, about once a second, so the
  // effect below reads only plain strings: it runs again when the print
  // changes, not on every report.
  const thumbnail = $derived(thumbnailOf(device));
  const thumbKey = $derived(thumbnail?.key);
  const thumbPath = $derived(thumbnail?.path);
  // The card waits for the shared download; until it lands, or when the file
  // has no picture, the printer's own picture stands in.
  let picture = $state<string | undefined>(undefined);
  $effect(() => {
    const key = thumbKey;
    const path = thumbPath;
    picture = undefined;
    if (key === undefined || path === undefined) return;
    let live = true;
    void pictureOf(host.client, { key, deviceId, path }).then((local) => {
      if (live) picture = local;
    });

    return () => {
      live = false;
    };
  });
</script>

<!-- The tag sits across the card's top edge, so it lives outside the card's
     clipping box. -->
<div class="relative h-full w-full">
  <div
    class={[
      'rounded-lg relative flex h-full w-full flex-col gap-1 overflow-hidden bg-card p-3 ps-4 text-card-foreground transition-shadow',
      frame,
      tone === 'quiet' && 'text-muted-foreground',
    ]}
  >
    <span
      class={['absolute inset-y-0 start-0 w-1.5', EDGE[tone]]}
      aria-hidden="true"
    ></span>
    <!-- The name gets up to two lines, so "CORE One L - 1157 - Orange handle"
       reads in full; anything longer ends in … with the rest on hover. While
       it prints, a small picture of the printer sits in the top-right corner. -->
    <div class="flex min-w-0 items-start gap-2">
      <span
        class="line-clamp-2 min-w-0 flex-1 text-sm leading-tight font-bold break-words"
        title={device.name}
      >
        {device.name}
      </span>
      {#if progress !== undefined}
        <PrinterIcon
          model={artworkOf(device)}
          reported={device.model}
          class="-my-1 -me-1 size-10 shrink-0"
        />
      {/if}
    </div>

    {#if progress !== undefined}
      <!-- The model being printed, then the percentage with the time left under
         it and the file. Connect's thumbnail is the PNG PrusaSlicer embeds,
         380x285 (4:3) in the stock profiles, so the box is 4:3 and the picture
         is fitted, never cropped, in case another slicer embeds another shape. -->
      <div class="flex min-h-0 min-w-0 flex-1 items-center gap-2.5">
        <span
          class="grid h-12 w-16 shrink-0 place-items-center overflow-hidden rounded-md bg-muted"
        >
          {#if picture}
            <img
              src={picture}
              alt={line}
              draggable="false"
              class="size-full object-contain"
            />
          {:else}
            <PrinterIcon
              model={artworkOf(device)}
              reported={device.model}
              class="size-10"
            />
          {/if}
        </span>
        <!-- Three tight lines, so they fit under a two-line name too: the
           percentage, then the time left (or PAUSED, ATTENTION…), then the file. -->
        <span class="flex min-w-0 flex-1 flex-col gap-0.5 text-xs leading-4">
          <span class="text-xl leading-none font-bold tabular-nums">
            {t('printer-canvas.card.progress', { progress })}
          </span>
          {#if timing}
            <span class="truncate text-muted-foreground tabular-nums">
              {timing}
            </span>
          {/if}
          <span class="truncate text-muted-foreground" title={line}>{line}</span
          >
        </span>
      </div>
      <Progress value={progress} max={100} class="h-2 w-full shrink-0" />
    {:else}
      <div class="relative min-h-0 flex-1">
        <PrinterIcon
          model={artworkOf(device)}
          reported={device.model}
          class={[
            'absolute inset-0 m-auto size-full',
            tone === 'quiet' && 'opacity-50',
          ]}
        />
        <Badge
          size="sm"
          variant={BADGE[tone]}
          class="absolute start-0 bottom-0 shadow-sm"
        >
          {t(PRINTER_STATE_LABEL_KEY[printerState])}
        </Badge>
      </div>
    {/if}
  </div>
  {#if tagged}
    <span
      class={[
        'absolute start-3 -top-2.5 z-10 rounded-md border-2 px-1.5 text-xs leading-4 font-bold tracking-wider uppercase shadow-sm',
        TAG[tone],
      ]}
    >
      {t(PRINTER_STATE_LABEL_KEY[printerState])}
    </span>
  {/if}
</div>
