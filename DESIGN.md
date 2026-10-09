---
version: 1.0.0
name: Gloss-Linear-Design-System
description: "A precision, software-craft Git GUI design system adapted from Linear (linear.app). Built on a near-black #010102 canvas with a strict four-step surface ladder, hairline borders, and the signature Linear lavender-blue (#5e6ad2) as the single chromatic accent. Calibrates Git-specific domain semantics (diffs, commit topology, ref badges) into a quiet, noise-free, high-density professional desktop experience."

colors:
  # Brand & Accent
  primary: "#5e6ad2"
  primary-hover: "#828fff"
  primary-focus: "#5e69d1"
  on-primary: "#ffffff"

  # Dark Canvas & Surface Ladder
  canvas: "#010102"
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

  # Light Mode Equivalents (macOS Native Support)
  inverse-canvas: "#ffffff"
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

  # Commit Graph Lanes (Craft / Harmonized Palette)
  graph-lane-0: "#5e6ad2" # Lavender (Primary / Main)
  graph-lane-1: "#3eb489" # Sage Green
  graph-lane-2: "#e09f3e" # Warm Amber
  graph-lane-3: "#9d8df1" # Soft Iris
  graph-lane-4: "#e5484d" # Coral
  graph-lane-5: "#45b6fe" # Sky
  graph-lane-6: "#d977a8" # Dusty Rose
  graph-lane-7: "#38b2ac" # Muted Teal

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
2. **单主色原则 (Single Chromatic Accent)**
   **Linear 薰衣草蓝 (`#5e6ad2`) 是全局唯一的品牌与主交互色**。它仅用于核心 CTA 按钮、键盘焦点环、主品牌标识与关键选定态，绝不用作大面积装饰性背景。
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

### 2.2 发丝边框 (Hairline Borders)
- **Hairline (`#23252a`)**：基础 1px 细线，用于面板分割、卡片边框、表格横线、不抢视线。
- **Hairline Strong (`#34343a`)**：增强 1px 细线，用于输入框外框、激活状态卡片、分割重点。
- **Hairline Tertiary (`#3e3e44`)**：次级强化线，用于悬停态边框与弹窗外围。

### 2.3 文本与灰阶 (Ink Hierarchy)
- **Ink (`#f7f8f8`)**：高亮标题、代码高亮基准正文、当前聚焦项。
- **Ink Muted (`#d0d6e0`)**：常规正文、提交标题、文件名、常规按钮文本。
- **Ink Subtle (`#8a8f98`)**：代码行号、辅助元信息、提交作者与相对时间、未激活图标。
- **Ink Tertiary (`#62666d`)**：快捷键提示文本、占位符（Placeholder）、禁用状态。

### 2.4 唯一主色与交互态 (Primary Accent)
- **Primary (`#5e6ad2`)**：Linear 经典薰衣草蓝。用于主提交按钮、当前工作区激活项、主分支节点。
- **Primary Hover (`#828fff`)**：主按钮悬停色，明度微提。
- **Primary Focus (`#5e69d1`)**：聚焦状态描边（`rgba(94, 106, 209, 0.55)`），2px 发丝外圈。

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
  - Hunk 右侧操作按钮（如暂存、解释）采用静音幽灵按钮。
- **分支与引用徽章 (Ref Badges)**：
  - 统一采用 **Surface 2 (`#141516`)** 底色 + 1px **Hairline (`#23252a`)** 边框 + **Ink Muted (`#d0d6e0`)** 文字。
  - 区别通过微型矢量图标（`iconGitBranch`, `iconGitCommit`, `iconTag`）或 6px 小圆点体现，不再整块涂成琥珀黄或亮蓝。

### 2.6 拓扑图色彩 (Commit Graph Lanes)
摒弃 8 种高饱和荧光彩虹色，采用经过亮度均衡的“莫兰迪 / 精密仪器”8 色谱系：
1. **Lane 0 (主干 / 主分支)**: `#5e6ad2` (Linear 薰衣草蓝)
2. **Lane 1**: `#3eb489` (鼠尾草绿)
3. **Lane 2**: `#e09f3e` (暖琥珀沙)
4. **Lane 3**: `#9d8df1` (柔鸢尾紫)
5. **Lane 4**: `#e5484d` (暗珊瑚红)
6. **Lane 5**: `#45b6fe` (静谧天青)
7. **Lane 6**: `#d977a8` (暮灰粉)
8. **Lane 7**: `#38b2ac` (暗松石绿)

---

## 3. 圆角与形态系统 (Shapes & Radii)

| Token | 像素值 | 适用组件 |
| :--- | :--- | :--- |
| `rounded.xs` | `4px` | 极小代码标签、文件树勾选框、微型图标底板 |
| `rounded.sm` | `6px` | 工具栏分支芯片、树节点选中底带、分段控制器单元 |
| `rounded.md` | `8px` | **全部通用按钮**、文本输入框、下拉框、搜索栏 |
| `rounded.lg` | `12px` | 面板卡片、悬浮卡片、下拉浮层、AI 解释卡片 |
| `rounded.xl` | `16px` | 模态弹窗（Settings, PR, Branch 对话框） |
| `rounded.pill`| `9999px` | 数量计数器（Count Badges）、状态圆角药丸 |

---

## 4. 组件规范 (Component Specifications)

### 4.1 按钮 (Buttons)
- **Primary Button (主操作键)**:
  - 底色：`#5e6ad2`，悬停：`#828fff`，按下：`#5e69d1`
  - 文字：`#ffffff`，字号：12.5px，粗细：Medium (500)
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
- **分支芯片 (Branch Chip)**：
  - 摒弃旧版琥珀黄色块，改用高质感深灰芯片：
  - 底色：`#0f1011` (Surface 1)，边框：1px `#23252a` (Hairline)，圆角：`6px`。
  - 图标与文字：`#d0d6e0` (Ink Muted)；鼠标悬停时底色提升为 `#141516`，边框提升为 `#34343a`。
  - 异常状态（如 Rebase 中或 Detached HEAD）仅使用微弱的 Warning 色文字做点缀提示。

### 4.3 侧边栏 (Sidebar Navigation)
- 底色：`#08090a`（介于 Canvas 与 Surface 1 之间）或半透明 Vibrant 材质。
- 分割线：1px `#23252a`。
- 导航激活态（如“改动”/“历史”）：
  - 摒弃大面积刺眼蓝色块，采用 `Surface 2` 底色搭配 `#5e6ad2` 的微弱强调（或左侧 2px 指示条 / 薰衣草蓝文字）。

### 4.4 提交栏 (Commit Box)
- 放置于主区底部，底色：`#0f1011` (Surface 1)，顶边框：1px `#23252a`。
- 输入框：底色 `#141516` (Surface 2)，边框 1px `#23252a`，聚焦时呈现 2px `#5e69d1` (50% alpha) 光晕。
- 提交按钮：Primary Button 规格，使用 `#5e6ad2`。

---

## 5. 设计守则：Do's & Don'ts (Strict Anti-Noise Rules)

### Do:
- ✅ **拥抱深度黑与微阶梯**：以 `#010102` 画布为基底，让层次自然浮现在 `Surface 1~4` 上。
- ✅ **克制使用薰衣草蓝**：全屏幕范围内，同时出现的 `#5e6ad2` 元素尽量不超过 1~2 处（核心动作焦点）。
- ✅ **用发丝线组织空间**：通过 1px `#23252a` 细线划定边界，而不是用厚重阴影或粗边框。
- ✅ **用文本明度传达信息权重**：主要信息用 `#f7f8f8`，次要信息坚决降至 `#8a8f98`，避免到处都是亮白或彩色。
- ✅ **保持 Diff 视图平静**：增删代码底色半透明度保持在 8~10% 以内，Hunk 栏坚决保持中性灰。

### Don't:
- ❌ **禁止彩色药丸大爆炸**：不要在分支、远程、标签、状态等同一行里涂抹多种高饱和度色块。
- ❌ **禁止使用亮蓝 @@ Hunk 栏**：不再使用 `#121d2f` 底色或 `#79c0ff` 荧光文字。
- ❌ **禁止使用霓虹彩虹拓扑线**：提交图颜色必须保持灰度与明度均衡。
- ❌ **禁止给常规按钮使用大圆角胶囊 (Pill)**：按钮统一为 `8px` 圆角，仅计数角标使用 Pill。
- ❌ **禁止非语义彩色修饰**：不要给卡片、侧栏条目增加花哨的环境渐变或外发光。
