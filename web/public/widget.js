/*
 * CallCenter widget loader.
 *
 *   <script src="https://YOUR-CALLCENTER-DOMAIN/widget.js" data-widget-key="wk_..." async></script>
 *
 * Draws a "Call us" button and opens the call window in an isolated iframe. Everything the caller
 * does happens inside that iframe; this script never touches the host page beyond its own
 * Shadow DOM root.
 */
(function () {
  "use strict";

  var script = document.currentScript;
  if (!script) return;
  var key = script.getAttribute("data-widget-key");
  if (!key || !/^wk_[A-Za-z0-9_-]{22}$/.test(key)) {
    console.warn("[CallCenter] widget.js: missing or invalid data-widget-key");
    return;
  }
  var registry = (window.__callcenterWidgets = window.__callcenterWidgets || {});
  if (registry[key]) return; // already loaded on this page
  registry[key] = true;

  var base = new URL(script.src).origin;
  var widgetUrl = base + "/widget/" + encodeURIComponent(key);

  fetch(widgetUrl + "/config", { credentials: "omit" })
    .then(function (res) { return res.ok ? res.json() : null; })
    .then(function (cfg) { if (cfg) mount(cfg); })
    .catch(function () { /* widget unavailable: render nothing */ });

  function mount(cfg) {
    var color = /^#[0-9a-f]{6}$/.test(cfg.accent_color) ? cfg.accent_color : "#4f46e5";
    var host = document.createElement("div");
    host.setAttribute("data-callcenter-widget", "");
    var root = host.attachShadow({ mode: "closed" });

    var style = document.createElement("style");
    style.textContent =
      ":host{all:initial}" +
      ".btn{position:fixed;right:20px;bottom:20px;z-index:2147483646;border:0;border-radius:9999px;padding:14px 20px;" +
      "font:600 15px/1 system-ui,-apple-system,sans-serif;color:#fff;cursor:pointer;box-shadow:0 8px 24px rgba(0,0,0,.2)}" +
      ".panel{position:fixed;right:20px;bottom:84px;z-index:2147483647;width:360px;height:560px;max-height:calc(100vh - 104px);" +
      "border-radius:14px;overflow:hidden;background:#fff;box-shadow:0 16px 48px rgba(0,0,0,.25);display:none;flex-direction:column}" +
      ".panel.open{display:flex}" +
      ".bar{display:flex;justify-content:flex-end;padding:4px;background:#f1f5f9}" +
      ".close{border:0;background:transparent;font:500 13px system-ui,sans-serif;color:#334155;cursor:pointer;padding:6px 10px}" +
      "iframe{border:0;width:100%;flex:1}" +
      "@media (max-width:480px){.panel{right:0;bottom:0;width:100%;height:100%;max-height:none;border-radius:0}}";

    var button = document.createElement("button");
    button.className = "btn";
    button.type = "button";
    button.style.background = color;
    button.textContent = String(cfg.button_label || "Call us");

    var panel = document.createElement("div");
    panel.className = "panel";
    panel.setAttribute("role", "dialog");
    panel.setAttribute("aria-label", button.textContent);

    var bar = document.createElement("div");
    bar.className = "bar";
    var close = document.createElement("button");
    close.className = "close";
    close.type = "button";
    close.textContent = "Minimise";
    bar.appendChild(close);
    panel.appendChild(bar);

    var iframe = null;
    button.addEventListener("click", function () {
      // The iframe is created on first open and then only hidden, so an ongoing call survives minimising.
      if (!iframe) {
        iframe = document.createElement("iframe");
        iframe.src = widgetUrl;
        iframe.title = button.textContent;
        iframe.allow = "microphone; autoplay";
        panel.appendChild(iframe);
      }
      panel.classList.toggle("open");
    });
    close.addEventListener("click", function () { panel.classList.remove("open"); });

    root.appendChild(style);
    root.appendChild(panel);
    root.appendChild(button);
    document.body.appendChild(host);
  }
})();
