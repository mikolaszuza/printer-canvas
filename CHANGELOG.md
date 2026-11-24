# Changelog

## 0.1.4

- Fixed: text typed into a new or renamed label was lost, and Enter saved the old text.
- The hover preview closes when you press on a card to drag it (or to pan a locked canvas), and opens only after the pointer rests for about half a second.

## 0.1.3

- Rest the pointer on a printer with a camera to see the camera live, next to the print's picture.
- A finished print keeps its card at 100 % with how long it took, until the printer starts something else.

## 0.1.2

- Rest the mouse on a printing card to see the print's picture large beside it. It keeps its size at any zoom.
- Each print's picture is downloaded once and shared by the card and the preview, so hovering never asks the printer again.
- The zoom, "Add label" and "Fit all" controls float in the canvas's top-left corner, the lock in its top-right.

## 0.1.1

- Text labels ("Hallway Shelf"): add, drag, rename with the pencil or a double-click, remove with ×.
- Printing cards show the print's thumbnail, the percentage, the time left and the file, with a small printer picture in the corner. Idle and other cards show a large printer picture.
- A printing printer that is paused or needs attention gets a border in the state's colour and a tag on the card's top-left corner.
- The lock in the canvas's corner locks the layout for you, in this browser: a drag anywhere pans, a click still opens the printer.
- Czech texts rewritten to read naturally, in the formal "vy" Connect uses.

## 0.1.0

- An infinite canvas with every printer as a card: name, state and, while it prints, a progress bar with the time left.
- Drag a card to move it. It snaps to the grid and is saved for everyone at once, with no edit mode.
- Drag the background or scroll to pan. Ctrl + scroll, a pinch or the toolbar zooms. "Fit all" shows every printer.
- Printers that are not on the canvas yet get a spot under the others automatically.
- Click a card to open that printer.
