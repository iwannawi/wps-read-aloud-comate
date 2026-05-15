(function () {
  "use strict";

  function fromBase64(text) {
    try {
      return decodeURIComponent(escape(atob(text || "")));
    } catch (_) {
      return "{}";
    }
  }

  function text(value) {
    return value === undefined || value === null ? "" : String(value);
  }

  function appendText(parent, tag, value, className) {
    var el = document.createElement(tag);
    if (className) {
      el.className = className;
    }
    el.textContent = text(value);
    parent.appendChild(el);
    return el;
  }

  function render() {
    var params = new URLSearchParams(window.location.search);
    var payload = {};
    try {
      payload = JSON.parse(fromBase64(params.get("payload")));
    } catch (_) {
      payload = {};
    }

    document.title = payload.title || "文档朗读";
    document.getElementById("dialogTitle").textContent = payload.title || "文档朗读";
    document.getElementById("dialogMessage").textContent = payload.message || "";

    var icon = document.getElementById("dialogIcon");
    icon.className = "dialog-icon " + (payload.variant || "info");
    icon.textContent = payload.variant === "success" ? "✓" : payload.variant === "warning" ? "!" : payload.variant === "error" ? "×" : "i";

    var fields = payload.fields || [];
    if (fields.length) {
      document.getElementById("fieldsSection").hidden = false;
      var dl = document.getElementById("fields");
      fields.forEach(function (item) {
        appendText(dl, "dt", item.label);
        appendText(dl, "dd", item.value);
      });
    }

    var details = payload.details || [];
    if (details.length) {
      document.getElementById("detailsSection").hidden = false;
      var detailsEl = document.getElementById("details");
      details.forEach(function (item) {
        var row = document.createElement("div");
        row.className = "detail-item";
        appendText(row, "span", (item.name || "播放器") + (item.message ? "：" + item.message : ""));
        appendText(row, "span", item.status || "", "detail-status");
        detailsEl.appendChild(row);
      });
    }

    var links = payload.links || [];
    if (links.length) {
      document.getElementById("linksSection").hidden = false;
      var linksEl = document.getElementById("links");
      links.forEach(function (item) {
        var row = document.createElement("div");
        row.className = "link-item";
        appendText(row, "span", item.label);
        var a = document.createElement("a");
        a.href = item.url;
        a.target = "_blank";
        a.rel = "noopener";
        a.textContent = "打开";
        row.appendChild(a);
        linksEl.appendChild(row);
      });
    }

    document.getElementById("closeBtn").onclick = function () {
      window.close();
    };
  }

  render();
})();
