"use client";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import { ApiError, request } from "@/infrastructure/api/client";
export function useResource<T>(path: string | null, interval = 5000) {
  const router = useRouter();
  const [snapshot, setSnapshot] = useState<{
    path: string | null;
    data?: T;
    error: string;
  }>({ path: null, error: "" });
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    if (!path) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let busy = false;
    let queued = false;
    let delay = interval;
    const load = async () => {
      clearTimeout(timer);
      if (controller.signal.aborted) return;
      if (busy) {
        queued = true;
        return;
      }
      if (document.hidden || !navigator.onLine) return;
      busy = true;
      try {
        const data = await request<T>(path, {
          signal: AbortSignal.any([
            controller.signal,
            AbortSignal.timeout(15000),
          ]),
        });
        if (!controller.signal.aborted) setSnapshot({ path, data, error: "" });
        delay = interval;
      } catch (e) {
        if (controller.signal.aborted) return;
        if (e instanceof ApiError && e.status === 401) {
          router.replace("/login/");
          controller.abort();
          return;
        }
        setSnapshot((previous) => ({
          path,
          data:
            previous.path === path &&
            !(e instanceof ApiError && [403, 404].includes(e.status))
              ? previous.data
              : undefined,
          error: e instanceof Error ? e.message : "Unable to load data",
        }));
        delay = Math.min(Math.max(delay * 2, 5000), 60000);
      } finally {
        busy = false;
        if (!controller.signal.aborted && interval)
          timer = setTimeout(() => void load(), queued ? 0 : delay);
        queued = false;
      }
    };
    const wake = () => {
      if (!document.hidden) void load();
    };
    const changed = () => {
      clearTimeout(timer);
      timer = setTimeout(() => void load(), 150);
    };
    void load();
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    window.addEventListener("online", wake);
    window.addEventListener("assethub:changed", changed);
    return () => {
      controller.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
      window.removeEventListener("online", wake);
      window.removeEventListener("assethub:changed", changed);
    };
  }, [path, revision, router, interval]);
  const current = snapshot.path === path;
  return {
    data: path && current ? snapshot.data : undefined,
    error: path && current ? snapshot.error : "",
    loading: !!path && !current,
    refresh,
  };
}
export function useDebounced<T>(value: T, delay = 250) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}
const subscribeLocation = (listener: () => void) => {
  window.addEventListener("popstate", listener);
  return () => window.removeEventListener("popstate", listener);
};
export function useLocationQuery() {
  return useSyncExternalStore(
    subscribeLocation,
    () => window.location.search,
    () => "",
  );
}
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = async (task: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await task();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Request failed");
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, run };
}
export function bytes(n: number) {
  if (!n) return "0 B";
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 4);
  return `${(n / 1024 ** i).toLocaleString(undefined, { maximumFractionDigits: i ? 1 : 0 })} ${["B", "KB", "MB", "GB", "TB"][i]}`;
}
