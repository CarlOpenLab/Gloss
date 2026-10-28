<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="resources/logo-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="resources/logo.png">
    <img alt="Gloss Logo" src="resources/logo.png" width="130" height="130">
  </picture>
</p>

<h1 align="center">Gloss</h1>

<p align="center">
  <strong>Next-generation native Git client with AI change auditing and commit intelligence.</strong><br>
  <em>Pure Go · GPU Direct Rendering · Zero WebViews · Single ~20MB Binary</em>
</p>

<p align="center">
  <a href="https://github.com/CarlOpenLab/Gloss/releases/latest"><img src="https://img.shields.io/badge/Release-v0.3.24-4F46E5?style=flat-square" alt="Release v0.3.24"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.27+"></a>
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Windows%20%7C%20Linux-000000?style=flat-square&logo=apple&logoColor=white" alt="macOS | Windows | Linux">
  <a href="https://github.com/egoist/mygo"><img src="https://img.shields.io/badge/GUI-mygo%20(GPU%20Direct)-4F46E5?style=flat-square" alt="mygo"></a>
  <img src="https://img.shields.io/badge/Architecture-Zero%20WebView-10B981?style=flat-square" alt="Zero WebView">
  <img src="https://img.shields.io/badge/AI-Claude%20%7C%20OpenAI%20%7C%20DeepSeek%20%7C%20Ollama-6366F1?style=flat-square" alt="Multi-LLM">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="License: MIT"></a>
</p>

<p align="center">
  <a href="https://carlopenlab.github.io/Gloss/">🌐 Official Website</a> •
  <a href="https://github.com/CarlOpenLab/Gloss/releases">📦 Download Releases</a> •
  <a href="#visual-showcase--features">✨ Key Features</a> •
  <a href="#keyboard-shortcuts">⌨️ Shortcuts</a> •
  English • <a href="README.zh_CN.md">简体中文</a>
</p>

---

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="pages/assets/shot-history-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="pages/assets/shot-history-light.png">
    <img alt="Gloss Main Interface Preview" src="pages/assets/shot-history-dark.png" width="100%">
  </picture>
</p>

---

## Why Gloss?

> **"Inspect clearly before you merge."**

In the era of AI coding assistants and rapid iteration, code changes are generated faster than ever. When reviewing a pull request or inspecting dirty worktrees, the bottleneck is rarely reading lines of code—it is understanding **why** changes were made, what the author intended, and whether dangerous edge cases or hardcoded secrets are lurking in the diff.

Traditional Git GUIs are often bloated Electron applications that consume hundreds of megabytes of RAM and stutter when rendering large diffs, or legacy tools that lack modern AI-assisted code intelligence.

**Gloss** is built from scratch in pure Go to deliver a featherweight, lightning-fast native desktop experience:

* **⚡ Native GPU Direct Rendering**: Built on [mygo](https://github.com/egoist/mygo). Zero WebViews, zero JavaScript runtimes, zero Electron overhead. Instant launch (<100ms), 60 FPS silky-smooth scrolling, and low memory consumption in a single ~20MB executable.
* **🛡️ Pre-Flight AI Risk Auditing**: One-click scanning of your staged diff. LLMs highlight hardcoded credentials, injection vulnerabilities, and regression risks grouped by severity, anchored directly to file and line numbers.
* **🤖 AI Commit & Branch Intelligence**: Automatically draft Conventional Commits messages from staged diffs with optional intent hints, and name git branches from a simple one-line objective.
* **🌲 Fork-Style Topology & Dual-Pane Review**: Intuitive branching graph, single-file focused diffs with word-level inline highlights, line-range and hunk staging, image diff previews, and progress tracking.
* **🦺 Human-Friendly Error Explanations**: No more cryptic git errors or raw HTTP dumps. Hook rejections, lock contention, and conflicts are translated into plain-language explanations with actionable advice.

---

## Visual Showcase & Features

### 1. Pre-Flight AI Risk Auditing
Catch potential security risks and inadvertent mistakes *before* committing. Gloss feeds your staged diff to your chosen LLM and generates a structured audit report categorized into **Critical**, **High**, **Medium**, and **Low** risks with exact file anchors and line numbers.

<p align="center">
  <img src="pages/assets/shot-riskscan-dark.png" alt="Pre-flight Risk Scan" width="100%">
</p>

### 2. Fork-Style Commit Topology & Ref Intelligence
Visualize the entire project branching structure with a high-density, GPU-rendered topology graph:
* **All Branches Overview**: View the unified history of all refs (local, remote, and tags) at a glance.
* **Upstream Sync Trackers**: Every tracking branch displays real-time `↑n ↓n` ahead/behind commit counts extracted in a single efficient query, alerting you if the upstream has been deleted on remote.
* **Instant Filtering (`/`)**: Real-time filtering across commit messages, hashes, authors, and ref names with streaming background loading.
* **Custom Lane Palettes**: Choose from Classic, Vivid, or Monochromatic schemes with per-lane custom color overrides.

### 3. Precision Code Review & Granular Staging
Review diffs one file at a time without cognitive clutter:

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="pages/assets/shot-diff-dark.png">
    <source media="(prefers-color-scheme: light)" srcset="pages/assets/shot-diff-light.png">
    <img alt="Gloss Diff View" src="pages/assets/shot-diff-dark.png" width="100%">
  </picture>
</p>

* **Word-Level Inline Highlighting**: Spot exact character and word alterations at a glance with dual-side line numbering and collapsible unchanged regions.
* **Per-Hunk & Line-Range Staging**: Hover over any diff block or selection to stage or unstage exactly that hunk.
* **Raster Image Diff Review**: Native side-by-side previews for **PNG, JPEG, GIF, WebP, and BMP** files, displaying dimensions, color depth, and file size comparisons.
* **Reviewed (`Viewed`) Tracking**: Mark files as reviewed to collapse them and keep your focus on pending files; automatically resets when file contents change.
* **In-Diff Search (`⌘F`)**: High-speed search inside the diff viewer, separated from fuzzy file searching (`⌘P`).

### 4. Inline Conflict Resolution & Safe Workflows
* **Visual Conflict Solving**: Open conflicted files directly in the review pane with side-by-side pickers: *Keep Ours*, *Keep Theirs*, *Keep Both*, or *Keep Neither*. The right side is directly editable, and clicking **Save & Resolve** writes the file and runs `git add` in a single step.
* **Persistent Amber Conflict Bar**: Active rebase, merge, or cherry-pick states surface an amber status bar with clear *Continue*, *Skip*, or *Abort* actions and remaining conflict counters.
* **Protected Force Push**: Safe force-pushing via `--force-with-lease` prevents accidentally overwriting teammates' remote commits.
* **Two-Step Branch Deletion**: Separate local branch deletion from remote branch deletion with dedicated confirmation dialogs.

### 5. Integrated Workspace & Multi-Session Terminal
* **Whole-Workspace File Explorer**: The built-in editor mode lists your entire repository tree (`git ls-files`) with status-based semantic coloring (VS Code style).
* **Ghostty-Powered Terminal**: Open multiple shell tabs per project repository with customizable monospace/Nerd fonts.
* **AI CLI Image Paste Bridge**: Seamlessly paste images (`⌘V`) into AI terminal CLIs (like Claude Code) by automatically translating clipboard image payloads.

### 6. 3-Layer Git Config & Multi-LLM Management
* **Git Identity Inspector**: Manage `user.name` and `user.email` across **System**, **Global** (`~/.gitconfig`), and **Local** (`.git/config`) scopes in one clean panel. Gloss preserves all comments, includes, and custom directives without full-file rewrites.
* **Multi-Provider AI Engine**: Built-in presets for **Anthropic Claude**, **OpenAI**, **DeepSeek**, **Kimi (Moonshot)**, **Qwen**, **Zhipu GLM**, **OpenRouter**, and local **Ollama** models. Reorder providers via drag-and-drop.

---

## Download & Installation

Pre-built binaries are available for macOS, Windows, and Linux.

### Pre-built Releases

Download the latest release from the [GitHub Releases](https://github.com/CarlOpenLab/Gloss/releases/latest) page:

| Platform | Format | Description |
| :--- | :--- | :--- |
| **macOS** | `.dmg` | Apple Silicon (`arm64`) & Intel (`x86_64`) |
| **Windows** | `.exe` / `.zip` | Setup Installer or Portable Zip (Zero admin rights, AMD64) |
| **Linux** | `tar.gz` | Portable 64-bit binary |

> [!NOTE]
> **Windows Defender False Positive Note:**
> Microsoft Defender's ML heuristics occasionally flag freshly compiled, unsigned installers as `Trojan:Script/Wacatac.C!ml`. Gloss contains zero obfuscation, zero telemetry, and zero installer-time downloads.
> * **Recommended quick option**: Download `gloss-<version>-windows-amd64-portable.zip`, unpack it, and run `Gloss.exe` directly.
> * **Or allow in Defender**: Open *Windows Security* → *Protection history* → Select detection → *Actions* → *Allow on device*.

---

## Build from Source

Gloss requires **Go 1.27+** (managed automatically via `mise`).

```bash
# 1. Clone the repository
git clone https://github.com/CarlOpenLab/Gloss.git
cd Gloss

# 2. Install the required toolchain
mise install

# 3. Run development mode with live hot-reload
go tool mygo dev

# 4. Build production release binaries (.app / .dmg)
go tool mygo build

# 5. Run headless test suite
go test ./...
```

---

## Keyboard Shortcuts

Gloss is engineered for keyboard-first efficiency:

| Shortcut (macOS) | Shortcut (Win / Linux) | Action |
| :--- | :--- | :--- |
| `⌘K` | `Ctrl+K` | Open Command Palette |
| `J` / `K` | `J` / `K` | Jump to next / previous hunk (Changes) or commit (History) |
| `↓` / `↑` | `↓` / `↑` | Select next / previous file (Changes) or commit (History) |
| `⌘F` | `Ctrl+F` | Find in active diff view |
| `↵` / `⇧↵` | `Enter` / `Shift+Enter` | Navigate to next / previous diff search match |
| `⌘P` | `Ctrl+P` | Fuzzy find and filter files |
| `/` | `/` | Focus commit search box in History |
| `⌘S` | `Ctrl+S` | Save edited file (auto `git add` for conflict files) |
| `⌘⏎` | `Ctrl+Enter` | Commit staged changes |
| `⌘O` | `Ctrl+O` | Open repository |
| `⌘1` / `⌘2` | `Ctrl+1` / `Ctrl+2` | Switch to **Changes** / **History** view |
| `⌘3` / `⌘4` | `Ctrl+3` / `Ctrl+4` | Switch to **Terminal** / **Editor** view |
| `⌘⇧B` | `Ctrl+Shift+B` | Toggle Sidebar |
| `⌘R` | `Ctrl+R` | Refresh repository status |
| `⌘⇧F` | `Ctrl+Shift+F` | Fetch all remotes |
| `⌘⇧P` | `Ctrl+Shift+P` | Pull from remote tracking branch |
| `⌘⇧U` | `Ctrl+Shift+U` | Push to remote branch |
| `⌘+` / `⌘-` / `⌘0` | `Ctrl++` / `Ctrl+-` / `Ctrl+0` | Zoom in / Zoom out / Reset interface scale |
| `⌘,` | `Ctrl+,` | Open Settings |
| `Esc` | `Esc` | Close search bar or dismiss modal overlay |

---

## LLM Configuration

Configure your AI provider directly in **Settings (`⌘,` / `Ctrl+,`) → Models**, or set standard environment variables:

```bash
# Anthropic Claude
export ANTHROPIC_API_KEY="sk-ant-..."
export ANTHROPIC_MODEL="claude-sonnet-4-5" # Optional

# OpenAI / Compatible API
export OPENAI_API_KEY="sk-..."
export OPENAI_BASE_URL="https://api.openai.com/v1" # Optional
```

Configuration files are stored locally with strict permissions (`0600`) at `~/Library/Application Support/Gloss/config.json` (macOS) or platform config directories.

---

## Architecture Overview

```
Gloss/
├── main.go / app.go         Application lifecycle, window management, event loop & frame dispatch
├── sidebar.go               Ref tree: branches, remotes, tags, and stashes
├── history.go               Commit list, column layouts, and selection tracking
├── graph.go                 GPU-rendered commit topology graph layout engine
├── review.go                Diff review pane: single-file viewer, hunk staging & conflict editor
├── imagereview.go           Raster image preview & visual diff engine (PNG, JPEG, GIF, WebP, BMP)
├── commitbox.go             Composer card: staged files, hunk actions & AI commit generation
├── workspace.go             Whole-repository file explorer for editor mode
├── palette.go / keys.go     Command palette and keyboard shortcut mappings
├── settings.go / theme.go   macOS-style floating settings panel & GitHub-aligned design system
├── gitconfig.go             3-Layer Git configuration inspector & editor
└── internal/
    ├── gitrepo/             Robust git CLI wrapper (status, diff, log, refs, write operations)
    ├── diff/                Unified diff parser, slice generator, and hunk patch range calculator
    ├── llm/                 Streaming multi-provider client (Anthropic, OpenAI, Ollama)
    ├── explain/             Prompt generation for commit messages, branch naming & risk scans
    └── filetree/            Fuzzy-filterable directory tree data structures
```

> [!IMPORTANT]
> **Threading Invariant**: All mutations to application state occur strictly on the main thread. Background goroutines process disk and network tasks asynchronously and dispatch immutable result payloads back to the main thread.

---

## Roadmap

- [x] High-performance GPU direct rendering interface
- [x] Pre-flight AI risk audit for staged diffs
- [x] Conventional Commits & branch naming generator
- [x] GPU topology graph with custom lane palettes
- [x] File-level and hunk-level / line-range staging
- [x] Raster image diff preview (PNG, JPEG, GIF, WebP, BMP)
- [x] Reviewed (`Viewed`) file tracking and auto-collapse
- [x] Human-friendly git error translations
- [x] Inline visual conflict resolver with one-click side picking
- [x] Multi-session Ghostty terminal with AI CLI image clipboard bridge
- [x] 3-layer Git config identity editor (System / Global / Local)
- [x] Opt-in background auto-fetch (fetch only, never pull)
- [x] Draggable floating settings dialog & multi-repo tabs
- [ ] Interactive rebase GUI with visual commit squash/reorder
- [ ] Built-in GitHub PR review & comment creation

---

## License

This project is open-source software licensed under the [MIT License](LICENSE).
