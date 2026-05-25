function readStorage(key) {
  try {
    return window.localStorage.getItem(`linuxsafetools:${key}`);
  } catch {
    return null;
  }
}

function writeStorage(key, value) {
  try {
    window.localStorage.setItem(`linuxsafetools:${key}`, value);
  } catch {
    // ignore persistence errors
  }
}

export function loadBooleanSetting(key, fallback) {
  const value = readStorage(key);
  if (value == null) {
    return fallback;
  }
  return value === "true";
}

export function saveBooleanSetting(key, value) {
  writeStorage(key, String(Boolean(value)));
}

export function loadStringSetting(key, fallback = "") {
  const value = readStorage(key);
  return value == null ? fallback : value;
}

export function saveStringSetting(key, value) {
  writeStorage(key, String(value ?? ""));
}
