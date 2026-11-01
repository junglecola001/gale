// Gale Capture: hands downloads to the Gale app through its native messaging
// host, and cancels the browser's own download once Gale has taken it.
//
// The listener of downloads.onDeterminingFilename must call suggest()
// exactly once, on every path: a download that Gale did not take has to
// continue in the browser, and one it took must not.

const HOST = "app.gale.host";
const ADD_TIMEOUT_MS = 6000;
const PING_EVERY_MINUTES = 0.5;

const DEFAULTS = {
  enabled: true,
  minSize: 1024 * 1024,
  skipExt: "exe,msi",
};

// Chrome answers native messages in the order they were sent, so the replies
// are matched to the requests by a queue.
const pending = [];
let port = null;
let lastPingAt = 0;
let lastReply = "";

function connect() {
  if (port) return port;
  port = chrome.runtime.connectNative(HOST);
  port.onMessage.addListener((msg) => {
    const reply = pending.shift();
    if (reply) reply(msg);
  });
  port.onDisconnect.addListener(() => {
    // Consume lastError: the host being missing is expected while the user
    // has not installed it, and Chrome otherwise logs it as unchecked.
    void chrome.runtime.lastError;
    port = null;
    while (pending.length) pending.shift()({ ok: false, error: "app_unavailable" });
  });
  return port;
}

// send talks to the app, through a connection kept open in the background.
function send(message, timeoutMs) {
  return new Promise((resolve) => {
    let done = false;
    const finish = (msg) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      resolve(msg);
    };
    const timer = setTimeout(() => finish({ ok: false, error: "timeout" }), timeoutMs);
    pending.push(finish);
    try {
      connect().postMessage(message);
    } catch (e) {
      const i = pending.indexOf(finish);
      if (i >= 0) pending.splice(i, 1);
      finish({ ok: false, error: "app_unavailable" });
    }
  });
}

async function settings() {
  const stored = await chrome.storage.local.get(DEFAULTS);
  return { ...DEFAULTS, ...stored };
}

function extensionOf(url) {
  try {
    const path = new URL(url).pathname;
    const name = path.slice(path.lastIndexOf("/") + 1);
    const dot = name.lastIndexOf(".");
    return dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
  } catch (e) {
    return "";
  }
}

function basename(url) {
  try {
    const path = decodeURIComponent(new URL(url).pathname);
    const name = path.slice(path.lastIndexOf("/") + 1);
    return name || "";
  } catch (e) {
    return "";
  }
}

async function shouldCapture(item) {
  const s = await settings();
  if (!s.enabled) return false;
  if (!/^https?:/i.test(item.url || "")) return false;
  if (item.byExtensionId === chrome.runtime.id) return false;
  if (item.fileSize > 0 && item.fileSize < s.minSize) return false;
  const skip = (s.skipExt || "")
    .split(",")
    .map((e) => e.trim().toLowerCase())
    .filter(Boolean);
  if (skip.includes(extensionOf(item.url))) return false;
  return true;
}

// cookiesFor gathers the cookies a download request would carry; without
// them a members-only link downloads as a login page.
async function cookiesFor(url) {
  try {
    const list = await chrome.cookies.getAll({ url });
    return list.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch (e) {
    return "";
  }
}

async function referrerFor(item) {
  if (item.referrer) return item.referrer;
  try {
    const tabs = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
    return tabs[0] && tabs[0].url ? tabs[0].url : "";
  } catch (e) {
    return "";
  }
}

// capture asks Gale to download the file; true means it took it.
async function capture(item) {
  const reply = await send(
    {
      type: "add",
      url: item.url,
      referrer: await referrerFor(item),
      userAgent: navigator.userAgent,
      cookies: await cookiesFor(item.url),
      fileName: item.filename ? basename(`http://x/${item.filename}`) : basename(item.url),
    },
    ADD_TIMEOUT_MS,
  );
  lastReply = reply.ok ? `已交给 Gale（#${reply.id}）` : String(reply.error || "失败");
  await chrome.storage.local.set({ lastReply, lastReplyAt: Date.now() });
  if (!reply.ok) {
    await chrome.action.setBadgeText({ text: "!" });
    await chrome.action.setBadgeBackgroundColor({ color: "#c7343f" });
    chrome.notifications.create({
      type: "basic",
      iconUrl: "icons/icon128.png",
      title: "未能交给 Gale",
      message: `浏览器继续下载：${basename(item.url) || item.url}`,
    });
  }
  return Boolean(reply.ok);
}

function installFilenameListener() {
  if (!chrome.downloads.onDeterminingFilename) return false;
  chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
    let finished = false;
    const settle = () => {
      if (finished) return;
      finished = true;
      suggest();
    };
    shouldCapture(item).then(async (take) => {
      if (!take) {
        settle();
        return;
      }
      const taken = await capture(item);
      settle();
      if (taken) {
        chrome.downloads.cancel(item.id).catch(() => {});
        chrome.downloads.erase({ id: item.id }).catch(() => {});
      }
    });
    return true;
  });
  return true;
}

// Older Chromium builds lack onDeterminingFilename: there the download
// starts first and is canceled a moment later, which is still one download
// of the file.
function installCreatedFallback() {
  chrome.downloads.onCreated.addListener((item) => {
    shouldCapture(item).then(async (take) => {
      if (!take) return;
      if (await capture(item)) {
        chrome.downloads.cancel(item.id).catch(() => {});
      }
    });
  });
}

async function ping() {
  const reply = await send({ type: "ping" }, 1500);
  const ok = Boolean(reply && reply.ok);
  lastPingAt = ok ? Date.now() : 0;
  await chrome.storage.local.set({ connected: ok, lastPingAt });
  await chrome.action.setBadgeText({ text: ok ? "" : "?" });
  await chrome.action.setBadgeBackgroundColor({ color: ok ? "#35d08a" : "#9aa3b2" });
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.alarms.create("ping", { periodInMinutes: PING_EVERY_MINUTES });
  ping();
  chrome.runtime.openOptionsPage();
});

chrome.runtime.onStartup.addListener(() => ping());

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "ping") ping();
});

chrome.action.onClicked.addListener(() => chrome.runtime.openOptionsPage());

if (!installFilenameListener()) installCreatedFallback();
ping();
