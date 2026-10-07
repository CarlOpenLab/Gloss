# Gloss

原生 macOS 的 Git 客户端，**每个变更块（hunk）都有一个 ✨ AI 解释按钮**。
review 别人（或 AI）写的大量的代码时，哪里不懂点哪里，AI 用人话讲清这段增删做了什么、为什么、有没有坑。

用 [mygo](https://github.com/egoist/mygo) 写的原生界面：GPU 直接绘制，没有 webview、没有 JS，单二进制。

## 功能

- **本地 git 仓库**：打开一个工作副本，看未提交的改动；侧栏列着分支 / 远程 / 标签 / 储藏，点一条分支就只看它的历史，点一条储藏直接看它的改动
- **GitHub 风格 diff 视图**：红增绿删、双侧行号、语法高亮（chroma）、文件树导航
- **逐块 AI 解释（悬浮卡片）**：每个 hunk 一个 ✨ 按钮，点开是贴着按钮弹出的浮窗，流式输出、不挤动 diff。一个文件有多个变更块时，prompt 会点名「当前是第几个块」并列出其它块的位置，文件上下文带真实行号，解释只讲这一个块。失败时不甩一坨 HTTP 错误：说清是什么事、怎么办（密钥不对直接给一条「模型设置…」），供应商的原话留着可查
- **Fork 式的上下分**：主区上半是提交列表（左边一条自己算的提交拓扑图），下半左边是文件列表、右边是选中提交的 diff；点文件列表里的一行，diff 直接滚到那个文件的头上
- **文件树**：可折叠的目录树（只含一个子目录的目录链合并成一行）、模糊筛选（`⌘F`）、每行右侧 `+N −N` 和状态字母、底部一条常驻栏脚写着总计和已 review 进度
- **Viewed 联动折叠**：标了 `已 review` 的文件在 diff 里折成一条文件头，一屏就能扫完剩下的改动；内容一变标记自动失效。点文件头可以手动折叠/展开
- **命令栏（`⌘K`）**：所有功能都能键盘调到
- **多供应商配置**：内置 Anthropic / DeepSeek / Kimi(Moonshot) / 通义千问 / 智谱 GLM / OpenRouter / Ollama 预设，任何 OpenAI 兼容接口都可用；也支持 `ANTHROPIC_API_KEY` 环境变量回退
- **结果缓存与持久化**：同一个变更块（按内容哈希 + 模型名）不重复调用模型；解释、已 review 状态、窗口设置都存在 `~/Library/Application Support/Gloss/`

## 快捷键

| 键 | 作用 |
|---|---|
| `⌘K` | 命令栏 |
| `J` / `K` | 跳到下一段 / 上一段改动 |
| `⌘F` | 聚焦文件筛选框 |
| `⌘O` | 打开仓库 |
| `⌘1` / `⌘2` | 改动 / 历史 |
| `⌘⇧B` | 收起 / 展开侧栏 |
| `⌘R` | 刷新 |
| `⌘+` / `⌘−` / `⌘0` | 代码字号放大 / 缩小 / 恢复默认 |
| `⌘,` | 模型设置 |
| `Esc` | 关掉解释卡片 |

菜单栏（`menu.go`）里每一项都带着同一个键：真实 macOS 应用里原生菜单会
先把键吃掉，视图那条路走不到，两处的键必须一致。

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
`~/Library/Application Support/Gloss/config.json`（权限 0600）。
没配的话回退到环境变量：

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# 可选：export ANTHROPIC_MODEL=claude-sonnet-5-5
# 可选：export ANTHROPIC_BASE_URL=https://...   # 中转/代理地址
```

## 架构

```
main.go / app.go         窗口、菜单、app 状态、每帧的根布局
  sidebar.go             侧栏：分支 / 远程 / 标签 / 储藏树
  history.go             上半区：提交列表 + 列头
  graph.go               提交拓扑图：布局（buildGraph）与逐行绘制
  filepane.go            下半区左边：文件列表
  diffview.go            diff 渲染、语法高亮、折叠
  treeview.go            两棵树共用的行画法（缩进 / 箭头 / 选中胶囊）
  refs.go                引用树的分组与筛选
  rows.go                扁平行模型（虚拟化列表的数据源）
  explain.go             ✨ 卡片：定位、流式节流、缓存
  palette.go / keys.go   命令栏、快捷键
  menu.go                菜单栏：同样那批命令，键和 keys.go 一一对应
  settings.go            模型设置对话框
  theme.go               配色（对齐 GitHub 那套）

internal/
  gitrepo/               git CLI 封装（status / diff / log / refs）
  diff/                  unified diff 解析
  highlight/             chroma：整文件 tokenize 后按行切片
  explain/               prompt 构建、上下文窗口（纯函数，不碰网络）
  llm/                   Anthropic + OpenAI 兼容接口的流式客户端
    failure.go           把供应商的错误翻成人话（说什么事 / 怎么办 / 原话留着）
  store/                 config / state / cache 持久化
  filetree/              树的构建、摊平与模糊筛选（侧栏和文件列表共用）
```

线程约定写在 `app.go` 顶部：只有主线程读写 app 的字段，后台 goroutine
只收值参数、结果一律走 `dispatch` 回主线程。

## 后续路线

- [ ] 暂存 + 提交（stage / unstage / commit）
- [ ] 分支的右键菜单：checkout / merge / rebase
- [ ] 储藏 apply / pop，侧栏可拖宽
- [ ] 提交列表的键盘上下选择（`J` / `K` 现在只在 diff 里跳）
- [ ] GitHub PR：直接看 PR 的 diff、把解释回写成评论
- [ ] 风险预扫：AI 预扫全量 hunk，可疑变更标红
- [ ] 单行级解释 / 选中追问
