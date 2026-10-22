(function () {
  "use strict";

  var root = document.documentElement;
  var THEME_KEY = "gloss_theme";

  /* ---------------------------------------------------------- appearance */
  var themeBtn = document.getElementById("theme-btn");
  function setTheme(theme) {
    root.setAttribute("data-theme", theme);
    try { localStorage.setItem(THEME_KEY, theme); } catch (e) { /* private mode */ }
    if (themeBtn) {
      themeBtn.setAttribute("aria-label", theme === "dark" ? "切换到浅色外观" : "切换到深色外观");
    }
  }
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      setTheme(root.getAttribute("data-theme") === "dark" ? "light" : "dark");
    });
  }
  setTheme(root.getAttribute("data-theme") === "light" ? "light" : "dark");

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
    }, { rootMargin: "0px 0px -10% 0px", threshold: 0.05 });

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
        if (copyLabel) copyLabel.textContent = "已复制";
        window.setTimeout(function () {
          copyBtn.classList.remove("is-done");
          if (copyLabel) copyLabel.textContent = "复制";
        }, 1800);
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

  if (dlLabel) {
    var platform = (navigator.userAgentData && navigator.userAgentData.platform) ||
                   navigator.platform || "";
    var ua = navigator.userAgent || "";
    var map = null;

    if (/mac/i.test(platform) || /Mac OS X/i.test(ua)) {
      map = { label: "下载 macOS 版", meta: ".dmg" };
    } else if (/win/i.test(platform) || /Windows/i.test(ua)) {
      map = { label: "下载 Windows 版", meta: ".exe" };
    } else if (/linux/i.test(platform) || /Linux/i.test(ua)) {
      map = { label: "下载 Linux 版", meta: ".tar.gz" };
    }

    if (map) {
      dlLabel.textContent = map.label;
      dlMeta.textContent = map.meta;
      if (dl2Label && dl2Meta) {
        dl2Label.textContent = map.label;
        dl2Meta.textContent = map.meta;
      }
    }
  }

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
})();
