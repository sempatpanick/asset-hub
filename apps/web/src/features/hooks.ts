"use client";
import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ApiError, request } from "@/infrastructure/api/client";
export function useResource<T>(path: string | null) {
  const router = useRouter();
  const [snapshot, setSnapshot] = useState<{
    path: string | null;
    revision: number;
    data?: T;
    error: string;
  }>({ path: null, revision: 0, error: "" });
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    if (!path) return;
    const controller = new AbortController();
    request<T>(path, { signal: controller.signal })
      .then((data) => {
        if (!controller.signal.aborted)
          setSnapshot({ path, revision, data, error: "" });
      })
      .catch((e) => {
        if (controller.signal.aborted) return;
        if (e instanceof ApiError && e.status === 401)
          router.replace("/login/");
        setSnapshot({
          path,
          revision,
          error: e.message ?? "Unable to load data",
        });
      });
    return () => controller.abort();
  }, [path, revision, router]);
  const current = snapshot.path === path && snapshot.revision === revision;
  return {
    data: path && current ? snapshot.data : undefined,
    error: path && current ? snapshot.error : "",
    loading: !!path && !current,
    refresh,
  };
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
