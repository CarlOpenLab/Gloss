---
version: 2.0.0
name: Gloss-Linear-Design-System
description: "A precision, software-craft Git GUI design system adapted from Linear (linear.app). Built on a near-black #010102 canvas with a strict four-step surface ladder and hairline borders. The accent is achromatic — an ink-white key on dark, a near-black key on light — so every hue on screen belongs to a semantic. Calibrates Git-specific domain semantics (diffs, commit topology, ref badges) into a quiet, noise-free, high-density professional desktop experience."

colors:
  # Accent —— 黑白两极，深色白键黑字 / 浅色黑键白字
  primary-dark: "#f7f8f8"
  primary-dark-hover: "#ffffff"
  primary-dark-pressed: "#d0d6e0"
  on-primary-dark: "#08090a"
  primary-light: "#0f1115"
  primary-light-hover: "#2b2f36"
  primary-light-pressed: "#000000"
  on-primary-light: "#ffffff"

  # Dark Canvas & Surface Ladder
  canvas: "#010102"
  sidebar-dark: "#0f1011" # 侧栏实色（= Surface 1，不透明）
  surface-1: "#0f1011"
  surface-2: "#141516"
  surface-3: "#18191a"
  surface-4: "#191a1b"

  # Hairline Borders
  hairline: "#23252a"
  hairline-strong: "#34343a"
  hairline-tertiary: "#3e3e44"

  # Text & Inks
  ink: "#f7f8f8"
  ink-muted: "#d0d6e0"
  ink-subtle: "#8a8f98"
  ink-tertiary: "#62666d"

  # Focus Ring & Text Selection —— 中性，不引入色相
  focus-dark: "rgba(247, 248, 248, 0.60)"
  focus-light: "rgba(15, 17, 21, 0.40)"
  selection-dark: "rgba(255, 255, 255, 0.20)"
  selection-light: "rgba(15, 17, 21, 0.12)"

  # Light Mode Equivalents (macOS Native Support)
  inverse-canvas: "#ffffff"
  sidebar-light: "#efeff0" # 侧栏实色：比主区白底暗一档，是「窗口」不是「内容」
  inverse-surface-1: "#f7f8f9"
  inverse-surface-2: "#f0f2f5"
  inverse-surface-3: "#e4e7eb"
  inverse-hairline: "#e1e4e8"
  inverse-ink: "#0f1115"
  inverse-ink-muted: "#4b535d"
  inverse-ink-subtle: "#6e7781"

  # Git Domain Semantics (Calibrated & De-saturated)
  semantic-diff-add-bg: "rgba(39, 166, 68, 0.09)"
  semantic-diff-add-gutter: "rgba(39, 166, 68, 0.14)"
  semantic-diff-add-bar: "#27a644"
  semantic-diff-del-bg: "rgba(229, 72, 77, 0.09)"
  semantic-diff-del-gutter: "rgba(229, 72, 77, 0.14)"
  semantic-diff-del-bar: "#e5484d"
  semantic-diff-hunk-bg: "#141516"
  semantic-diff-hunk-text: "#8a8f98"
  semantic-warning: "#e09f3e"
  semantic-danger: "#e5484d"

  # Commit Graph Lanes (Craft / Harmonized Palette) —— 8 条里只有 1 支蓝
  graph-lane-0: "#e6e6e6" # Ink (Main) —— 深色；浅色下取 #0f1115
  graph-lane-1: "#3eb489" # Sage Green
  graph-lane-2: "#e09f3e" # Warm Amber
  graph-lane-3: "#4c86e8" # Steel Blue —— 全图唯一的蓝
  graph-lane-4: "#e5484d" # Coral
  graph-lane-5: "#d977a8" # Dusty Rose
  graph-lane-6: "#38b2ac" # Muted Teal
  graph-lane-7: "#9aab45" # Moss —— 替掉原来的第二支蓝（#45b6fe）

rounded:
  xs: 4px
  sm: 6px
  md: 8px
  lg: 12px
  xl: 16px
  pill: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px

typography:
  display:
    fontSize: 20px
    fontWeight: 600
    letterSpacing: -0.4px
  headline:
    fontSize: 16px
    fontWeight: 600
    letterSpacing: -0.2px
  body:
    fontSize: 13px
    fontWeight: 400
    letterSpacing: -0.05px
  body-sm:
    fontSize: 12px
    fontWeight: 400
    letterSpacing: 0
  caption:
    fontSize: 11px
    fontWeight: 400
    letterSpacing: 0
  button:
    fontSize: 12.5px
    fontWeight: 500
    letterSpacing: -0.05px
  mono:
    fontSize: 12px
    fontWeight: 400
    letterSpacing: 0
---

# Gloss Design System (Linear.app Edition)

## 1. 核心设计哲学 (Design Philosophy)

Gloss 是一个面向工程师的现代 Git GUI 客户端。本项目的设计系统全面参考 **Linear (linear.app)** 的设计准则，核心原则包括：

1. **安静的奢华与软件工艺感 (Quiet Luxury & Software Craft)**
   界面永远是代码和提交历史的舞台，而非喧宾夺主的调色盘。消除不必要的高饱和度色块，追求像素级的对齐与微对比度。
2. **黑白强调原则 (Achromatic Accent)**
   **界面本身不含色相**：强调色是黑白两极的 Ink —— 深色下是白键黑字（`#f7f8f8` / `#08090a`），浅色下是黑键白字（`#0f1115` / `#ffffff`）。它仅用于核心 CTA 按钮、键盘焦点环、当前分支圆点与关键选定态，绝不用作大面积装饰性背景。
3. **明度阶梯代替阴影 (Surface Ladder over Drop Shadows)**
   采用严格的四级表面提升系统（Canvas → Surface 1 → Surface 2 → Surface 3 → Surface 4），依靠明度微调和 1px 发丝边框表达层级深度，暗色模式下不使用粗重投影。
4. **领域降噪 (Domain Noise Reduction for Git)**
   Git 固有的 Diff（红/绿）与分支拓扑图（多线并存）经过低饱和度沉浸式标定。消除刺眼的亮蓝 Hunk 色带与彩虹糖果式徽章，保证信息高密度且舒适耐看。

---

## 2. 颜色体系 (Color System)

### 2.1 画布与表面阶梯 (Canvas & Surfaces)
- **Canvas (`#010102`)**：系统基底画布，极致深邃，带微不可察的冷青/蓝偏调。
- **Surface 1 (`#0f1011`)**：一级提升面，用于侧边栏、代码主区背景、提交消息面板、主要容器。
- **Surface 2 (`#141516`)**：二级提升面，用于卡片容器、Diff Hunk 栏（`@@`）、列表项悬停态、徽章底色。
- **Surface 3 (`#18191a`)**：三级提升面，用于浮层、下拉菜单（Dropdown Popovers）、右键菜单。
- **Surface 4 (`#191a1b`)**：四级提升面，用于模态对话框（Dialog Modal Cards）、设置窗口顶层面板。
- **侧栏与代码主区都落在 Surface 1**：两者同色、同材质、**不透明**，中间用 1px Hairline 分界，Canvas 只从窗口边缘和面板缝隙里露出来。层次交给明度阶梯，不交给半透明 —— 侧栏曾经铺的是 `#08090a` 再乘 `α(0.55)`（配合窗口 Vibrant 毛玻璃），明度差被那个 α 吃掉一多半，深色下糊进近黑画布、浅色下糊进白画布，而且最终颜色由桌面壁纸决定：同一套配色在不同机器上是两个样子。

### 2.2 发丝边框 (Hairline Borders)
- **Hairline (`#23252a`)**：基础 1px 细线，用于面板分割、卡片边框、表格横线、不抢视线。
- **Hairline Strong (`#34343a`)**：增强 1px 细线，用于输入框外框、激活状态卡片、分割重点。
- **Hairline Tertiary (`#3e3e44`)**：次级强化线，用于悬停态边框与弹窗外围。

### 2.3 文本与灰阶 (Ink Hierarchy)
- **Ink (`#f7f8f8`)**：高亮标题、代码高亮基准正文、当前聚焦项。
- **Ink Muted (`#d0d6e0`)**：常规正文、提交标题、文件名、常规按钮文本。
- **Ink Subtle (`#8a8f98`)**：代码行号、辅助元信息、提交作者与相对时间、未激活图标。
- **Ink Tertiary (`#62666d`)**：快捷键提示文本、占位符（Placeholder）、禁用状态。

### 2.4 强调色与交互态 (Accent & Interaction)
强调是**明度**的两极，不是色相。深浅两套互为反相，同一颗按钮在两边都保持最高对比：

- **深色模式**：Primary `#f7f8f8`（Ink）/ Hover `#ffffff` / Pressed `#d0d6e0`（Ink Muted），文字 `#08090a`。
- **浅色模式**：Primary `#0f1115`（Ink）/ Hover `#2b2f36` / Pressed `#000000`，文字 `#ffffff`。
- 用于主提交按钮、当前工作区激活项、主分支节点（拓扑图 Lane 0）、侧栏导航指示条。
- **焦点环**：中性 —— 深色 `rgba(247, 248, 248, 0.60)`，浅色 `rgba(15, 17, 21, 0.40)`，2px 发丝外圈。
- **文本选中**：中性 —— 深色 `rgba(255, 255, 255, 0.20)`，浅色 `rgba(15, 17, 21, 0.12)`。

### 2.5 Git 领域语义色（降噪校准）
针对此前“花花绿绿”的核心改造：
- **新增代码 (Diff Added)**：
  - 行底色：`rgba(39, 166, 68, 0.09)`（极淡半透明暗苔绿，不糊住代码）。
  - 行号栏底色：`rgba(39, 166, 68, 0.14)`。
  - 左侧标识条：`#27a644`（2px 极细竖条）。
- **删除代码 (Diff Deleted)**：
  - 行底色：`rgba(229, 72, 77, 0.09)`（极淡半透明暗绯红）。
  - 行号栏底色：`rgba(229, 72, 77, 0.14)`。
  - 左侧标识条：`#e5484d`（2px 极细竖条）。
- **代码段 Hunk (@@ -a,b +c,d @@)**：
  - **彻底摒弃旧版亮蓝带**：底色统一采用 **Surface 2 (`#141516`)**，搭配 1px Hairline (`#23252a`) 边框。
  - 文本采用 `ink-subtle` (`#8a8f98`)，等宽字体显示。
  - Hunk 右侧操作按钮（如暂存 / 取消暂存）采用静音幽灵按钮。
- **分支与引用徽章 (Ref Badges)**：
  - 统一采用 **Surface 2 (`#141516`)** 底色 + 1px **Hairline (`#23252a`)** 边框 + **Ink Muted (`#d0d6e0`)** 文字。
  - 区别通过微型矢量图标（`iconGitBranch`, `iconGitCommit`, `iconTag`）或 6px 小圆点体现，不再整块涂成琥珀黄或亮蓝。
  - 那颗 6px 圆点是**唯一**带色的地方，四类引用各一支（小标签亮色，见下）：**HEAD = Ink**（跟主干、「我在哪」同一色，全屏少数几处实心强调之一）、本地分支 = 钢青蓝、远程跟踪 = 暗青、标签 = 暖琥珀。原来本地分支与 HEAD 同色，一列芯片上全是白点。
- **小标签亮色 (Label Accents)** —— 界面整体灰阶 + 黑白强调，这一组是仅有的例外，只喂给界面上最小的两处：文件列表里 10px 的**状态字母**与上面那颗 6px **圆点**。灰阶落在一个 10px 的字上分不出层级，「M 和 D 都是灰的」等于让读的人把每个字母读一遍。
  - 状态字母：`A`（新增/未跟踪）= diff 的绿 `#27a644`，`D` / `!`（删除/冲突）= diff 的红 `#e5484d`，`M`（改动）= **暖琥珀**（深色 `#e09f3e` / 浅色 `#d97706`），`R` / `C`（重命名/复制）= **钢青蓝**（`#4c86e8` / `#2f6fd0`）。
  - 三支色**借自拓扑图的 8 色谱系**（§2.6），不另配一套：图里那条蓝线和列表里这个 `R`、芯片上这颗本地分支点，说的是同一件事、同一种蓝。改那套色板时这里跟着变。
  - A/D 不另取绿红：一行里出现两个相近的绿，比没有绿更糟。
  - **琥珀只有一支**：`M` 的字母、标签芯片的点、告警圆点用的是同一支 `warning`（浅色 `#d97706`）。它和拓扑图 2 号线的「暖琥珀沙」浅色值 `#c9822b` 不是同一支 —— 图表色板按八条线的明度均衡选色，不跟界面的语义色混用；深色下两者恰好同值（`#e09f3e`）。
- **语义三色只有一个定义处**：`success` / `warning` / `danger` 在 `glossTheme` 里定义（框架自绘的控件、toast 也读那一份），`palette.success/warning/danger` 只是把同一份搬进调色板。主包一律写 `pal.*`，不直接写 `t.Danger` —— 同一种红在代码里有两个名字，评审时没人能一眼看出它们是不是同一个值。风险预检的严重度圆点、PR 的成败横幅都走这三个。
- **引用类微件一律不用 Ink**：PR 编号芯片的圆点与本地分支同一支钢青蓝。Ink 只留给 HEAD /「当前分支」/ 主干（§2.4），每多一处实心强调，全屏的强调预算就少一格。
- **查找命中 (Find Hits)**：
  - **半透明琥珀**，而不是实色块：它要叠在普通行、新增行、删除行三种底色上，透出来的底才能保住「这一行还是绿的 / 红的」，实色会把增删信息整个盖掉。
  - 全部命中：深色 `rgba(224, 159, 62, 0.26)`，浅色 `rgba(217, 119, 6, 0.20)`。
  - 当前命中更重一档（深色 `0.55` / 浅色 `0.42`），另加一道同色系下划线（深色 `#e09f3e` / 浅色 `#b45309`）—— 连按「下一个」时只靠深浅不够快，视线需要一道能立刻钉住的东西。
  - 不用 `WavyUnderline`：12.5px 等宽字体上会糊成一条抖动的线。

### 2.6 拓扑图色彩 (Commit Graph Lanes)
摒弃 8 种高饱和荧光彩虹色，采用经过亮度均衡的“莫兰迪 / 精密仪器”8 色谱系。**0 号留给主干**，取中性 Ink（深色 `#e6e6e6`、浅色 `#0f1115`）—— 主线是整张图上最亮/最重的一条，侧栏「当前分支」圆点与提交徽章上的本地分支点也读它；彩色只落在其余 7 条支线上，且**整份色板不含紫**：

1. **Lane 0 (主干 / 主分支)**: `#e6e6e6` (Ink，浅色下 `#0f1115`)
2. **Lane 1**: `#3eb489` (鼠尾草绿)
3. **Lane 2**: `#e09f3e` (暖琥珀沙)
4. **Lane 3**: `#4c86e8` (钢青蓝) —— **全图唯一的蓝**
5. **Lane 4**: `#e5484d` (暗珊瑚红)
6. **Lane 5**: `#d977a8` (暮灰粉)
7. **Lane 6**: `#38b2ac` (暗松石绿)
8. **Lane 7**: `#9aab45` (苔绿)

两条硬约束（`theme_test.go` 各有一条断言守着）：

- **主干必须是中性 Ink**，且是整张图上最亮（浅色下最重）的一条。主干色同时是「我在哪条分支」的圆点、提交行里本地分支芯片上那颗 6px 点、提交栏状态胶囊 —— 它一变色，这几处会跟着一起变。
- **全图只有一支蓝**。三支蓝并排在屏上时，整张图会被读成「一条蓝色渐变 + 几个别的色」，支线之间分不出彼此，图就白画了；`#58a6ff` / `#45b6fe` 那一系正是这么被换掉的。

**这套色是默认值，不是唯一值**（`theme_test.go` 的 `TestLanePresetsKeepDesignRules`
与 `TestClassicLanesMatchDesignSpec` 守着）。设置 → 通用偏好里的「拓扑图配色」
给三套内置方案加逐号覆盖，存的是 `graphPreset` / `graphLanesDark` / `graphLanesLight`：

| 方案 id | 名字 | 什么时候用 |
| :--- | :--- | :--- |
| `classic` | 经典 | 出厂值，就是上面这一份莫兰迪 8 色（深浅各一套） |
| `vivid` | 鲜艳 | 投影、外接屏、屏幕反光的房间：同一套色相、彩度拉高一档 |
| `mono` | 灰阶 | 打印、色觉辅助、只想看结构：八阶明度，不靠色相分辨 |

三条约定：

- **自定义只覆盖，默认不给改**。方案名认不出来（旧配置、手抄错）一律落回 `classic`；覆盖里写坏的条目只影响它自己那一号，不让整张图失色。
- **深浅分开存**。同一支色落在 `#010102` 与白底上的可读性差得很远，而「随系统」的人两个外观都会看到，所以两套覆盖各存一份，设置页两排色井都摆出来。
- **覆盖只管拓扑图**。0 号那几处复用（当前分支圆点、本地分支点、状态胶囊）跟着 0 号走，那是「主干是谁」该有的连带；§2.5 的小标签亮色（`accentAmber` / `accentBlue` / `accentTeal`）不进覆盖，它们是状态字母与芯片圆点的固定语义色。三套方案同样守着上面两条硬约束，也都不含紫 / 鸢尾那一带的色（色相 260°~320°，`TestLanePresetsKeepDesignRules` 逐支查）；再往蓝偏的靛由「只有一支蓝」挡掉。

---

## 3. 圆角与形态系统 (Shapes & Radii)

| Token | 像素值 | 适用组件 |
| :--- | :--- | :--- |
| `rounded.xs` | `4px` | 极小代码标签、文件树勾选框、微型图标底板 |
| `rounded.sm` | `6px` | 工具栏分支芯片、树节点选中底带、分段控制器单元 |
| `rounded.md` | `8px` | **全部通用按钮**、文本输入框、下拉框、搜索栏 |
| `rounded.lg` | `12px` | 面板卡片、悬浮卡片、下拉浮层 |
| `rounded.xl` | `16px` | 模态弹窗（Settings, PR, Branch 对话框） |
| `rounded.pill`| `9999px` | 数量计数器（Count Badges）、状态圆角药丸 |

---

## 4. 组件规范 (Component Specifications)

### 4.1 按钮 (Buttons)
- **Primary Button (主操作键)**:
  - 深色：底色 `#f7f8f8`，悬停 `#ffffff`，按下 `#d0d6e0`；文字 `#08090a`
  - 浅色：底色 `#0f1115`，悬停 `#2b2f36`，按下 `#000000`；文字 `#ffffff`
  - 字号：12.5px，粗细：Medium (500)
  - 边框：无，圆角：`8px`，内边距：`6px 14px`
- **Secondary Button (次级操作键)**:
  - 底色：`#0f1011` (Surface 1)，悬停：`#141516` (Surface 2)
  - 文字：`#d0d6e0` (Ink Muted)
  - 边框：1px `#23252a` (Hairline)，圆角：`8px`，内边距：`6px 14px`
- **Ghost / Tool Button (图标工具键)**:
  - 底色：透明，悬停：`#141516` (Surface 2)
  - 图标颜色：`#8a8f98` (Ink Subtle)，悬停时提升为 `#f7f8f8` (Ink)
  - 圆角：`6px`

### 4.2 工具栏与分支芯片 (Toolbar & Branch Chip)
- **工具栏容器**：高度 52px，底色沉浸在 `#010102` (Canvas)，底边框 1px `#23252a`。
- **视图分段控件（改动 / 历史 / 终端 / 编辑器）**：图标四格一条，「现在看哪一块」的四个选择只在这一处。改动与历史原先长在侧栏顶部，和这里的终端 / 编辑器各说一半；并成一条之后 ⌘1~⌘4 依次对应四格，一个键一种意思。前两格要有仓库才点得动（置灰的是单格，不是整条），后两格不必。选中态：`Surface 3` 底 + `hairline-strong` 边 + 图标提到 Ink。
- **分支芯片 (Branch Chip)**：
  - 摒弃旧版琥珀黄色块，改用高质感深灰芯片：
  - 底色：`#0f1011` (Surface 1)，边框：1px `#23252a` (Hairline)，圆角：`6px`。
  - 图标与文字：`#d0d6e0` (Ink Muted)；鼠标悬停时底色提升为 `#141516`，边框提升为 `#34343a`。
  - 异常状态（如 Rebase 中或 Detached HEAD）仅使用微弱的 Warning 色文字做点缀提示。

### 4.3 侧边栏 (Sidebar Navigation)
- 底色：深色 `#0f1011`（Surface 1）、浅色 `#efeff0`；**实色不透明**（顶栏项目标签栏同色同材质）。
  半透明材质已废弃：`Alpha(0.55)` 会把侧栏与画布的明度差（本就只有 7~8 阶）再乘掉一多半，深色下糊进 `#010102` 画布、浅色下糊进白画布，且最终颜色由桌面壁纸决定。
- 分割线：1px `#23252a`。
- 侧栏只放分支 / 远程 / 标签 / 储藏树（顶部一枚分支筛选框）。「改动 / 历史」导航已移入工具栏的四格分段（§4.2）—— 「现在看哪一块」的选择在屏幕两端各摆一份，读起来像两个东西。

### 4.4 提交栏 (Commit Box)
- 放置于主区底部，底色：`#0f1011` (Surface 1)，顶边框：1px `#23252a`。
- 输入框：底色 `#141516` (Surface 2)，边框 1px `#23252a`，聚焦时换成 2px 中性焦点环（§2.4）。
- 提交按钮：Primary Button 规格（深色白键黑字 / 浅色黑键白字）。

### 4.5 顶栏与项目标签页 (Title Bar & Project Tabs)
- **项目标签页**：窗口顶栏承载多项目标签，每个标签只报「名字 · 待提交圆点」。分支名不上标签（它归工具栏的分支徽章与侧栏的分支树）—— 并排几个项目时，标签只回答「这是哪个项目」。悬停提示给完整仓库路径。
- **标签激活态不换几何**：激活只换颜色 —— `Surface 2` 底 + `hairline-strong` 边 + 文字提到 Ink（未激活为 Ink Muted）；边框三态一律 1px（只在透明与可见之间换色），字重不变。semibold 比 regular 宽一个多百分点，激活一次，这个框就胖一点，右边所有标签连同 [+] 一起挪 —— 点一下、晃一下。
- **红绿灯安全边距**：标签从窗口左沿 `红绿灯右沿 + 18px` 起排（全屏、红绿灯隐藏时顶栏自己留 12px）。贴着按钮排（8px 以内）时，标签的圆角底色和窗口按钮会连成一片。
- **项目名只出现一次**：当前项目的名字归顶栏标签，侧栏顶部不再另摆一张仓库卡片 —— 同一个名字在一屏里出现两遍，读起来像两个东西，而不是两处强调。
- **路径不占常驻位置**：项目标签与工具栏那块仓库名都不显示路径（侧栏收起时的下拉块只报「仓库名 + 分支」，不再附一行压短的路径）。要看完整路径：悬停项目标签、最近项目下拉里的每条（侧栏收起时的工具栏下拉、菜单栏「文件 → 最近打开的项目」）、或命令面板（⌘K）。
- **拖着换位置**：按住一枚标签左右拖，指针落在目标标签左/右半边就是插到它前/后面，落点处画一道 2px 插入提示线。拖的是顺序不是选择 —— 活动标签跟着自己走（按指针认回来，不按老下标现算）；新顺序落进 store，下一次启动看到的就是拖完的样子。提示线上下各探出标签一框：框架的拖动副本（0.75 不透明度、画在整棵元素树之后）正好盖住标签本身这一框，探出框外才看得见。

### 4.6 终端会话芯片带 (Terminal Session Chips)
- **一枚芯片一个会话**：终端视图顶上一条 34px 的芯片带（芯片 28px 高，和项目标签同规格）。芯片写 shell 自己报上来的标题（OSC 0/2），不报就回落到「终端 N」—— 一屏两个 shell 都叫「终端」，认不出哪一个是刚在跑构建的那个。编号按创建顺序发：关掉中间一个，剩下的不该集体改名。
- **激活态只换颜色**：激活是 `Surface 2` 底 + `hairline-strong` 边 + Ink 文字，未激活是透明底 + `hairline` 边 + Ink Subtle 文字（悬停提一档到 `Surface 1`）。边框两态都是 1px，内边距、字号、字重一律不动 —— 见 §4.5 同一条理由：切一次会话整条芯片带跟着重排，第二下就点不准了。
- **× 常驻，不悬停才长出来**：一颗只在悬停时出现的 × 会让芯片宽度在指针扫过时变来变去，右边所有芯片跟着挪。
- **拖着换位置**：和项目标签页同一套（§4.5）—— 按住一枚芯片拖到另一枚的左/右半边完成插入，当前展示的会话跟着自己走。落点提示线同样上下探出芯片一框，理由相同。
- **关掉最后一个自动补一个**：能点那一下的人手上正在用终端，他要的是「这个 shell 不要了」，不是「关掉终端」这个功能。
- **带子底取 `appBg`、正文取 `codeBg`**，两者之间压 1px `hairline`：这两块面只差一档明度，不画线就是糊在一起的一片。
- **字体与字号可调**（设置 → 通用偏好 → 内置终端）：字号 9~28 DIP（0 表示用插件默认 13）；字体族是**下拉选一个常见等宽字体**（`termFontOptions`，不去扫系统字体 —— macOS 上扫一遍要么跑 `system_profiler`（实测 14 秒）要么自己解析上百个字体文件，为一个下拉不值当），手写配置文件里填逗号候选表也认。改动**推给已经开着的会话**，不重启 shell —— 重启会把屏幕上的输出和正在跑的命令一起丢掉，为了换个字号不值当；起新会话时则直接带着当前字体（`terminal.Options.Font`）。
- **字体族尾巴上永远挂一个 `monospace`**（`fontFamilyWithFallback`）：下拉里那些「这台机器可能没装」的字体靠它退到系统等宽。这个尾巴是量出来的 —— 同一个 1000px 宽的终端，空 / `monospace` / `Menlo` 都是 87 列，而不存在的字体族**没有兜底时只有 53 列**（引擎换了一支比例字体），终端会错行。

---

## 5. 设计守则：Do's & Don'ts (Strict Anti-Noise Rules)

### Do:
- ✅ **拥抱深度黑与微阶梯**：以 `#010102` 画布为基底，让层次自然浮现在 `Surface 1~4` 上。
- ✅ **克制使用强调白**：全屏幕范围内，同时出现的实心强调（白键 / 指示条 / 当前分支圆点）尽量不超过 1~2 处（核心动作焦点）。
- ✅ **彩度让给语义**：界面自身只用灰阶，色相留给 diff 红绿、告警琥珀、拓扑支线 —— 彩色出现时，它一定在说一件事。
- ✅ **用发丝线组织空间**：通过 1px `#23252a` 细线划定边界，而不是用厚重阴影或粗边框。
- ✅ **用文本明度传达信息权重**：主要信息用 `#f7f8f8`，次要信息坚决降至 `#8a8f98`，避免到处都是亮白或彩色。
- ✅ **保持 Diff 视图平静**：增删代码底色半透明度保持在 8~10% 以内，Hunk 栏坚决保持中性灰。

### Don't:
- ❌ **禁止彩色药丸大爆炸**：不要在分支、远程、标签、状态等同一行里涂抹多种高饱和度色块。
- ❌ **禁止使用亮蓝 @@ Hunk 栏**：不再使用 `#121d2f` 底色或 `#79c0ff` 荧光文字。
- ❌ **禁止使用霓虹彩虹拓扑线**：提交图颜色必须保持灰度与明度均衡。
- ❌ **禁止给常规按钮使用大圆角胶囊 (Pill)**：按钮统一为 `8px` 圆角，仅计数角标使用 Pill。
- ❌ **禁止品牌紫 / 薰衣草蓝**：`#5e6ad2` 那一系（以及任何紫、鸢尾、靛）不再出现在界面与拓扑图里 —— 强调靠明度，不靠色相。
- ❌ **禁止非语义彩色修饰**：不要给卡片、侧栏条目增加花哨的环境渐变或外发光。
- ❌ **禁止用几何表达状态**：悬停 / 激活 / 选中只许改颜色，不许改字重、字号、内边距或边框宽度 —— 状态每变一次，布局就跟着抖一下（标签页那条 semibold 就是这么抖的）。
- ❌ **禁止给侧栏 / 顶栏铺半透明材质**：`Color.Alpha(0.3~0.6)` 这类「让系统毛玻璃透上来」的写法，等于把这两块面的最终颜色交给桌面壁纸。侧栏与画布的明度差只有 7~8 阶，乘一次 α 就只剩 3~4 阶 —— 深色下糊进近黑画布、浅色下糊进白画布，同一套配色在不同机器上不是一个样子。层次用明度阶梯和发丝线表达。
