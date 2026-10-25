(function () {
  "use strict";

  var root = document.documentElement;
  var THEME_KEY = "gloss_theme";
  var LANG_KEY = "gloss_lang";
  var ZH = window.GLOSS_I18N && window.GLOSS_I18N.zh ? window.GLOSS_I18N.zh : {};

  function store(key, value) {
    try { localStorage.setItem(key, value); } catch (e) { /* private mode */ }
  }
  function readStore(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  /* ---------------------------------------------------------- language */
  var lang = readStore(LANG_KEY) === "zh" ? "zh" : "en";

  // The static HTML is English; snapshot it once so we can always switch back
  // without a reload. Chinese strings live in i18n.js.
  var i18nNodes = Array.prototype.slice.call(document.querySelectorAll("[data-i18n], [data-i18n-html]"));
  var enTexts = i18nNodes.map(function (el) { return el.innerHTML; });
  var titleNodes = Array.prototype.slice.call(document.querySelectorAll("[data-i18n-title]"));
  var enTitles = titleNodes.map(function (el) { return el.getAttribute("title") || ""; });

  var EN_META = {
    title: document.title,
    desc: (document.querySelector('meta[name="description"]') || {}).content || "",
    ogTitle: (document.querySelector('meta[property="og:title"]') || {}).content || "",
    ogDesc: (document.querySelector('meta[property="og:description"]') || {}).content || ""
  };

  function setMeta(selector, value) {
    var el = document.querySelector(selector);
    if (el) el.setAttribute("content", value);
  }

  function applyLang(next) {
    lang = next;
    store(LANG_KEY, lang);

    i18nNodes.forEach(function (el, i) {
      var key = el.getAttribute("data-i18n") || el.getAttribute("data-i18n-html");
      if (lang === "en") {
        el.innerHTML = enTexts[i];
      } else if (Object.prototype.hasOwnProperty.call(ZH, key)) {
        el.innerHTML = ZH[key];
      }
      // missing zh entry → keep the English snapshot
    });
    titleNodes.forEach(function (el, i) {
      var key = el.getAttribute("data-i18n-title");
      el.setAttribute("title", lang === "zh" && Object.prototype.hasOwnProperty.call(ZH, key) ? ZH[key] : enTitles[i]);
    });

    root.lang = lang === "zh" ? "zh-CN" : "en";
    if (lang === "zh") {
      document.title = ZH["!title"] || EN_META.title;
      setMeta('meta[name="description"]', ZH["!desc"] || EN_META.desc);
      setMeta('meta[property="og:title"]', ZH["!ogTitle"] || EN_META.ogTitle);
      setMeta('meta[property="og:description"]', ZH["!ogDesc"] || EN_META.ogDesc);
    } else {
      document.title = EN_META.title;
      setMeta('meta[name="description"]', EN_META.desc);
      setMeta('meta[property="og:title"]', EN_META.ogTitle);
      setMeta('meta[property="og:description"]', EN_META.ogDesc);
    }

    var enBtn = document.getElementById("lang-en");
    var zhBtn = document.getElementById("lang-zh");
    if (enBtn) enBtn.classList.toggle("is-on", lang === "en");
    if (zhBtn) zhBtn.classList.toggle("is-on", lang === "zh");

    setTheme(root.getAttribute("data-theme") === "light" ? "light" : "dark");
    updateDlLabels();
    resetCopyLabel();
  }

  var langEn = document.getElementById("lang-en");
  var langZh = document.getElementById("lang-zh");
  if (langEn) langEn.addEventListener("click", function () { applyLang("en"); });
  if (langZh) langZh.addEventListener("click", function () { applyLang("zh"); });

  /* ---------------------------------------------------------- appearance */
  var themeBtn = document.getElementById("theme-btn");
  function themeAria(theme) {
    if (lang === "zh") {
      return theme === "dark" ? (ZH["theme.toLight"] || "切换到浅色外观") : (ZH["theme.toDark"] || "切换到深色外观");
    }
    return theme === "dark" ? "Switch to light appearance" : "Switch to dark appearance";
  }
  function setTheme(theme) {
    root.setAttribute("data-theme", theme);
    store(THEME_KEY, theme);
    if (themeBtn) themeBtn.setAttribute("aria-label", themeAria(theme));
  }
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      setTheme(root.getAttribute("data-theme") === "dark" ? "light" : "dark");
    });
  }

  /* ---------------------------------------------------------- mock scaling */
  var frames = Array.prototype.slice.call(document.querySelectorAll(".frame"));

  function fitFrames() {
    frames.forEach(function (frame) {
      var width = frame.clientWidth || 1240;
      var scale = Math.min(1, width / 1240);
      var win = frame.querySelector(".win");
      if (win) win.style.setProperty("--z", scale);
    });
  }

  if ("ResizeObserver" in window) {
    frames.forEach(function (frame) {
      new ResizeObserver(fitFrames).observe(frame);
    });
  } else {
    window.addEventListener("resize", fitFrames);
  }
  window.addEventListener("load", fitFrames);
  fitFrames();

  /* ---------------------------------------------------------- reveal */
  var staticMode = /(?:^|[?&])static(?:=1)?(?:&|$)/.test(window.location.search);

  var reveals = Array.prototype.slice.call(document.querySelectorAll("[data-reveal]"));

  function showAll() {
    reveals.forEach(function (el) { el.classList.add("is-in"); });
  }

  if (staticMode) root.classList.add("no-reveal");

  if ("IntersectionObserver" in window && reveals.length) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add("is-in");
          io.unobserve(entry.target);
        }
      });
    }, { rootMargin: "0px -10% 0px", threshold: 0.05 });

    reveals.forEach(function (el) { io.observe(el); });
    window.setTimeout(showAll, 5000);
  } else {
    showAll();
  }

  /* ---------------------------------------------------------- command palette */
  var palette = document.getElementById("palette");

  if (palette) {
    var palItems = Array.prototype.slice.call(palette.querySelectorAll(".pal-item"));
    var palCard = palette.querySelector(".pal-card");

    function openPalette() {
      palette.classList.add("is-open");
      if (palCard) palCard.setAttribute("aria-hidden", "false");
    }

    function closePalette() {
      palette.classList.remove("is-open");
      if (palCard) palCard.setAttribute("aria-hidden", "true");
    }

    palette.addEventListener("click", function () {
      if (!palette.classList.contains("is-open")) openPalette();
    });

    palItems.forEach(function (item) {
      item.addEventListener("click", function (event) {
        event.stopPropagation();
        palItems.forEach(function (other) { other.classList.remove("is-on"); });
        item.classList.add("is-on");
      });
    });

    document.addEventListener("keydown", function (event) {
      var mod = event.metaKey || event.ctrlKey;

      if (mod && (event.key === "k" || event.key === "K")) {
        event.preventDefault();
        var rect = palette.getBoundingClientRect();
        var offscreen = rect.top > window.innerHeight || rect.bottom < 0;
        if (offscreen) palette.scrollIntoView({ behavior: "smooth", block: "center" });
        palette.classList.contains("is-open") ? closePalette() : openPalette();
        return;
      }

      if (event.key === "Escape") closePalette();
    });

    if (palCard) palCard.setAttribute("aria-hidden", palette.classList.contains("is-open") ? "false" : "true");
  }

  /* ---------------------------------------------------------- copy command */
  var copyBtn = document.getElementById("copy-btn");
  var copyLabel = document.getElementById("copy-label");
  var cliCmd = document.getElementById("cli-cmd");
  var copyTimer = null;

  function copyStrings() {
    return lang === "zh"
      ? { copy: ZH["copy.copy"] || "复制", copied: ZH["copy.copied"] || "已复制" }
      : { copy: "Copy", copied: "Copied" };
  }

  function resetCopyLabel() {
    if (!copyLabel) return;
    if (copyTimer) { window.clearTimeout(copyTimer); copyTimer = null; }
    copyBtn.classList.remove("is-done");
    copyLabel.textContent = copyStrings().copy;
  }

  function fallbackCopy(text) {
    var area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    try { document.execCommand("copy"); } catch (e) { /* noop */ }
    document.body.removeChild(area);
  }

  if (copyBtn && cliCmd) {
    copyBtn.addEventListener("click", function () {
      var text = cliCmd.textContent.trim();
      var done = function () {
        copyBtn.classList.add("is-done");
        copyLabel.textContent = copyStrings().copied;
        if (copyTimer) window.clearTimeout(copyTimer);
        copyTimer = window.setTimeout(resetCopyLabel, 1800);
      };

      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, function () {
          fallbackCopy(text);
          done();
        });
      } else {
        fallbackCopy(text);
        done();
      }
    });
  }

  /* ---------------------------------------------------------- platform label */
  var dlLabel = document.getElementById("main-dl-label");
  var dlMeta = document.getElementById("main-dl-meta");
  var dl2Label = document.getElementById("main-dl2-label");
  var dl2Meta = document.getElementById("main-dl2-meta");
  var dlPlatform = null;

  function detectPlatform() {
    var platform = (navigator.userAgentData && navigator.userAgentData.platform) ||
                   navigator.platform || "";
    var ua = navigator.userAgent || "";

    if (/mac/i.test(platform) || /Mac OS X/i.test(ua)) return "mac";
    if (/win/i.test(platform) || /Windows/i.test(ua)) return "win";
    if (/linux/i.test(platform) || /Linux/i.test(ua)) return "linux";
    return null;
  }

  function updateDlLabels() {
    if (!dlLabel || !dlPlatform) return;
    var zh = lang === "zh";
    var map = {
      mac:   { label: zh ? "下载 macOS 版" : "Download for macOS",   meta: ".dmg" },
      win:   { label: zh ? "下载 Windows 版" : "Download for Windows", meta: ".exe" },
      linux: { label: zh ? "下载 Linux 版" : "Download for Linux",    meta: ".tar.gz" }
    }[dlPlatform];

    dlLabel.textContent = map.label;
    dlMeta.textContent = map.meta;
    if (dl2Label && dl2Meta) {
      dl2Label.textContent = map.label;
      dl2Meta.textContent = map.meta;
    }
  }

  dlPlatform = detectPlatform();
  updateDlLabels();

  /* ---------------------------------------------------------- mock interaction */
  var commitRows = Array.prototype.slice.call(document.querySelectorAll(".cl-row"));
  commitRows.forEach(function (row) {
    row.addEventListener("click", function () {
      commitRows.forEach(function (other) { other.classList.remove("is-on"); });
      row.classList.add("is-on");
    });
  });

  var projRows = Array.prototype.slice.call(document.querySelectorAll(".sb-proj-row"));
  projRows.forEach(function (row) {
    row.addEventListener("click", function () {
      projRows.forEach(function (other) { other.classList.remove("is-on"); });
      row.classList.add("is-on");
    });
  });

  var viewTabs = Array.prototype.slice.call(document.querySelectorAll(".tb-view-tab"));
  viewTabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      viewTabs.forEach(function (other) { other.classList.remove("is-on"); });
      tab.classList.add("is-on");
    });
  });

  /* ---------------------------------------------------------- boot language */
  if (lang === "zh") applyLang("zh");
})();
