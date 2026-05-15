(function () {
  "use strict";

  var SERVICE_BASE = "http://127.0.0.1:19860";
  var MAX_TEXT_LENGTH = 200000;
  var MAX_SENTENCES = 1000;
  var MAX_SENTENCE_LENGTH = 1000;
  var SENTENCE_END = /[。！？!?；;]+|[\r\n]+/g;

  var rate = 1.0;
  var volume = 80;
  var currentAbortController = null;
  var playbackToken = 0;
  var isReading = false;
  var isPaused = false;
  var lastActionAt = 0;

  function notify(message) {
    try {
      window.alert(message);
    } catch (_) {
      console.log(message);
    }
  }

  function status(message) {
    console.log("[wps-read-aloud] " + message);
  }

  function controlId(control) {
    if (typeof control === "string") {
      return control;
    }
    return (control && (control.Id || control.id || control.ID)) || "";
  }

  function onGetImage(control) {
    var icons = {
      speakSelection: "assets/icons/read-selection.png",
      speakDocument: "assets/icons/read-document.png",
      pauseSpeak: "assets/icons/pause.png",
      resumeSpeak: "assets/icons/play.png",
      stopSpeak: "assets/icons/stop.png",
      checkStatus: "assets/icons/status.png",
      aboutAddin: "assets/icons/about.png"
    };
    return icons[controlId(control)] || "assets/icons/read-selection.png";
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
    if (/NotAllowedError|play\(\)|user.*interact/i.test(raw)) {
      return "WPS 内置浏览器阻止了音频播放。请升级到使用系统播放接口的最新安装包后重试。";
    }
    if (/AbortError|aborted|timeout/i.test(raw)) {
      return "朗读合成超时，请缩短选中文本后重试。";
    }
    return raw || "操作失败，请稍后重试。";
  }

  function throttleAction() {
    var now = Date.now();
    if (now - lastActionAt < 500) {
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

  function readSelectionSource() {
    var app = getWpsApplication();
    var selection = app.Selection;
    if (!selection) {
      return { text: "", start: 0 };
    }
    var range = selection.Range || selection;
    return {
      text: range.Text !== undefined ? String(range.Text || "") : "",
      start: range.Start !== undefined ? Number(range.Start) : 0
    };
  }

  function readDocumentSource() {
    var app = getWpsApplication();
    var doc = app.ActiveDocument;
    if (!doc) {
      return { text: "", start: 0 };
    }
    var range = null;
    if (doc.Content) {
      range = doc.Content;
    } else if (doc.Range && typeof doc.Range === "function") {
      range = doc.Range();
    }
    if (!range) {
      return { text: "", start: 0 };
    }
    return {
      text: range.Text !== undefined ? String(range.Text || "") : "",
      start: range.Start !== undefined ? Number(range.Start) : 0
    };
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
      var app = getWpsApplication();
      var doc = app.ActiveDocument;
      if (!doc || !doc.Range) {
        return;
      }
      var range = doc.Range(segment.start, segment.end);
      if (range && typeof range.Select === "function") {
        range.Select();
      }
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
    } catch (error) {
      if (/301|Moved Permanently|404|page not found|<!doctype|<html/i.test(text)) {
        throw new Error("本地朗读服务版本不匹配或尚未重启。请重新安装最新安装包，或重启 wps-tts.service 后再打开 WPS。");
      }
      throw new Error("本地朗读服务返回了无法识别的数据，接口：" + path + "。请重启 WPS 和 wps-tts.service 后重试。");
    }
  }

  function clearAudio() {
    if (currentAbortController) {
      currentAbortController.abort();
      currentAbortController = null;
    }
  }

  async function postControl(path) {
    try {
      await request(path, { method: "POST" });
    } catch (error) {
      status(userMessage(error));
    }
  }

  async function playSegment(segment, token) {
    var controller = new AbortController();
    currentAbortController = controller;
    var timer = setTimeout(function () {
      controller.abort();
    }, 180000);
    try {
      var response = await fetch(SERVICE_BASE + "/play", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: controller.signal,
        body: JSON.stringify({
          text: segment.text,
          voice: "default",
          rate: rate,
          volume: volume
        })
      });
      var data = await parseJsonResponse(response, "/play");
      if (!response.ok) {
        throw new Error(data.error || response.statusText);
      }
      if (token !== playbackToken) {
        throw new Error("朗读已取消。");
      }
    } finally {
      clearTimeout(timer);
      if (currentAbortController === controller) {
        currentAbortController = null;
      }
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
      notify("没有可朗读的文本，请先选中文本或打开包含正文的文档。");
      return;
    }
    if (normalized.length > MAX_TEXT_LENGTH) {
      notify("文档内容过长，请先选中一部分内容朗读。");
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
    isPaused = false;
    isReading = true;

    try {
      for (var i = 0; i < segments.length; i += 1) {
        if (token !== playbackToken) {
          break;
        }
        while (isPaused && token === playbackToken) {
          await sleep(100);
        }
        var segment = segments[i];
        selectDocumentRange(segment);
        status("正在朗读第 " + (i + 1) + " / " + segments.length + " 句。");
        await playSegment(segment, token);
      }
      if (token === playbackToken) {
        status("朗读完成。");
      }
    } catch (error) {
      if (token === playbackToken) {
        notify(userMessage(error));
      }
    } finally {
      if (token === playbackToken) {
        isReading = false;
      }
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
    isPaused = false;
    isReading = false;
    clearAudio();
    postControl("/stop");
    if (!silent) {
      status("已停止。");
    }
  }

  function pausePlayback() {
    if (throttleAction()) {
      return;
    }
    if (!isReading) {
      notify("当前没有正在朗读的内容。");
      return;
    }
    isPaused = true;
    postControl("/pause");
    status("已暂停。");
  }

  function resumePlayback() {
    if (throttleAction()) {
      return;
    }
    isPaused = false;
    postControl("/resume");
    status("已继续。");
  }

  function onRateChanged(control, selectedId) {
    var map = {
      rate06: 0.6,
      rate08: 0.8,
      rate10: 1.0,
      rate12: 1.2,
      rate14: 1.4,
      rate16: 1.6
    };
    var id = selectedId || controlId(control);
    rate = map[id] || 1.0;
    status("语速已设置为 " + rate + "x。");
  }

  function onVolumeChanged(control, selectedId) {
    var map = {
      volume40: 40,
      volume60: 60,
      volume80: 80,
      volume100: 100
    };
    var id = selectedId || controlId(control);
    volume = map[id] || 80;
    status("音量已设置为 " + volume + "%。");
  }

  async function onCheckStatus() {
    try {
      var health = await request("/health");
      if (!health.version) {
        notify("本地朗读服务版本较旧或尚未重启。请重新安装最新安装包，或重启 wps-tts.service 后再打开 WPS。");
        return;
      }
      if (health.ok) {
        var probe = health.audio_probe || {};
        var probeInfo = probe.probed_at ? "\n探测时间：" + probe.probed_at : "";
        notify("本地朗读服务正常。\n服务版本：" + health.version + "\n当前引擎：" + health.engine + "\n当前播放器：" + (health.audio_player || "未检测到") + probeInfo);
      } else {
        notify("本地朗读服务已启动，但语音引擎不可用。请联系管理员重新安装。");
      }
    } catch (error) {
      notify(userMessage(error));
    }
  }

  function onAbout() {
    notify([
      "WPS 文档朗读加载项",
      "开发者：zhangjingyao",
      "发布时间：20260515",
      "版本：1.0.7",
      "服务地址：127.0.0.1:19860",
      "",
      "说明文件：",
      "发布说明：http://127.0.0.1:19860/docs/RELEASE_NOTES.md",
      "验收测试：http://127.0.0.1:19860/docs/ACCEPTANCE_TEST.md",
      "第三方声明：http://127.0.0.1:19860/docs/THIRD_PARTY_NOTICES.md",
      "源码说明：http://127.0.0.1:19860/docs/SOURCE_OFFER.md"
    ].join("\n"));
  }

  function onSpeakSelection() {
    try {
      speakSource(readSelectionSource());
    } catch (error) {
      notify(userMessage(error));
    }
  }

  function onSpeakDocument() {
    try {
      speakSource(readDocumentSource());
    } catch (error) {
      notify(userMessage(error));
    }
  }

  function onPauseSpeak() {
    pausePlayback();
  }

  function onResumeSpeak() {
    resumePlayback();
  }

  function onStopSpeak() {
    stopPlayback(false);
  }

  function onRibbonAction(control) {
    var id = controlId(control);
    var actions = {
      speakSelection: onSpeakSelection,
      speakDocument: onSpeakDocument,
      pauseSpeak: onPauseSpeak,
      resumeSpeak: onResumeSpeak,
      stopSpeak: onStopSpeak,
      checkStatus: onCheckStatus,
      aboutAddin: onAbout
    };
    if (actions[id]) {
      actions[id]();
      return;
    }
    notify("未识别的文档朗读按钮：" + (id || "未知按钮") + "。");
  }

  function onAddinLoad(ribbonUI) {
    window.__wpsReadAloudRibbon = ribbonUI || null;
    status("文档朗读加载项已初始化。");
  }

  window.onSpeakSelection = onSpeakSelection;
  window.onSpeakDocument = onSpeakDocument;
  window.onPauseSpeak = onPauseSpeak;
  window.onResumeSpeak = onResumeSpeak;
  window.onStopSpeak = onStopSpeak;
  window.onAction = onRibbonAction;
  window.OnAction = onRibbonAction;
  window.onAddinLoad = onAddinLoad;
  window.OnAddinLoad = onAddinLoad;
  window.GetImage = onGetImage;
  window.OnGetImage = onGetImage;
  window.ribbon = {
    OnAddinLoad: onAddinLoad,
    OnAction: onRibbonAction,
    GetImage: onGetImage,
    OnGetImage: onGetImage,
    OnSpeakSelection: onSpeakSelection,
    OnSpeakDocument: onSpeakDocument,
    OnPauseSpeak: onPauseSpeak,
    OnResumeSpeak: onResumeSpeak,
    OnStopSpeak: onStopSpeak,
    OnRateChanged: onRateChanged,
    OnVolumeChanged: onVolumeChanged,
    OnCheckStatus: onCheckStatus,
    OnAbout: onAbout
  };
  window.onRateChanged = onRateChanged;
  window.onVolumeChanged = onVolumeChanged;
  window.onCheckStatus = onCheckStatus;
  window.onAbout = onAbout;
  window.onGetImage = onGetImage;
})();
