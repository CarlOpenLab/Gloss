# diff-explainer

GitHub PR 风格的 diff 视图 + 每个代码块（hunk）的 **✨ AI 解释按钮**。
review 别人（或 AI）写的大量的代码时，哪里不懂点哪里，AI 用人话讲清这段增删做了什么、为什么、有没有坑。

## 功能

- **两种数据源**：粘贴 GitHub PR 链接（公开仓库免 token）/ 粘贴本地 `git diff` 输出
- **GitHub 风格 diff 视图**：红增绿删、双侧行号、语法高亮、文件树导航
- **逐块 AI 解释（悬浮卡片）**：每个 hunk 一个按钮，点开是贴着按钮弹出的浮窗（GitHub hovercard 那种），流式输出、不挤动 diff；卡片里直接勾选 review。一个文件有多个变更块时，prompt 会点名「当前是第几个块」并列出其它块的位置，文件上下文带真实行号，解释只讲这一个块
- **文件树侧边栏**（布局参考 [godiff](https://github.com/egoist/godiff)）：可折叠的目录树（只含一个子目录的目录链会合并成一行）、`Filter files` 模糊筛选（`/` 唤起）、每行右侧 `+N −N` 和状态字母、底部 `Total:` 统计；正文滚动时自动高亮当前文件
- **review 勾选**：每个 hunk 一个 checkbox，标记「这块我看过了」；文件头的 `已 review` 药丸一次勾掉整个文件并顺手折叠（等同 godiff 的 Viewed），文件树里已看完的文件名变灰、进行中的显示 `1/2`。顶栏显示总进度（`✓ 4/12`）。状态按 hunk 内容哈希存在 localStorage，刷新、重开、换 session 都还在
- **只看未 review + 快捷键**：勾上「只看未 review」自动滤掉已看过的块，剩多少一眼可见；按 `R` 直接标记当前正在读的那一块，`Esc` 关掉解释浮窗
- **多供应商配置**：内置 Anthropic / DeepSeek / Kimi(Moonshot) / 通义千问 / 智谱 GLM / OpenRouter / Ollama 预设，任何 OpenAI 兼容接口都可用；支持多配置切换、测试连接
- **结果缓存**：同一个代码块（按内容哈希 + 模型名）不重复调用模型

## 快速开始

```bash
# 需要 Go 1.23+ 和 Node 18+（前端包管理用 pnpm）

# 1. 构建前端
cd web && pnpm install && pnpm build && cd ..

# 2. 配置模型供应商（二选一）
#    a. 页面右上角「设置」→ 添加供应商（推荐，支持任意 OpenAI 兼容接口）
#    b. 环境变量回退（Anthropic）：
export ANTHROPIC_API_KEY=sk-ant-...
# 可选：export ANTHROPIC_MODEL=claude-sonnet-5-5   # 默认模型
# 可选：export ANTHROPIC_BASE_URL=https://...      # 中转/代理地址
# 可选：export GITHUB_TOKEN=ghp_...                # 私有仓库 / 提高限流

# 3. 构建并启动
go build -o diff-explainer . && ./diff-explainer
# 打开 http://localhost:8080
```

GitHub Token 在页面右上角「设置」里填（存浏览器 localStorage，通过请求头发给本地后端）。

## 供应商配置说明

| 类型 | 适用 | 说明 |
|---|---|---|
| Anthropic | Claude 官方 / 中转站 | Base URL 留空走官方；模型留空默认 `claude-sonnet-5-5` |
| OpenAI 兼容 | DeepSeek、Kimi、通义(DashScope 兼容模式)、智谱 GLM、OpenRouter、Ollama… | 走 `/chat/completions` SSE；Base URL 和模型必填 |

切换供应商后，已生成解释的代码块会重新生成（不同模型的解释分开缓存）。

## 前端开发模式

```bash
./diff-explainer &        # 后端 :8080
cd web && pnpm dev        # Vite 开发服 :5173，/api 已代理到 8080
```

## 架构

```
┌─ 数据源适配 ─────────────────────────────┐
│ GitHub PR → GitHub API (元数据+diff)     │
│ 本地粘贴 → git diff 文本                 │
└──────────┬───────────────────────────────┘
           ▼  internal/diff 统一解析
   FileDiff[] / Hunk[] / Line[]
           ▼
┌─ 前端 Vue3 ──────────────┐  ┌─ Go 后端 ─────────────────┐
│ 文件树 + diff 视图        │  │ POST /api/session/pr      │
│ 每 hunk ✨ 解释按钮       │─▶│ POST /api/session/diff    │
│ SSE 流式渲染解释          │◀─│ POST /api/explain (SSE)   │
└──────────────────────────┘  │  hunk+文件全文+PR描述 → LLM │
                              │  内存缓存 (hunk 内容哈希)   │
                              └────────────────────────────┘
```

- 后端零第三方依赖（仅 Anthropic 官方 Go SDK），单二进制内嵌前端
- 模型默认 `claude-sonnet-5-5`，解释类任务性价比最高

## 后续路线

- [ ] diff split 双栏视图
- [ ] 风险预扫：AI 预扫全量 hunk，可疑变更标红
- [ ] 单行级解释 / 选中追问
- [ ] 解释回写成 GitHub PR 评论
- [ ] 本地 CLI：`git diff | diff-explainer` 直接开浏览器
- [ ] 解释持久化缓存（SQLite）
