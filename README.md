# Printer Canvas

A Prusa Connect extension that puts every printer on one infinite canvas, so the
screen can look like your print room.

- **Drag a printer** to move it. It snaps to a 20 px grid and is saved for
  everyone at once. There is no edit mode.
- **Drag the background** (or scroll) to pan, **Ctrl + scroll** or pinch to zoom,
  **Fit all** to see every printer.
- Each card shows the printer's **name**, its **state** and, while it prints, a
  **progress bar** with the percentage and the time left.
- New printers get a spot under the others automatically. Click a card to open
  the printer.

Moves are merged per printer on the server, so two people moving different
printers never overwrite each other. Your own pan and zoom are remembered per
person.

## Build

Needs Go 1.26.11+, Node 22.12+ and pnpm. Until the Prusa Connect SDK and the
`@prusa` packages are published, both halves build against a checkout of Prusa
Connect in `.core/` (see `CORE_REF` in the Makefile).

```bash
make check     # Go vet and tests, the UI's lint, types and build
make release   # release/: module.yaml, ui/, one binary per platform
```

Install a release into a running server (as the service user on Linux):

```bash
prusa-connect module add github.com/mikolaszuza/printer-canvas@0.1.4 -from release -approve -config /etc/prusa-connect/config.yaml
```

## Releasing

Releases go to the [Prusa Plugin Marketplace](https://plugin-marketplace.prusa3d.com) through
[prusa-connect-marketplace-publish](https://github.com/prusa3d/prusa-connect-marketplace-publish):
raise `version` in `module.yaml`, add the same version as the top heading of `CHANGELOG.md`, and
publish a GitHub release tagged `v<version>`. The `release` workflow builds every platform, signs the
release with the key in the `release` environment and uploads it; a person at Prusa reviews it before
it can be installed. The extension id, `com.github.aux.mikolaszuza.printer-canvas`, comes from the
module path in `go.mod` and never changes.

