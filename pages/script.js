/**
 * Gloss Landing Page — Redesigned
 * Real screenshots, typographic rhythm, zero fake mockups.
 */

// 1. i18n
const i18n = {
  zh: {
    nav_features: "核心能力",
    nav_benchmark: "性能对比",
    nav_shortcuts: "快捷键",
    nav_install: "下载",
    hero_title: "纯 Go 编写。<br>GPU 直出渲染。<br>专为审查而生。",
    hero_desc: "没有 Chromium，没有 Node.js，没有 400MB 的空载内存。<br>遵循 Linear 2.0 设计规范的跨平台 Git 客户端。",
    stat_boot: "冷启动",
    stat_mem: "待机内存",
    stat_bin: "二进制体积",
    btn_dl_mac: "获取 macOS 通用版 (.dmg)",
    btn_dl_win: "获取 Windows 版",
    btn_dl_linux: "获取 Linux 二进制",
    btn_dl_general: "下载最新发行版",
    copied: "已复制",
    f1_label: "PRE-FLIGHT AUDIT",
    f1_title: "提交前自动风险审计",
    f1_desc: "一键静态扫描暂存区中的代码变动，揪出意外带入的私钥、AWS/OpenAI 凭据、SQL 注入隐患和调试断点。按文件行号精准定位，按严重级别分类展示。",
    f2_label: "TOPOLOGY GRAPH",
    f2_title: "高精度贝塞尔曲线拓扑图",
    f2_desc: "自研贝塞尔分支车道算法，支持经典、鲜艳与全灰度三种调色盘体系。分支合并与分叉路径一目了然，双击分支芯片直接检出。",
    f3_label: "MULTI-LLM NATIVE",
    f3_title: "直连私有大模型",
    f3_desc: "支持 Claude、OpenAI、DeepSeek 及本地 Ollama 实例。API Key 完全保留在本地。深度理解代码差异块（Hunk），一键总结严谨的 Conventional Commits 格式。",
    f4_label: "BUILT-IN TERMINAL",
    f4_title: "多会话终端与全键盘驱动",
    f4_desc: "内置多标签终端，支持 OSC 标题、字体字号自定义、图片粘贴直通 Claude Code。全键盘操作行云流水，已审文件一键标记 Viewed 折叠。",
    p1_head: "GPU 直接绘制通道",
    p1_body: "基于纯 Go 与 <code>mygo</code> 底层图形管线，直接调用系统原生 Metal / Direct3D 接口提交图元。没有 DOM 解析和重排重绘，万行历史与大型 Diff 在 60fps 下保持平滑。",
    p2_head: "无彩色克制视觉规范",
    p2_body: "继承 Linear 经典的黑白两极系统。界面本身不使用多余杂色，将注意力 100% 留给 Git 变动的语义增减与分支车道。",
    p3_head: "~35MB 极轻常驻开销",
    p3_body: "启动约 80 毫秒，待机内存约为 Electron 类工具的十分之一。整天常驻后台处理庞大 Monorepo，也不会成为系统的资源黑洞。",
    sec_bench: "DATA & BENCHMARK",
    bench_title: "客观指标对照",
    tb_metric: "指标",
    tb_m1: "技术栈",
    tb_m2: "冷启动时间",
    tb_m3: "静默待机内存",
    tb_m4: "二进制包体积",
    tb_m5: "AI 与网络依赖",
    tb_v5_gloss: "本地优先 / 直连自有模型",
    tb_v5_other: "多数需要云端登录",
    sec_shortcuts: "KEYBOARD-FIRST",
    shortcuts_title: "全键盘驱动，指尖行云流水",
    sc_1: "暂存 / 取消暂存当前选中文件或 Hunk 块",
    sc_2: "呼出全局命令面板 (Palette)",
    sc_3: "提交当前已暂存的变动",
    sc_4: "标记当前文件为"已审查 (Viewed)"并自动折叠",
    sc_5: "在前后差异块 (Hunk) 之间快速跳转",
    sc_6: "一键触发暂存区代码安全预检",
    sec_install: "DOWNLOADS",
    install_title: "开箱即用，支持主流操作系统",
    mac_desc: "Apple Silicon (M1/M2/M3/M4) 与 Intel 架构通用 DMG",
    btn_dl_dmg: "下载 .dmg 安装镜像",
    win_desc: "提供 NSIS 官方安装包及便携免安装绿色 ZIP",
    linux_desc: "Linux x86_64 二进制压缩包，或通过 Go 工具链直接安装"
  },
  en: {
    nav_features: "Features",
    nav_benchmark: "Benchmark",
    nav_shortcuts: "Shortcuts",
    nav_install: "Download",
    hero_title: "Pure Go.<br>GPU Direct.<br>Built for Review.",
    hero_desc: "Zero Chromium. Zero Node.js. Zero 400 MB idle footprints.<br>A distraction-free Git GUI calibrated to Linear 2.0.",
    stat_boot: "Cold boot",
    stat_mem: "Idle RAM",
    stat_bin: "Binary size",
    btn_dl_mac: "Download macOS Universal (.dmg)",
    btn_dl_win: "Download for Windows",
    btn_dl_linux: "Download for Linux",
    btn_dl_general: "Download Latest Release",
    copied: "COPIED",
    f1_label: "PRE-FLIGHT AUDIT",
    f1_title: "Pre-flight Security Audit",
    f1_desc: "One click scans staged hunks for leaked tokens, private keys, SQL injection patterns, and debug remnants — anchored to exact lines and files, sorted by severity.",
    f2_label: "TOPOLOGY GRAPH",
    f2_title: "High-Precision Topology Graph",
    f2_desc: "Custom Bezier branch lanes supporting Classic, Vivid, and Grayscale palettes. Merge flows are instantly intelligible; double-click branch chips to check out.",
    f3_label: "MULTI-LLM NATIVE",
    f3_title: "Direct Multi-LLM Connectors",
    f3_desc: "Works with Claude, OpenAI, DeepSeek, and local Ollama instances. API keys remain on your machine. Generates conventional commits straight from hunk semantics.",
    f4_label: "BUILT-IN TERMINAL",
    f4_title: "Multi-Session Terminal & Keyboard-First",
    f4_desc: "Built-in tabbed terminal with OSC titles, custom fonts, and image paste pass-through to Claude Code. Full keyboard operation with viewed-file collapse.",
    p1_head: "GPU Direct Pipeline",
    p1_body: "Pure Go drawing directly to Metal / Direct3D via <code>mygo</code>. No DOM layout trees. Thousands of commits scroll at a persistent 60 fps.",
    p2_head: "Achromatic Discipline",
    p2_body: "Calibrated to Linear's strict two-tone discipline. Zero decorative gradients so your retina focuses entirely on semantic diffs and branch lanes.",
    p3_head: "~35 MB Idle Footprint",
    p3_body: "Cold starts in ~80 ms. Idle memory is one-tenth of Electron clients. Stays open all day inside large monorepos without consuming workstation memory.",
    sec_bench: "DATA & BENCHMARK",
    bench_title: "Objective Metrics",
    tb_metric: "Metric",
    tb_m1: "Tech Stack",
    tb_m2: "Cold Startup",
    tb_m3: "Idle RAM",
    tb_m4: "Binary Size",
    tb_m5: "AI & Network",
    tb_v5_gloss: "Local-first / direct model access",
    tb_v5_other: "Most require cloud sign-in",
    sec_shortcuts: "KEYBOARD-FIRST",
    shortcuts_title: "Keyboard-driven speed",
    sc_1: "Stage / unstage current file or hunk",
    sc_2: "Trigger global command palette",
    sc_3: "Commit currently staged changes",
    sc_4: "Mark file as viewed and collapse",
    sc_5: "Navigate between diff hunks",
    sc_6: "Trigger pre-flight security scan",
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

// 2. Theme
const themeBtn = document.getElementById("theme-btn");

function setTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem("gloss_theme", theme);
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

// 3. OS detection & download button
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

// 4. CLI copy
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

// 5. Language toggle
const langBtn = document.getElementById("lang-btn");
if (langBtn) {
  langBtn.addEventListener("click", () => {
    const next = currentLang === "zh" ? "en" : "zh";
    applyI18n(next);
  });
}

// 6. Init
document.addEventListener("DOMContentLoaded", () => {
  applyI18n(currentLang);
});
