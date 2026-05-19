(function () {
  "use strict";

  var SERVICE_BASE = "http://127.0.0.1:19860";
  var MAX_TEXT_LENGTH = 200000;
  var MAX_SENTENCES = 1000;
  var MAX_SENTENCE_LENGTH = 1000;
  var SENTENCE_END = /[。！？!?；;]+|[\r\n]+/g;
  var WD_GO_TO_PAGE = 1;
  var WD_GO_TO_ABSOLUTE = 1;
  var WD_ACTIVE_END_PAGE_NUMBER = 3;

  var rate = 1.2;
  var readMode = "continuous";
  var playbackToken = 0;
  var isReading = false;
  var lastActionAt = 0;
  var lastSelectedIndex = -1;
  var startupPopup = null;
  var startupDialogId = "";

  var RATE_OPTIONS = [
    { id: "rate075", value: 0.75, label: "0.75x" },
    { id: "rate10", value: 1.0, label: "1x" },
    { id: "rate12", value: 1.2, label: "1.2x" },
    { id: "rate15", value: 1.5, label: "1.5x" }
  ];

  function notify(message, title, variant) {
    showDialog({
      title: title || "文档朗读",
      variant: variant || "info",
      message: message
    });
  }

  function showDialog(options) {
    var payload = encodeURIComponent(toBase64(JSON.stringify(options || {})));
    var url = SERVICE_BASE + "/dialog.html?payload=" + payload;
    var title = options && options.title ? options.title : "文档朗读";
    var width = options && options.width ? Number(options.width) : 880;
    var height = options && options.height ? Number(options.height) : 680;

    try {
      if (window.wps && typeof window.wps.ShowDialog === "function") {
        window.wps.ShowDialog(url, title, width, height, false);
        return null;
      }
      if (window.Application && typeof window.Application.ShowDialog === "function") {
        window.Application.ShowDialog(url, title, width, height, false);
        return null;
      }
    } catch (_) {
      // Continue to window.open below.
    }

    try {
      var popup = window.open(url, "wpsReadAloudDialog", "width=" + width + ",height=" + height + ",resizable=yes,scrollbars=yes");
      if (popup && typeof popup.focus === "function") {
        popup.focus();
        return popup;
      }
    } catch (_) {
      // Fall back below.
    }
    dialogFallback(options);
    return null;
  }

  function showStartupDialog() {
    startupDialogId = "startup-" + Date.now() + "-" + Math.floor(Math.random() * 1000000);
    var options = {
      title: "朗读正在启动",
      variant: "info",
      compact: true,
      startup: true,
      startupId: startupDialogId,
      width: 500,
      height: 150,
      message: "朗读服务正在启动，请耐心等待..."
    };
    return showDialog(options);
  }

  function toBase64(text) {
    return btoa(unescape(encodeURIComponent(text)));
  }

  function dialogFallback(options) {
    var lines = [];
    if (options && options.title) {
      lines.push(options.title);
      lines.push("");
    }
    if (options && options.message) {
      lines.push(options.message);
    }
    if (options && options.fields) {
      for (var i = 0; i < options.fields.length; i += 1) {
        lines.push(options.fields[i].label + "：" + options.fields[i].value);
      }
    }
    if (options && options.links) {
      lines.push("");
      for (var j = 0; j < options.links.length; j += 1) {
        lines.push(options.links[j].label + "：" + options.links[j].url);
      }
    }
    try {
      window.alert(lines.join("\n"));
    } catch (_) {
      console.log(lines.join("\n"));
    }
  }

  function status(message) {
    console.log("[wps-read-aloud] " + message);
    try {
      getWpsApplication().StatusBar = message;
    } catch (_) {}
  }

  function setReadingState(value) {
    isReading = !!value;
    invalidateControls();
  }

  function controlId(control) {
    if (typeof control === "string") {
      return control;
    }
    return (control && (control.Id || control.id || control.ID)) || "";
  }

  function onGetImage(control) {
    var icons = {
      startSpeak: "assets/icons/start.png",
      stopSpeak: "assets/icons/stop.png",
      modeMenu: "assets/icons/mode.png",
      rateMenu: "assets/icons/rate.png",
      checkStatus: "assets/icons/status.png",
      aboutAddin: "assets/icons/about.png"
    };
    return icons[controlId(control)] || icons.startSpeak;
  }

  function userMessage(error) {
    var raw = error && error.message ? error.message : String(error || "");
    try {
      var parsed = JSON.parse(raw);
      if (parsed && parsed.error) {
        return parsed.error;
      }
    } catch (_) {
      // Keep the original message below.
    }
    if (/Failed to fetch|NetworkError|Load failed|fetch/i.test(raw)) {
      return "本地朗读服务未连接，请确认安装已完成并重启 WPS。";
    }
    if (/AbortError|aborted|timeout/i.test(raw)) {
      return "朗读合成超时，请缩短选区或稍后重试。";
    }
    return raw || "操作失败，请稍后重试。";
  }

  function throttleAction() {
    var now = Date.now();
    if (now - lastActionAt < 450) {
      status("操作过快，请稍等。");
      return true;
    }
    lastActionAt = now;
    return false;
  }

  function getWpsApplication() {
    if (window.wps && typeof window.wps.WpsApplication === "function") {
      return window.wps.WpsApplication();
    }
    if (window.Application) {
      return window.Application;
    }
    throw new Error("未找到 WPS JS API，请在 WPS 文字加载项环境中运行。");
  }

  function normalizeText(text) {
    return String(text || "")
      .replace(/\r/g, "\n")
      .replace(/[ \t]+\n/g, "\n")
      .replace(/\n{3,}/g, "\n\n")
      .trim();
  }

  function activeDocument() {
    var app = getWpsApplication();
    var doc = app.ActiveDocument;
    if (!doc) {
      throw new Error("未找到当前 WPS 文档。");
    }
    return doc;
  }

  function documentStart(doc) {
    if (doc.Content && doc.Content.Start !== undefined) {
      return Number(doc.Content.Start) || 0;
    }
    return 0;
  }

  function documentEnd(doc) {
    if (doc.Content && doc.Content.End !== undefined) {
      return Number(doc.Content.End) || 0;
    }
    var range = doc.Range && doc.Range();
    return range && range.End !== undefined ? Number(range.End) || 0 : 0;
  }

  function selectionLocation() {
    try {
      var app = getWpsApplication();
      var selection = app.Selection;
      if (!selection) {
        return { hasCursor: false, start: 0, end: 0, page: 1 };
      }
      var range = selection.Range || selection;
      if (!range || range.Start === undefined) {
        return { hasCursor: false, start: 0, end: 0, page: 1 };
      }
      return {
        hasCursor: true,
        start: Number(range.Start) || 0,
        end: range.End !== undefined ? Number(range.End) || Number(range.Start) || 0 : Number(range.Start) || 0,
        page: pageNumber(selection, range)
      };
    } catch (_) {
      return { hasCursor: false, start: 0, end: 0, page: 1 };
    }
  }

  function pageNumber(selection, range) {
    try {
      if (selection && typeof selection.Information === "function") {
        return Number(selection.Information(WD_ACTIVE_END_PAGE_NUMBER)) || 1;
      }
    } catch (_) {}
    try {
      if (range && typeof range.Information === "function") {
        return Number(range.Information(WD_ACTIVE_END_PAGE_NUMBER)) || 1;
      }
    } catch (_) {}
    return 1;
  }

  function goToPage(doc, page) {
    var app = getWpsApplication();
    var attempts = [
      function () { return doc.GoTo(WD_GO_TO_PAGE, WD_GO_TO_ABSOLUTE, page); },
      function () { return doc.Range(0, 0).GoTo(WD_GO_TO_PAGE, WD_GO_TO_ABSOLUTE, page); },
      function () { return app.Selection.GoTo(WD_GO_TO_PAGE, WD_GO_TO_ABSOLUTE, page); }
    ];
    for (var i = 0; i < attempts.length; i += 1) {
      try {
        var range = attempts[i]();
        if (range && range.Start !== undefined) {
          return range;
        }
      } catch (_) {}
    }
    return null;
  }

  function pageStart(doc, page) {
    if (page <= 1) {
      return documentStart(doc);
    }
    var range = goToPage(doc, page);
    return range && range.Start !== undefined ? Number(range.Start) : null;
  }

  function pageEnd(doc, page) {
    var next = pageStart(doc, page + 1);
    var end = documentEnd(doc);
    if (next !== null && next > 0) {
      return Math.max(documentStart(doc), Math.min(next - 1, end));
    }
    return end;
  }

  function rangeText(doc, start, end) {
    var range = doc.Range && doc.Range(start, end);
    if (!range) {
      return { text: "", start: start };
    }
    return {
      text: range.Text !== undefined ? String(range.Text || "") : "",
      start: range.Start !== undefined ? Number(range.Start) : start
    };
  }

  function readContinuousSource() {
    var doc = activeDocument();
    var loc = selectionLocation();
    var start = loc.hasCursor ? loc.start : documentStart(doc);
    return rangeText(doc, start, documentEnd(doc));
  }

  function readCurrentPageSource() {
    var doc = activeDocument();
    var loc = selectionLocation();
    var page = loc.page || 1;
    var start = loc.hasCursor ? loc.start : pageStart(doc, page);
    if (start === null || start === undefined) {
      start = documentStart(doc);
    }
    var end = pageEnd(doc, page);
    if (end <= start) {
      end = documentEnd(doc);
    }
    return rangeText(doc, start, end);
  }

  function currentSource() {
    return readMode === "page" ? readCurrentPageSource() : readContinuousSource();
  }

  function splitSentences(source) {
    var raw = String(source.text || "");
    var base = Number(source.start || 0);
    var segments = [];
    var start = 0;
    var match;

    SENTENCE_END.lastIndex = 0;
    while ((match = SENTENCE_END.exec(raw)) !== null) {
      var end = match.index + match[0].length;
      pushSegment(segments, raw, base, start, end);
      if (segments.length >= MAX_SENTENCES) {
        break;
      }
      start = end;
    }
    if (segments.length < MAX_SENTENCES) {
      pushSegment(segments, raw, base, start, raw.length);
    }
    return segments;
  }

  function pushSegment(segments, raw, base, start, end) {
    var text = raw.slice(start, end);
    var trimmed = text.trim();
    if (!trimmed) {
      return;
    }
    var leading = text.search(/\S/);
    var trailing = text.length - text.trimEnd().length;
    var localStart = start + (leading < 0 ? 0 : leading);
    var localEnd = end - trailing;
    segments.push({
      text: trimmed.length > MAX_SENTENCE_LENGTH ? trimmed.slice(0, MAX_SENTENCE_LENGTH) : trimmed,
      start: base + localStart,
      end: base + Math.min(localEnd, localStart + MAX_SENTENCE_LENGTH)
    });
  }

  function selectDocumentRange(segment) {
    try {
      var doc = activeDocument();
      if (!doc.Range) {
        return;
      }
      var range = doc.Range(segment.start, segment.end);
      if (range && typeof range.Select === "function") {
        range.Select();
      }
      var app = getWpsApplication();
      if (app.ActiveWindow && typeof app.ActiveWindow.ScrollIntoView === "function") {
        app.ActiveWindow.ScrollIntoView(range, true);
      }
    } catch (error) {
      status("选中当前语句失败：" + userMessage(error));
    }
  }

  async function request(path, options) {
    var response = await fetch(SERVICE_BASE + path, options || {});
    var data = await parseJsonResponse(response, path);
    if (!response.ok) {
      throw new Error(data.error || response.statusText);
    }
    return data;
  }

  async function parseJsonResponse(response, path) {
    var text = await response.text();
    if (!text) {
      return {};
    }
    try {
      return JSON.parse(text);
    } catch (_) {
      if (/301|Moved Permanently|404|page not found|<!doctype|<html/i.test(text)) {
        throw new Error("本地朗读服务版本不匹配或尚未重启，请重新安装最新安装包，或重启 wps-tts.service 后再打开 WPS。");
      }
      throw new Error("本地朗读服务返回了无法识别的数据，接口：" + path + "。请重启 WPS 和 wps-tts.service 后重试。");
    }
  }

  async function speakSource(source) {
    if (throttleAction()) {
      return;
    }
    if (isReading) {
      stopPlayback(true);
      await sleep(120);
    }

    var normalized = normalizeText(source.text);
    if (!normalized) {
      notify("没有可朗读的文本，请确认文档中有正文内容。");
      return;
    }
    if (normalized.length > MAX_TEXT_LENGTH) {
      notify("文档内容过长，请缩短朗读范围后重试。");
      return;
    }

    var segments = splitSentences(source);
    if (!segments.length) {
      notify("没有可朗读的完整语句。");
      return;
    }
    if (segments.length >= MAX_SENTENCES) {
      notify("文档内容较长，本次最多朗读前 " + MAX_SENTENCES + " 句。");
    }

    playbackToken += 1;
    var token = playbackToken;
    setReadingState(true);
    lastSelectedIndex = -1;
    status("朗读服务正在启动，请耐心等待...");
    startupPopup = showStartupDialog();

    try {
      await request("/read/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sentences: segments.map(function (segment) {
            return { text: segment.text };
          }),
          rate: rate,
          prefetch: 0
        })
      });
      await pollReadStatus(token, segments, startupPopup);
    } catch (error) {
      if (token === playbackToken) {
        notify(userMessage(error));
      }
    } finally {
      closeStartupDialog();
      if (token === playbackToken) {
        setReadingState(false);
      }
    }
  }

  function closePopup(popup) {
    try {
      if (popup && !popup.closed && typeof popup.close === "function") {
        popup.close();
      }
    } catch (_) {}
  }

  function closeStartupDialog() {
    if (startupDialogId) {
      try {
        localStorage.setItem("wpsReadAloudCloseStartup", startupDialogId);
      } catch (_) {}
    }
    closePopup(startupPopup);
    startupPopup = null;
    startupDialogId = "";
  }

  async function pollReadStatus(token, segments, startupPopup) {
    while (token === playbackToken) {
      var data = await request("/read/status");
      if (data.state === "playing") {
        closePopup(startupPopup);
        startupPopup = null;
      }
      var index = Number(data.current_index);
      if (index >= 0 && index < segments.length && index !== lastSelectedIndex) {
        lastSelectedIndex = index;
        selectDocumentRange(segments[index]);
      }
      if (data.message) {
        status(data.message);
      }
      if (data.state === "done") {
        status("朗读完成。");
        break;
      }
      if (data.state === "stopped" || data.state === "idle") {
        break;
      }
      if (data.state === "error") {
        throw new Error(data.error || data.message || "朗读失败。");
      }
      await sleep(200);
    }
  }

  function sleep(ms) {
    return new Promise(function (resolve) {
      setTimeout(resolve, ms);
    });
  }

  function stopPlayback(silent) {
    if (!silent && throttleAction()) {
      return;
    }
    playbackToken += 1;
    closeStartupDialog();
    setReadingState(false);
    lastSelectedIndex = -1;
    postControl("/read/stop");
    if (!silent) {
      status("已停止朗读。");
    }
  }

  async function postControl(path) {
    try {
      await request(path, { method: "POST" });
    } catch (error) {
      status(userMessage(error));
    }
  }

  function rateIdForValue(value) {
    for (var i = 0; i < RATE_OPTIONS.length; i += 1) {
      if (Math.abs(RATE_OPTIONS[i].value - value) < 0.001) {
        return RATE_OPTIONS[i].id;
      }
    }
    return "rate12";
  }

  function rateLabelForValue(value) {
    for (var i = 0; i < RATE_OPTIONS.length; i += 1) {
      if (Math.abs(RATE_OPTIONS[i].value - value) < 0.001) {
        return RATE_OPTIONS[i].label;
      }
    }
    return "1.2x";
  }

  function setRateById(id) {
    if (isReading) {
      status("朗读过程中不能切换语速，请停止后再调整。");
      return true;
    }
    for (var i = 0; i < RATE_OPTIONS.length; i += 1) {
      if (RATE_OPTIONS[i].id === id) {
        rate = RATE_OPTIONS[i].value;
        sendReadSettings();
        invalidateControls();
        status("朗读语速已设置为 " + RATE_OPTIONS[i].label + "。");
        return true;
      }
    }
    return false;
  }

  function sendReadSettings() {
    if (!isReading) {
      return;
    }
    request("/read/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ rate: rate })
    }).catch(function (error) {
      status(userMessage(error));
    });
  }

  function setReadMode(mode) {
    if (isReading) {
      status("朗读过程中不能切换朗读方式，请停止后再调整。");
      return;
    }
    readMode = mode === "page" ? "page" : "continuous";
    invalidateControls();
    status(readMode === "page" ? "已切换为当页朗读。" : "已切换为连页朗读。");
  }

  function invalidateControls() {
    var ui = window.__wpsReadAloudRibbon;
    if (!ui || typeof ui.InvalidateControl !== "function") {
      return;
    }
    [
      "startSpeak",
      "stopSpeak",
      "modeMenu",
      "rateMenu",
      "modeContinuousItem",
      "modePageItem",
      "rate075",
      "rate10",
      "rate12",
      "rate15",
      "checkStatus",
      "aboutAddin"
    ].forEach(function (id) {
      try {
        ui.InvalidateControl(id);
      } catch (_) {}
    });
  }

  function onGetPressed(control) {
    return false;
  }

  function onGetEnabled(control) {
    var id = controlId(control);
    if (id === "startSpeak") {
      return !isReading;
    }
    if (id === "stopSpeak") {
      return isReading;
    }
    if (id === "modeMenu" || id === "rateMenu" || id === "checkStatus" || id === "aboutAddin") {
      return !isReading;
    }
    return true;
  }

  function onGetLabel(control) {
    var id = controlId(control);
    if (id === "modeMenu") {
      return "朗读方式 " + (readMode === "page" ? "当页朗读" : "连页朗读");
    }
    if (id === "modeContinuousItem") {
      return (readMode === "continuous" ? "✓ " : "") + "连页朗读";
    }
    if (id === "modePageItem") {
      return (readMode === "page" ? "✓ " : "") + "当页朗读";
    }
    if (id === "rateMenu") {
      return "朗读语速 " + rateLabelForValue(rate);
    }
    for (var i = 0; i < RATE_OPTIONS.length; i += 1) {
      if (RATE_OPTIONS[i].id === id) {
        return (rateIdForValue(rate) === id ? "✓ " : "") + RATE_OPTIONS[i].label;
      }
    }
    return "";
  }

  async function onCheckStatus() {
    try {
      var health = await request("/health");
      if (!health.version) {
        notify("本地朗读服务版本较旧或尚未重启。请重新安装最新安装包，或重启 wps-tts.service 后再打开 WPS。", "服务状态", "warning");
        return;
      }
      if (health.ok) {
        var probe = health.audio_probe || {};
        var probeResults = probe.results || [];
        var probeSummary = probeResults.length
          ? "已检测 " + probeResults.length + " 个候选播放器，当前使用 " + (health.audio_player || "未检测到") + "。"
          : "已完成播放器探测。";
        showDialog({
          title: "服务状态",
          variant: "success",
          width: 860,
          height: 600,
          message: "本地朗读服务运行正常",
          fields: [
            { label: "服务版本", value: health.version },
            { label: "语音引擎", value: health.engine || "未知" },
            { label: "当前播放器", value: health.audio_player || "未检测到" },
            { label: "探测时间", value: probe.probed_at || "尚未探测" },
            { label: "探测摘要", value: probeSummary }
          ]
        });
      } else {
        notify("本地朗读服务已启动，但语音引擎不可用。请联系管理员重新安装。", "服务状态", "error");
      }
    } catch (error) {
      notify(userMessage(error), "服务状态", "error");
    }
  }

  function onAbout() {
    showDialog({
      title: "WPS 文档朗读助手",
      variant: "info",
      about: true,
      width: 960,
      height: 720,
      message: "面向 WPS Office 的本地离线文档朗读加载项。",
      fields: [
        { label: "版本", value: "1.0.31" },
        { label: "发布日期", value: "20260519" },
        { label: "开发者", value: "Zhang Jingyao" },
        { label: "软件包", value: "wps-read-aloud-comate" },
        { label: "适用操作系统", value: "x86/x64 Windows；银河麒麟 x64/ARM64；UOS x64/ARM64；以及兼容 WPS JS 加载项和本地离线服务的同类系统" },
        { label: "Windows 平台", value: "WPS Office 2019 或更高版本，推荐 WPS Office 最新稳定版" },
        { label: "Linux 平台", value: "WPS Office 2019 或更高版本，推荐最新版 WPS Office for Linux" },
        { label: "服务地址", value: "127.0.0.1:19860" },
        { label: "版权", value: "Copyright © 2026 Zhang Jingyao" },
        { label: "开源组件", value: "本软件包含第三方开源组件，相关版权和许可见第三方声明。" }
      ],
      links: [
        { label: "发布说明", url: SERVICE_BASE + "/docs/RELEASE_NOTES.md" },
        { label: "第三方声明", url: SERVICE_BASE + "/docs/THIRD_PARTY_NOTICES.md" }
      ]
    });
  }

  function onStartSpeak() {
    try {
      speakSource(currentSource());
    } catch (error) {
      notify(userMessage(error));
    }
  }

  function onStopSpeak() {
    stopPlayback(false);
  }

  function onRibbonAction(control) {
    var id = controlId(control);
    if (id === "startSpeak") {
      onStartSpeak();
      return;
    }
    if (id === "stopSpeak") {
      onStopSpeak();
      return;
    }
    if (id === "modeContinuousItem") {
      setReadMode("continuous");
      return;
    }
    if (id === "modePageItem") {
      setReadMode("page");
      return;
    }
    if (setRateById(id)) {
      return;
    }
    if (id === "checkStatus") {
      onCheckStatus();
      return;
    }
    if (id === "aboutAddin") {
      onAbout();
      return;
    }
    notify("未识别的文档朗读按钮：" + (id || "未知按钮") + "。");
  }

  function onAddinLoad(ribbonUI) {
    window.__wpsReadAloudRibbon = ribbonUI || null;
    status("文档朗读加载项已初始化。");
  }

  window.onStartSpeak = onStartSpeak;
  window.onStopSpeak = onStopSpeak;
  window.onAction = onRibbonAction;
  window.OnAction = onRibbonAction;
  window.onAddinLoad = onAddinLoad;
  window.OnAddinLoad = onAddinLoad;
  window.GetImage = onGetImage;
  window.OnGetImage = onGetImage;
  window.GetPressed = onGetPressed;
  window.OnGetPressed = onGetPressed;
  window.GetEnabled = onGetEnabled;
  window.OnGetEnabled = onGetEnabled;
  window.GetLabel = onGetLabel;
  window.OnGetLabel = onGetLabel;
  window.ribbon = {
    OnAddinLoad: onAddinLoad,
    OnAction: onRibbonAction,
    GetImage: onGetImage,
    OnGetImage: onGetImage,
    GetPressed: onGetPressed,
    OnGetPressed: onGetPressed,
    GetEnabled: onGetEnabled,
    OnGetEnabled: onGetEnabled,
    GetLabel: onGetLabel,
    OnGetLabel: onGetLabel,
    OnStartSpeak: onStartSpeak,
    OnStopSpeak: onStopSpeak,
    OnCheckStatus: onCheckStatus,
    OnAbout: onAbout
  };
  window.onCheckStatus = onCheckStatus;
  window.onAbout = onAbout;
  window.onGetImage = onGetImage;
  window.onGetPressed = onGetPressed;
  window.onGetEnabled = onGetEnabled;
  window.onGetLabel = onGetLabel;
})();

