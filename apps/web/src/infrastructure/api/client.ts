let csrf = "";
export function setCsrf(value: string) {
  csrf = value;
}
export function uploadPart(
  path: string,
  body: Blob,
  hash: string,
  signal: AbortSignal,
  progress: (loaded: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    if (signal.aborted) {
      reject(new DOMException("Paused", "AbortError"));
      return;
    }
    signal.addEventListener("abort", abort, { once: true });
    xhr.open("PUT", `/v1${path}`);
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("X-CSRF-Token", csrf);
    xhr.setRequestHeader("X-Content-SHA256", hash);
    xhr.upload.onprogress = (e) => progress(e.loaded);
    xhr.onload = () => {
      signal.removeEventListener("abort", abort);
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else {
        let message = "Part transfer failed";
        try {
          message = JSON.parse(xhr.responseText).error?.message ?? message;
        } catch {}
        reject(new ApiError(xhr.status, "upload_failed", message));
      }
    };
    xhr.onerror = () => {
      signal.removeEventListener("abort", abort);
      reject(new Error("Network error. Resume the transfer to retry."));
    };
    xhr.onabort = () => {
      signal.removeEventListener("abort", abort);
      reject(new DOMException("Paused", "AbortError"));
    };
    xhr.send(body);
  });
}
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
export async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (options.method && !["GET", "HEAD"].includes(options.method))
    headers.set("X-CSRF-Token", csrf);
  const response = await fetch(`/v1${path}`, {
    ...options,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });
  const data = await response.json().catch(() => null);
  if (!response.ok)
    throw new ApiError(
      response.status,
      data?.error?.code ?? "unavailable",
      data?.error?.message ?? "AssetHub is unavailable. Check the API service.",
    );
  return data as T;
}
export async function mutate<T>(path: string, body?: unknown, method = "POST") {
  const result = await request<T>(path, {
    method,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  window.dispatchEvent(new Event("assethub:changed"));
  return result;
}
