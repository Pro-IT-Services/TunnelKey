// Calls into the Go app (Wails bindings) and the Wails runtime.

const go = () => window.go?.main?.App;

/** Calls App.<name>(...args); rejects with the Go error text. */
export function call(name, ...args) {
  const app = go();
  if (!app) return Promise.reject(new Error("backend unavailable"));
  return app[name](...args);
}

export const on = (event, fn) => window.runtime?.EventsOn(event, fn);

export async function clipboardText() {
  try {
    return (await window.runtime?.ClipboardGetText()) ?? "";
  } catch {
    return "";
  }
}

export async function copyText(text) {
  try {
    if (await window.runtime?.ClipboardSetText(text)) return true;
  } catch {
    /* fall through */
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
