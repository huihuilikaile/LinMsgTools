<script>
  import { onMount, tick } from "svelte";
  import {
    addQuickCommand,
    appendCommandQueryLog,
    appendCommandToJSONShell,
    cancelConnect,
    connect,
    connectSecondary,
    connected as getConnected,
    deleteQuickCommand,
    deleteServerRecord,
    disconnect,
    closeTerminalWindow,
    disconnectSecondary,
    getTerminalDetached,
    listJSONShellFiles,
    listQuickCommands,
    listServerRecords,
    loadJSONShellCommands,
    onRuntimeEvent,
    openTerminalWindow,
    resizeSecondaryTerminal,
    resizeTerminal,
    returnTerminalToMain,
    runCommand,
    runQuickCommand,
    saveServerRecord,
    secondaryConnected as getSecondaryConnected,
    sendInput,
    sendSecondaryInput,
    touchServerRecord,
    windowClose,
    windowIsMaximised,
    windowMaximise,
    windowMinimise,
    windowRestore,
  } from "./lib/api/wails.js";
  import {
    formatTime,
    normalizeJSONShellFileName,
    recordIdFor,
  } from "./lib/utils/format.js";
  import {
    loadBooleanSetting,
    loadStringSetting,
    saveBooleanSetting,
    saveStringSetting,
  } from "./lib/utils/storage.js";
  import TitleBar from "./lib/components/TitleBar.svelte";
  import Sidebar from "./lib/components/Sidebar.svelte";
  import ConnectionPanel from "./lib/components/ConnectionPanel.svelte";
  import ResponsePanel from "./lib/components/ResponsePanel.svelte";
  import TerminalPanel from "./lib/components/TerminalPanel.svelte";
  import SettingsModal from "./lib/components/SettingsModal.svelte";
  import QuickAddModal from "./lib/components/QuickAddModal.svelte";
  import JSONShellModal from "./lib/components/JSONShellModal.svelte";
  import ToastStack from "./lib/components/ToastStack.svelte";

  const CONNECTION_HEALTHCHECK_INTERVAL_MS = 30000;
  const TERMINAL_SESSION_BUFFER_LIMIT = 512 * 1024;
  const FIRST_LAUNCH_STATE_KEY = "firstLaunchState";
  const standalone = new URLSearchParams(window.location.search).get("window") === "terminal";

  let activeTab = standalone ? "terminal" : "connection";
  let connected = false;
  let terminalDetached = false;
  let connectionHealthTimer = 0;
  let connectionHealthChecking = false;
  let statusMessage = "等待连接";
  let statusTone = "idle";
  let sidebarCollapsed = false;
  let splashEnabled = true;
  let splashVisible = false;
  let splashTimer = 0;
  let launchInfoEnabled = false;
  let launchInfoOpen = false;
  let quickOutputLineNumbers = true;
  let connectModalOpen = false;
  let connectModalUser = "";
  let connectModalTarget = "";
  let connectModalError = "";
  let connectModalElapsedSeconds = "0.0";
  let connectModalTimer = 0;
  let connectAttemptSequence = 0;
  let activeConnectAttemptId = 0;
  let closeConfirmOpen = false;
  let closeConfirmBusy = false;
  let closeConfirmScope = "main";
  let settingsOpen = false;
  let quickAddOpen = false;
  let jsonShellOpen = false;
  let selectedJSONShellFile = "";
  let jsonShellCreateTargetCommandId = "";
  let currentCommandTitle = "";
  let currentCommand = "";
  let quickResultTitle = "";
  let quickResultCommand = "";
  let quickResultOutput = "尚未执行命令。";
  let quickCommandStatuses = {};
  let quickCommandResults = {};
  let quickOverviewOpen = false;
  let quickOverviewResults = [];
  let quickContextMenuOpen = false;
  let quickContextMenuPosition = { x: 0, y: 0 };
  let quickContextTargetCommandId = "";
  let terminalApi = null;
  let terminalSessions = [];
  let terminalSessionSeed = 0;
  let activeTerminalSessionId = "";
  let liveTerminalSessionId = "";
  let suppressNextDisconnectEventUi = false;
  let terminalSessionCloseConfirmOpen = false;
  let terminalSessionCloseConfirmBusy = false;
  let terminalSessionCloseTargetId = "";
  let replaceConnectConfirmOpen = false;
  let replaceConnectConfirmBusy = false;
  let replaceConnectTarget = null;
  let toasts = [];
  let toastSeed = 0;
  let currentConnectionIdentity = { user: "", host: "", port: 22 };
  let pendingConnectPreviousState = null;
  const cancelledConnectAttemptIds = new Set();

  let form = {
    serverName: "",
    serverDescription: "",
    host: "",
    port: 22,
    user: "",
    password: "",
    keyPath: "",
    cols: 120,
    rows: 32,
    logging: false,
  };

  let quickAddForm = {
    name: "",
    command: "",
  };

  let jsonShellName = "";
  let serverRecords = [];
  let quickCommands = [];
  let jsonShellFiles = [];
  let selectedJSONShellCommands = [];

  $: visibleQuickCommands = [...quickCommands, ...selectedJSONShellCommands];
  $: quickOverviewResults = visibleQuickCommands
    .map((item) => quickCommandResults[item.id])
    .filter(Boolean);
  $: terminalSessionCloseTarget = terminalSessionById(terminalSessionCloseTargetId);
  $: connectedRecordId = connected
    ? recordIdFor(currentConnectionIdentity.user, currentConnectionIdentity.host, currentConnectionIdentity.port)
    : "";
  $: statusIdentityLabel = connected
    ? `${currentConnectionIdentity.user || form.user || "--"}@${currentConnectionIdentity.host || form.host || "--"}`
    : "未连接";

  function setStatus(message, tone = connected ? "connected" : "idle") {
    statusMessage = String(message || "等待连接");
    statusTone = tone;
  }

  function removeToast(id) {
    toasts = toasts.filter((item) => item.id !== id);
  }

  function dismissToast(id) {
    const index = toasts.findIndex((item) => item.id === id);
    if (index < 0 || toasts[index].leaving) {
      return;
    }
    toasts = toasts.map((item) => item.id === id ? { ...item, leaving: true } : item);
    window.setTimeout(() => removeToast(id), 220);
  }

  function showToast(message, tone = "info") {
    const id = ++toastSeed;
    const order = toasts.length;
    toasts = [...toasts, { id, message: String(message), tone, leaving: false }];
    window.setTimeout(() => dismissToast(id), 3000 + order * 120);
  }

  function setConnectionIdentity(user, host, port = 22) {
    currentConnectionIdentity = {
      user: String(user || "").trim(),
      host: String(host || "").trim(),
      port: Number(port) || 22,
    };
  }

  function rememberPendingConnectState() {
    pendingConnectPreviousState = {
      activeTab,
      connected,
      statusMessage,
      statusTone,
      identity: { ...currentConnectionIdentity },
      terminalSessionId: activeTerminalSessionId,
    };
  }

  function restorePendingConnectState() {
    if (!pendingConnectPreviousState) {
      return;
    }
    activeTab = pendingConnectPreviousState.activeTab;
    connected = pendingConnectPreviousState.connected;
    statusMessage = pendingConnectPreviousState.statusMessage;
    statusTone = pendingConnectPreviousState.statusTone;
    currentConnectionIdentity = { ...pendingConnectPreviousState.identity };
    activeTerminalSessionId = pendingConnectPreviousState.terminalSessionId || liveTerminalSessionId;
  }

  function clearPendingConnectState() {
    pendingConnectPreviousState = null;
  }

  function formatTerminalSessionLabel(user, host) {
    const safeUser = String(user || "").trim() || "--";
    const safeHost = String(host || "").trim() || "--";
    return `${safeUser}@${safeHost}`;
  }

  function formatTerminalSessionUser(user) {
    return String(user || "").trim() || "--";
  }

  function formatTerminalSessionTime(value) {
    return new Date(value).toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function formatDisconnectTime(value = Date.now()) {
    return new Date(value).toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }

  function trimTerminalSessionBuffer(buffer) {
    if (buffer.length <= TERMINAL_SESSION_BUFFER_LIMIT) {
      return buffer;
    }
    return buffer.slice(buffer.length - TERMINAL_SESSION_BUFFER_LIMIT);
  }

  function buildTerminalDisconnectOutput(message) {
    const safeMessage = String(message || "连接已断开").trim() || "连接已断开";
    return `\r\n\x1b[90m[${safeMessage} · ${formatDisconnectTime()}]\x1b[0m\r\n`;
  }

  function updateTerminalSession(id, updater) {
    if (!id) {
      return;
    }
    terminalSessions = terminalSessions.map((item) => item.id === id ? updater(item) : item);
  }

  function createPendingTerminalSession(user, host) {
    const id = `terminal-session-${Date.now()}-${++terminalSessionSeed}`;
    const createdAt = Date.now();
    terminalSessions = [
      ...terminalSessions,
      {
        id,
        label: formatTerminalSessionLabel(user, host),
        userName: formatTerminalSessionUser(user),
        meta: formatTerminalSessionTime(createdAt),
        status: "connecting",
        buffer: "",
        createdAt,
      },
    ];
    activeTerminalSessionId = id;
    return id;
  }

  function finalizeTerminalSession(id) {
    if (!id) {
      return;
    }
    const previousLiveId = liveTerminalSessionId;
    if (previousLiveId && previousLiveId !== id) {
      updateTerminalSession(previousLiveId, (item) => ({
        ...item,
        status: "disconnected",
      }));
    }
    updateTerminalSession(id, (item) => ({
      ...item,
      status: "connected",
    }));
    liveTerminalSessionId = id;
    activeTerminalSessionId = id;
  }

  function removeTerminalSession(id) {
    if (!id) {
      return;
    }
    terminalSessions = terminalSessions.filter((item) => item.id !== id);
    if (activeTerminalSessionId === id) {
      const fallbackId = terminalSessions.length ? terminalSessions[terminalSessions.length - 1].id : "";
      activeTerminalSessionId = liveTerminalSessionId && liveTerminalSessionId !== id
        ? liveTerminalSessionId
        : fallbackId;
    }
    if (liveTerminalSessionId === id) {
      liveTerminalSessionId = "";
    }
  }

  function terminalSessionById(id) {
    return terminalSessions.find((item) => item.id === id) || null;
  }

  function requestTerminalSessionClose(id) {
    if (!id || standalone) {
      return;
    }
    terminalSessionCloseTargetId = id;
    terminalSessionCloseConfirmBusy = false;
    terminalSessionCloseConfirmOpen = true;
  }

  async function confirmTerminalSessionClose() {
    if (terminalSessionCloseConfirmBusy) {
      return;
    }
    const target = terminalSessionById(terminalSessionCloseTargetId);
    if (!target) {
      terminalSessionCloseConfirmOpen = false;
      terminalSessionCloseTargetId = "";
      return;
    }

    terminalSessionCloseConfirmBusy = true;
    const closingActiveId = target.id === activeTerminalSessionId;

    if (target.status === "connected" && target.id === liveTerminalSessionId) {
      await disconnect().catch((error) => showToast(error, "error"));
    } else if (target.status === "connecting") {
      await cancelConnect().catch((error) => showToast(error, "error"));
      stopConnectModalTimer();
      connectModalOpen = false;
      connectModalError = "";
      restorePendingConnectState();
      clearPendingConnectState();
    }

    removeTerminalSession(target.id);
    terminalSessionCloseTargetId = "";
    terminalSessionCloseConfirmOpen = false;
    terminalSessionCloseConfirmBusy = false;

    if (closingActiveId) {
      await tick();
      if (activeTerminalSessionId) {
        syncTerminalToSession(activeTerminalSessionId);
      } else {
        terminalApi?.reset?.();
      }
    }
  }

  function appendTerminalSessionOutput(id, data) {
    if (!id || !data) {
      return;
    }
    updateTerminalSession(id, (item) => ({
      ...item,
      buffer: trimTerminalSessionBuffer(`${item.buffer || ""}${data}`),
    }));
  }

  function markTerminalSessionDisconnected(message) {
    if (!liveTerminalSessionId) {
      return;
    }
    const disconnectOutput = buildTerminalDisconnectOutput(message);
    appendTerminalSessionOutput(liveTerminalSessionId, disconnectOutput);
    updateTerminalSession(liveTerminalSessionId, (item) => ({
      ...item,
      status: "disconnected",
    }));
  }

  function syncTerminalToSession(id) {
    if (!terminalApi || !id) {
      return;
    }
    const target = terminalSessions.find((item) => item.id === id);
    if (!target) {
      return;
    }
    terminalApi.replaceBuffer?.(target.buffer || "");
  }

  function stopConnectModalTimer() {
    if (connectModalTimer) {
      window.clearInterval(connectModalTimer);
      connectModalTimer = 0;
    }
  }

  function stopSplashTimer() {
    if (splashTimer) {
      window.clearTimeout(splashTimer);
      splashTimer = 0;
    }
  }

  function startConnectModalTimer(startedAt) {
    stopConnectModalTimer();
    connectModalElapsedSeconds = ((Date.now() - startedAt) / 1000).toFixed(1);
    connectModalTimer = window.setInterval(() => {
      connectModalElapsedSeconds = ((Date.now() - startedAt) / 1000).toFixed(1);
    }, 100);
  }

  async function waitForMinimumConnectModalDuration(startedAt) {
    const remaining = 1000 - (Date.now() - startedAt);
    if (remaining > 0) {
      await new Promise((resolve) => window.setTimeout(resolve, remaining));
    }
  }

  async function waitForMinimumCloseConfirmDuration(startedAt) {
    const remaining = 500 - (Date.now() - startedAt);
    if (remaining > 0) {
      await new Promise((resolve) => window.setTimeout(resolve, remaining));
    }
  }

  function requestClose(scope) {
    closeConfirmScope = scope;
    closeConfirmBusy = false;
    closeConfirmOpen = true;
  }

  function closeConfirmTitle() {
    if (closeConfirmBusy) {
      if (closeConfirmScope === "main" && connected) {
        return `${currentConnectedLabel()} 已关闭`;
      }
      if (closeConfirmScope === "main" && terminalDetached) {
        return "正在关闭主窗口和独立终端窗口";
      }
      return "正在关闭";
    }
    if (closeConfirmScope === "standalone") {
      return "关闭独立终端窗口";
    }
    return "关闭主窗口";
  }

  function closeConfirmMessage() {
    if (closeConfirmBusy) {
      return closeConfirmScope === "standalone"
        ? "正在关闭窗口…"
        : "正在关闭窗口";
    }
    if (closeConfirmScope === "standalone") {
      return "确认关闭独立终端窗口？";
    }

    const connectionNotice = connected
      ? `当前已连接 ${currentConnectedLabel()}。`
      : "";
    const detachedNotice = terminalDetached
      ? "这将同时关闭独立终端窗口。"
      : "";
    const suffix = [connectionNotice, detachedNotice].filter(Boolean).join(" ");

    if (suffix) {
      return `确认关闭主窗口？ ${suffix}`.trim();
    }
    return "确认关闭主窗口？";
  }

  async function confirmClose() {
    if (closeConfirmBusy) {
      return;
    }
    closeConfirmBusy = true;
    const startedAt = Date.now();
    if (closeConfirmScope === "standalone") {
      await returnDetachedTerminalToMainWindow();
    } else if (terminalDetached) {
      await disconnectSecondary().catch(() => {});
      await closeTerminalWindow().catch(() => {});
      terminalDetached = false;
    }
    await waitForMinimumCloseConfirmDuration(startedAt);
    closeConfirmOpen = false;
    await windowClose();
  }

  async function cancelConnectModal() {
    const attemptId = activeConnectAttemptId;
    stopConnectModalTimer();
    connectModalOpen = false;
    connectModalError = "";
    restorePendingConnectState();
    if (!attemptId) {
      clearPendingConnectState();
      return;
    }
    cancelledConnectAttemptIds.add(attemptId);
    if (!pendingConnectPreviousState?.connected) {
      setStatus("已取消连接", "idle");
      setConnectionIdentity("", "", 22);
    }
    try {
      await cancelConnect();
    } catch {
      // ignore cancellation errors while closing the modal
    }
  }

  function resetConnectionOutputs() {
    quickResultTitle = "";
    quickResultCommand = "";
    quickResultOutput = "尚未执行命令。";
  }

  function resetResponseState() {
    quickCommandStatuses = {};
    quickCommandResults = {};
    quickOverviewOpen = false;
    currentCommandTitle = "";
    currentCommand = "";
    resetConnectionOutputs();
  }

  function processTerminalOutput(data) {
    if (!data) {
      return;
    }
    if (!standalone) {
      appendTerminalSessionOutput(liveTerminalSessionId, data);
    }
    if (!terminalApi) {
      return;
    }
    if (!standalone && activeTerminalSessionId && activeTerminalSessionId !== liveTerminalSessionId) {
      return;
    }
    terminalApi.write(data);
  }

  function handleTerminalInput(data) {
    const sender = standalone ? sendSecondaryInput : sendInput;
    sender(data).catch((error) => showToast(error, "error"));
  }

  function handleTerminalClear() {
    handleTerminalInput("clear\r");
  }

  function handleTerminalSessionSelect(id) {
    if (!id || standalone) {
      return;
    }
    if (id === activeTerminalSessionId) {
      terminalApi?.focus();
      return;
    }
    activeTerminalSessionId = id;
    syncTerminalToSession(id);
    terminalApi?.focus();
  }


  function currentRecordId() {
    return recordIdFor(form.user, form.host, form.port);
  }

  function currentConnectedRecordId() {
    return recordIdFor(
      currentConnectionIdentity.user,
      currentConnectionIdentity.host,
      currentConnectionIdentity.port,
    );
  }

  function currentConnectedLabel() {
    const host = currentConnectionIdentity.host || "--";
    const user = currentConnectionIdentity.user || "--";
    return `${user}@${host}`;
  }

  function isSameConnectionTarget(host, user, port) {
    return connected
      && currentConnectedRecordId() === recordIdFor(user, host, port);
  }

  function fillForm(record) {
    form = {
      ...form,
      serverName: record.name || "",
      serverDescription: record.description || "",
      host: record.host || "",
      port: record.port || 22,
      user: record.user || "",
      password: record.password || "",
      keyPath: record.keyPath || "",
    };
    if (!connected) {
      setConnectionIdentity(record.user, record.host, record.port);
    }
  }

  async function loadRecords() {
    try {
      serverRecords = await listServerRecords();
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function loadCommands() {
    try {
      quickCommands = await listQuickCommands();
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function loadShellFiles() {
    try {
      jsonShellFiles = await listJSONShellFiles();
      if (!jsonShellFiles.some((item) => item.fileName === selectedJSONShellFile)) {
        selectedJSONShellFile = "";
        selectedJSONShellCommands = [];
      }
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function loadSelectedShellCommands(fileName) {
    selectedJSONShellFile = fileName || "";
    if (!selectedJSONShellFile) {
      selectedJSONShellCommands = [];
      return;
    }
    try {
      selectedJSONShellCommands = await loadJSONShellCommands(selectedJSONShellFile);
    } catch (error) {
      selectedJSONShellCommands = [];
      showToast(error, "error");
    }
  }

  async function syncTerminalSize() {
    if (!terminalApi) {
      return;
    }
    terminalApi.fit();
    if (!connected) {
      return;
    }
    const size = terminalApi.getSize();
    form = {
      ...form,
      cols: size.cols,
      rows: size.rows,
    };
    await resizeTerminal(size.cols, size.rows);
  }

  function stopConnectionHealthCheck() {
    if (connectionHealthTimer) {
      window.clearInterval(connectionHealthTimer);
      connectionHealthTimer = 0;
    }
  }

  async function runConnectionHealthCheck() {
    if (standalone || activeTab === "terminal" || terminalDetached || !connected || connectionHealthChecking) {
      return;
    }
    connectionHealthChecking = true;
    try {
      await runCommandRequest("true", "__healthcheck__", false);
    } catch {
      stopConnectionHealthCheck();
      try {
        await disconnect();
      } catch {
        connected = false;
        setConnectionIdentity("", "", 22);
        setStatus("SSH 连接已断开", "idle");
      }
      showToast("SSH 连接已断开", "warning");
    } finally {
      connectionHealthChecking = false;
    }
  }

  function startConnectionHealthCheck() {
    if (standalone || activeTab === "terminal" || terminalDetached || connectionHealthTimer) {
      return;
    }
    connectionHealthTimer = window.setInterval(() => {
      runConnectionHealthCheck().catch(() => {});
    }, CONNECTION_HEALTHCHECK_INTERVAL_MS);
  }

  async function runCommandRequest(command, title = "自定义命令", appendLog = true) {
    if (!connected) {
      throw new Error("请先建立 SSH 连接");
    }
    const result = await runCommand({
      title,
      command,
    });
    if (appendLog && result) {
      await appendCommandQueryLog(result.title, result.command, result.output);
    }
    return result;
  }

  function setQuickResult(title, command, output) {
    currentCommandTitle = title || "";
    currentCommand = command || "";
    quickResultTitle = title || "";
    quickResultCommand = command || "";
    quickResultOutput = output || "(无输出)";
  }

  function setQuickCommandStatus(item, status, message) {
    quickCommandStatuses = {
      ...quickCommandStatuses,
      [item.id]: { status, message },
    };
  }

  function setQuickCommandSummary(summary) {
    quickCommandResults = {
      ...quickCommandResults,
      [summary.item.id]: summary,
    };
    if (summary.result) {
      setQuickResult(summary.result.title, summary.result.command, summary.result.output);
    }
  }

  async function executeQuickCommand(item, notify = false, force = false) {
    if (!connected) {
      setStatus("请先建立 SSH 连接", "idle");
      showToast("请先建立 SSH 连接", "warning");
      return;
    }
    if (!force && quickCommandStatuses[item.id]?.status === "success") {
      const summary = quickCommandResults[item.id];
      if (summary?.result) {
        setQuickResult(summary.result.title, summary.result.command, summary.result.output);
      }
      return summary;
    }
    currentCommandTitle = item.name;
    currentCommand = item.command;
    setQuickCommandStatus(item, "running", "执行中");
    try {
      const result = item.source
        ? await runCommandRequest(item.command, item.name)
        : await runQuickCommand(item.id);
      if (result) {
        setQuickCommandStatus(item, "success", "成功执行");
        if (notify) {
          showToast(`${result.title || item.name} 执行完成`, "success");
        }
        const summary = { item, result, status: "success", message: "成功执行" };
        setQuickCommandSummary(summary);
        return summary;
      }
      setQuickCommandStatus(item, "success", "成功执行");
      const summary = { item, result: null, status: "success", message: "成功执行" };
      setQuickCommandSummary(summary);
      return summary;
    } catch (error) {
      const message = String(error);
      setQuickCommandStatus(item, "error", "执行失败");
      if (notify) {
        showToast(`${item.name} 执行失败`, "error");
      }
      const summary = { item, result: { title: item.name, command: item.command, output: message }, status: "error", message: "执行失败" };
      setQuickCommandSummary(summary);
      return summary;
    }
  }

  async function executeAllQuickCommands() {
    if (!connected) {
      setStatus("请先建立 SSH 连接", "idle");
      showToast("请先建立 SSH 连接", "warning");
      return;
    }
    quickOverviewOpen = false;
    for (const item of visibleQuickCommands) {
      await executeQuickCommand(item, true, true);
    }
  }

  async function deleteCommand(id) {
    try {
      quickCommands = await deleteQuickCommand(id);
      quickContextMenuOpen = false;
      if (quickContextTargetCommandId === id) {
        quickContextTargetCommandId = "";
      }
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function moveCommandToShell(id, fileName) {
    const command = quickCommands.find((item) => item.id === id);
    if (!command) {
      return;
    }
    try {
      await appendCommandToJSONShell(fileName, {
        id: command.id,
        name: command.name,
        command: command.command,
      });
      quickCommands = await deleteQuickCommand(id);
      await loadShellFiles();
      await loadSelectedShellCommands(fileName);
      selectedJSONShellFile = fileName;
      showToast("命令已转到扩展 JSON", "success");
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function saveCurrentRecord() {
    const host = String(form.host || "").trim();
    const user = String(form.user || "").trim();
    if (!host || !user) {
      showToast("host 和 user 不能为空", "warning");
      return;
    }
    try {
      serverRecords = await saveServerRecord({
        id: currentRecordId(),
        name: String(form.serverName || "").trim(),
        host,
        port: Number(form.port) || 22,
        user,
        password: form.password || "",
        keyPath: String(form.keyPath || "").trim(),
        description: String(form.serverDescription || "").trim(),
      });
      showToast("服务器记录已保存", "success");
    } catch (error) {
      showToast(error, "error");
    }
  }

  function requestReplaceConnect(target) {
    replaceConnectTarget = target;
    replaceConnectConfirmBusy = false;
    replaceConnectConfirmOpen = true;
  }

  async function confirmReplaceConnect() {
    if (replaceConnectConfirmBusy || !replaceConnectTarget) {
      return;
    }
    replaceConnectConfirmBusy = true;
    const nextTarget = replaceConnectTarget;
    try {
      await disconnect().catch((error) => {
        throw new Error(String(error));
      });
      replaceConnectConfirmOpen = false;
      replaceConnectTarget = null;
      await connectWithCurrentForm({ forceReplace: true, target: nextTarget });
      return;
    } catch (error) {
      showToast(error, "error");
    } finally {
      replaceConnectConfirmBusy = false;
      if (replaceConnectTarget === nextTarget && !replaceConnectConfirmOpen) {
        replaceConnectTarget = null;
      }
    }
  }

  async function connectWithCurrentForm(options = {}) {
    const target = options.target || form;
    const host = String(target.host || "").trim();
    const user = String(target.user || "").trim();
    const port = Number(target.port) || 22;
    const password = String(target.password ?? form.password ?? "");
    const keyPath = String(target.keyPath ?? form.keyPath ?? "").trim();
    const logging = Boolean(target.logging ?? form.logging);
    if (connected && isSameConnectionTarget(host, user, port)) {
      activeTab = "response";
      showToast(`已连接到 ${currentConnectedLabel()}，已切换到应急响应`, "info");
      return;
    }
    if (connected && !options.forceReplace) {
      requestReplaceConnect({
        user,
        host,
        port,
        password,
        keyPath,
        logging,
        label: `${user || "--"}@${host || "--"}`,
        currentLabel: currentConnectedLabel(),
      });
      return;
    }
    const attemptId = ++connectAttemptSequence;
    const connectModalStartedAt = Date.now();
    const hadActiveConnection = connected;
    activeConnectAttemptId = attemptId;
    rememberPendingConnectState();
    const pendingTerminalSessionId = standalone ? "" : createPendingTerminalSession(user, host);
    setStatus("正在建立 SSH PTY 连接...", "busy");
    setConnectionIdentity(user, host, port);
    await tick();
    connectModalUser = user || "用户";
    connectModalTarget = host ? `${user || "--"}@${host}` : `${user || "--"}@--`;
    connectModalError = "";
    connectModalOpen = true;
    startConnectModalTimer(connectModalStartedAt);
    try {
      await connect({
        host,
        port,
        user,
        password,
        keyPath,
        cols: terminalApi?.getSize().cols || Number(form.cols) || 120,
        rows: terminalApi?.getSize().rows || Number(form.rows) || 32,
        logging,
      });
      if (cancelledConnectAttemptIds.has(attemptId)) {
        await waitForMinimumConnectModalDuration(connectModalStartedAt);
        stopConnectModalTimer();
        removeTerminalSession(pendingTerminalSessionId);
        suppressNextDisconnectEventUi = true;
        await disconnect().catch(() => {});
        return;
      }
      finalizeTerminalSession(pendingTerminalSessionId);
      resetResponseState();
      syncTerminalToSession(pendingTerminalSessionId);
      await touchServerRecord(
        host,
        port,
        user,
        password,
        keyPath,
      );
      await loadRecords();
      await waitForMinimumConnectModalDuration(connectModalStartedAt);
      stopConnectModalTimer();
      activeTab = "response";
      connectModalOpen = false;
      connectModalError = "";
      showToast("SSH 连接已建立", "success");
      clearPendingConnectState();
    } catch (error) {
      const message = String(error);
      await waitForMinimumConnectModalDuration(connectModalStartedAt);
      stopConnectModalTimer();
      if (cancelledConnectAttemptIds.has(attemptId)) {
        removeTerminalSession(pendingTerminalSessionId);
        return;
      }
      removeTerminalSession(pendingTerminalSessionId);
      const stillConnected = await getConnected().catch(() => false);
        if (hadActiveConnection && stillConnected) {
          restorePendingConnectState();
        } else {
          setStatus(message, "idle");
          setConnectionIdentity("", "", 22);
      }
      connectModalError = message;
      showToast(error, "error");
      terminalApi?.write(`\r\n\x1b[37m${message}\x1b[0m\r\n`);
    } finally {
      cancelledConnectAttemptIds.delete(attemptId);
      if (activeConnectAttemptId === attemptId) {
        activeConnectAttemptId = 0;
      }
      if (pendingConnectPreviousState && !connectModalOpen) {
        clearPendingConnectState();
      }
    }
  }

  async function executeManualCommand() {
    const matchedCommand = visibleQuickCommands.find((item) => item.command === currentCommand);
    if (matchedCommand) {
      setQuickCommandStatus(matchedCommand, "running", "执行中");
    }
    try {
      const result = await runCommandRequest(currentCommand, currentCommandTitle || "自定义命令");
      if (result) {
        setQuickResult(result.title, result.command, result.output);
        if (matchedCommand) {
          setQuickCommandStatus(matchedCommand, "success", "成功执行");
          setQuickCommandSummary({ item: matchedCommand, result, status: "success", message: "成功执行" });
        }
      }
    } catch (error) {
      const message = String(error);
      showToast(error, "error");
      setQuickResult(currentCommandTitle || "自定义命令", currentCommand, message);
      if (matchedCommand) {
        setQuickCommandStatus(matchedCommand, "error", "执行失败");
        setQuickCommandSummary({
          item: matchedCommand,
          result: { title: matchedCommand.name, command: matchedCommand.command, output: message },
          status: "error",
          message: "执行失败",
        });
      }
    }
  }

  async function addCommand() {
    const name = String(quickAddForm.name || "").trim();
    const command = String(quickAddForm.command || "").trim();
    if (!name || !command) {
      showToast("命令名称和内容不能为空", "warning");
      return;
    }
    try {
      quickCommands = await addQuickCommand({ name, command });
      quickAddForm = { name: "", command: "" };
      quickAddOpen = false;
      showToast("命令已添加", "success");
    } catch (error) {
      showToast(error, "error");
    }
  }

  async function createShellAndMoveCommand() {
    const fileName = normalizeJSONShellFileName(jsonShellName);
    if (!fileName) {
      showToast("扩展 JSON 名称无效", "warning");
      return;
    }
    const targetId = jsonShellCreateTargetCommandId;
    jsonShellName = "";
    jsonShellCreateTargetCommandId = "";
    jsonShellOpen = false;
    await moveCommandToShell(targetId, fileName);
  }

  function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed;
    saveBooleanSetting("sidebarCollapsed", sidebarCollapsed);
  }

  function setSplashMode(enabled) {
    splashEnabled = enabled;
    saveBooleanSetting("splashEnabled", enabled);
  }

  function setLaunchInfoMode(enabled) {
    launchInfoEnabled = enabled;
    saveBooleanSetting("launchInfoEnabled", enabled);
  }

  function setQuickOutputLineNumbersState(enabled) {
    quickOutputLineNumbers = enabled;
    saveBooleanSetting("quickLineNumbers", enabled);
  }

  async function selectTab(tab) {
    activeTab = tab;
    if (tab !== "terminal" || standalone) {
      return;
    }
    await tick();
    await syncTerminalSize();
    terminalApi?.focus();
  }

  async function openDetachedTerminalWindow() {
    if (!connected) {
      showToast("请先建立 SSH 连接", "warning");
      return;
    }
    const host = String(form.host || currentConnectionIdentity.host || "").trim();
    const user = String(form.user || currentConnectionIdentity.user || "").trim();
    if (!host || !user) {
      showToast("当前连接信息不完整", "warning");
      return;
    }
    await disconnectSecondary().catch(() => {});
    await connectSecondary({
      host,
      port: Number(currentConnectionIdentity.port || form.port) || 22,
      user,
      password: form.password || "",
      keyPath: String(form.keyPath || "").trim(),
      cols: Number(form.cols) || 120,
      rows: Number(form.rows) || 32,
      logging: false,
    });
    terminalDetached = true;
    await openTerminalWindow();
  }

  async function returnDetachedTerminalToMainWindow() {
    terminalDetached = false;
    await disconnectSecondary().catch(() => {});
    await returnTerminalToMain();
  }

  async function handleWindowAction(action) {
    if (action === "minimise") {
      await windowMinimise();
      return;
    }
    if (action === "maximise") {
      if (await windowIsMaximised()) {
        await windowRestore();
      } else {
        await windowMaximise();
      }
      return;
    }
    if (action === "close") {
      requestClose(standalone ? "standalone" : "main");
    }
  }

  function handleTerminalReady(api) {
    terminalApi = api;
    if (!terminalApi) {
      return;
    }
    queueMicrotask(async () => {
      if (!standalone && activeTerminalSessionId) {
        syncTerminalToSession(activeTerminalSessionId);
      }
      if (standalone || activeTab === "terminal") {
        await syncTerminalSize().catch(() => {});
      }
    });
  }

  async function initialiseState() {
    sidebarCollapsed = loadBooleanSetting("sidebarCollapsed", false);
    splashEnabled = loadBooleanSetting("splashEnabled", false);
    launchInfoEnabled = loadBooleanSetting("launchInfoEnabled", false);
    quickOutputLineNumbers = loadBooleanSetting("quickLineNumbers", true);
    selectedJSONShellFile = loadStringSetting("selectedJSONShellFile", "");
    const firstLaunch = !standalone && loadStringSetting(FIRST_LAUNCH_STATE_KEY, "") !== "done";

    splashVisible = (splashEnabled && !standalone) || firstLaunch;
    launchInfoOpen = false;
    if (splashVisible) {
      stopSplashTimer();
      splashTimer = window.setTimeout(() => {
        splashVisible = false;
        splashTimer = 0;
        if (launchInfoEnabled || firstLaunch) {
          launchInfoOpen = true;
        }
      }, 3000);
    } else if ((launchInfoEnabled || firstLaunch) && !standalone) {
      launchInfoOpen = true;
    }
    if (firstLaunch) {
      saveStringSetting(FIRST_LAUNCH_STATE_KEY, "done");
    }

    await Promise.all([
      loadRecords(),
      loadCommands(),
      loadShellFiles(),
    ]);

    if (selectedJSONShellFile) {
      await loadSelectedShellCommands(selectedJSONShellFile);
    }

    try {
      connected = standalone ? await getSecondaryConnected() : await getConnected();
      if (connected) {
        if (!standalone) {
          startConnectionHealthCheck();
        }
        setStatus("已连接到远程终端", "connected");
      }
    } catch (error) {
      setStatus(error, "idle");
    }

    try {
      terminalDetached = await getTerminalDetached();
    } catch (error) {
      showToast(error, "error");
    }

  }

  onMount(() => {
    const outputEventName = standalone ? "ssh:output:secondary" : "ssh:output";
    const stateEventName = standalone ? "ssh:state:secondary" : "ssh:state";
    const disposeOutput = onRuntimeEvent(outputEventName, (data) => {
      processTerminalOutput(data);
    });
    const disposeState = onRuntimeEvent(stateEventName, async (data) => {
      connected = Boolean(data.connected);
      const suppressDisconnectUi = !connected && !standalone && suppressNextDisconnectEventUi;
      if (suppressDisconnectUi) {
        suppressNextDisconnectEventUi = false;
      }
      if (!connected) {
        if (!standalone) {
          stopConnectionHealthCheck();
          if (!suppressDisconnectUi) {
            markTerminalSessionDisconnected(data.message);
          }
          liveTerminalSessionId = "";
        }
        if (!standalone) {
          setConnectionIdentity("", "", 22);
        }
      } else {
        if (!standalone) {
          startConnectionHealthCheck();
          if (!currentConnectionIdentity.user || !currentConnectionIdentity.host) {
            setConnectionIdentity(form.user, form.host, form.port);
          }
          if (!liveTerminalSessionId && activeTerminalSessionId) {
            finalizeTerminalSession(activeTerminalSessionId);
          }
        }
      }
      if (!suppressDisconnectUi) {
        setStatus(data.message, connected ? "connected" : "idle");
        showToast(data.message, connected ? "success" : "error");
      }
      if (connected) {
        await tick();
        processTerminalOutput("\r\n");
        if (!standalone) {
          syncTerminalSize().catch(() => {});
        } else {
          const size = terminalApi?.getSize?.();
          if (size) {
            resizeSecondaryTerminal(size.cols, size.rows).catch(() => {});
          }
        }
        terminalApi?.focus();
      } else if (!suppressDisconnectUi) {
        if (!standalone) {
          const disconnectOutput = buildTerminalDisconnectOutput(data.message);
          if (activeTerminalSessionId) {
            syncTerminalToSession(activeTerminalSessionId);
          } else {
            processTerminalOutput(disconnectOutput);
          }
        } else {
          processTerminalOutput(buildTerminalDisconnectOutput(data.message));
        }
      }
    });
    const disposeDetached = onRuntimeEvent("terminal:detached", async (data) => {
      terminalDetached = Boolean(data);
      if (!terminalDetached && (standalone || activeTab === "terminal")) {
        await tick();
        await syncTerminalSize().catch(() => {});
      }
    });

    initialiseState();

    const handleResize = () => {
      if (standalone || activeTab === "terminal") {
        syncTerminalSize().catch(() => {});
      }
      quickContextMenuOpen = false;
    };
    const handleKeydown = (event) => {
      if (event.key !== "Escape") {
        quickContextMenuOpen = false;
        return;
      }
      quickContextMenuOpen = false;
      if (jsonShellOpen) {
        jsonShellOpen = false;
        return;
      }
      if (quickAddOpen) {
        quickAddOpen = false;
        return;
      }
      if (settingsOpen) {
        settingsOpen = false;
      }
    };
    const handleClick = (event) => {
      if (event.target?.closest?.(".context-menu")) {
        return;
      }
      quickContextMenuOpen = false;
    };

    window.addEventListener("resize", handleResize);
    window.addEventListener("keydown", handleKeydown);
    window.addEventListener("click", handleClick);

    return () => {
      stopConnectionHealthCheck();
      stopSplashTimer();
      disposeOutput?.();
      disposeState?.();
      disposeDetached?.();
      window.removeEventListener("resize", handleResize);
      window.removeEventListener("keydown", handleKeydown);
      window.removeEventListener("click", handleClick);
    };
  });

  $: saveStringSetting("selectedJSONShellFile", selectedJSONShellFile);
  $: {
    if (standalone || activeTab === "terminal" || terminalDetached || !connected) {
      stopConnectionHealthCheck();
    } else {
      startConnectionHealthCheck();
    }
  }
</script>

{#if standalone}
  <div class="terminal-window-shell">
    <TitleBar title="独立终端" on:action={(event) => handleWindowAction(event.detail)} />

    <main class="terminal-window-main">
      <TerminalPanel
        mode="standalone"
        {terminalApi}
        {connected}
        detached={false}
        statusMessage={statusMessage}
        sessions={[]}
        activeSessionId=""
        onTerminalReady={handleTerminalReady}
        onTerminalInput={handleTerminalInput}
        onClearTerminal={handleTerminalClear}
        onSelectSession={() => {}}
        onCloseSession={() => {}}
        onReturnToMain={async () => { await returnDetachedTerminalToMainWindow(); await windowClose(); }}
      />
    </main>
  </div>
{:else}
  <div class="app-shell" class:sidebar-collapsed={sidebarCollapsed}>
    {#if splashVisible}
      <div class="splash-screen" aria-hidden="true">
        <div class="splash-butterfly-wrap">
          <svg class="splash-butterfly" viewBox="0 0 320 240" role="img" aria-label="Butterfly">
            <path class="splash-line splash-line-body" pathLength="1" style="--delay:0.10s;--duration:0.42s;" d="M156 76 C148 98, 149 121, 160 141 C171 121, 172 98, 164 76" />
            <path class="splash-line splash-line-body" pathLength="1" style="--delay:0.40s;--duration:0.46s;" d="M160 140 C150 159, 149 183, 160 208 C171 183, 170 159, 160 140" />
            <path class="splash-line splash-line-accent" pathLength="1" style="--delay:0.02s;--duration:0.30s;" d="M156 74 C147 57, 139 45, 127 33" />
            <path class="splash-line splash-line-accent" pathLength="1" style="--delay:0.08s;--duration:0.30s;" d="M164 74 C173 57, 181 45, 193 33" />

            <path class="splash-line splash-line-wing-primary" pathLength="1" style="--delay:0.68s;--duration:0.86s;" d="M160 118 C136 64, 76 26, 24 40 C6 74, 10 124, 44 154 C78 182, 120 182, 152 146" />
            <path class="splash-line splash-line-wing-primary" pathLength="1" style="--delay:0.86s;--duration:0.86s;" d="M160 118 C184 64, 244 26, 296 40 C314 74, 310 124, 276 154 C242 182, 200 182, 168 146" />
            <path class="splash-line splash-line-wing-secondary" pathLength="1" style="--delay:1.08s;--duration:0.78s;" d="M156 124 C128 152, 98 188, 58 210 C30 224, 8 218, 2 194 C10 154, 38 126, 78 118 C112 112, 136 114, 156 124" />
            <path class="splash-line splash-line-wing-secondary" pathLength="1" style="--delay:1.24s;--duration:0.78s;" d="M164 124 C192 152, 222 188, 262 210 C290 224, 312 218, 318 194 C310 154, 282 126, 242 118 C208 112, 184 114, 164 124" />

            <path class="splash-line splash-line-detail" pathLength="1" style="--delay:1.46s;--duration:0.54s;" d="M151 110 C122 92, 89 84, 60 92" />
            <path class="splash-line splash-line-detail" pathLength="1" style="--delay:1.56s;--duration:0.54s;" d="M169 110 C198 92, 231 84, 260 92" />
            <path class="splash-line splash-line-detail" pathLength="1" style="--delay:1.68s;--duration:0.58s;" d="M149 136 C124 148, 98 164, 78 184" />
            <path class="splash-line splash-line-detail" pathLength="1" style="--delay:1.80s;--duration:0.58s;" d="M171 136 C196 148, 222 164, 242 184" />
          </svg>
          <div class="splash-wordmark">LinuxSafeTools</div>
        </div>
      </div>
    {/if}

    <ToastStack {toasts} on:dismiss={(event) => dismissToast(event.detail)} />
    <TitleBar title="LinuxSafeTools" on:action={(event) => handleWindowAction(event.detail)} />

    <Sidebar
      {activeTab}
      {sidebarCollapsed}
      {connected}
      {statusMessage}
      {statusTone}
      statusIdentityLabel={statusIdentityLabel}
      on:selecttab={(event) => selectTab(event.detail).catch((error) => showToast(error, "error"))}
      on:togglecollapse={toggleSidebar}
      on:disconnect={() => disconnect().catch((error) => showToast(error, "error"))}
      on:opensettings={() => settingsOpen = true}
    />

    <main class="workspace">
      <ConnectionPanel
        active={activeTab === "connection"}
        {form}
        {serverRecords}
        currentRecordId={currentRecordId()}
        {connectedRecordId}
        formatTime={formatTime}
        on:updateform={(event) => form = { ...form, ...event.detail }}
        on:connect={connectWithCurrentForm}
        on:disconnect={() => disconnect().catch((error) => showToast(error, "error"))}
        on:saverecord={saveCurrentRecord}
        on:userecord={(event) => fillForm(event.detail)}
        on:connectrecord={async (event) => { fillForm(event.detail); await connectWithCurrentForm(); }}
        on:deleterecord={async (event) => {
          try {
            serverRecords = await deleteServerRecord(event.detail.id);
          } catch (error) {
            showToast(error, "error");
          }
        }}
      />

      <ResponsePanel
        active={activeTab === "response"}
        quickCommands={visibleQuickCommands}
        shellFiles={jsonShellFiles}
        {selectedJSONShellFile}
        {currentCommand}
        {currentCommandTitle}
        resultTitle={quickResultTitle}
        resultCommand={quickResultCommand}
        resultOutput={quickResultOutput}
        {quickCommandStatuses}
        overviewOpen={quickOverviewOpen}
        overviewResults={quickOverviewResults}
        {quickOutputLineNumbers}
        contextMenuOpen={quickContextMenuOpen}
        contextMenuPosition={quickContextMenuPosition}
        on:runquick={(event) => executeQuickCommand(event.detail)}
        on:runall={executeAllQuickCommands}
        on:openoverview={() => quickOverviewOpen = true}
        on:closeoverview={() => quickOverviewOpen = false}
        on:selectshell={(event) => loadSelectedShellCommands(event.detail)}
        on:openaddmodal={() => quickAddOpen = true}
        on:refresh={async () => {
          await loadCommands();
          await loadShellFiles();
          await loadSelectedShellCommands(selectedJSONShellFile);
        }}
        on:updatemanual={(event) => {
          currentCommand = event.detail.command;
          currentCommandTitle = event.detail.title;
        }}
        on:runmanual={executeManualCommand}
        on:togglenumbers={(event) => setQuickOutputLineNumbersState(event.detail)}
        on:opencontext={(event) => {
          quickContextMenuOpen = true;
          quickContextMenuPosition = event.detail.position;
          quickContextTargetCommandId = event.detail.command.id;
        }}
        on:deletecommand={() => deleteCommand(quickContextTargetCommandId)}
        on:moveshell={(event) => moveCommandToShell(quickContextTargetCommandId, event.detail)}
        on:createshell={() => {
          jsonShellCreateTargetCommandId = quickContextTargetCommandId;
          jsonShellName = "";
          jsonShellOpen = true;
          quickContextMenuOpen = false;
        }}
        on:closecontext={() => quickContextMenuOpen = false}
      />

      <TerminalPanel
        active={activeTab === "terminal"}
        mode="main"
        {terminalApi}
        {connected}
        detached={terminalDetached}
        statusMessage={statusMessage}
        sessions={terminalSessions}
        activeSessionId={activeTerminalSessionId}
        onTerminalReady={handleTerminalReady}
        onTerminalInput={handleTerminalInput}
        onClearTerminal={handleTerminalClear}
        onSelectSession={handleTerminalSessionSelect}
        onCloseSession={requestTerminalSessionClose}
        onDetach={() => openDetachedTerminalWindow().catch((error) => showToast(error, "error"))}
      />
    </main>
  </div>

  {#if connectModalOpen}
    <div class="modal-overlay">
      <section class="settings-modal connect-modal">
        <header class="settings-header connect-modal-header">
          <div>
            <h3>{connectModalError ? "连接失败" : `正在连接 ${connectModalUser}`}</h3>
            <p class="settings-note">{connectModalTarget}</p>
          </div>
          <button class="titlebar-btn settings-close overview-close" type="button" aria-label="取消连接" on:click={() => cancelConnectModal()}>取消连接</button>
        </header>
        <div class="connect-modal-body">
          {#if connectModalError}
            <div class="connect-modal-error">{connectModalError}</div>
            <p class="connect-modal-copy">已尝试 {connectModalElapsedSeconds}s</p>
          {:else}
            <div class="connect-spinner" aria-hidden="true"></div>
            <p class="connect-modal-copy">正在连接 {connectModalTarget}</p>
            <p class="connect-modal-copy">已用时 {connectModalElapsedSeconds}s</p>
          {/if}
        </div>
      </section>
    </div>
  {/if}

  {#if replaceConnectConfirmOpen}
    <div class="modal-overlay">
      <section class="settings-modal close-confirm-modal">
        <header class="settings-header connect-modal-header">
          <div>
            <h3>{replaceConnectConfirmBusy ? "正在切换连接" : "切换连接"}</h3>
            <p class="settings-note">
              当前已连接 {replaceConnectTarget?.currentLabel || currentConnectedLabel()}，是否断开并连接到 {replaceConnectTarget?.label || "新的服务器"}？
            </p>
          </div>
          {#if !replaceConnectConfirmBusy}
            <button class="titlebar-btn settings-close overview-close" type="button" aria-label="取消切换连接" on:click={() => { replaceConnectConfirmOpen = false; replaceConnectTarget = null; }}>取消</button>
          {/if}
        </header>
        <div class="connect-modal-body">
          {#if replaceConnectConfirmBusy}
            <div class="connect-spinner" aria-hidden="true"></div>
            <p class="connect-modal-copy">正在断开当前连接并建立新连接…</p>
          {:else}
            <div class="close-confirm-actions">
              <button class="btn btn-secondary" type="button" on:click={() => { replaceConnectConfirmOpen = false; replaceConnectTarget = null; }}>返回</button>
              <button class="btn btn-primary" type="button" on:click={() => confirmReplaceConnect()}>确认切换</button>
            </div>
          {/if}
        </div>
      </section>
    </div>
  {/if}

  {#if launchInfoOpen}
    <div class="modal-overlay">
      <section class="settings-modal launch-info-modal">
        <header class="settings-header">
          <div>
            <p class="section-label">Startup</p>
            <h3>LinuxSafeTools</h3>
          </div>
          <button class="titlebar-btn settings-close" type="button" aria-label="关闭开屏说明窗" on:click={() => launchInfoOpen = false}>X</button>
        </header>

        <div class="launch-info-grid">
          <article class="launch-info-copy">
            <p><strong>工具信息:</strong> Linux 信息查询工具</p>
            <p><strong>技术栈:</strong> Wails3 / Svelte / Golang</p>
            <p><strong>使用:</strong> 通过连接远程服务器执行命令，输出相关信息</p>
          </article>

          <aside class="launch-info-options">
            <div class="launch-info-option-card">
              <p class="section-label">启动动画</p>
              <div class="inline-radio-group" role="radiogroup" aria-label="是否显示启动动画">
                <label class="inline-radio-option settings-toggle-option">
                  <input name="launch-modal-splash-enabled" type="radio" checked={splashEnabled} on:change={() => setSplashMode(true)} />
                  <span>显示</span>
                </label>
                <label class="inline-radio-option settings-toggle-option">
                  <input name="launch-modal-splash-enabled" type="radio" checked={!splashEnabled} on:change={() => setSplashMode(false)} />
                  <span>关闭</span>
                </label>
              </div>
            </div>

            <div class="launch-info-option-card">
              <p class="section-label">开屏说明窗</p>
              <div class="inline-radio-group" role="radiogroup" aria-label="是否显示开屏说明窗">
                <label class="inline-radio-option settings-toggle-option">
                  <input name="launch-modal-info-enabled" type="radio" checked={launchInfoEnabled} on:change={() => setLaunchInfoMode(true)} />
                  <span>显示</span>
                </label>
                <label class="inline-radio-option settings-toggle-option">
                  <input name="launch-modal-info-enabled" type="radio" checked={!launchInfoEnabled} on:change={() => setLaunchInfoMode(false)} />
                  <span>关闭</span>
                </label>
              </div>
            </div>

            <div class="launch-info-actions">
              <button class="btn btn-primary compact" type="button" on:click={() => launchInfoOpen = false}>进入工具</button>
            </div>
          </aside>
        </div>
      </section>
    </div>
  {/if}

  <SettingsModal
    open={settingsOpen}
    splashEnabled={splashEnabled}
    launchInfoEnabled={launchInfoEnabled}
    on:close={() => settingsOpen = false}
    on:toggleSplash={(event) => setSplashMode(event.detail)}
    on:toggleLaunchInfo={(event) => setLaunchInfoMode(event.detail)}
  />

  <QuickAddModal
    open={quickAddOpen}
    form={quickAddForm}
    on:close={() => quickAddOpen = false}
    on:update={(event) => quickAddForm = { ...quickAddForm, ...event.detail }}
    on:submit={addCommand}
  />

  <JSONShellModal
    open={jsonShellOpen}
    value={jsonShellName}
    on:close={() => jsonShellOpen = false}
    on:update={(event) => jsonShellName = event.detail}
    on:submit={createShellAndMoveCommand}
  />
{/if}

{#if closeConfirmOpen}
  <div class="modal-overlay">
    <section class="settings-modal close-confirm-modal">
      <header class="settings-header connect-modal-header">
        <div>
          <h3>{closeConfirmTitle()}</h3>
          <p class="settings-note">{closeConfirmMessage()}</p>
        </div>
        {#if !closeConfirmBusy}
          <button class="titlebar-btn settings-close overview-close" type="button" aria-label="取消关闭" on:click={() => closeConfirmOpen = false}>取消</button>
        {/if}
      </header>
      <div class="connect-modal-body">
        {#if closeConfirmBusy}
          <div class="connect-spinner" aria-hidden="true"></div>
          <p class="connect-modal-copy">正在关闭，请稍候…</p>
        {:else}
          <div class="close-confirm-actions">
            <button class="btn btn-secondary" type="button" on:click={() => closeConfirmOpen = false}>返回</button>
            <button class="btn btn-primary" type="button" on:click={() => confirmClose()}>确认关闭</button>
          </div>
        {/if}
      </div>
    </section>
  </div>
{/if}

{#if terminalSessionCloseConfirmOpen}
  <div class="modal-overlay">
    <section class="settings-modal close-confirm-modal">
      <header class="settings-header connect-modal-header">
        <div>
          <h3>
            {#if terminalSessionCloseConfirmBusy}
              正在关闭终端标签
            {:else if terminalSessionCloseTarget?.status === "connected"}
              关闭活动终端标签
            {:else if terminalSessionCloseTarget?.status === "connecting"}
              取消终端连接
            {:else}
              关闭终端标签
            {/if}
          </h3>
          <p class="settings-note">
            {#if terminalSessionCloseTarget?.status === "connected"}
              确认断开并关闭 {terminalSessionCloseTarget.userName || terminalSessionCloseTarget.label}？
            {:else if terminalSessionCloseTarget?.status === "connecting"}
              确认取消并关闭 {terminalSessionCloseTarget.userName || terminalSessionCloseTarget.label}？
            {:else}
              确认关闭 {terminalSessionCloseTarget?.userName || terminalSessionCloseTarget?.label || "该终端标签"}？
            {/if}
          </p>
        </div>
        {#if !terminalSessionCloseConfirmBusy}
          <button class="titlebar-btn settings-close overview-close" type="button" aria-label="取消关闭终端标签" on:click={() => { terminalSessionCloseConfirmOpen = false; terminalSessionCloseTargetId = ""; }}>取消</button>
        {/if}
      </header>
      <div class="connect-modal-body">
        {#if terminalSessionCloseConfirmBusy}
          <div class="connect-spinner" aria-hidden="true"></div>
          <p class="connect-modal-copy">正在处理，请稍候…</p>
        {:else}
          <div class="close-confirm-actions">
            <button class="btn btn-secondary" type="button" on:click={() => { terminalSessionCloseConfirmOpen = false; terminalSessionCloseTargetId = ""; }}>返回</button>
            <button class="btn btn-primary" type="button" on:click={() => confirmTerminalSessionClose()}>确认关闭</button>
          </div>
        {/if}
      </div>
    </section>
  </div>
{/if}
