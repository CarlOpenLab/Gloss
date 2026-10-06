# diff-explainer

原生 macOS 的 diff review 工具，**每个变更块（hunk）都有一个 ✨ AI 解释按钮**。
review 别人（或 AI）写的大量的代码时，哪里不懂点哪里，AI 用人话讲清这段增删做了什么、为什么、有没有坑。

用 [mygo](https://github.com/egoist/mygo) 写的原生界面：GPU 直接绘制，没有 webview、没有 JS，单二进制。

## 功能

- **本地 git 仓库**：打开一个工作副本，看未提交的改动；也可以在「历史」tab 里翻任意提交的 diff
- **GitHub 风格 diff 视图**：红增绿删、双侧行号、语法高亮（chroma）、文件树导航
- **逐块 AI 解释（悬浮卡片）**：每个 hunk 一个 ✨ 按钮，点开是贴着按钮弹出的浮窗，流式输出、不挤动 diff。一个文件有多个变更块时，prompt 会点名「当前是第几个块」并列出其它块的位置，文件上下文带真实行号，解释只讲这一个块
- **文件树侧栏**：可折叠的目录树（只含一个子目录的目录链合并成一行）、模糊筛选（`⌘F`）、每行右侧 `+N −N` 和状态字母、底部 `Total:` 统计
- **Viewed 联动折叠**：标了 `已 review` 的文件在 diff 里折成一条文件头，一屏就能扫完剩下的改动；内容一变标记自动失效。点文件头可以手动折叠/展开
- **命令栏（`⌘K`）**：所有功能都能键盘调到
- **多供应商配置**：内置 Anthropic / DeepSeek / Kimi(Moonshot) / 通义千问 / 智谱 GLM / OpenRouter / Ollama 预设，任何 OpenAI 兼容接口都可用；也支持 `ANTHROPIC_API_KEY` 环境变量回退
- **结果缓存与持久化**：同一个变更块（按内容哈希 + 模型名）不重复调用模型；解释、已 review 状态、窗口设置都存在 `~/Library/Application Support/Diff Explainer/`

## 快捷键

| 键 | 作用 |
|---|---|
| `⌘K` | 命令栏 |
| `J` / `K` | 跳到下一段 / 上一段改动 |
| `⌘F` | 聚焦文件筛选框 |
| `⌘1` / `⌘2` | 改动 / 历史 |
| `⌘⇧B` | 收起 / 展开侧栏 |
| `⌘R` | 刷新 |
| `⌘+` / `⌘−` / `⌘0` | 代码字号放大 / 缩小 / 恢复默认 |
| `Esc` | 关掉解释卡片 |

## 构建

需要 Go 1.27+（mygo 的硬要求，`mise.toml` 里已经 pin 好了）。

```bash
mise install                 # 装 Go 1.27.1
go tool mygo dev             # 开发模式，改完自动重启
go tool mygo build           # 出 .app 和 .dmg（build/darwin-arm64/）
go test ./...                # 测试（界面是无头的，不需要开窗口）
```

## 配置模型

界面右上角「模型设置…」里加一个供应商即可。API key 存在
`~/Library/Application Support/Diff Explainer/config.json`（权限 0600）。
没配的话回退到环境变量：

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# 可选：export ANTHROPIC_MODEL=claude-sonnet-5-5
# 可选：export ANTHROPIC_BASE_URL=https://...   # 中转/代理地址
```

## 架构

```
main.go / app.go         窗口、菜单、app 状态、每帧的根布局
  sidebar.go             文件树 / 提交列表
  diffview.go            diff 渲染、语法高亮、折叠
  rows.go                扁平行模型（虚拟化列表的数据源）
  explain.go             ✨ 卡片：定位、流式节流、缓存
  history.go             历史 tab
  palette.go / keys.go   命令栏、快捷键
  settings.go            模型设置对话框
  theme.go               配色（对齐 GitHub 那套）

internal/
  gitrepo/               git CLI 封装（status / diff / log）
  diff/                  unified diff 解析
  highlight/             chroma：整文件 tokenize 后按行切片
  explain/               prompt 构建、上下文窗口（纯函数，不碰网络）
  llm/                   Anthropic + OpenAI 兼容接口的流式客户端
  store/                 config / state / cache 持久化
  filetree/              侧栏树的构建与模糊筛选
```

线程约定写在 `app.go` 顶部：只有主线程读写 app 的字段，后台 goroutine
只收值参数、结果一律走 `dispatch` 回主线程。

## 后续路线

- [ ] 暂存 + 提交（stage / unstage / commit）
- [ ] GitHub PR：直接看 PR 的 diff、把解释回写成评论
- [ ] 风险预扫：AI 预扫全量 hunk，可疑变更标红
- [ ] 单行级解释 / 选中追问
