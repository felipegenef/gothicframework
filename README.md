# Gothic Framework has moved 🚚

> [!IMPORTANT]
> ## ⚠️ Active development moved to [github.com/gothicframework](https://github.com/gothicframework)
>
> This repository (`felipegenef/gothicframework`) holds the **v1 / v2** history and is **no longer actively developed**. Gothic **v3** was rewritten and split into focused modules under a new organization, and the CLI was renamed **`gothicframework` → `gothic`**.

**Gothic Framework** is a developer-first toolset for building fast, scalable, modern web apps in Go with the **GOTTH stack** — **Go**, **TailwindCSS**, **Templ**, and **HTMX**.

---

## Where everything lives now (v3)

| Module | Repo | What it is |
|---|---|---|
| **CLI** | [`gothicframework/cli`](https://github.com/gothicframework/cli) | The `gothic` command (scaffold, dev, build, deploy) |
| **Core** | [`gothicframework/core`](https://github.com/gothicframework/core) | Runtime library: file-based routing, caching/ISR, WASM runtime, assets |
| **Components** | [`gothicframework/components`](https://github.com/gothicframework/components) | Reusable Templ components (`RuntimeScripts`, `Styles`, `OptimizedImage`, …) |
| **Middlewares** | [`gothicframework/middlewares`](https://github.com/gothicframework/middlewares) | The single chi middleware that wires the whole runtime |

### Install the v3 CLI

```bash
go install github.com/gothicframework/cli/v3/cmd/gothic@latest
gothic init github.com/you/my-app
```

The libraries version independently of the CLI — you never add them by hand. `gothic init` scaffolds a project that imports them at the right versions.

---

## Already on v2? Migrate in one command

From your existing v2 project root, using this (final) v2 CLI:

```bash
gothicframework migrate-v3
```

That command installs the new `gothic` CLI, runs the migration for you, and then hands you over to the new command name. Equivalently, if you already installed the v3 CLI:

```bash
go install github.com/gothicframework/cli/v3/cmd/gothic@latest
gothic migrate-v3          # add --dry-run to preview
```

After migrating, use **`gothic`** instead of **`gothicframework`**:

| Before (v2) | After (v3) |
|---|---|
| `gothicframework dev` | `gothic dev` |
| `gothicframework build` | `gothic build` |
| `gothicframework deploy` | `gothic deploy` |

See the **[v3 CLI repo](https://github.com/gothicframework/cli)** for the full list of breaking changes and new features (OpenTofu-based deploys, Go-based config, the rearchitected WASM runtime, and more).

---

## v1 / v2 reference

The last v2 release (**v2.18.0**) is this repository at its final state: the full v2 toolchain **plus** the `migrate-v3` bridge above. Older tags remain available for historical reference. For anything new, start with the [v3 CLI](https://github.com/gothicframework/cli).
