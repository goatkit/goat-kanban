# Changelog

All notable changes to the GoatKit Kanban plugin are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.2] - 2026-11-01

Requires GoatFlow 0.10.0 or later.

### Changed
- **The plugin binary is signed too.** `make sign` signs the `kanban` binary and puts its
  `.sig` in the package, then signs the package. GoatFlow requires the binary signature to load
  a plugin; the package signature alone is no longer enough.

## [0.1.1] - 2026-10-30

Requires GoatFlow 0.10.0 or later.

### Added
- **Release packages carry SLSA build provenance.** The release workflow attests `goat-kanban.zip`
  with GitHub's `attest-build-provenance` (SLSA v1.0 Build Level 2), alongside the existing ed25519
  signature. Check it with `gh attestation verify goat-kanban.zip --repo goatkit/goat-kanban`.

## [0.1.0] - 2026-10-03

First public release. Requires GoatFlow 0.10.0 or later.

### Added
- **Drag-and-drop Kanban boards over ticket states.** Board columns are real ticket states; moving a
  card changes the ticket's state through the platform (`ChangeTicketStatus`), so pending due-time
  rules still apply.
- **Queue-scoped boards.** A board can cover one queue or all queues; every board only shows tickets
  in queues the agent may access. Admins see everything.
- **Per-board column config.** Choose and order visible states; hidden states stay re-enableable.
- **Per-board ticket view override**, a palette to add unboarded tickets, and an agent nav menu entry.
- **English and German translations.**
