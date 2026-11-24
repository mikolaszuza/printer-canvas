<!-- One text label on the canvas. Drag it like a printer; the pencil (or a
     double-click) renames it; × removes it. Both buttons show on hover. -->
<script lang="ts">
  import PencilIcon from '@lucide/svelte/icons/pencil';
  import XIcon from '@lucide/svelte/icons/x';
  import { Button, Input } from '@prusa/connect-ui/primitives';

  import { heldHost } from './host';

  let {
    text,
    editing,
    lifted = false,
    locked = false,
    oncommit,
    oncancel,
    onedit,
    onremove,
  }: {
    text: string;
    editing: boolean;
    lifted?: boolean;
    /** The layout is locked: no hover frame and no buttons. */
    locked?: boolean;
    /** The new text, trimmed; empty means the person cleared it. */
    oncommit: (text: string) => void;
    oncancel: () => void;
    onedit: () => void;
    onremove: () => void;
  } = $props();

  const t = heldHost().t;

  // The field starts with the label's text and is the person's from then on:
  // what they typed is read off it when they commit. Keeping a copy in state
  // lost every keystroke, because writing that copy re-ran the attachment
  // below, which set it back to the old text.
  let field: HTMLInputElement | undefined;

  /** Focuses the field and selects its text the moment it appears. */
  const focusAll = (el: HTMLInputElement): (() => void) => {
    field = el;
    queueMicrotask(() => {
      el.focus();
      el.select();
    });

    return () => {
      if (field === el) field = undefined;
    };
  };

  const commit = (): void => {
    if (!editing) return;
    oncommit((field?.value ?? text).trim());
  };
</script>

<div
  class={[
    'group relative flex items-center gap-1 rounded-md border-2 px-2 py-1 whitespace-nowrap',
    lifted ? 'border-primary bg-card shadow-lg' : 'border-transparent',
    !lifted && !locked && 'hover:border-dashed hover:border-border',
  ]}
>
  {#if editing}
    <Input
      {@attach focusAll}
      value={text}
      maxlength={120}
      aria-label={t('printer-canvas.label.text')}
      class="h-auto w-auto min-w-64 bg-background px-1 py-0 text-4xl font-bold text-foreground"
      onpointerdown={(e: PointerEvent) => {
        e.stopPropagation();
      }}
      onkeydown={(e: KeyboardEvent) => {
        e.stopPropagation();
        if (e.key === 'Enter') commit();
        if (e.key === 'Escape') oncancel();
      }}
      onblur={commit}
    />
  {:else}
    <span class="text-4xl font-bold tracking-tight text-foreground">
      {text}
    </span>
    {#if !locked}
      <Button
        variant="ghost"
        size="icon-sm"
        label={t('printer-canvas.label.edit')}
        title={t('printer-canvas.label.edit')}
        class="invisible rounded-full text-muted-foreground group-focus-within:visible group-hover:visible"
        onpointerdown={(e: PointerEvent) => {
          e.stopPropagation();
        }}
        onclick={(e: MouseEvent) => {
          e.stopPropagation();
          onedit();
        }}
      >
        <PencilIcon class="size-4" aria-hidden="true" />
      </Button>
    {/if}
  {/if}
  {#if !locked}
    <Button
      variant="ghost"
      size="icon-sm"
      label={t('printer-canvas.label.remove')}
      title={t('printer-canvas.label.remove')}
      class={[
        'rounded-full text-muted-foreground hover:bg-destructive hover:text-destructive-foreground',
        editing ? 'visible' : 'invisible group-hover:visible',
      ]}
      onpointerdown={(e: PointerEvent) => {
        e.stopPropagation();
      }}
      onmousedown={(e: MouseEvent) => {
        // Keeps the field's blur from committing before the removal.
        e.preventDefault();
      }}
      onclick={(e: MouseEvent) => {
        e.stopPropagation();
        onremove();
      }}
    >
      <XIcon class="size-4" aria-hidden="true" />
    </Button>
  {/if}
</div>
