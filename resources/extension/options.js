// The extension's options page: three settings and the connection state.

const DEFAULTS = { enabled: true, minSize: 1024 * 1024, skipExt: "exe,msi" };
const HOST = "app.gale.host";

const $ = (id) => document.getElementById(id);

async function load() {
  const s = await chrome.storage.local.get({ ...DEFAULTS, connected: false, lastPingAt: 0 });
  $("enabled").checked = s.enabled;
  $("minSize").value = Math.round(s.minSize / (1024 * 1024));
  $("skipExt").value = s.skipExt;
  showStatus(s.connected, s.lastPingAt);
}

function showStatus(connected, at) {
  const el = $("status");
  if (connected && at) {
    el.className = "status ok";
    el.textContent = `已连接 Gale（${new Date(at).toLocaleTimeString()} 心跳）`;
    return;
  }
  el.className = "status bad";
  el.textContent = "未连接：请确认 Gale 正在运行，并在 Gale 设置里启用浏览器集成。";
}

async function save() {
  const minSize = Math.max(0, Number($("minSize").value || 0)) * 1024 * 1024;
  await chrome.storage.local.set({
    enabled: $("enabled").checked,
    minSize,
    skipExt: $("skipExt").value,
  });
}

// test sends one ping through the native host, which does not start Gale:
// a ping says whether the app is there, not make it be.
function test() {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (ok, note) => {
      if (settled) return;
      settled = true;
      $("status").className = ok ? "status ok" : "status bad";
      $("status").textContent = note;
      resolve();
    };
    const timer = setTimeout(() => finish(false, "Gale 没有应答（宿主已在但应用未运行）"), 3000);
    try {
      const port = chrome.runtime.connectNative(HOST);
      port.onMessage.addListener((msg) => {
        clearTimeout(timer);
        port.disconnect();
        if (msg && msg.ok) finish(true, `已连接 Gale ${msg.version || ""}`.trim());
        else finish(false, "Gale 未运行，或原生宿主未安装");
      });
      port.onDisconnect.addListener(() => {
        clearTimeout(timer);
        // Reading lastError here also consumes it, so Chrome does not log an
        // unchecked error next to the message we show.
        const message = chrome.runtime.lastError?.message;
        finish(false, message ? `原生宿主连接失败：${message}` : "原生宿主未安装：请在 Gale 设置中点击“安装/修复浏览器集成”");
      });
      port.postMessage({ type: "ping" });
    } catch (e) {
      clearTimeout(timer);
      finish(false, e?.message ? `原生宿主连接失败：${e.message}` : "原生宿主未安装：请在 Gale 设置中点击“安装/修复浏览器集成”");
    }
  });
}

$("enabled").addEventListener("change", save);
$("minSize").addEventListener("change", save);
$("skipExt").addEventListener("change", save);
$("test").addEventListener("click", test);

chrome.storage.onChanged.addListener((changes) => {
  if (changes.connected || changes.lastPingAt) {
    chrome.storage.local.get({ connected: false, lastPingAt: 0 }).then((s) => showStatus(s.connected, s.lastPingAt));
  }
});

load();
