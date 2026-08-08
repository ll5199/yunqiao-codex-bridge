(() => {
  const incoming = Array.isArray(window.__YUNQIAO_INJECT_MODELS__)
    ? window.__YUNQIAO_INJECT_MODELS__.filter((x) => typeof x === "string" && x.trim()).map((x) => x.trim())
    : [];
  window.__yunqiaoCodexModels = Array.from(new Set(incoming));
  window.__yunqiaoCodexDefaultModel = String(window.__YUNQIAO_INJECT_DEFAULT__ || window.__yunqiaoCodexModels[0] || "");

  if (window.__yunqiaoCodexBridgeInstalled === "1.3.3") {
    window.__yunqiaoCodexBridgeRefresh?.();
    return;
  }
  window.__yunqiaoCodexBridgeInstalled = "1.3.3";

  function installChineseLocale() {
    const locale = "zh-CN";
    const languages = ["zh-CN", "zh", "en-US", "en"];
    try {
      document.documentElement.lang = locale;
    } catch {
    }
    const defineNavigatorGetter = (name, value) => {
      try {
        Object.defineProperty(Navigator.prototype, name, {
          configurable: true,
          get: () => value,
        });
      } catch {
        try {
          Object.defineProperty(navigator, name, {
            configurable: true,
            get: () => value,
          });
        } catch {
        }
      }
    };
    defineNavigatorGetter("language", locale);
    defineNavigatorGetter("languages", languages);

    const waitForBridge = () => new Promise((resolve) => {
      const started = Date.now();
      const check = () => {
        const bridge = window.electronBridge;
        if (bridge && typeof bridge.sendMessageFromView === "function") return resolve(bridge);
        if (Date.now() - started > 6000) return resolve(null);
        setTimeout(check, 50);
      };
      check();
    });

    const callSetting = (bridge, method, params) => new Promise((resolve, reject) => {
      const requestId = typeof crypto?.randomUUID === "function"
        ? crypto.randomUUID()
        : `yunqiao-locale-${Date.now()}-${Math.random().toString(16).slice(2)}`;
      let timer;
      const cleanup = () => {
        clearTimeout(timer);
        window.removeEventListener("message", onMessage);
      };
      const onMessage = (event) => {
        const message = event?.data;
        if (message?.type !== "fetch-response" || message.requestId !== requestId) return;
        cleanup();
        if (message.responseType !== "success") return reject(new Error(message.error || "locale setting failed"));
        try {
          resolve(JSON.parse(message.bodyJsonString || "null"));
        } catch (error) {
          reject(error);
        }
      };
      window.addEventListener("message", onMessage);
      timer = setTimeout(() => {
        cleanup();
        reject(new Error("locale setting timeout"));
      }, 5000);
      Promise.resolve(bridge.sendMessageFromView({
        type: "fetch",
        requestId,
        method: "POST",
        url: `vscode://codex/${method}`,
        body: JSON.stringify({ params }),
      })).catch((error) => {
        cleanup();
        reject(error);
      });
    });

    const localeSyncStarted = Date.now();
    const syncOfficialLocale = async () => {
      try {
        const bridge = await waitForBridge();
        if (!bridge) throw new Error("electron bridge unavailable");
        const response = await callSetting(bridge, "get-setting", { key: "localeOverride" });
        if ((response?.value ?? null) === locale) return;
        await callSetting(bridge, "set-setting", { key: "localeOverride", value: locale });
        const marker = "zh-CN";
        if (sessionStorage.getItem("yunqiao.locale.reload") !== marker) {
          sessionStorage.setItem("yunqiao.locale.reload", marker);
          window.location.reload();
        }
      } catch {
        if (Date.now() - localeSyncStarted < 60000) setTimeout(syncOfficialLocale, 1500);
      }
    };
    void syncOfficialLocale();

    const patchI18nConfig = (config) => {
      if (!config || typeof config !== "object") return config;
      const value = config.value && typeof config.value === "object" ? config.value : {};
      try {
        config.value = { ...value, enable_i18n: true, locale_source: "SYSTEM" };
      } catch {
      }
      if (typeof config.get === "function" && !config.__yunqiaoI18nGetPatched) {
        const originalGet = config.get.bind(config);
        config.get = (key, fallback) => {
          if (key === "enable_i18n") return true;
          if (key === "locale_source") return "SYSTEM";
          return originalGet(key, fallback);
        };
        config.__yunqiaoI18nGetPatched = true;
      }
      return config;
    };

    const patchI18nClient = (client) => {
      if (!client || typeof client.getDynamicConfig !== "function") return;
      if (!client.__yunqiaoI18nPatched) {
        const original = client.getDynamicConfig.bind(client);
        client.getDynamicConfig = (name, options) => {
          const config = original(name, options);
          return name === "72216192" ? patchI18nConfig(config) : config;
        };
        client.__yunqiaoI18nPatched = true;
      }
      try {
        patchI18nConfig(client.getDynamicConfig("72216192", { disableExposureLog: true }));
      } catch {
      }
    };

    const patchI18nRoot = (root) => {
      if (!root || typeof root !== "object") return;
      const clients = [root.firstInstance, typeof root.instance === "function" ? root.instance() : null];
      if (root.instances && typeof root.instances === "object") clients.push(...Object.values(root.instances));
      clients.filter(Boolean).forEach(patchI18nClient);
    };

    const installStatsigSetter = () => {
      const descriptor = Object.getOwnPropertyDescriptor(window, "__STATSIG__");
      if (descriptor && descriptor.configurable === false) {
        patchI18nRoot(window.__STATSIG__);
        return;
      }
      let root = window.__STATSIG__;
      patchI18nRoot(root);
      try {
        Object.defineProperty(window, "__STATSIG__", {
          configurable: true,
          get: () => root,
          set: (next) => {
            root = next;
            patchI18nRoot(next);
          },
        });
      } catch {
      }
    };

    installStatsigSetter();
    const started = Date.now();
    const timer = setInterval(() => {
      patchI18nRoot(window.__STATSIG__ || globalThis.__STATSIG__);
      if (Date.now() - started > 60000) clearInterval(timer);
    }, 100);
  }

  installChineseLocale();

  const names = () => Array.isArray(window.__yunqiaoCodexModels) ? window.__yunqiaoCodexModels : [];
  const descriptor = (model) => ({
    model,
    id: model,
    slug: model,
    name: model,
    displayName: model,
    description: "Yunqiao API",
    hidden: false,
    isDefault: model === window.__yunqiaoCodexDefaultModel,
    defaultReasoningEffort: "medium",
    supportedReasoningEfforts: ["minimal", "low", "medium", "high", "xhigh"].map((reasoningEffort) => ({
      reasoningEffort,
      description: `${reasoningEffort} effort`,
    })),
  });

  function patchNameArray(value) {
    if (!Array.isArray(value) || !value.every((item) => typeof item === "string")) return false;
    let changed = false;
    for (const model of names()) {
      if (!value.includes(model)) {
        value.push(model);
        changed = true;
      }
    }
    return changed;
  }

  function patchModelArray(value, allowEmpty = false) {
    if (!Array.isArray(value) || (!allowEmpty && value.length === 0)) return false;
    if (value.length && !value.every((item) => item && typeof item === "object" && typeof item.model === "string")) {
      return false;
    }
    let changed = false;
    const existing = new Map(value.map((item) => [item.model, item]));
    for (const item of value) {
      if (names().includes(item.model) && item.hidden !== false) {
        item.hidden = false;
        changed = true;
      }
    }
    for (const model of names()) {
      if (!existing.has(model)) {
        value.push(descriptor(model));
        changed = true;
      }
    }
    return changed;
  }

  function patchContainer(value) {
    if (!value || typeof value !== "object") return false;
    let changed = false;
    if (Array.isArray(value)) {
      return false;
    }
    if (patchModelArray(value.models, "defaultModel" in value || "availableModels" in value)) changed = true;
    if (patchNameArray(value.models)) changed = true;
    for (const key of ["data", "result"]) {
      if (patchModelArray(value[key])) changed = true;
    }
    if (patchModelArray(value.pages?.[0]?.data)) changed = true;
    if (patchModelArray(value.result?.data)) changed = true;
    if (patchModelArray(value.result?.models)) changed = true;
    if (patchModelArray(value.message?.result?.data)) changed = true;
    if (patchModelArray(value.message?.result?.models)) changed = true;
    for (const key of ["availableModels", "available_models"]) {
      const list = value[key];
      if (Array.isArray(list)) {
        for (const model of names()) {
          if (!list.includes(model)) {
            list.push(model);
            changed = true;
          }
        }
      } else if (list instanceof Set) {
        for (const model of names()) {
          if (!list.has(model)) {
            list.add(model);
            changed = true;
          }
        }
      }
    }
    for (const key of ["hiddenModels", "hidden_models"]) {
      if (Array.isArray(value[key])) {
        const filtered = value[key].filter((model) => !names().includes(model));
        if (filtered.length !== value[key].length) {
          value[key] = filtered;
          changed = true;
        }
      }
    }
    if (value.defaultModel == null && names().length) {
      value.defaultModel = descriptor(window.__yunqiaoCodexDefaultModel || names()[0]);
      changed = true;
    }
    return changed;
  }

  function patchGraph(root, visited = new WeakSet(), depth = 0) {
    if (!root || typeof root !== "object" || visited.has(root) || depth > 6) return false;
    visited.add(root);
    let changed = patchContainer(root);
    if (root instanceof Element || root === window || root === document) return changed;
    for (const key of Object.keys(root)) {
      if (["ownerDocument", "parentElement", "parentNode", "children", "childNodes"].includes(key)) continue;
      let value;
      try {
        value = root[key];
      } catch {
        continue;
      }
      if (value && typeof value === "object" && patchGraph(value, visited, depth + 1)) changed = true;
    }
    return changed;
  }

  const capturedImages = window.__yunqiaoGeneratedImages = window.__yunqiaoGeneratedImages || [];
  const capturedImageKeys = window.__yunqiaoGeneratedImageKeys = window.__yunqiaoGeneratedImageKeys || new Set();
  const renderedImageKeys = window.__yunqiaoRenderedImageKeys = window.__yunqiaoRenderedImageKeys || new Set();
  let lastConversationKey = "";
  const draftKey = sessionStorage.getItem("yunqiao.conversation.draft") ||
    `draft-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  sessionStorage.setItem("yunqiao.conversation.draft", draftKey);

  function currentConversationKey() {
    const selected = document.querySelector(
      "[data-app-action-sidebar-thread-id][aria-current='page'], " +
      "[data-app-action-sidebar-thread-id][data-state='active'], " +
      "[data-app-action-sidebar-thread-id][aria-selected='true']"
    );
    const selectedID = selected?.getAttribute?.("data-app-action-sidebar-thread-id");
    if (selectedID) return `thread:${selectedID}`;
    const route = `${location.pathname}${location.search}`;
    const match = route.match(/(?:thread|conversation|session|task)[/=:]([A-Za-z0-9_.-]+)/i);
    if (match?.[1]) return `thread:${match[1]}`;
    return `draft:${draftKey}`;
  }

  async function associateImages(images, conversationKey) {
    const imageIDs = images.map((image) => image.id).filter(Boolean);
    if (!conversationKey || !imageIDs.length) return;
    try {
      await fetch("http://127.0.0.1:9230/yunqiao/associate", {
        method: "POST",
        cache: "no-store",
        credentials: "omit",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ image_ids: imageIDs, conversation_key: conversationKey }),
      });
      for (const image of images) image.conversationKey = conversationKey;
    } catch {
    }
  }

  function looksLikeBase64(value) {
    return typeof value === "string" && value.length > 256 &&
      /^[A-Za-z0-9+/=\r\n]+$/.test(value);
  }

  function addCapturedImage(source, metadata = {}) {
    if (typeof source !== "string" || !source) return;
    const key = metadata.id || (source.length > 512 ? `${source.slice(0, 256)}:${source.length}` : source);
    if (capturedImageKeys.has(key)) return;
    capturedImageKeys.add(key);
    capturedImages.push({
      key,
      id: metadata.id || "",
      source,
      downloadURL: metadata.download_url || metadata.downloadURL || source,
      fileName: metadata.file_name || metadata.fileName || "yunqiao-image.png",
      conversationKey: metadata.conversation_key || metadata.conversationKey || "",
      createdAt: Number(metadata.created_at || metadata.createdAt || Date.now()),
    });
    if (capturedImages.length > 20) capturedImages.splice(0, capturedImages.length - 20);
  }

  window.__yunqiaoAcceptProxyImages = (images) => {
    const activeKey = currentConversationKey();
    const newlyCaptured = [];
    for (const image of Array.isArray(images) ? images : []) {
      if (typeof image?.source !== "string") continue;
      const belongsHere = image.conversation_key === activeKey;
      const unassignedAndNew = !image.conversation_key;
      if (!belongsHere && !unassignedAndNew) continue;
      addCapturedImage(image.source, image);
      const added = capturedImages.find((item) => item.id && item.id === image.id);
      if (added && unassignedAndNew) newlyCaptured.push(added);
    }
    if (newlyCaptured.length) void associateImages(newlyCaptured, activeKey);
    renderCapturedImages();
    return capturedImages.length;
  };

  function captureImages(root, visited = new WeakSet(), depth = 0, parentType = "") {
    if (!root || depth > 9) return;
    if (typeof root === "string") {
      if (root.startsWith("data:image/")) addCapturedImage(root);
      return;
    }
    if (typeof root !== "object" || visited.has(root) || root instanceof Element) return;
    visited.add(root);
    const type = String(root.type || root.kind || parentType || "").toLowerCase();
    const mimeType = String(root.mime_type || root.mimeType || "").toLowerCase();
    for (const [key, value] of Object.entries(root)) {
      const lowerKey = key.toLowerCase();
      if (typeof value === "string") {
        if (value.startsWith("data:image/")) {
          addCapturedImage(value);
          continue;
        }
        const imageKey = ["image_url", "imageurl", "b64_json", "b64json", "image_base64"].includes(lowerKey);
        const imageType = type.includes("image_generation") || type === "output_image" ||
          type === "image" || type === "images" || mimeType.startsWith("image/");
        if ((lowerKey === "b64_json" || lowerKey === "image_base64" || (lowerKey === "result" && imageType) ||
            (lowerKey === "data" && imageType)) && looksLikeBase64(value.replace(/\s/g, ""))) {
          addCapturedImage(`data:${mimeType || "image/png"};base64,${value.replace(/\s/g, "")}`);
          continue;
        }
        if ((imageKey || imageType) && /^https?:\/\//i.test(value)) {
          addCapturedImage(value);
          continue;
        }
        if ((lowerKey === "url" || lowerKey === "src") && /^https?:\/\/.+\.(png|jpe?g|webp|gif)(\?|$)/i.test(value)) {
          addCapturedImage(value);
        }
      } else if (value && typeof value === "object") {
        const childType = lowerKey.includes("image") || lowerKey === "inline_data" || lowerKey === "inlinedata"
          ? "image"
          : type;
        captureImages(value, visited, depth + 1, childType);
      }
    }
  }

  function visibleConversationNodes(selector) {
    const conversationRoot = document.querySelector("main") ||
      document.querySelector("[data-testid*='conversation'], [data-testid*='thread']") ||
      document.body;
    if (!conversationRoot) return [];
    return Array.from(conversationRoot.querySelectorAll(selector)).filter((node) => {
      if (node.closest?.("aside, [data-yunqiao-image-key]")) return false;
      const rect = node.getBoundingClientRect?.();
      return !rect || (rect.width > 0 && rect.height > 0);
    });
  }

  function conversationImageMount() {
    const assistant = visibleConversationNodes(
      "[data-message-author-role='assistant'], [data-testid*='assistant-message']"
    );
    if (assistant.length) return { mode: "append", target: assistant.at(-1) };

    const turns = visibleConversationNodes("[data-testid='conversation-turn']");
    if (turns.length) return { mode: "append", target: turns.at(-1) };

    const prose = visibleConversationNodes(".prose");
    if (prose.length) return { mode: "append", target: prose.at(-1) };

    const conversationRoot = document.querySelector("main") ||
      document.querySelector("[data-testid*='conversation'], [data-testid*='thread']") ||
      document.body;
    const composer = conversationRoot?.querySelector(
      ".composer-footer, [data-testid*='composer'], form:has(textarea), form:has([contenteditable='true']), textarea, [contenteditable='true'], .ProseMirror"
    );
    if (!composer) return { mode: "overlay", target: document.body };
    const anchor = composer.closest?.(".composer-footer, form, [data-testid*='composer']")
      || composer;
    if (!anchor.parentElement) return { mode: "overlay", target: document.body };
    return { mode: "before", target: anchor };
  }

  function mountImageCard(mount, card) {
    if (mount?.mode === "append") {
      mount.target.appendChild(card);
      return true;
    }
    if (mount?.mode === "before" && mount.target?.parentElement) {
      mount.target.parentElement.insertBefore(card, mount.target);
      return true;
    }
    if (mount?.mode === "overlay" && mount.target) {
      let tray = document.querySelector("[data-yunqiao-image-tray]");
      if (!tray) {
        tray = document.createElement("div");
        tray.dataset.yunqiaoImageTray = "true";
        tray.style.cssText = "position:fixed;right:20px;bottom:86px;z-index:2147483640;width:min(520px,calc(100vw - 40px));max-height:70vh;overflow:auto;padding:4px";
        mount.target.appendChild(tray);
      }
      tray.appendChild(card);
      return true;
    }
    return null;
  }

  function focusConversationComposer() {
    const composer = document.querySelector(
      "main textarea, main [contenteditable='true'], main .ProseMirror, main [data-testid*='composer']"
    );
    composer?.scrollIntoView?.({ behavior: "smooth", block: "center" });
    setTimeout(() => composer?.focus?.(), 250);
  }

  function renderCapturedImages() {
    const activeKey = currentConversationKey();
    if (!capturedImages.length) return;
    for (const card of document.querySelectorAll("[data-yunqiao-image-key]")) {
      const image = capturedImages.find((item) => item.key.slice(-80) === card.dataset.yunqiaoImageKey);
      card.hidden = !!image?.conversationKey && image.conversationKey !== activeKey;
    }
    const mount = conversationImageMount();
    if (!mount) return;
    for (const image of capturedImages) {
      if (image.conversationKey && image.conversationKey !== activeKey) continue;
      const renderKey = image.key.slice(-80);
      if (renderedImageKeys.has(renderKey) ||
          document.querySelector(`[data-yunqiao-image-key="${CSS.escape(renderKey)}"]`)) continue;
      const card = document.createElement("div");
      card.dataset.yunqiaoImageKey = renderKey;
      card.style.cssText = "box-sizing:border-box;width:min(100%,760px);margin:12px 0;padding:10px;border:1px solid rgba(127,127,127,.35);border-radius:10px;background:rgba(127,127,127,.06)";
      const header = document.createElement("div");
      header.style.cssText = "display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:8px";
      const title = document.createElement("div");
      title.textContent = "生成的图片";
      title.style.cssText = "font-size:12px;opacity:.75";
      const collapseButton = document.createElement("button");
      collapseButton.type = "button";
      collapseButton.textContent = "折叠图片";
      collapseButton.style.cssText = "border:0;background:transparent;color:inherit;cursor:pointer;font-size:12px;opacity:.8";
      header.append(title, collapseButton);
      const body = document.createElement("div");
      const element = document.createElement("img");
      element.src = image.source;
      element.alt = "生成的图片";
      element.loading = "eager";
      element.style.cssText = "display:block;width:auto;max-width:100%;max-height:480px;border-radius:8px;object-fit:contain";
      const actions = document.createElement("div");
      actions.style.cssText = "display:flex;align-items:center;gap:14px;margin-top:8px;font-size:12px";
      const downloadButton = document.createElement("button");
      downloadButton.type = "button";
      downloadButton.textContent = "下载原图";
      downloadButton.style.cssText = "border:0;padding:0;background:transparent;color:inherit;cursor:pointer;font-size:12px;text-decoration:underline";
      downloadButton.addEventListener("click", async () => {
        const originalText = downloadButton.textContent;
        downloadButton.disabled = true;
        downloadButton.textContent = "正在下载…";
        try {
          const response = await fetch(image.downloadURL || image.source, { cache: "no-store", credentials: "omit" });
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          const blob = await response.blob();
          const objectURL = URL.createObjectURL(blob);
          const anchor = document.createElement("a");
          anchor.href = objectURL;
          anchor.download = image.fileName || "yunqiao-image.png";
          anchor.style.display = "none";
          document.body.appendChild(anchor);
          anchor.click();
          anchor.remove();
          setTimeout(() => URL.revokeObjectURL(objectURL), 30000);
          downloadButton.textContent = "已开始下载";
        } catch {
          downloadButton.textContent = "下载失败，请重试";
        } finally {
          setTimeout(() => {
            downloadButton.disabled = false;
            downloadButton.textContent = originalText;
          }, 1800);
        }
      });
      const continueButton = document.createElement("button");
      continueButton.type = "button";
      continueButton.textContent = "继续对话";
      continueButton.style.cssText = "border:0;padding:0;background:transparent;color:inherit;cursor:pointer;font-size:12px;text-decoration:underline";
      continueButton.addEventListener("click", focusConversationComposer);
      actions.append(downloadButton, continueButton);
      body.append(element, actions);
      collapseButton.addEventListener("click", () => {
        const collapsed = body.hidden = !body.hidden;
        collapseButton.textContent = collapsed ? "展开图片" : "折叠图片";
        if (collapsed) focusConversationComposer();
      });
      card.append(header, body);
      if (!mountImageCard(mount, card)) continue;
      renderedImageKeys.add(renderKey);
    }
  }

  let proxyImagePollInFlight = false;
  let proxyImageRecoveryChecked = false;
  async function pollProxyImages() {
    if (proxyImagePollInFlight) return;
    proxyImagePollInFlight = true;
    try {
      const conversationKey = currentConversationKey();
      const response = await fetch(`http://127.0.0.1:9230/yunqiao/images?conversation_key=${encodeURIComponent(conversationKey)}`, {
        cache: "no-store",
        credentials: "omit",
      });
      if (!response.ok) return;
      const payload = await response.json();
      for (const image of Array.isArray(payload?.images) ? payload.images : []) {
        if (typeof image?.source === "string") addCapturedImage(image.source, image);
      }
      if (!proxyImageRecoveryChecked && !(payload?.images || []).length) {
        proxyImageRecoveryChecked = true;
        const recoveryResponse = await fetch("http://127.0.0.1:9230/yunqiao/images", {
          cache: "no-store",
          credentials: "omit",
        });
        if (recoveryResponse.ok) {
          const recoveryPayload = await recoveryResponse.json();
          const recoverable = (Array.isArray(recoveryPayload?.images) ? recoveryPayload.images : [])
            .filter((image) => {
              const key = String(image?.conversation_key || "");
              const recent = Date.now() - Number(image?.created_at || 0) <= 2 * 60 * 60 * 1000;
              return recent && (!key || key.startsWith("draft:"));
            })
            .sort((left, right) => Number(right?.created_at || 0) - Number(left?.created_at || 0))
            .slice(0, 1);
          for (const image of recoverable) {
            if (typeof image?.source === "string") {
              addCapturedImage(image.source, { ...image, conversation_key: conversationKey });
            }
          }
          const recovered = capturedImages.filter((item) => recoverable.some((image) => image.id === item.id));
          if (recovered.length) void associateImages(recovered, conversationKey);
        }
      }
    } catch {
    } finally {
      proxyImagePollInFlight = false;
    }
  }

  if (typeof Response !== "undefined" && typeof Response.prototype.json === "function" && !Response.prototype.__yunqiaoOriginalJson) {
    const original = Response.prototype.json;
    const patched = async function (...args) {
      const payload = await original.apply(this, args);
      try {
        captureImages(payload);
        patchGraph(payload);
      } catch {
      }
      return payload;
    };
    patched.__yunqiaoOriginalJson = original;
    Response.prototype.json = patched;
  }

  const requestIds = new Set();
  if (!window.__yunqiaoDispatchEventPatched) {
    window.__yunqiaoDispatchEventPatched = true;
    const originalDispatchEvent = window.dispatchEvent;
    window.dispatchEvent = function (event) {
      try {
        const detail = event?.detail;
        const request = detail?.request;
        if (event?.type === "codex-message-from-view" && detail?.type === "mcp-request" && request?.method === "model/list") {
          request.params = { ...(request.params || {}), includeHidden: true };
          if (request.id != null) requestIds.add(String(request.id));
        }
        if (event?.type === "message") {
          captureImages(event.data);
          patchMcpResponse(event.data);
        }
      } catch {
      }
      return originalDispatchEvent.call(this, event);
    };
    window.addEventListener("message", (event) => {
      try {
        captureImages(event?.data);
        patchMcpResponse(event?.data);
      } catch {
      }
    }, true);
  }

  function patchMcpResponse(data) {
    captureImages(data);
    if (data?.type !== "mcp-response") return false;
    const message = data.message || data.response;
    const id = message?.id == null ? "" : String(message.id);
    if (requestIds.size && !requestIds.has(id)) return false;
    requestIds.delete(id);
    return patchGraph(data);
  }

  function assetURL(namePart) {
    const urls = [
      ...Array.from(document.scripts || []).map((item) => item.src),
      ...Array.from(document.querySelectorAll("link[href]") || []).map((item) => item.href),
      ...performance.getEntriesByType("resource").map((item) => item.name),
    ].filter(Boolean);
    return urls.find((value) => value.includes("/assets/") && value.includes(namePart) && value.split("?")[0].endsWith(".js")) || "";
  }

  async function assetURLFromScripts(namePart) {
    for (const source of Array.from(document.scripts || []).map((item) => item.src).filter(Boolean)) {
      if (!source.includes("/assets/") || !source.split("?")[0].endsWith(".js")) continue;
      try {
        const text = await fetch(source).then((response) => response.ok ? response.text() : "");
        const escaped = namePart.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
        const match = text.match(new RegExp(`["'](\\./assets/${escaped}[^"']+\\.js)["']`));
        if (match) return new URL(match[1], source).href;
      } catch {
      }
    }
    return "";
  }

  let appServerPatchAttempts = 0;
  async function patchAppServer() {
    if (window.__yunqiaoAppServerPatched || appServerPatchAttempts > 12) return;
    appServerPatchAttempts++;
    try {
      const source = assetURL("app-server-manager-signals-") || await assetURLFromScripts("app-server-manager-signals-");
      if (!source) return;
      const module = await import(source);
      let count = 0;
      for (const candidate of Object.values(module)) {
        const clients = [candidate];
        if (candidate && typeof candidate.get === "function") {
          try {
            clients.push(candidate.get());
          } catch {
          }
        }
        for (const client of clients) {
          if (!client || typeof client.sendRequest !== "function" || client.__yunqiaoPatched) continue;
          const original = client.sendRequest.bind(client);
          client.sendRequest = async function (method, params, options) {
            const result = await original(method, params, options);
            captureImages(result);
            const actual = method === "send-cli-request-for-host" && params?.method ? String(params.method) : String(method || "");
            if (actual === "list-models-for-host" || actual === "model/list") {
              try {
                patchGraph(result);
              } catch {
              }
            }
            return result;
          };
          client.__yunqiaoPatched = true;
          count++;
        }
      }
      if (count) window.__yunqiaoAppServerPatched = true;
    } catch {
    }
  }

  function patchStatsig() {
    const root = window.__STATSIG__ || globalThis.__STATSIG__;
    if (!root || typeof root !== "object") return;
    const clients = [root.firstInstance, typeof root.instance === "function" ? root.instance() : null];
    if (root.instances && typeof root.instances === "object") clients.push(...Object.values(root.instances));
    for (const client of clients.filter(Boolean)) {
      if (typeof client.getDynamicConfig !== "function") continue;
      if (!client.__yunqiaoPatched) {
        const original = client.getDynamicConfig.bind(client);
        client.getDynamicConfig = (name, options) => {
          const config = original(name, options);
          try {
            patchGraph(config);
          } catch {
          }
          return config;
        };
        client.__yunqiaoPatched = true;
      }
      try {
        patchGraph(client.getDynamicConfig("107580212", { disableExposureLog: true }));
      } catch {
      }
    }
  }

  function patchReactState() {
    const visited = new WeakSet();
    const nodes = [
      document.body,
      ...document.querySelectorAll("[role='menu'], [role='dialog'], [role='listbox'], [data-radix-popper-content-wrapper]"),
    ].filter(Boolean);
    for (const node of nodes.slice(0, 240)) {
      for (const key of Object.keys(node)) {
        if (key.startsWith("__reactFiber") || key.startsWith("__reactInternalInstance") || key.startsWith("__reactProps")) {
          try {
            patchGraph(node[key], visited);
          } catch {
          }
        }
      }
    }
  }

  function cleanupSidebarArtifacts() {
    const sidebar = document.querySelector("main aside") || document.querySelector("aside");
    if (!sidebar) return;
    const modelNames = new Set(names().map((name) => name.toLowerCase()));
    const rows = sidebar.querySelectorAll(
      "[data-app-action-sidebar-thread-id], [data-app-action-sidebar-project-row], [role='listitem'], a, button"
    );
    for (const row of rows) {
      const text = String(row.textContent || "").trim().toLowerCase();
      const isInjectedModel = modelNames.has(text);
      const isLegacyProject = text === "yunqiao";
      if (isInjectedModel || isLegacyProject) {
        row.style.setProperty("display", "none", "important");
        row.dataset.yunqiaoBridgeHidden = isLegacyProject ? "legacy-project" : "model-artifact";
      }
    }
  }

  let refreshPending = false;
  function refresh() {
    if (refreshPending) return;
    refreshPending = true;
    setTimeout(() => {
      refreshPending = false;
      const activeKey = currentConversationKey();
      if (lastConversationKey && lastConversationKey.startsWith("draft:") &&
          activeKey.startsWith("thread:") && activeKey !== lastConversationKey) {
        const draftImages = capturedImages.filter((image) => image.conversationKey === lastConversationKey);
        if (draftImages.length) void associateImages(draftImages, activeKey);
      }
      lastConversationKey = activeKey;
      patchStatsig();
      patchReactState();
      cleanupSidebarArtifacts();
      void pollProxyImages();
      renderCapturedImages();
      void patchAppServer();
      window.__yunqiaoCodexBridgeStatus = {
        installed: true,
        models: names().length,
        defaultModel: window.__yunqiaoCodexDefaultModel,
        appServerPatched: !!window.__yunqiaoAppServerPatched,
      };
    }, 50);
  }

  window.__yunqiaoCodexBridgeRefresh = refresh;
  new MutationObserver(refresh).observe(document.body || document.documentElement, { childList: true, subtree: true });
  setInterval(refresh, 1000);
  refresh();
})();
