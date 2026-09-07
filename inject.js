(() => {
  const localizationOnly = window.__YUNQIAO_LOCALIZATION_ONLY__ === true;
  const bridgeMode = localizationOnly ? "localization" : "relay";
  const incoming = Array.isArray(window.__YUNQIAO_INJECT_MODELS__)
    ? window.__YUNQIAO_INJECT_MODELS__.filter((x) => typeof x === "string" && x.trim()).map((x) => x.trim())
    : [];
  window.__yunqiaoCodexModels = Array.from(new Set(incoming));
  window.__yunqiaoCodexDefaultModel = String(window.__YUNQIAO_INJECT_DEFAULT__ || window.__yunqiaoCodexModels[0] || "");

  if (window.__yunqiaoCodexBridgeInstalled === "1.4.7" && window.__yunqiaoCodexBridgeMode === bridgeMode) {
    window.__yunqiaoCodexBridgeRefresh?.();
    return;
  }
  window.__yunqiaoCodexBridgeInstalled = "1.4.7";
  window.__yunqiaoCodexBridgeMode = bridgeMode;

  function installChineseLocale() {
    const locale = "zh-CN";
    const languages = ["zh-CN", "zh", "en-US", "en"];
    const localeStatus = window.__yunqiaoChineseLocaleStatus = {
      installed: true,
      active: false,
      officialSetting: false,
      fallbackInstalled: false,
      restartRequired: false,
      translated: 0,
      mode: "starting",
      error: "",
      heartbeat: Date.now(),
    };
    const updateLocaleStatus = (next) => {
      Object.assign(localeStatus, next, { heartbeat: Date.now() });
    };
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
    const localeProcessRestartKey = "yunqiao.locale.process-restart.v1";
    const syncOfficialLocale = async () => {
      try {
        const bridge = await waitForBridge();
        if (!bridge) throw new Error("electron bridge unavailable");
        const response = await callSetting(bridge, "get-setting", { key: "localeOverride" });
        if ((response?.value ?? null) === locale) {
          try {
            localStorage.removeItem(localeProcessRestartKey);
          } catch {
          }
          updateLocaleStatus({ officialSetting: true, restartRequired: false, mode: localeStatus.fallbackInstalled ? "official+fallback" : "official", error: "" });
          try {
            sessionStorage.removeItem("yunqiao.locale.reload");
          } catch {
          }
          return;
        }
        await callSetting(bridge, "set-setting", { key: "localeOverride", value: locale });
        try {
          localStorage.setItem(localeProcessRestartKey, String(Date.now()));
        } catch {
        }
        // Electron chooses its message catalog in the main process. A renderer
        // reload is not sufficient after changing localeOverride, so let the
        // launcher perform one clean full-process restart.
        updateLocaleStatus({ officialSetting: true, restartRequired: true, mode: "official-updated", error: "" });
      } catch (error) {
        updateLocaleStatus({ error: String(error?.message || error || "locale setting failed").slice(0, 160) });
        if (Date.now() - localeSyncStarted < 120000) setTimeout(syncOfficialLocale, 1500);
      }
    };
    void syncOfficialLocale();

    // The official zh-CN catalog is still preferred. This exact-match fallback
    // only touches application chrome and never translates conversation text,
    // prompts, editor contents, code, terminals, or model output. It keeps the
    // launcher useful when the Codex locale/Statsig service initializes late or
    // is unreachable on the user's network.
    const uiTranslations = new Map([
      ["New task", "新建任务"], ["New Task", "新建任务"],
      ["New chat", "新建对话"], ["New Chat", "新建对话"],
      ["Start a new task", "开始新任务"], ["Start new task", "开始新任务"],
      ["Open folder", "打开文件夹"], ["Open Folder", "打开文件夹"],
      ["Open Folder…", "打开文件夹…"], ["Add folder", "添加文件夹"],
      ["Choose folder", "选择文件夹"], ["Select folder", "选择文件夹"],
      ["Recent", "最近"], ["Projects", "项目"], ["Project", "项目"],
      ["Threads", "对话"], ["Chats", "对话"], ["History", "历史记录"],
      ["Archived", "已归档"], ["Archive", "归档"], ["Unarchive", "取消归档"],
      ["Settings", "设置"], ["Settings…", "设置…"], ["General", "常规"],
      ["Appearance", "外观"], ["Theme", "主题"], ["System", "跟随系统"],
      ["Light", "浅色"], ["Dark", "深色"], ["Language", "语言"],
      ["Notifications", "通知"], ["Account", "账户"],
      ["Sign out", "退出登录"], ["Log out", "退出登录"], ["Log Out", "退出登录"],
      ["Help", "帮助"], ["About", "关于"],
      ["Keyboard shortcuts", "键盘快捷键"], ["Keyboard Shortcuts", "键盘快捷键"],
      ["Check for updates", "检查更新"], ["Check for Updates…", "检查更新…"],
      ["What's new", "更新内容"], ["Search", "搜索"],
      ["Search chats", "搜索对话"], ["Search Chats…", "搜索对话…"],
      ["Search files", "搜索文件"], ["Search Files…", "搜索文件…"],
      ["Ask anything", "输入任何问题"], ["Describe a task", "描述一个任务"],
      ["Send", "发送"], ["Stop", "停止"], ["Retry", "重试"],
      ["Continue", "继续"], ["Cancel", "取消"], ["Save", "保存"],
      ["Done", "完成"], ["Close", "关闭"], ["Delete", "删除"],
      ["Rename", "重命名"], ["Pin", "置顶"], ["Unpin", "取消置顶"],
      ["Copy", "复制"], ["Download", "下载"], ["Open", "打开"],
      ["Show in Explorer", "在资源管理器中显示"], ["Open in Explorer", "在资源管理器中打开"],
      ["Terminal", "终端"], ["Files", "文件"], ["Browser", "浏览器"],
      ["Code", "代码"], ["Review", "审查"], ["Local", "本地"], ["Cloud", "云端"],
      ["Automations", "自动化"], ["Skills", "技能"],
      ["Model Context Protocol", "模型上下文协议"], ["Models", "模型"], ["Model", "模型"],
      ["Reasoning effort", "推理强度"], ["Effort", "推理强度"],
      ["Low", "低"], ["Medium", "中"], ["High", "高"], ["Extra high", "极高"],
      ["Workspace", "工作区"], ["Worktree", "工作树"], ["Worktrees", "工作树"],
      ["Create worktree", "创建工作树"], ["No conversations yet", "暂无对话"],
      ["Get started", "开始使用"], ["Connect", "连接"], ["Reconnect", "重新连接"],
      ["Install", "安装"], ["Manage", "管理"], ["Enabled", "已启用"],
      ["Disabled", "已禁用"], ["On", "开"], ["Off", "关"],
      ["Default", "默认"], ["Recommended", "推荐"], ["Always", "始终"],
      ["Never", "从不"], ["Allow", "允许"], ["Deny", "拒绝"],
      ["Approve", "批准"], ["Submit", "提交"], ["Feedback", "反馈"],
      ["Loading…", "正在加载…"], ["Loading...", "正在加载..."],
      ["Working…", "正在处理…"], ["Working...", "正在处理..."],
      ["Toggle Sidebar", "切换侧边栏"], ["Toggle Bottom Panel", "切换底部面板"],
      ["Open Terminal", "打开终端"], ["Open Browser Tab", "打开浏览器标签页"],
      ["Find", "查找"], ["Back", "后退"], ["Forward", "前进"],
      ["Zoom In", "放大"], ["Zoom Out", "缩小"], ["Actual Size", "实际大小"],
    ]);
    const excludedContentSelector = [
      "[data-testid='conversation-turn']", "[data-testid*='message']", "[data-message-author-role]",
      "article", "pre", "code", "textarea", "[contenteditable='true']", ".monaco-editor",
      ".xterm", "[role='textbox']",
    ].join(",");
    const uiSurfaceSelector = [
      "button", "a", "label", "nav", "aside", "header", "main", "[role='menu']",
      "[role='menuitem']", "[role='tab']", "[role='dialog']", "[role='navigation']",
      "[data-testid*='sidebar']", "[data-testid*='settings']", "[data-testid*='composer']",
    ].join(",");
    let translatedTotal = 0;
    let fallbackObserver = null;
    const translatedValue = (raw) => {
      const value = String(raw || "");
      const trimmed = value.trim();
      const translated = uiTranslations.get(trimmed);
      if (!translated) return null;
      const start = value.slice(0, value.indexOf(trimmed));
      const end = value.slice(value.indexOf(trimmed) + trimmed.length);
      return start + translated + end;
    };
    const translateElementAttributes = (element) => {
      if (!(element instanceof Element) || element.closest(excludedContentSelector)) return 0;
      let changed = 0;
      for (const attribute of ["aria-label", "title", "placeholder"]) {
        if (!element.hasAttribute(attribute)) continue;
        const translated = translatedValue(element.getAttribute(attribute));
        if (translated && translated !== element.getAttribute(attribute)) {
          element.setAttribute(attribute, translated);
          changed++;
        }
      }
      return changed;
    };
    const translateTextNode = (node) => {
      const parent = node?.parentElement;
      if (!parent || parent.closest(excludedContentSelector) || !parent.closest(uiSurfaceSelector)) return 0;
      const translated = translatedValue(node.nodeValue);
      if (!translated || translated === node.nodeValue) return 0;
      node.nodeValue = translated;
      return 1;
    };
    const measureVisibleLocale = () => {
      const root = document.body || document.documentElement;
      let chinese = 0;
      let english = 0;
      if (root) {
        const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
        let node;
        while ((node = walker.nextNode())) {
          const parent = node.parentElement;
          if (!parent || parent.closest(excludedContentSelector) || !parent.closest(uiSurfaceSelector)) continue;
          const value = String(node.nodeValue || "").trim();
          if (!value) continue;
          if (/[\u3400-\u9fff]/.test(value)) chinese++;
          if (uiTranslations.has(value)) english++;
        }
      }
      updateLocaleStatus({
        active: chinese > 0,
        fallbackInstalled: !!fallbackObserver,
        translated: translatedTotal,
        visibleChinese: chinese,
        visibleEnglish: english,
        mode: localeStatus.officialSetting ? (translatedTotal > 0 ? "official+fallback" : "official") : "fallback",
      });
    };
    const translateTree = (root) => {
      if (!root) return 0;
      if (root.nodeType === Node.TEXT_NODE) return translateTextNode(root);
      let changed = root instanceof Element ? translateElementAttributes(root) : 0;
      const owner = root.ownerDocument || document;
      const walker = owner.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
      let node;
      while ((node = walker.nextNode())) {
        if (node.nodeType === Node.TEXT_NODE) changed += translateTextNode(node);
        else changed += translateElementAttributes(node);
      }
      if (changed) translatedTotal += changed;
      return changed;
    };
    const startFallback = () => {
      const root = document.body || document.documentElement;
      if (!root) return setTimeout(startFallback, 50);
      if (!fallbackObserver) {
        fallbackObserver = new MutationObserver((records) => {
          for (const record of records) {
            for (const node of record.addedNodes || []) translateTree(node);
            if (record.type === "characterData") translateTextNode(record.target);
          }
          measureVisibleLocale();
        });
        fallbackObserver.observe(root, { childList: true, subtree: true, characterData: true });
      }
      translateTree(root);
      measureVisibleLocale();
    };
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", startFallback, { once: true });
    else startFallback();

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
      updateLocaleStatus({});
      if (Date.now() - started > 120000) clearInterval(timer);
    }, 100);
    return measureVisibleLocale;
  }

  const refreshChineseLocale = installChineseLocale();
  if (localizationOnly) {
    const refreshLocalizationOnly = () => refreshChineseLocale?.();
    window.__yunqiaoCodexBridgeRefresh = refreshLocalizationOnly;
    const startLocalizationWatch = () => {
      const root = document.body || document.documentElement;
      if (!root) return setTimeout(startLocalizationWatch, 50);
      new MutationObserver(refreshLocalizationOnly).observe(root, { childList: true, subtree: true });
      setInterval(refreshLocalizationOnly, 1000);
      refreshLocalizationOnly();
    };
    startLocalizationWatch();
    return;
  }

  const names = () => Array.isArray(window.__yunqiaoCodexModels) ? window.__yunqiaoCodexModels : [];
  const descriptor = (model, template = null) => {
    const value = template && typeof template === "object" ? { ...template } : {};
    const isAuto = model === "yunqiao-auto";
    const displayName = isAuto ? "智能路由（Auto）" : model;
    value.model = model;
    value.id = model;
    value.slug = model;
    value.name = displayName;
    value.displayName = displayName;
    value.display_name = displayName;
    value.description = isAuto ? "Grok 优先 · 文档智能分流 · Astra 仅手动" : "Yunqiao API";
    value.hidden = false;
    value.isDefault = model === window.__yunqiaoCodexDefaultModel;
    value.is_default = value.isDefault;
    if (!value.defaultReasoningEffort) value.defaultReasoningEffort = "medium";
    if (!Array.isArray(value.supportedReasoningEfforts) || value.supportedReasoningEfforts.length === 0) {
      value.supportedReasoningEfforts = ["low", "medium", "high"].map((reasoningEffort) => ({
        reasoningEffort,
        description: `${reasoningEffort} effort`,
      }));
    }
    return value;
  };

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
    const template = value.find((item) => item && typeof item === "object") || null;
    for (const item of value) {
      if (names().includes(item.model) && item.hidden !== false) {
        item.hidden = false;
        changed = true;
      }
    }
    for (const model of names()) {
      if (!existing.has(model)) {
        value.push(descriptor(model, template));
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

  function modelResponseLooksPatchable(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return false;
    const arrays = [
      value.models,
      value.data,
      value.result,
      value.pages?.[0]?.data,
      value.result?.data,
      value.result?.models,
      value.message?.result?.data,
      value.message?.result?.models,
    ];
    if (arrays.some((items) => Array.isArray(items) && items.length > 0 &&
        items.every((item) => item && typeof item === "object" && typeof item.model === "string"))) {
      return true;
    }
    const hasContainerSignal = ["defaultModel", "default_model", "availableModels", "available_models",
      "hiddenModels", "hidden_models", "modelMetadata", "model_metadata"].some((key) => key in value);
    return hasContainerSignal && Array.isArray(value.models) &&
      value.models.every((item) => typeof item === "string");
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
        if (modelResponseLooksPatchable(payload)) patchContainer(payload);
      } catch {
      }
      return payload;
    };
    patched.__yunqiaoOriginalJson = original;
    Response.prototype.json = patched;
  }

  const requestIds = window.__yunqiaoModelListRequestIds = window.__yunqiaoModelListRequestIds || new Set();
  if (!window.__yunqiaoModelMessagePatchInstalled) {
    window.__yunqiaoModelMessagePatchInstalled = true;
    window.addEventListener("codex-message-from-view", (event) => {
      try {
        const detail = event?.detail;
        const request = detail?.request;
        if (detail?.type === "mcp-request" && request?.method === "model/list") {
          request.params = { ...(request.params || {}), includeHidden: true };
          if (request.id != null) {
            const requestId = String(request.id);
            requestIds.add(requestId);
            if (requestIds.size > 64) requestIds.delete(requestIds.values().next().value);
            setTimeout(() => requestIds.delete(requestId), 30000);
          }
        }
      } catch {
      }
    }, true);
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
    if (requestIds.size === 0 || !requestIds.has(id)) return false;
    requestIds.delete(id);
    let changed = false;
    if (patchModelArray(message?.result?.data, true)) changed = true;
    if (patchModelArray(message?.result?.models, true)) changed = true;
    return changed;
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

  function appServerAssetURLs() {
    const urls = [
      ...Array.from(document.scripts || []).map((item) => item.src),
      ...Array.from(document.querySelectorAll("link[href]") || []).map((item) => item.href),
      ...performance.getEntriesByType("resource").map((item) => item.name),
    ].filter((value) => value && value.includes("/assets/") && value.split("?")[0].endsWith(".js"));
    const preferred = urls.filter((value) => /use-host-config|app-server-manager-signals|app-initial|app-main|page-|signals|server-manager/i.test(value));
    return Array.from(new Set(preferred)).slice(0, 16);
  }

  function appServerCandidates(module) {
    const result = [];
    const seen = new Set();
    const add = (value) => {
      if (!value || typeof value !== "object" || seen.has(value)) return;
      seen.add(value);
      result.push(value);
    };
    for (const value of Object.values(module || {})) {
      add(value);
      if (typeof value?.get === "function") {
        try { add(value.get()); } catch {
        }
        try { add(value.get("local")); } catch {
        }
      }
      if (value && typeof value === "object") {
        try { Object.values(value).slice(0, 100).forEach(add); } catch {
        }
      }
    }
    return result;
  }

  let appServerPatchAttempts = 0;
  let appServerPatchDisabled = false;
  async function patchAppServer() {
    if (window.__yunqiaoAppServerPatched || appServerPatchDisabled) return;
    appServerPatchAttempts++;
    try {
      const sources = [];
      for (const prefix of ["use-host-config-", "app-server-manager-signals-"]) {
        const source = assetURL(prefix) || await assetURLFromScripts(prefix);
        if (source) sources.push(source);
      }
      sources.push(...appServerAssetURLs());
      let count = 0;
      for (const source of Array.from(new Set(sources)).slice(0, 16)) {
        let module;
        try { module = await import(source); } catch { continue; }
        for (const client of appServerCandidates(module)) {
          if (!client || typeof client.sendRequest !== "function" || client.__yunqiaoModelRequestPatched) continue;
          const original = client.sendRequest.bind(client);
          client.sendRequest = async function (method, params, options) {
            const result = await original(method, params, options);
            captureImages(result);
            const actual = method === "send-cli-request-for-host" && params?.method ? String(params.method) : String(method || "");
            if (actual === "list-models-for-host") {
              try {
                if (Array.isArray(result)) patchModelArray(result, true);
                if (Array.isArray(result?.data)) patchModelArray(result.data, true);
                if (Array.isArray(result?.models)) patchModelArray(result.models, true);
              } catch {
              }
            }
            return result;
          };
          client.__yunqiaoModelRequestPatched = true;
          count++;
        }
      }
      if (count) window.__yunqiaoAppServerPatched = true;
    } catch {
    }
    if (!window.__yunqiaoAppServerPatched && appServerPatchAttempts >= 8) appServerPatchDisabled = true;
  }

  function patchStatsigModelConfig(config) {
    if (!config?.value || typeof config.value !== "object") return config;
    const available = Array.isArray(config.value.available_models) ? [...config.value.available_models] : [];
    for (const model of names()) if (!available.includes(model)) available.push(model);
    const next = { ...config.value, available_models: available };
    if (window.__yunqiaoCodexDefaultModel) next.default_model = window.__yunqiaoCodexDefaultModel;
    try { config.value = next; } catch {
      return { ...config, value: next };
    }
    return config;
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
          return String(name) === "107580212" ? patchStatsigModelConfig(config) : config;
        };
        client.__yunqiaoPatched = true;
      }
      try {
        patchStatsigModelConfig(client.getDynamicConfig("107580212", { disableExposureLog: true }));
      } catch {
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
      refreshChineseLocale?.();
      patchStatsig();
      cleanupSidebarArtifacts();
      void pollProxyImages();
      renderCapturedImages();
      void patchAppServer();
      window.__yunqiaoCodexBridgeStatus = {
        installed: true,
        models: names().length,
        defaultModel: window.__yunqiaoCodexDefaultModel,
        appServerPatched: !!window.__yunqiaoAppServerPatched,
        appServerPatchDisabled,
        heartbeat: Date.now(),
      };
    }, 50);
  }

  window.__yunqiaoCodexBridgeRefresh = refresh;
  new MutationObserver(refresh).observe(document.body || document.documentElement, { childList: true, subtree: true });
  setInterval(refresh, 1000);
  refresh();
})();
