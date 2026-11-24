<!--
  The canvas. Everything is direct manipulation, with no edit mode:
  - drag a printer card or a label to move it; it snaps to the grid when dropped and is saved at once
  - click a card (without dragging) to open that printer
  - "Add label" puts a text label in the middle of the view; the pencil (or a double-click) renames a label, × removes it
  - the lock in the canvas's top-right corner locks the layout: nothing moves or changes, a drag anywhere pans,
    and a click on a card still opens the printer. The lock is this person's, in this browser.
  - drag the empty background to pan, or scroll with two fingers / the wheel
  - Ctrl + wheel or a trackpad pinch zooms around the pointer
  - resting the mouse on a card shows the printer's camera, live, and the print's picture, large, beside it
  - the controls float in the canvas's top-left corner, the lock in its top-right
-->
<script lang="ts">
  import LockIcon from '@lucide/svelte/icons/lock';
  import LockOpenIcon from '@lucide/svelte/icons/lock-open';
  import MaximizeIcon from '@lucide/svelte/icons/maximize';
  import MinusIcon from '@lucide/svelte/icons/minus';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import TypeIcon from '@lucide/svelte/icons/type';
  import {
    Button,
    EmptyState,
    PageHeader,
    toast,
  } from '@prusa/connect-ui/primitives';

  import {
    canvasApi,
    type Canvas,
    type CanvasPatch,
    type Label,
    newLabelId,
    pendingSet,
    type Point,
  } from './canvas-api';
  import CanvasLabel from './canvas-label.svelte';
  import {
    fitView,
    GRID,
    placeNew,
    snap,
    TILE_H,
    TILE_W,
    type View,
    zoomAt,
  } from './geometry';
  import { heldHost } from './host';
  import PrinterCard from './printer-card.svelte';
  import CameraView from './camera-view.svelte';
  import { type PrinterCamera, readPrinterCameras } from './cameras';
  import { pictureOf } from './pictures';
  import { thumbnailOf } from './printer';

  const host = heldHost();
  const t = host.t;
  const api = canvasApi(host);
  const fleet = host.fleet;

  const REFRESH_MS = 8000;
  /** Pointer travel, in screen pixels, before a press becomes a drag. */
  const DRAG_THRESHOLD = 4;

  // --- What the server holds, and what this tab is changing ----------------
  let tiles = $state<Record<string, Point>>({});
  let labels = $state<Record<string, Label>>({});
  let loaded = $state(false);
  /** `tile:<id>` and `label:<id>` with a change on its way to the server: a refresh leaves them alone. */
  const saving = pendingSet();
  /** The label whose text is being typed, and whether it was just made (Escape then drops it). */
  let editing = $state<string | undefined>(undefined);
  let editingIsNew = false;

  // --- The view: pan and zoom, remembered per person -----------------------
  // Kept in this browser: the host's per-person store answers 501 in
  // 0.1.0-beta.1, and pan and zoom are a convenience, not shared state.
  const viewKey = `printer-canvas:${host.principal.id}:view`;
  const readView = (): View | undefined => {
    try {
      const v = JSON.parse(
        globalThis.localStorage.getItem(viewKey) ?? 'null',
      ) as View | null;
      return v && [v.x, v.y, v.zoom].every(Number.isFinite) ? v : undefined;
    } catch {
      return undefined;
    }
  };
  let view = $state<View>(readView() ?? { x: 40, y: 40, zoom: 1 });
  let fitted = false;
  let viewTimer: ReturnType<typeof setTimeout> | undefined;
  const keepView = (): void => {
    clearTimeout(viewTimer);
    viewTimer = setTimeout(() => {
      try {
        globalThis.localStorage.setItem(viewKey, JSON.stringify(view));
      } catch {
        // Site data blocked: the view is simply not remembered.
      }
    }, 400);
  };

  // --- The lock: a locked layout cannot be changed by a stray drag ---------
  const lockKey = `printer-canvas:${host.principal.id}:locked`;
  const readLock = (): boolean => {
    try {
      return globalThis.localStorage.getItem(lockKey) === '1';
    } catch {
      return false;
    }
  };
  let locked = $state(readLock());
  const toggleLock = (): void => {
    locked = !locked;
    try {
      if (locked) globalThis.localStorage.setItem(lockKey, '1');
      else globalThis.localStorage.removeItem(lockKey);
    } catch {
      // Site data blocked: the lock lasts until the page is left.
    }
  };

  // --- The stage's size ----------------------------------------------------
  let stage = $state<HTMLElement>();
  let width = $state(800);
  let height = $state(480);
  $effect(() => {
    if (!stage) return;
    const el = stage;
    const measure = (): void => {
      width = el.clientWidth;
      height = Math.max(
        360,
        window.innerHeight - el.getBoundingClientRect().top - 24,
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    window.addEventListener('resize', measure);

    return () => {
      observer.disconnect();
      window.removeEventListener('resize', measure);
    };
  });

  // --- Reading the canvas, now and every few seconds -----------------------
  /** Takes the server's canvas, keeping whatever this tab is in the middle of changing. */
  const take = (got: Canvas): void => {
    const nextTiles: Record<string, Point> = { ...got.tiles };
    const nextLabels: Record<string, Label> = { ...got.labels };
    const keepTile = (id: string): void => {
      const mine = tiles[id];
      if (mine) nextTiles[id] = mine;
    };
    const keepLabel = (id: string): void => {
      const mine = labels[id];
      if (mine) nextLabels[id] = mine;
    };
    for (const key of saving) {
      const [kind, id = ''] = key.split(/:(.*)/s);
      if (kind === 'tile') keepTile(id);
      if (kind === 'label') keepLabel(id);
    }
    if (drag?.moved && drag.kind === 'tile') keepTile(drag.id);
    if (drag?.moved && drag.kind === 'label') keepLabel(drag.id);
    if (editing) keepLabel(editing);
    tiles = nextTiles;
    labels = nextLabels;
  };

  const refresh = async (): Promise<void> => {
    if (document.hidden && loaded) return;
    take(await api.read());
    loaded = true;
  };

  $effect(() => {
    refresh().catch(() => {
      toast.error(t('printer-canvas.error.load'));
    });
    const timer = setInterval(() => {
      refresh().catch(() => undefined);
    }, REFRESH_MS);

    return () => {
      clearInterval(timer);
    };
  });

  // --- Saving changes ------------------------------------------------------
  const save = async (change: CanvasPatch): Promise<void> => {
    const keys = [
      ...Object.keys(change.tiles ?? {}).map((id) => `tile:${id}`),
      ...Object.keys(change.labels ?? {}).map((id) => `label:${id}`),
    ];
    for (const key of keys) saving.add(key);
    try {
      const got = await api.patch(change);
      for (const key of keys) saving.delete(key);
      take(got);
    } catch (e) {
      for (const key of keys) saving.delete(key);
      const denied =
        typeof e === 'object' &&
        e !== null &&
        'status' in e &&
        e.status === 403;
      toast.error(
        t(denied ? 'printer-canvas.error.denied' : 'printer-canvas.error.move'),
      );
      await refresh().catch(() => undefined);
    }
  };

  // --- Printers nobody placed yet get a spot under the others --------------
  const devices = $derived(fleet.devices);
  $effect(() => {
    if (!loaded || !fleet.loaded) return;
    const fresh = devices
      .filter((d) => !tiles[d.id] && !saving.has(`tile:${d.id}`))
      .map((d) => d.id);
    if (fresh.length === 0) return;
    const spots = placeNew(fresh, Object.values(tiles));
    tiles = { ...tiles, ...spots };
    void save({ tiles: spots });
  });

  /** The printers on the canvas, in a stable order, each with its spot. */
  const shown = $derived(
    devices.flatMap((d) => {
      const at = tiles[d.id];
      return at ? [{ device: d, at }] : [];
    }),
  );
  const labelList = $derived(
    Object.entries(labels).map(([id, label]) => ({ id, label })),
  );

  // --- Fit everything, the first time this person opens the canvas --------
  const fitAll = (): void => {
    view = fitView(
      [...shown.map((s) => s.at), ...labelList.map((l) => l.label)],
      width,
      height,
    );
    keepView();
  };
  $effect(() => {
    if (fitted || !loaded || !fleet.loaded || shown.length === 0) return;
    fitted = true;
    // The person's store may have arrived after the page was first drawn.
    const remembered = readView();
    if (remembered) view = remembered;
    else fitAll();
  });

  // --- Labels --------------------------------------------------------------
  const addLabel = (): void => {
    const id = newLabelId();
    const at = {
      x: snap((width / 2 - view.x) / view.zoom - 100),
      y: snap((height / 2 - view.y) / view.zoom - 24),
    };
    labels = {
      ...labels,
      [id]: { ...at, text: t('printer-canvas.label.default') },
    };
    editingIsNew = true;
    editing = id;
  };

  const dropLabel = (id: string, wasSaved: boolean): void => {
    labels = Object.fromEntries(
      Object.entries(labels).filter(([key]) => key !== id),
    );
    if (wasSaved) void save({ labels: { [id]: null } });
  };

  const commitLabel = (id: string, text: string): void => {
    const label = labels[id];
    const isNew = editingIsNew;
    editing = undefined;
    editingIsNew = false;
    if (!label) return;
    if (text === '') {
      dropLabel(id, !isNew);
      return;
    }
    if (!isNew && text === label.text) return;
    const next = { ...label, text };
    labels = { ...labels, [id]: next };
    void save({ labels: { [id]: next } });
  };

  const cancelLabel = (id: string): void => {
    const isNew = editingIsNew;
    editing = undefined;
    editingIsNew = false;
    if (isNew) dropLabel(id, false);
  };

  // --- On hover: the printer's camera, live, and the print's picture, large --
  // Drawn in the stage, outside the zoomed layer, so it keeps its size at any
  // zoom, and placed from the card's spot, so it follows a pan or a zoom. The
  // camera connects only while the panel shows, so a camera is read only
  // while somebody looks at it.
  const PEEK_H = 216;
  const CAMERA_W = 384; // 16:9, the Buddy3D's own shape
  const PICTURE_W = 288; // 4:3, the slicer's thumbnail
  const PEEK_GAP = 8;
  const PEEK_PAD = 18;
  /** The pictures, the padding, and two lines of caption. */
  const PEEK_BOX_H = PEEK_H + 64;
  // Long enough that a pointer passing over a card, or resting before a drag, opens nothing.
  const PEEK_DELAY_MS = 450;
  const CAMERAS_MS = 30_000;
  let peekId = $state<string | undefined>(undefined);
  let peekTimer: ReturnType<typeof setTimeout> | undefined;
  /** The shared copy of the hovered print's picture, once it is at hand. */
  let peekPicture = $state<{ key: string; src: string } | undefined>(undefined);

  // The cameras, read now and every half minute. A person who may not list
  // cameras simply sees none.
  let cameras = $state<Map<string, PrinterCamera>>(new Map());
  $effect(() => {
    const read = (): void => {
      readPrinterCameras(host.client)
        .then((found) => {
          cameras = found;
        })
        .catch(() => undefined);
    };
    read();
    const timer = setInterval(read, CAMERAS_MS);

    return () => {
      clearInterval(timer);
    };
  });

  const startPeek = (e: PointerEvent, id: string): void => {
    if (e.pointerType !== 'mouse') return;
    clearTimeout(peekTimer);
    peekTimer = setTimeout(() => {
      peekId = id;
    }, PEEK_DELAY_MS);
  };
  const stopPeek = (): void => {
    clearTimeout(peekTimer);
    peekId = undefined;
  };

  const peekSpot = $derived(
    peekId === undefined
      ? undefined
      : shown.find((s) => s.device.id === peekId),
  );
  const peekThumb = $derived(
    peekSpot ? thumbnailOf(peekSpot.device) : undefined,
  );
  $effect(() => {
    const wanted = peekThumb;
    if (!wanted) return;
    let live = true;
    void pictureOf(host.client, wanted).then((src) => {
      if (live) peekPicture = src ? { key: wanted.key, src } : undefined;
    });

    return () => {
      live = false;
    };
  });

  const MARGIN = 8;
  const clamp = (value: number, low: number, high: number): number =>
    Math.max(low, Math.min(high, value));
  /**
   * Where the panel goes: beside the card, right or left, where it fits;
   * otherwise under or over it. Always inside the canvas.
   */
  const placePeek = (at: Point, boxW: number): { x: number; y: number } => {
    const cardLeft = view.x + at.x * view.zoom;
    const cardTop = view.y + at.y * view.zoom;
    const cardW = TILE_W * view.zoom;
    const cardH = TILE_H * view.zoom;
    const maxX = Math.max(MARGIN, width - boxW - MARGIN);
    const maxY = Math.max(MARGIN, height - PEEK_BOX_H - MARGIN);
    const right = cardLeft + cardW + 12;
    const left = cardLeft - 12 - boxW;
    const besideY = clamp(cardTop + cardH / 2 - PEEK_BOX_H / 2, MARGIN, maxY);
    if (right + boxW <= width - MARGIN) return { x: right, y: besideY };
    if (left >= MARGIN) return { x: left, y: besideY };
    const x = clamp(cardLeft + cardW / 2 - boxW / 2, MARGIN, maxX);
    const below = cardTop + cardH + 12;
    if (below + PEEK_BOX_H <= height - MARGIN) return { x, y: below };
    const above = cardTop - 12 - PEEK_BOX_H;
    if (above >= MARGIN) return { x, y: above };
    return { x, y: besideY };
  };

  const peek = $derived.by(() => {
    const spot = peekSpot;
    if (!spot || drag?.moved || pan?.moved) return undefined;
    const camera = cameras.get(spot.device.id);
    const picture =
      peekThumb && peekPicture?.key === peekThumb.key
        ? peekPicture.src
        : undefined;
    if (!camera && !picture) return undefined;
    const inner =
      (camera ? CAMERA_W : 0) +
      (picture ? PICTURE_W : 0) +
      (camera && picture ? PEEK_GAP : 0);
    const boxW = inner + PEEK_PAD;
    const place = placePeek(spot.at, boxW);
    return {
      device: spot.device,
      camera,
      picture,
      boxW,
      ...place,
    };
  });

  // --- Pointer handling: drag a card or a label, or pan the background -----
  interface Drag {
    kind: 'tile' | 'label';
    id: string;
    pointer: number;
    startX: number;
    startY: number;
    from: Point;
    moved: boolean;
  }
  interface Pan {
    pointer: number;
    startX: number;
    startY: number;
    from: View;
    moved: boolean;
  }
  let drag = $state<Drag | undefined>(undefined);
  let pan = $state<Pan | undefined>(undefined);
  /** Set when a press ended a drag, so the click that follows does not open the printer. */
  let swallowClick = false;

  const startDrag = (
    e: PointerEvent,
    kind: Drag['kind'],
    id: string,
    from: Point,
  ): void => {
    swallowClick = false;
    // A press is the start of a drag or a pan, never a look: the preview
    // closes and opens again only when the pointer next comes onto a card.
    stopPeek();
    if (e.button !== 0) return;
    e.stopPropagation();
    // Locked, the press pans. The card holds the pointer, not the background,
    // so a click without a drag still lands on the card and opens the printer.
    if (locked) {
      startPan(e);
      return;
    }
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    drag = {
      kind,
      id,
      pointer: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      from: { x: from.x, y: from.y },
      moved: false,
    };
  };

  const moveDrag = (e: PointerEvent): void => {
    if (drag?.pointer !== e.pointerId) return;
    const dx = e.clientX - drag.startX;
    const dy = e.clientY - drag.startY;
    if (!drag.moved && Math.hypot(dx, dy) < DRAG_THRESHOLD) return;
    drag.moved = true;
    const at = {
      x: drag.from.x + dx / view.zoom,
      y: drag.from.y + dy / view.zoom,
    };
    if (drag.kind === 'tile') tiles = { ...tiles, [drag.id]: at };
    else {
      const label = labels[drag.id];
      if (label) labels = { ...labels, [drag.id]: { ...label, ...at } };
    }
  };

  const endDrag = (e: PointerEvent): void => {
    if (drag?.pointer !== e.pointerId) return;
    const ended = drag;
    drag = undefined;
    if (!ended.moved) return;
    swallowClick = true;
    if (ended.kind === 'tile') {
      const at = tiles[ended.id] ?? ended.from;
      const spot = { x: snap(at.x), y: snap(at.y) };
      tiles = { ...tiles, [ended.id]: spot };
      if (spot.x !== ended.from.x || spot.y !== ended.from.y)
        void save({ tiles: { [ended.id]: spot } });
      return;
    }
    const label = labels[ended.id];
    if (!label) return;
    const next = { ...label, x: snap(label.x), y: snap(label.y) };
    labels = { ...labels, [ended.id]: next };
    if (editing === ended.id) return;
    if (next.x !== ended.from.x || next.y !== ended.from.y)
      void save({ labels: { [ended.id]: next } });
  };

  const openPrinter = (e: MouseEvent): void => {
    if (swallowClick) {
      e.preventDefault();
      swallowClick = false;
    }
  };

  const startPan = (e: PointerEvent): void => {
    if (e.button !== 0 && e.button !== 1) return;
    stopPeek();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    pan = {
      pointer: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      from: view,
      moved: false,
    };
  };

  const movePan = (e: PointerEvent): void => {
    if (pan?.pointer !== e.pointerId) return;
    const dx = e.clientX - pan.startX;
    const dy = e.clientY - pan.startY;
    if (!pan.moved && Math.hypot(dx, dy) < DRAG_THRESHOLD) return;
    pan.moved = true;
    view = {
      ...pan.from,
      x: pan.from.x + dx,
      y: pan.from.y + dy,
    };
  };

  const endPan = (e: PointerEvent): void => {
    if (pan?.pointer !== e.pointerId) return;
    // A pan that began on a card (the layout is locked) must not open it.
    swallowClick = pan.moved;
    pan = undefined;
    keepView();
  };

  // Wheel: two-finger scroll pans, Ctrl + wheel (and a trackpad pinch) zooms.
  // Added by hand because Svelte's onwheel is passive and cannot stop the page scrolling.
  $effect(() => {
    if (!stage) return;
    const el = stage;
    const wheel = (e: WheelEvent): void => {
      e.preventDefault();
      if (e.ctrlKey || e.metaKey) {
        const box = el.getBoundingClientRect();
        view = zoomAt(
          view,
          view.zoom * Math.exp(-e.deltaY * 0.01),
          e.clientX - box.left,
          e.clientY - box.top,
        );
      } else {
        view = { ...view, x: view.x - e.deltaX, y: view.y - e.deltaY };
      }
      keepView();
    };
    el.addEventListener('wheel', wheel, { passive: false });

    return () => {
      el.removeEventListener('wheel', wheel);
    };
  });

  const zoomBy = (factor: number): void => {
    view = zoomAt(view, view.zoom * factor, width / 2, height / 2);
    keepView();
  };

  const dots = $derived(GRID * 2 * view.zoom);
  const dotSize = $derived(Math.max(1, 1.5 * view.zoom));
</script>

<div class="flex flex-col gap-3">
  <PageHeader title={t('printer-canvas.page.title')}>
    {#snippet actions()}
      <span class="hidden text-xs text-muted-foreground xl:inline">
        {t('printer-canvas.page.hint')}
      </span>
    {/snippet}
  </PageHeader>

  <div
    bind:this={stage}
    role="application"
    aria-label={t('printer-canvas.page.title')}
    class={[
      'rounded-lg relative w-full touch-none overflow-hidden border border-border bg-muted/30 select-none',
      pan ? 'cursor-grabbing' : 'cursor-grab',
    ]}
    style:height="{height}px"
    style:background-image="radial-gradient(circle, var(--border) {dotSize}px,
    transparent 0)"
    style:background-size="{dots}px {dots}px"
    style:background-position="{view.x - dots / 2}px {view.y - dots / 2}px"
    onpointerdown={startPan}
    onpointermove={movePan}
    onpointerup={endPan}
    onpointercancel={endPan}
  >
    <div
      class="absolute top-0 left-0 origin-top-left"
      style:transform="translate({view.x}px, {view.y}px) scale({view.zoom})"
    >
      {#each labelList as { id, label } (id)}
        {@const lifted = drag?.id === id && drag.moved}
        <div
          role="button"
          tabindex="0"
          aria-label={label.text}
          class={[
            'absolute',
            lifted ? 'z-20 cursor-grabbing' : 'z-0',
            !lifted && !locked && 'cursor-grab',
          ]}
          style:left="{label.x}px"
          style:top="{label.y}px"
          style:transition={lifted || pan ? 'none' : 'left 150ms, top 150ms'}
          onpointerdown={(e) => {
            startDrag(e, 'label', id, label);
          }}
          onpointermove={moveDrag}
          onpointerup={endDrag}
          onpointercancel={endDrag}
          ondblclick={(e) => {
            if (locked) return;
            e.stopPropagation();
            editingIsNew = false;
            editing = id;
          }}
          onkeydown={(e) => {
            if (e.key === 'Enter' && editing !== id && !locked) {
              editingIsNew = false;
              editing = id;
            }
          }}
        >
          <CanvasLabel
            text={label.text}
            editing={editing === id}
            {lifted}
            {locked}
            oncommit={(text: string) => {
              commitLabel(id, text);
            }}
            oncancel={() => {
              cancelLabel(id);
            }}
            onedit={() => {
              editingIsNew = false;
              editing = id;
            }}
            onremove={() => {
              const wasSaved = !(editing === id && editingIsNew);
              editing = undefined;
              editingIsNew = false;
              dropLabel(id, wasSaved);
            }}
          />
        </div>
      {/each}

      {#each shown as { device, at } (device.id)}
        {@const lifted = drag?.id === device.id && drag.moved}
        <a
          href="/printers/{encodeURIComponent(device.id)}"
          draggable="false"
          class={[
            'rounded-lg absolute z-10 block no-underline outline-offset-2 focus-visible:outline-2 focus-visible:outline-primary',
            lifted ? 'z-20 cursor-grabbing' : 'cursor-pointer',
          ]}
          style:left="{at.x}px"
          style:top="{at.y}px"
          style:width="{TILE_W}px"
          style:height="{TILE_H}px"
          style:transition={lifted || pan ? 'none' : 'left 150ms, top 150ms'}
          onpointerdown={(e) => {
            startDrag(e, 'tile', device.id, at);
          }}
          onpointermove={moveDrag}
          onpointerup={endDrag}
          onpointercancel={endDrag}
          onclick={openPrinter}
          onpointerenter={(e) => {
            startPeek(e, device.id);
          }}
          onpointerleave={stopPeek}
          ondragstart={(e) => {
            e.preventDefault();
          }}
        >
          <PrinterCard {device} {lifted} />
        </a>
      {/each}
    </div>

    {#if peek}
      <div
        class="rounded-lg pointer-events-none absolute z-40 flex flex-col gap-1.5 border border-border bg-card p-2 shadow-xl"
        style:left="{peek.x}px"
        style:top="{peek.y}px"
        style:width="{peek.boxW}px"
      >
        <div class="flex" style:gap="{PEEK_GAP}px">
          {#if peek.camera}
            {#key peek.camera.name}
              <div style:width="{CAMERA_W}px" style:height="{PEEK_H}px">
                <CameraView camera={peek.camera} />
              </div>
            {/key}
          {/if}
          {#if peek.picture}
            <img
              src={peek.picture}
              alt=""
              aria-hidden="true"
              draggable="false"
              class="rounded-md bg-muted object-contain"
              style:width="{PICTURE_W}px"
              style:height="{PEEK_H}px"
            />
          {/if}
        </div>
        <span class="min-w-0 px-1 text-sm leading-tight">
          <span class="block truncate font-bold">{peek.device.name}</span>
          <span class="block truncate text-xs text-muted-foreground">
            {peek.device.job?.running
              ? peek.device.job.display_name
              : peek.device.model}
          </span>
        </span>
      </div>
    {/if}

    <!-- The controls float in the top-left corner; a press on them never
         reaches the background. -->
    <div
      role="toolbar"
      tabindex="-1"
      class="rounded-lg absolute start-3 top-3 z-30 flex items-center gap-1 border border-border bg-card/90 p-1 shadow-sm backdrop-blur-sm"
      onpointerdown={(e) => {
        e.stopPropagation();
      }}
    >
      <Button
        variant="secondary"
        size="sm"
        disabled={!loaded || locked}
        onclick={addLabel}
      >
        <TypeIcon class="size-3.5" aria-hidden="true" />
        {t('printer-canvas.toolbar.addLabel')}
      </Button>
      <Button
        variant="secondary"
        size="sm"
        aria-label={t('printer-canvas.toolbar.zoomOut')}
        title={t('printer-canvas.toolbar.zoomOut')}
        onclick={() => {
          zoomBy(1 / 1.25);
        }}
      >
        <MinusIcon class="size-3.5" aria-hidden="true" />
      </Button>
      <span class="w-12 text-center text-sm tabular-nums">
        {Math.round(view.zoom * 100)}&nbsp;%
      </span>
      <Button
        variant="secondary"
        size="sm"
        aria-label={t('printer-canvas.toolbar.zoomIn')}
        title={t('printer-canvas.toolbar.zoomIn')}
        onclick={() => {
          zoomBy(1.25);
        }}
      >
        <PlusIcon class="size-3.5" aria-hidden="true" />
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={shown.length === 0 && labelList.length === 0}
        onclick={fitAll}
      >
        <MaximizeIcon class="size-3.5" aria-hidden="true" />
        {t('printer-canvas.toolbar.fit')}
      </Button>
    </div>

    <!-- Above the content, and its press never reaches the background. -->
    <div
      class="rounded-lg absolute end-3 top-3 z-30 border border-border bg-card/90 p-1 shadow-sm backdrop-blur-sm"
    >
      <Button
        variant={locked ? 'default' : 'secondary'}
        size="icon-sm"
        label={t(
          locked
            ? 'printer-canvas.toolbar.unlock'
            : 'printer-canvas.toolbar.lock',
        )}
        title={t(
          locked
            ? 'printer-canvas.toolbar.unlock'
            : 'printer-canvas.toolbar.lock',
        )}
        aria-pressed={locked}
        onpointerdown={(e: PointerEvent) => {
          e.stopPropagation();
        }}
        onclick={toggleLock}
      >
        {#if locked}
          <LockIcon class="size-4" aria-hidden="true" />
        {:else}
          <LockOpenIcon class="size-4" aria-hidden="true" />
        {/if}
      </Button>
    </div>

    {#if fleet.loaded && devices.length === 0 && labelList.length === 0}
      <div class="absolute inset-0 grid place-items-center p-6">
        <EmptyState
          size="sm"
          heading={t('printer-canvas.empty.title')}
          description={t('printer-canvas.empty.body')}
        />
      </div>
    {/if}
  </div>
</div>
