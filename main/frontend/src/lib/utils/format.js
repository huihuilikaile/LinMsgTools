export function formatTime(value) {
  if (!value) {
    return "未使用";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString("zh-CN", {
    year: "2-digit",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function recordIdFor(user, host, port) {
  const safeUser = String(user || "").trim();
  const safeHost = String(host || "").trim();
  const safePort = Number(port) || 22;
  if (!safeUser || !safeHost) {
    return "";
  }
  return `${safeUser}@${safeHost}:${safePort}`;
}

export function normalizeJSONShellFileName(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (!raw) {
    return "";
  }
  const normalized = raw
    .replace(/\.json$/i, "")
    .replace(/[^a-z0-9_-]+/g, "_")
    .replace(/^[_-]+|[_-]+$/g, "");
  return normalized ? `${normalized}.json` : "";
}

export function splitOutputLines(value) {
  return String(value || "").split("\n");
}

export function tokenizeOutputLine(value) {
  const line = String(value ?? "");
  const tokens = [];
  const pattern = /((?:~|\.{1,2}|[A-Za-z]:)?(?:\/[^\s]+)+|\b\d+(?:\.\d+)?(?:%|ms|s|m|h|d|[KMGTP]?B)?\b)/g;
  let lastIndex = 0;

  for (const match of line.matchAll(pattern)) {
    const [text] = match;
    const index = match.index ?? 0;
    if (index > lastIndex) {
      tokens.push({ type: "text", value: line.slice(lastIndex, index) });
    }
    tokens.push({
      type: text.includes("/") ? "path" : "number",
      value: text,
    });
    lastIndex = index + text.length;
  }

  if (lastIndex < line.length) {
    tokens.push({ type: "text", value: line.slice(lastIndex) });
  }

  return tokens.length ? tokens : [{ type: "text", value: line }];
}
