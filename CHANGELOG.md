# Changelog

## v0.4.0 — New Filesystem Architecture
**Major redesign of filesystem checkpointing and restore pipeline**
- Introduced a fully new filesystem architecture with parent-tracking logic.
- Re-implemented `init`, `create`, and `restore` to match the new model.
- Verified all functionalities through manual tests.
- Added various minor fixes and debugging improvements.

## v0.3.0 — Sandbox Mode & Customizable Path Config
**Filesystem isolation + configuration overhaul**
- Added Sandbox Mode using Linux namespaces and `pivot_root`.
- Optimized checkpoint/restore by excluding the working directory.
- Added customizable path configuration with a 5-level precedence order.

## v0.2.0 — Performance & Usability Improvements
**Incremental improvements before the major FS redesign**
- Added quiet CSV output mode.
- Supported skipping memory checkpoints.
- Improved default cleanup behavior and cleanup levels.
- Updated installation instructions, documentation, and workflow diagram (v0.2.1).
- Added multithreaded reader-writer test.

## v0.1.0 — Foundational Features & Experimental Work
**Initial system architecture, CLI, and experimental features**
- Designed core structures and initial CLI.
- Refactored core checkpoint/restore logic.
- Fixed CRIU TTY and background-process issues.
- Added experimental features:
    - Parallel-Checkpoints (validated)
    - Unsafe-FsRestore (disabled)
- Added a test target program.
- Improved README and added Go installation guide.

## v0.0.1 — Initial Setup
**Project bootstrap**
- Initial commit and technical architecture selection.
- Set up Go environment and drafted core structures.
- Added CLI usage documentation.
