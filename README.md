# Ren'Py Universal Save Editor

A universal web-based Ren'Py save editor capable of reading, inspecting, modifying, and writing Ren'Py save files.

## Architecture

*   **Go Backend:** Serves the frontend, manages file uploads/downloads, and orchestrates long-lived worker processes using standard OS process management.
*   **Python Bridge (`bridge.py`):** Runs inside the Ren'Py SDK engine. It deserializes `.save` files with actual Ren'Py classes, translates the object graph to JSON, and correctly serializes modified JSON back into Ren'Py's proprietary pickle format using an internal object cache to preserve unknown types and `__getstate__`/`__setstate__` semantics.
*   **Vanilla JS Frontend:** A lightweight, dependency-free (no Node/NPM) frontend for navigating and editing complex nested object trees.
*   **Build System (`build/main.go`):** Automatically downloads the Ren'Py 8.5.3 SDK, extracts required cross-platform runtime binaries, bundles them with the python bridge, and zips them. The Go compiler then embeds this zip into the final executable.

## How to build
```
./build.sh
```

## How to run
```
./renpy-save-editor -port 8040
```
