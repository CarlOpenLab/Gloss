<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="resources/logo-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="resources/logo.png">
    <img alt="Gloss Logo" src="resources/logo.png" width="130" height="130">
  </picture>
</p>

<h1 align="center">Gloss</h1>

<p align="center">
  <strong>Cross-platform Git client with AI change audits and commit messages.</strong><br>
  <em>Pure Go · GPU Direct Rendering · Zero WebViews</em>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.27+"></a>
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-000000?style=flat-square&logo=apple&logoColor=white" alt="macOS | Linux | Windows">
  <a href="https://github.com/egoist/mygo"><img src="https://img.shields.io/badge/GUI-mygo%20(GPU%20Direct)-4F46E5?style=flat-square" alt="mygo"></a>
  <img src="https://img.shields.io/badge/AI-Claude%20%7C%20DeepSeek%20%7C%20Ollama%20%7C%20OpenAI-6366F1?style=flat-square" alt="Multi-LLM">
  <img src="https://img.shields.io/badge/Architecture-Zero%20WebView-10B981?style=flat-square" alt="Zero WebView">
</p>

<p align="center">
  English • <a href="README_zh.md">简体中文</a>
</p>

---

## Why Gloss?

When reviewing large pull requests or AI-generated diffs, the biggest bottleneck isn't looking at lines of code—it's understanding **why** each hunk was changed, what the author intended, and whether there are hidden pitfalls.

**Gloss** is a cross-platform Git client (macOS, Linux, Windows) crafted for swift code review and safe everyday Git operations:

- **Catch risky changes early**: One click scans the staged diff and lists suspicious spots—hardcoded secrets, injection risks, and the like—ranked by severity and anchored to file and line.
- **Blazing native performance**: Powered by [mygo](https://github.com/egoist/mygo), rendered directly via GPU. **Zero WebViews, zero JavaScript runtimes, single native binary**. Instant startup, silky smooth 60fps scrolling, minimal memory footprint.

---

## Features

- ⚡ **GPU Direct Rendering**: Pure Go native interface built with `mygo`, styled around GitHub's visual palette.
- 🛡 **Pre-flight Risk Scan**: One click scans the staged diff for hardcoded secrets, injection risks, and other suspicious changes, grouped by severity and anchored to file and line.
- 🤖 **AI Commit Messages & Branch Names**: Generate Conventional Commits messages from the staged diff (optionally steered by an intent hint), and name new branches from a one-line description.
- 🌲 **Fork-Style Layout & Topology Graph**: Custom commit topology graph to visualize branching histories; split / unified diff toggle with chroma syntax highlighting.
- 🔍 **Viewed Tracking**: Mark files as reviewed to collapse them into clean headers; automatically resets when file contents change.
- 🛠 **Full Git Workflow**:
  - **Staging & Commits**: Whole-file staging, **per-hunk staging**, persistent drafts, and Amend (`--amend`).
  - **Branches & Remotes**: Switch/create/rename/delete branches, safe merge and rebase, push tags, and `--force-with-lease` protected force push.
  - **Stash Management**: Modal stash dialog with message and untracked files (`-u`); right-click to inspect diff, Apply, Pop, or Drop with confirmation.
- 🦺 **Human-Friendly Error & Conflict Resolution**:
  - Plain-language explanations for hook rejections, lock contention, missing credentials, or detached HEAD states.
  - Dedicated amber banner during active rebase/merge conflicts with two-way side picking and deletion handling.
- ⌨️ **Keyboard-Driven & Command Palette**: Press `⌘K` for the command palette, `J` / `K` to jump across diff hunks, and full macOS native menu shortcuts.
- 🤖 **Universal LLM Compatibility**: Built-in presets for Anthropic Claude, DeepSeek, Kimi, Qwen, GLM, OpenRouter, Ollama, and any OpenAI-compatible API.

---

## Detailed Capabilities

- **Local Repositories**: Open any working copy to inspect uncommitted changes. The sidebar organizes Branches, Remotes, Tags, and Stashes. Click a branch to focus on its commit history; click a stash to view its exact changes.
- **Staging & Commits**: Stage entire files or individual hunks via dedicated buttons on hunk headers. Work is cleanly divided between "Staged" and "Unstaged" areas. Draft messages are preserved per repository even when switching contexts (`⌘⏎` to commit).
- **File Row Context Menu**: Right-click any file row in the Changes, History, or Stash views — stage / unstage (conflict rows get "Use Ours / Use Theirs" plus "Mark Resolved" instead), discard changes… (untracked files say "Delete File…"; both ask first), open in editor, reveal in Finder, copy path / relative path / file name, view file history…, mark as reviewed, and add to `.gitignore`. Rows in History and Stash views are read-only, so actions that would touch the working tree simply aren't there.
- **View File History…**: Lists the commits that touched this file (`git log --follow`, so it follows renames) and jumps straight to the selected commit in the History tab — no digging through the whole branch history to find which commits it appeared in.
- **Branch Switching, Checkout & Tags**: Double-click branches or tags in the sidebar to check them out. Checking out remote branches automatically sets up local tracking branches. Detached HEAD states trigger warning badges in the toolbar.
- **Branch Integration & Remote Management**:
  - Right-click local branches to merge or rebase onto target branches (with safety confirmations and conflict assistance).
  - Right-click remote branches to checkout or safely delete them with confirmation.
  - Push tags to `origin` with a single click.
  - Protected force pushing via `--force-with-lease` prevents overwriting teammates' work.
- **Stash Management & Amend**:
  - Save dirty working tree states with custom messages and untracked files (`-u`).
  - Inspect stash diffs directly; apply, pop, or drop stashes with confirmation dialogs.
  - Amend commits with pre-filled previous commit messages, supporting message-only adjustments or staged additions.
- **Fetch, Pull & Push**: Dedicated toolbar actions with intelligent disable states for detached HEAD or repos without remotes. Provides plain-language translations for push rejections, divergences, and authentication requirements.
- **GitHub-Style Diff View**: Red/green diff highlighting, dual-side line numbers, syntax highlighting (chroma), and file tree navigation.
- **Pre-flight Risk Scan**: The 🛡 Audit Changes button hands the full staged diff to your model and lists suspicious spots (hardcoded secrets, injection risks, …) grouped by severity—critical / high / medium / low—each anchored to a file and line. Provider failures are translated into plain language instead of raw HTTP errors.
- **AI Commit Messages & Branch Names**: The AI button in the commit box drafts a Conventional Commits message from the staged diff, treating your notes in the message box as an intent hint; a zh/EN toggle picks the language. The new-branch dialog can generate a branch name from a one-line description.
- **Fork-Style Commit Graph**: Top pane displays commits alongside a calculated topology graph; bottom pane displays file trees on the left and diffs on the right.
- **File Tree & Viewed Folding**: Collapsible directory tree with single-child path flattening and fuzzy search (`⌘P`). Marking files as `Viewed` collapses them to fit reviews on a single screen; automatically invalidates on content changes.
- **Find in Diff (`⌘F`)**: Search the diff as you type — every match is highlighted with live counting, and `↵` / `⌘G` walk through them. Matches inside unchanged regions or collapsed files are revealed automatically, so a hit is never somewhere you can't see it.
- **Streamlined Conflict Resolution**: Amber status bar during merge/rebase/cherry-pick conflicts with step indicators (Continue / Skip / Abort). Interactive conflict cards allow two-way side picking and accept-deletion confirmations.
- **Command Palette (`⌘K`)**: Quick access to all application actions with keyboard-first workflows.

---

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `⌘K` | Open Command Palette |
| `J` / `K` | Jump to Next / Previous hunk (Changes) or commit (History) |
| `↓` / `↑` | Select Next / Previous commit (History) or file (Changes) |
| `⌘F` | Find in diff (search the changed lines) |
| `↵` / `⇧↵` | Next / previous match (in the find box) |
| `⌘G` / `⌘⇧G` | Next / previous match |
| `Esc` | Close the find bar |
| `⌘P` | Focus file filter input |
| `⌘⏎` | Commit staged changes |
| `⌘O` | Open repository |
| `⌘1` / `⌘2` | Switch between Changes / History |
| `⌘⇧B` | Toggle Sidebar |
| `⌘R` | Refresh repository status |
| `⌘⇧F` / `⌘⇧P` / `⌘⇧U` | Fetch / Pull / Push |
| `⌘+` / `⌘−` / `⌘0` | Zoom in / Zoom out / Reset font size |
| `⌘,` | Model settings |
| `Esc` | Exit branch compare / PR review |

The table uses macOS notation. **On Linux and Windows, ⌘ means `Ctrl`** — the same key does the same thing, and the app displays `Ctrl+…` there (see `keys_i18n.go`). ⌥ becomes `Alt`, ⌫ becomes `Backspace`, ⏎ becomes `Enter`.

> Menu items (`menu.go`) share identical shortcut mappings with `keys.go` to keep the native menu and the in-app shortcuts in sync.

---

## Build & Run

Requires **Go 1.27+** (enforced by `mygo`, pre-configured in `mise.toml`).

```bash
# 1. Install toolchain via mise
mise install                 # Installs Go 1.27.1

# 2. Local development with live reload
go tool mygo dev             # Hot reload on code changes

# 3. Production release
go tool mygo build           # Builds .app and .dmg into build/darwin-arm64/

# 4. Automated tests
go test ./...                # Headless rendering tests, no display required
```

---

## LLM Configuration

Open **Settings (`⌘,`)** from the top right menu to configure your API provider and key. The configuration is securely stored at `~/Library/Application Support/Gloss/config.json` (permissions `0600`).

The provider column lists only the providers you've added; drag the handle at the start of a row to reorder them (the order is part of the draft — it lands on Save, and Cancel throws it away). New ones come from the **Add Provider** button's menu, which also shows built-in presets for Anthropic Claude, DeepSeek, Kimi, Qwen, GLM, OpenRouter and Ollama, plus custom OpenAI-compatible endpoints; providers already in the list are greyed out there.

Alternatively, Gloss falls back to standard environment variables:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# Optional: export ANTHROPIC_MODEL=claude-sonnet-5-5
# Optional: export ANTHROPIC_BASE_URL=https://...
```

---

## Architecture

```
main.go / app.go         Window, menus, app state, root frame layout
  sidebar.go             Sidebar: branches / remotes / tags / stashes tree
  branchops.go           Branch & remote operations: merge, rebase, delete, force push
  renamebranchdialog.go  Branch rename dialog with name validation
  stashdialog.go         Stash modal dialog (save dirty state with optional message)
  stashops.go            Stash actions: apply, pop, drop with confirmation
  history.go             Top pane: commit table and column headers
  commitops.go           Commit row actions: cherry-pick, revert, checkout, copy hash
  resetdialog.go         Branch reset dialog (soft / mixed / hard with warning)
  newbranchdialog.go     Create branch from selected commit
  graph.go               Topology graph: layout computation & line drawing
  filepane.go            Bottom pane left: file list & staging controls
  filemenu.go            File row context menu: stage / discard / copy / history / ignore
  filehist.go            "View File History…" dialog: this file's own commits, jump to one
  diffview.go            Diff rendering, syntax highlighting, folding
  commitbox.go           Staging checkboxes, hunk staging buttons, commit box
  checkout.go            Checkout handlers, double-click, cleanup
  tagdialog.go           Tag creation dialog (lightweight & annotated)
  remote.go              Toolbar remote actions: fetch, pull, push
  opbanner.go            Operation feedback banners (error bar / success toast)
  pendingbar.go          Amber conflict status bar: continue / skip / abort
  confirmdialog.go       Destructive action confirmation dialogs
  resolve.go             Conflict resolution: two-way side picking & cards (menu lives in filemenu.go)
  treeview.go            Shared tree rows (indentation, arrows, selection capsules)
  refs.go                Git reference grouping & filtering
  rows.go                Virtual list row models
  palette.go / keys.go   Command palette & shortcut bindings
  menu.go                macOS native menu bar commands
  settings.go            Model settings modal
  theme.go               GitHub-aligned theme palette (light / dark)

internal/
  gitrepo/               Git CLI wrapper (status / diff / log / refs / write ops)
    write.go             Staging, hunk staging, commit, amend
    stash.go             Stash operations (push / apply / pop / drop)
    commitops.go         Cherry-pick, revert, reset (--soft / --mixed / --hard)
    branch.go            Create, rename, delete branch, merge, rebase
    remote.go            Fetch, pull, push (with --force-with-lease, tags, delete)
    pending.go           In-progress operation detection + continue / skip / abort
    conflict.go          Conflict side resolution (ours / theirs, delete-modify)
    failure.go           Plain-language git error translator
  diff/                  Unified diff parser
    slice.go             Slice hunk patches for hunk-level staging
  highlight/             Chroma syntax tokenization and line slicing
  explain/               Prompt construction for commit messages, branch names, PR review, and risk scans (pure functions)
  llm/                   Streaming client for Anthropic & OpenAI-compatible APIs
    failure.go           Plain-language provider error translator
  store/                 Config and state persistence
  filetree/              Tree building, flattening, and fuzzy filtering
```

> **Threading Invariant** (see top of `app.go`): All app field mutations occur strictly on the main thread. Background goroutines receive value parameters and dispatch results back to the main thread.

---

## Roadmap

- [x] Staging + Commits (file-level + hunk-level)
- [x] Branching / Checkout / Tags (double-click + context menu)
- [x] Remotes: Fetch / Pull / Push
- [x] Conflict management: Amber status bar + Continue / Skip / Abort
- [x] Conflict side picking: Use ours / Use theirs / Mark resolved
- [x] Commit context menu: Cherry-pick / Revert / Reset / Branch from here
- [x] Local branch operations: New branch (`⌘⇧N`), rename, safe deletion
- [x] Remote branch & integration: Delete remote branch, merge/rebase, force-with-lease push
- [x] Draggable sidebar resizing
- [x] Keyboard navigation for the file list: `J` / `K` and `↑` / `↓` move the selected commit on the history tab; on the changes tab `↑` / `↓` move the selection in the changed-files list while `J` / `K` jump between hunks
- [x] AI commit message & branch generation
- [x] GitHub PR: list open PRs and review PR diffs
- [x] Pre-flight risk scan: Full-diff scan highlighting potential vulnerabilities
- [x] File row context menu: stage / discard (delete for untracked) / reveal in Finder / copy path three ways / view file history / mark reviewed / add to `.gitignore`
- [x] Drag-to-reorder providers, with presets moved into the Add Provider menu
- [ ] Free cross-line text selection in a diff: render each hunk as one multi-line selectable text (chunk long hunks by N lines), with native drag-select and ⌘C
