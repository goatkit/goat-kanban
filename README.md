# GoatKit Kanban Plugin

Drag-and-drop Kanban boards for GoatFlow. Board columns are real ticket states. Moving a card changes the ticket's state through the platform, and every board only shows tickets in queues the agent may access.

Licensed under Apache-2.0. Requires GoatFlow 0.10.0 or later (the plugin declares the permissions 0.10.0 enforces).

## Features

- **Boards over ticket states.** Each column is a ticket state. A new board starts with every valid state as a column. You can hide or reorder columns per board, and hidden states can be turned back on.
- **Real state changes.** Dropping a card calls `HostAPI.ChangeTicketStatus`, so the normal platform rules apply. For example, a pending state asks for a pending time.
- **Palette.** Search for tickets that aren't on the board yet and add them. A per-queue board only offers tickets from its queue. An all-queues board only offers tickets from queues you can access.
- **Queue-scoped access.** The access rules copy the platform's queue access service: direct `group_user` grants plus role grants (`role_user` → `roles` → `group_role`), with `rw` covering every other permission. Members of the `admin` group skip queue scoping.
- **Board ownership.** Only a board's creator or an admin can change or delete it.
- **Ticket view override.** Each board can open cards in a ticket view provided by another enabled plugin.
- **Age hints.** Cards are marked once a ticket is older than 7 days, and again after 30 days.
- **i18n.** English and German, under the `goat-kanban` namespace.

## Quick start

```sh
make build     # build the gRPC plugin binary in Docker (no local Go needed)
make test      # run the tests
make lint      # check SQL works on both MySQL and PostgreSQL (gk-sql-lint)
make package   # bin/goat-kanban.zip (binary + plugin.yaml)
make deploy    # upload to http://localhost:8080 using ../goatflow/.env credentials
make deploy GOATFLOW_URL=https://goatflow.example.com
```

`go.mod` replaces `github.com/goatkit/goatflow` with `../goatflow`, so clone GoatFlow next to this repo. The build mounts both into the container.

`make deploy` reads `ADMIN_API_KEY`, or `ADMIN_USER` + `ADMIN_PASSWORD`, from `../goatflow/.env`. The plugin itself stores no credentials.

## Routes

The UI is an `agent_app` using the standard shell. It is mounted at `/ui/goat-kanban_board/`, and the agent menu has a **Kanban Board** item.

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/` | `render_boards` | Board list with create form |
| GET | `/board/:id` | `render_board` | One board |
| GET | `/api/palette` | `api_palette` | Search accessible tickets not yet on the board |
| POST | `/api/move` | `api_move` | Move a card to another state |
| POST | `/api/board/create` | `api_board_create` | Create a board (all queues, or one accessible queue) |
| POST | `/api/board/delete` | `api_board_delete` | Delete a board (creator or admin) |
| POST | `/api/board/config` | `api_board_config` | Save visible columns, their order and the ticket view (creator or admin) |
| POST | `/api/board/add` | `api_board_add` | Add a ticket to the board |

## Data

Migrations run at start-up and are recorded in `gk_kanban_schema_version`. They work on both MySQL/MariaDB and PostgreSQL.

| Table | Purpose |
|-------|---------|
| `gk_kanban_board` | Boards: `name`, optional `queue_id` (NULL means all queues), `created_by`, `ticket_view` |
| `gk_kanban_column` | Visible columns per board: `state_id`, `sort_order` |
| `gk_kanban_ticket` | Which tickets are on which board |

The plugin only writes to its own `gk_kanban_*` tables. It reads ticket, queue, group and role tables to make access decisions. The only change it makes to tickets is the state change through `ChangeTicketStatus`.

## Layout

```
cmd/kanban-plugin/   entry point
internal/kanban/
  plugin.go          registration, routes, dispatch
  handlers.go        page and API handlers
  access.go          queue access rules
  schema.go          dialect-aware migrations
  ui.go, uijs.go     board markup and script
  i18n.go            translations
```
