# tasks-plugin

Kanban for humans, MCP for agents. One board, the same tasks.

A fully interactive kanban surface with per-project columns and task types,
comments tagged by author (human or agent), and eighteen MCP tools so every
compatible AI client can create, query, update, and close work alongside
you. Emits and subscribes to EventBus events so other Plekt plugins can
react in real time.

## Overview

- **Version:** 1.0.0
- **License:** MIT`` ``
- **Optional dependency:** `projects-plugin`
- **Frontend:** `plugin.js`, `plugin.css` (page-scoped)

## Schema

Four tables:

- **`tasks`** — `id`, `title`, `description`, `status`, `priority`,
  `due_date`, `project_id`, `type_id`, `assignee_id`, `created_at`,
  `updated_at`. Indexes on status, priority, due_date, project_id, type_id,
  assignee_id.
- **`board_columns`** — custom kanban statuses, scoped per project. Columns:
  `id`, `name`, `label`, `position`, `color`, `project_id`, `is_default`.
  Unique `(name, project_id)`.
- **`task_types`** — custom task types, scoped per project. Columns: `id`,
  `name`, `label`, `color`, `icon`, `project_id`. Unique `(name, project_id)`.
- **`comments`** — `id`, `task_id`, `author_type` (`user`\|`agent`),
  `author_id`, `author_name`, `body`, `created_at`.

Default seed on first board access: columns `pending / in_progress / done`,
task types `task / feature / bug_fix`.

## MCP Tools (18)

### Task CRUD

| Tool | Purpose |
|---|---|
| `list_tasks` | Filters: `status`, `project_id`, `type_id`, `assignee_id`, `limit` |
| `create_task` | Requires `title`. Defaults: `priority=3`, `status=pending` |
| `get_task` | Single task by ID |
| `get_task_detail` | Task + comments + type info + board columns |
| `update_task` | Partial update: title, description, status, priority, due_date, type_id, assignee_id |
| `delete_task` | Cascade-deletes comments |

### Board Columns

| Tool | Purpose |
|---|---|
| `list_board_columns` | Ordered by `position`, optionally scoped by `project_id` |
| `create_board_column` | `name`, `label`, `position`, `color` |
| `update_board_column` | Update label / position / color |
| `delete_board_column` | Cannot delete defaults or columns with tasks |

### Task Types

| Tool | Purpose |
|---|---|
| `list_task_types` | All task types |
| `create_task_type` | `name`, `label`, `color`, `icon` |
| `update_task_type` | Update label / color / icon |
| `delete_task_type` | Clears `type_id` on affected tasks |

### Comments

| Tool | Purpose |
|---|---|
| `list_comments` | Comments for a task, ordered by `created_at` |
| `create_comment` | Adds a comment. Agents MUST pass `author_name` + `author_type='agent'` (no auto-injection here, unlike notes-plugin) |
| `update_comment` | Update body |
| `delete_comment` | Delete by ID |

## Events

**Emits:** `task.created`, `task.updated`, `task.deleted`, `task.completed`
(on status transition to `done`), `comment.created`, `comment.deleted`.

**Subscribes:** `project.deleted` — clears `project_id` on affected tasks.

## Extension points

Tasks-plugin exposes two extension points that pomodoro-plugin uses:

- `task-card-badge` — timer status badge on task cards
- `task-card-actions` — Start/Stop Timer action buttons on task cards

## Build

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
```

Requires the Extism Go PDK (`github.com/extism/go-pdk`).

## See also

- `docs/plugins/tasks-plugin.md` — dashboard widget, domain types, UI pages
