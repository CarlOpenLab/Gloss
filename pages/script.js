/**
 * Gloss Landing Page
 * Precision, lightweight JavaScript. Zero runtime baggage.
 */

// 1. 国际化字典
const i18n = {
  zh: {
    nav_principles: "架构理念",
    nav_workflow: "审查流",
    nav_benchmark: "底层指标",
    nav_install: "获取",
    hero_badge: "DIRECT GPU PIPELINE · ZERO WEBVIEWS",
    hero_title: "纯 Go 编写。<br>GPU 直出渲染。<br>专为审查而生。",
    hero_desc: "没有 Chromium，没有 Node.js，没有 400MB 的空载内存。Gloss 是一款遵循 Linear 设计系统规范的专业 Git GUI：冷启仅需 80 毫秒，代码提交前自动预检隐患，全键盘驱动。",
    btn_dl_mac: "获取 macOS 通用版 (.dmg)",
    btn_dl_win: "获取 Windows 版",
    btn_dl_linux: "获取 Linux 二进制",
    btn_dl_general: "下载最新发行版",
    copied: "已复制",
    pane_graph: "TOPOLOGY / GRAPH",
    pane_staged: "STAGED (2)",
    badge_viewed: "VIEWED",
    btn_toggle_audit: "Pre-flight Audit (1)",
    audit_flag: "SECURITY PRE-FLIGHT (LINE 42)",
    audit_detail: "检测到硬编码字符串 <code>sk_live_...</code>。建议迁移至 <code>os.Getenv(\"AUTH_SECRET\")</code> 并注入配置。",
    sec_arch: "ARCHITECTURE",
    arch_title: "摒弃 Web 封装，回归单一本地可执行文件",
    p1_head: "GPU 直接绘制通道",
    p1_body: "基于纯 Go 与 mygo 底层图形管线，直接调用系统原生 Metal / Direct3D 接口提交图元。没有 Chromium 的 DOM 解析和重排重绘，长达万行的提交历史与大型 Diff 在 60fps 下保持绝对平滑。",
    p2_head: "无彩色克制视觉规范",
    p2_body: "继承 Linear 经典的黑白两极（Achromatic）系统：深色下白键黑字，浅色下黑键白字。界面本身不使用多余杂色，将视网膜的注意力 100% 留给 Git 变动的语义增减与分支车道。",
    p3_head: "~35MB 极轻常驻开销",
    p3_body: "启动用时约 80 毫秒，待机内存占用约为 Electron 类工具的十分之一。即使开一整天、常驻后台处理庞大 Monorepo，也不会成为系统的资源黑洞。",
    sec_flow: "WORKFLOW",
    flow_title: "把安全隐患与杂乱信息，拦截在提交之前",
    w1_head: "提交前自动风险审计 (Pre-flight Risk Scan)",
    w1_desc: "当你审查别人提交的 Pull Request 或准备提交本地暂存时，Gloss 一键静态扫描暂存区中的代码变动，揪出意外带入的私钥、AWS/OpenAI 凭据、SQL 注入隐患和调试断点，按文件行号精准定位。",
    w2_head: "直连私有大模型 (Multi-LLM Native)",
    w2_desc: "支持 Claude、OpenAI、DeepSeek 及本地 Ollama 实例。API Key 完全保留在本地。Gloss 深度理解代码差异块（Hunk），一键总结严谨的 Conventional Commits 格式，或根据单行需求生成规范分支名称。",
    w3_head: "已审查状态追踪与分块暂存 (Viewed Tracking)",
    w3_desc: "大文件变更不再眼花缭乱。确认无误的文件一键折叠（Viewed），文件内容变动后自动重置状态。支持按变更块（Hunk）独立暂存与丢弃，全键盘操作不依赖鼠标。",
    sec_bench: "DATA & BENCHMARK",
    bench_title: "客观指标对照",
    tb_metric: "指标",
    tb_m1: "技术栈",
    tb_m2: "冷启动时间",
    tb_m3: "静默待机内存",
    tb_m4: "二进制包体积",
    tb_m5: "AI 与网络依赖",
    sec_install: "DOWNLOADS",
    install_title: "开箱即用，支持主流操作系统",
    mac_desc: "Apple Silicon (M1/M2/M3/M4) 与 Intel 架构通用 DMG",
    btn_dl_dmg: "下载 .dmg 安装镜像",
    win_desc: "提供 NSIS 官方安装包及便携免安装绿色 ZIP",
    linux_desc: "Linux x86_64 二进制压缩包，或通过 Go 工具链直接安装"
  },
  en: {
    nav_principles: "Principles",
    nav_workflow: "Workflow",
    nav_benchmark: "Benchmark",
    nav_install: "Download",
    hero_badge: "DIRECT GPU PIPELINE · ZERO WEBVIEWS",
    hero_title: "Pure Go.<br>GPU Direct.<br>Built for Review.",
    hero_desc: "Zero Chromium. Zero Node.js. Zero 400MB idle footprints. Gloss is a distraction-free Git GUI calibrated to the Linear design system: boots in 80ms, audits secrets pre-flight, and stays keyboard-driven.",
    btn_dl_mac: "Download macOS Universal (.dmg)",
    btn_dl_win: "Download for Windows",
    btn_dl_linux: "Download for Linux",
    btn_dl_general: "Download Latest Release",
    copied: "COPIED",
    pane_graph: "TOPOLOGY / GRAPH",
    pane_staged: "STAGED (2)",
    badge_viewed: "VIEWED",
    btn_toggle_audit: "Pre-flight Audit (1)",
    audit_flag: "SECURITY PRE-FLIGHT (LINE 42)",
    audit_detail: "Hardcoded key pattern detected: <code>sk_live_...</code>. Migrate to <code>os.Getenv(\"AUTH_SECRET\")</code> prior to committing.",
    sec_arch: "ARCHITECTURE",
    arch_title: "Single native binary. No web runtimes.",
    p1_head: "GPU Direct Pipeline",
    p1_body: "Pure Go drawing directly to Metal / Direct3D graphics pipelines via mygo. No DOM layout trees, no style recalculation overhead. Thousands of commits and massive diffs scroll at a persistent 60fps.",
    p2_head: "Achromatic Discipline",
    p2_body: "Calibrated to Linear's strict two-tone discipline: ink-white keys on dark, black keys on light. Zero decorative gradients so your retina focuses entirely on semantic additions, deletions, and lanes.",
    p3_head: "~35MB Idle Footprint",
    p3_body: "Cold starts in ~80 milliseconds. Idle memory usage is one-tenth of Electron clients. Stays open all day inside large monorepos without consuming workstation memory.",
    sec_flow: "WORKFLOW",
    flow_title: "Catch vulnerabilities before git commit",
    w1_head: "Pre-flight Security Audit",
    w1_desc: "One click scans staged hunks for leaked tokens, private keys, SQL injection patterns, and debug remnants anchored to exact lines and files.",
    w2_head: "Direct Multi-LLM Connectors",
    w2_desc: "Works with Claude, OpenAI, DeepSeek, and local Ollama instances. API keys remain on your machine. Generates conventional commits and branch naming straight from hunk semantics.",
    w3_head: "Viewed Tracking & Hunk Staging",
    w3_desc: "Tame large PRs by marking reviewed files as viewed to collapse them. Stage or discard individual hunks with single keystrokes.",
    sec_bench: "DATA & BENCHMARK",
    bench_title: "Objective Metrics",
    tb_metric: "Metric",
    tb_m1: "Tech Stack",
    tb_m2: "Cold Startup",
    tb_m3: "Idle RAM",
    tb_m4: "Binary Size",
    tb_m5: "AI & Network",
    sec_install: "DOWNLOADS",
    install_title: "Standalone, cross-platform binaries",
    mac_desc: "Universal DMG for Apple Silicon (M1–M4) and Intel x86_64",
    btn_dl_dmg: "Download .dmg Installer",
    win_desc: "Official NSIS installer and portable zero-install ZIP",
    linux_desc: "Linux x86_64 tarball, or install via Go toolchain"
  }
};

let currentLang = localStorage.getItem("gloss_lang") || 
  (navigator.language && navigator.language.startsWith("zh") ? "zh" : "en");

function applyI18n(lang) {
  currentLang = lang;
  localStorage.setItem("gloss_lang", lang);
  document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";

  const dict = i18n[lang] || i18n.zh;
  document.querySelectorAll("[data-i18n]").forEach(el => {
    const key = el.getAttribute("data-i18n");
    if (dict[key]) {
      el.innerHTML = dict[key];
    }
  });

  const langLabel = document.getElementById("lang-label");
  if (langLabel) {
    langLabel.textContent = lang === "zh" ? "EN" : "中";
  }

  updateDownloadTarget(lang);
}

// 2. 主题管理 (Dark / Light)
const themeBtn = document.getElementById("theme-btn");
const brandLogo = document.getElementById("brand-logo");

function setTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem("gloss_theme", theme);
  if (brandLogo) {
    brandLogo.src = theme === "light" ? "assets/logo.png" : "assets/logo-dark.png";
  }
}

const savedTheme = localStorage.getItem("gloss_theme");
if (savedTheme) {
  setTheme(savedTheme);
} else {
  const prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  setTheme(prefersDark ? "dark" : "light");
}

if (themeBtn) {
  themeBtn.addEventListener("click", () => {
    const current = document.documentElement.getAttribute("data-theme") || "dark";
    setTheme(current === "dark" ? "light" : "dark");
  });
}

// 3. 操作系统探测与下载按钮
function updateDownloadTarget(lang) {
  const ua = navigator.userAgent.toLowerCase();
  const pf = (navigator.platform || "").toLowerCase();
  const dict = i18n[lang] || i18n.zh;

  const dlLabel = document.getElementById("main-dl-label");
  const dlBtn = document.getElementById("main-dl-btn");
  const deckMac = document.getElementById("deck-mac");
  const deckWin = document.getElementById("deck-win");
  const deckLinux = document.getElementById("deck-linux");

  [deckMac, deckWin, deckLinux].forEach(el => el && el.classList.remove("active-target"));

  if (pf.includes("mac") || ua.includes("macintosh")) {
    if (dlLabel) dlLabel.textContent = dict.btn_dl_mac;
    if (dlBtn) dlBtn.href = "#deck-mac";
    if (deckMac) deckMac.classList.add("active-target");
  } else if (pf.includes("win") || ua.includes("windows")) {
    if (dlLabel) dlLabel.textContent = dict.btn_dl_win;
    if (dlBtn) dlBtn.href = "#deck-win";
    if (deckWin) deckWin.classList.add("active-target");
  } else if (pf.includes("linux") || ua.includes("linux")) {
    if (dlLabel) dlLabel.textContent = dict.btn_dl_linux;
    if (dlBtn) dlBtn.href = "#deck-linux";
    if (deckLinux) deckLinux.classList.add("active-target");
  } else {
    if (dlLabel) dlLabel.textContent = dict.btn_dl_general;
    if (dlBtn) dlBtn.href = "#install";
  }
}

// 4. CLI 复制
const copyBtn = document.getElementById("copy-btn");
const copyTag = document.getElementById("copy-tag");

if (copyBtn) {
  copyBtn.addEventListener("click", () => {
    const text = "go install github.com/CarlOpenLab/Gloss@latest";
    navigator.clipboard.writeText(text).then(() => {
      if (copyTag) {
        copyTag.classList.add("show");
        setTimeout(() => copyTag.classList.remove("show"), 1800);
      }
    });
  });
}

// 5. 语言切换
const langBtn = document.getElementById("lang-btn");
if (langBtn) {
  langBtn.addEventListener("click", () => {
    const next = currentLang === "zh" ? "en" : "zh";
    applyI18n(next);
  });
}

// 6. 界面审计卡片折叠交互
const auditToggleBtn = document.getElementById("audit-toggle-btn");
const auditCallout = document.getElementById("audit-callout");

if (auditToggleBtn && auditCallout) {
  auditToggleBtn.addEventListener("click", () => {
    const hidden = auditCallout.style.display === "none";
    auditCallout.style.display = hidden ? "block" : "none";
  });
}

document.addEventListener("DOMContentLoaded", () => {
  applyI18n(currentLang);
});
