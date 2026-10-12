# Gloss 自动更新方案分析

> 基于 mygo 框架的 signed auto-update 系统，通过 GitHub Releases 分发。

---

## 1. 现状

| 项目 | 状态 |
|------|------|
| `github.com/egoist/mygo` 依赖 | ✅ 已有 v0.2.13 |
| `tool github.com/egoist/mygo/cmd/mygo` | ✅ 已有 |
| `mygo.json` 配置 | ✅ 已有，但缺少 `updates` 字段 |
| 签名密钥 | ❌ 未生成 |
| 代码集成 | ❌ 未接入 |
| CI/CD 发布流程 | ❌ 未配置 |
| 当前版本 | `0.1.0` |

---

## 2. mygo 自动更新架构概览

```
┌─────────────────────────────────────────────────────────┐
│                    构建时                                │
│                                                         │
│  mygo keygen                                            │
│    ├── mygo-update.key  (私钥，存密码管理器/CI secrets)  │
│    └── mygo-update.pub  (公钥，写进 mygo.config.ts)      │
│                                                         │
│  mygo build                                             │
│    ├── 编译应用                                         │
│    ├── 签名 archive (ed25519)                           │
│    ├── 生成 manifest JSON (version/notes/url/sig)       │
│    ├── 生成 delta 文件 (bsdiff)                         │
│    └── -upload → 上传 GitHub Release (draft)            │
│                                                         │
│  ldflags -X 注入:                                       │
│    ├── packageUpdateFeed  → manifest URL               │
│    └── packageUpdateKey   → ed25519 公钥               │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    运行时                                │
│                                                         │
│  mygo.Updater.Enabled()  → 检查是否支持更新              │
│  mygo.Updater.Check(ctx) → 拉取 manifest，比较版本       │
│  update.Install(ctx, progress) → 下载+验证+替换          │
│  mygo.App.Relaunch()     → 重启应用                      │
│                                                         │
│  或：updater.Plugin（Sparkle 风格窗口，自动处理以上）     │
└─────────────────────────────────────────────────────────┘
```

---

## 3. 实施方案

### 方式 A：updater.Plugin（推荐，最快集成）

mygo 提供了开箱即用的 Sparkle 风格更新窗口，适合大多数应用。

**优点：**
- 代码量极小（3 行）
- 自动处理检查、下载、安装、重启全流程
- 内置 release notes 展示、进度条、跳过版本
- 支持自动检查（每天一次）和手动检查
- 内置多语言（含简体中文）
- 有偏好设置 API，可对接 Gloss 的设置面板

**缺点：**
- 窗口样式是 mygo 默认的，不是 Gloss 自定义设计
- 需要 WebKitGTK (Linux) / WebView2 (Windows 10+)，或改用 `native.Plugin`

**集成代码：**

```go
// main.go
import "github.com/egoist/mygo/plugins/updater"

func main() {
    mygo.Use(updater.Plugin)  // ← 加这一行即可
    // ... 其余不变
}
```

菜单集成（在 Gloss 菜单栏加"检查更新"）：

```go
import "github.com/egoist/mygo/plugins/updater"

// 在 buildAppMenu 里加一项
updater.MenuItem(),  // "检查更新…"
```

### 方式 B：自定义 UI（用 mygo.Updater）

Gloss 有自己的设计语言（原生 UI 工具包），可以用 `mygo.Updater` API 构建内置更新体验。

**优点：
- 完全控制 UI 风格，与 Gloss 设计一致
- 可以在设置面板里加"自动更新"开关
- 可以在应用内显示更新提示横幅

**缺点：**
- 需要自己写更新窗口/横幅 UI
- 需要自己管理偏好设置（虽然 updater 包有 API）
- 代码量更大

**核心 API：**

```go
import "context"
import "github.com/egoist/mygo"

// 检查更新
upd, err := mygo.Updater.Check(context.Background())
if err != nil {
    if err == mygo.ErrUpdatesDisabled {
        // dev 构建或未配置更新，静默跳过
    }
    // 网络错误等，可以提示用户
} else if upd != nil {
    // 有新版本！upd.Version, upd.Notes, upd.Date
    // 显示更新提示...
}

// 安装更新
err = upd.Install(context.Background(), func(downloaded, total int64) {
    // 进度回调，更新进度条
})
if err == nil {
    mygo.App.Relaunch()  // 重启
}
```

---

## 4. 完整接入步骤

### Step 1: 生成签名密钥

```bash
mygo keygen
# 生成在 mygo/update-keys/ 下：
#   mygo-update.key  (私钥)
#   mygo-update.pub  (公钥)
```

**⚠️ 安全要点：**
- 私钥存密码管理器 + GitHub Secrets（`MYGO_UPDATER_PRIVATE_KEY`）
- 公钥写进 `mygo.config.ts`
- **永远不要把私钥提交到仓库**

### Step 2: 配置 mygo.json（或 mygo.config.ts）

当前 `mygo.json`：
```json
{
  "name": "Gloss",
  "identifier": "dev.carl.gloss",
  "version": "0.1.0",
  "icon": "resources/icon.png",
  "out": "build"
}
```

需要添加 `updates` 字段。由于需要嵌入公钥（较长的字符串），建议改用 `mygo.config.ts`：

```ts
import { defineConfig } from "mygo/config"

export default defineConfig({
  name: "Gloss",
  identifier: "dev.carl.gloss",
  version: "0.1.0",
  icon: "resources/icon.png",
  out: "build",
  updates: {
    publicKey: "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----",
    github: "CarlOpenLab/Gloss",
    // 如果 repo 里还有其他 release（比如 CLI），用 tagPrefix 区分：
    // tagPrefix: "desktop-v",
  },
})
```

> **注意**：Gloss 是纯原生 UI 项目（无 web 前端），构建用 `go tool mygo build` 而非 `bun run build`。

### Step 3: 代码集成

#### 选方式 A（updater.Plugin）：

```go
// main.go 顶部
import "github.com/egoist/mygo/plugins/updater"

func main() {
    mygo.Use(updater.Plugin)
    // ... 其余不变
}
```

#### 选方式 B（自定义 UI）：

在 `main.go` 的 `WhenReady` 里加检查逻辑，在设置面板加开关，在 app 里加更新横幅。

### Step 4: CHANGELOG.md

mygo 从 `CHANGELOG.md` 的 `## x.y.z` 标题下提取 release notes：

```markdown
## [0.2.0] - 2026-10-15

### 新增
- 支持 PR review 的 diff 检视

### 修复
- 修复了大文件 diff 渲染卡顿的问题
```

### Step 5: GitHub Actions 发布流程

创建 `.github/workflows/release.yml`：

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  draft-release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Create draft release
        run: |
          gh release create "$GITHUB_REF_NAME" \
            --draft \
            --verify-tag \
            --generate-notes \
            --title "Gloss $GITHUB_REF_NAME" \
            || true  # 如果已存在就跳过
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  build:
    needs: draft-release
    strategy:
      fail-fast: false
      matrix:
        include:
          - platform: darwin/arm64
            os: macos-latest
          - platform: darwin/amd64
            os: macos-latest
          - platform: windows/amd64
            os: ubuntu-latest
          - platform: linux/amd64
            os: ubuntu-latest
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build and upload
        run: |
          go tool mygo build \
            -platform "${{ matrix.platform }}" \
            -upload
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          MYGO_UPDATER_PRIVATE_KEY: ${{ secrets.MYGO_UPDATER_PRIVATE_KEY }}
```

### Step 6: 配置 GitHub Secrets

```bash
gh secret set MYGO_UPDATER_PRIVATE_KEY < mygo-update.key
```

### Step 7: 发布流程

```bash
# 1. 更新版本号（mygo.json 或 mygo.config.ts）
# 2. 更新 CHANGELOG.md
# 3. 提交并打 tag
git tag v0.2.0
git push origin v0.2.0

# 4. CI 自动构建、签名、上传 draft release
# 5. 去 GitHub 检查 draft release，点 Publish
# 6. 用户收到更新通知
```

---

## 5. 平台特定注意事项

### macOS（不签名不公证）

**不签名的后果：** 用户下载后双击，Gatekeeper 显示「已损坏，无法打开」。需要右键 → 打开 → 确认才能首次启动。

**应对：**
- README 和下载页加「首次打开指引」
- 更新窗口里提示用户如果遇到「已损坏」提示的处理方法
- 目标用户是开发者，右键打开对他们来说不是门槛
- 后续有收入了再补签名+公证（$99/年）

**不需要的配置：** 跳过 `macos.signingIdentity`、`macos.notarize`、相关 secrets。

### Windows
- 未签名的 exe 会触发 SmartScreen 警告（蓝色窗口）
- 用户点「仍要运行」即可，体验比 macOS 好
- 后续可买代码签名证书（比 Apple 便宜）

### Linux
- 生成 `.tar.gz` + `install.sh`
- 不需要签名（没有代码签名基础设施）
- 用户安装到 `~/.local/gloss/app` 这类可写目录

---

## 6. 更新源选择

| 来源 | 配置 | 适用场景 |
|------|------|----------|
| **GitHub Releases** | `github: "owner/repo"` | ✅ 推荐，最简单，利用现有 GitHub 托管 |
| **S3 / R2** | `url: "https://...", s3: {...}` | 大量用户时节省 GitHub 带宽 |
| **自建服务器** | `url: "https://downloads.example.com/gloss"` | 完全控制 |

Gloss 选 **GitHub Releases** 即可。

---

## 7. 两种方式对比

| 维度 | updater.Plugin | 自定义 UI |
|------|---------------|-----------|
| 代码量 | ~3 行 | ~200+ 行 |
| UI 风格 | mygo 默认（Sparkle 风格） | 完全自定义 |
| 开发时间 | 10 分钟 | 1-2 天 |
| 与 Gloss 设计一致 | ❌ | ✅ |
| 设置面板集成 | 偏好 API 可用 | 完全控制 |
| 多语言 | 内置（含简中） | 需自己实现 |
| 自动检查 | 内置 | 需自己实现 |
| 进度条 | 内置 | 需自己实现 |
| 跳过版本 | 内置 | 需自己实现 |

---

## 8. 建议

### 第一阶段（快速上线）
用 **方式 A（updater.Plugin）**，10 分钟集成，先让更新能力跑起来。

### 第二阶段（体验优化）
如果觉得默认窗口风格不够 Gloss，再迁移到 **方式 B（自定义 UI）**：
- 在设置面板加"自动更新"开关（对接 `updater.AutomaticChecks()` / `SetAutomaticChecks()`）
- 在应用内加更新提示横幅
- 用 Gloss 的 design system 画更新窗口

---

## 9. 关键文件清单

| 文件 | 操作 |
|------|------|
| `mygo.json` → `mygo.config.ts` | 添加 `updates` 配置 |
| `main.go` | 添加 `mygo.Use(updater.Plugin)` 或自定义逻辑 |
| `CHANGELOG.md` | 新建，按 Keep a Changelog 格式 |
| `.github/workflows/release.yml` | 新建，CI 构建发布 |
| `mygo-update.key` | 生成后存入密码管理器 + GitHub Secrets |
| `mygo-update.pub` | 公钥写入 `mygo.config.ts` |
| `.gitignore` | 确保 `mygo-update.key` 不被提交 |

---

## 10. 风险与注意事项

1. **私钥安全**：丢失私钥 = 用户永远无法更新；私钥泄露 = 攻击者可以推送恶意代码
2. **GitHub API 限流**：未认证 60 次/小时，用 `GH_TOKEN` 可提升到 5000 次/小时
3. **macOS Gatekeeper**：不签名不公证 → 用户看到「已损坏」，需右键→打开。README 加指引
4. **Windows SmartScreen**：未签名的 exe 会触发蓝色警告窗口，用户点「仍要运行」即可
5. **delta 更新**：mygo 自动生成，但首次发布没有历史版本，只能全量下载
6. **版本号管理**：tag 必须与 `mygo.config.ts` 的 `version` 一致，否则 CI 应该报错
7. **Linux 安装位置**：必须安装在用户可写目录（`~/.local/...`），否则 `Enabled()` 返回 false
