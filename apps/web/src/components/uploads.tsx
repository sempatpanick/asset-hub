"use client";
import { useEffect, useRef, useState } from "react";
import { CloudUpload, Pause, Play, RefreshCw, X } from "lucide-react";
import type { Asset, Items, Project, Upload } from "@assethub/api-client";
import { mutate, request, uploadPart } from "@/infrastructure/api/client";
import { bytes } from "@/features/hooks";
import { Badge, Empty, ErrorNotice, Modal } from "./ui";

type Transfer = { file: File; status: string; loaded: number; error?: string };
export function UploadDialog({
  projects,
  project,
  userID,
  onClose,
  onSaved,
}: {
  projects: Project[];
  project: string;
  userID: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [selected, setSelected] = useState(project || projects[0]?.id || "");
  const [transfers, setTransfers] = useState<Transfer[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [dragging, setDragging] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);
  const update = (file: File, patch: Partial<Transfer>) =>
    setTransfers((values) =>
      values.map((v) => (v.file === file ? { ...v, ...patch } : v)),
    );
  const add = (files: File[]) => {
    if (busy) return;
    if (files.some((f) => f.size < 1 || f.size > 512 * 1024 ** 2)) {
      setError("Choose non-empty files of up to 512 MB each.");
      return;
    }
    setError("");
    setTransfers((values) => [
      ...values,
      ...files
        .filter(
          (f) =>
            !values.some(
              (v) =>
                v.file.name === f.name &&
                v.file.size === f.size &&
                v.file.lastModified === f.lastModified,
            ),
        )
        .map((file) => ({ file, status: "pending", loaded: 0 })),
    ]);
  };
  const start = async () => {
    if (!selected || busy) return;
    if (!crypto.subtle) {
      setError("Uploads require HTTPS or localhost for checksum validation.");
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    setError("");
    for (const transfer of transfers.filter((v) => v.status !== "queued")) {
      const file = transfer.file;
      const key = `assethub:upload:${userID}:${selected}:${encodeURIComponent(file.name)}:${file.size}:${file.lastModified}`;
      try {
        if (controller.signal.aborted) break;
        update(file, { status: "uploading", error: undefined });
        let upload: Upload | undefined;
        const id = localStorage.getItem(key);
        if (id) {
          try {
            upload = await request<Upload>(`/uploads/${id}`, {
              signal: controller.signal,
            });
            if (
              !["receiving", "completing", "queued", "complete"].includes(
                upload.status,
              ) ||
              new Date(upload.expires_at) < new Date()
            )
              upload = undefined;
          } catch (e) {
            if (e instanceof Error && "status" in e && e.status === 404)
              upload = undefined;
            else throw e;
          }
        }
        if (!upload) {
          upload = await mutate<Upload>("/uploads", {
            project_id: selected,
            name: file.name,
            size_bytes: file.size,
          });
          localStorage.setItem(key, upload.id);
        }
        if (!upload.asset_id) {
          for (
            let index = 0;
            index < Math.ceil(file.size / upload.part_bytes);
            index++
          ) {
            if (controller.signal.aborted)
              throw new DOMException("Paused", "AbortError");
            const offset = index * upload.part_bytes;
            const part = file.slice(
              offset,
              Math.min(offset + upload.part_bytes, file.size),
            );
            const digest = await crypto.subtle.digest(
              "SHA-256",
              await part.arrayBuffer(),
            );
            const hash = Array.from(new Uint8Array(digest), (b) =>
              b.toString(16).padStart(2, "0"),
            ).join("");
            const existing = upload.parts.find((p) => p.index === index);
            if (existing) {
              if (existing.sha256 !== hash)
                throw new Error(
                  "The selected file differs from this upload session. Cancel the session and select the original file.",
                );
              update(file, { loaded: offset + part.size });
              continue;
            }
            await uploadPart(
              `/uploads/${upload.id}/parts/${index}`,
              part,
              hash,
              controller.signal,
              (loaded) =>
                update(file, { loaded: Math.min(file.size, offset + loaded) }),
            );
          }
          if (controller.signal.aborted)
            throw new DOMException("Paused", "AbortError");
          update(file, { status: "validating" });
          await mutate<Asset>(`/uploads/${upload.id}/complete`);
        }
        localStorage.removeItem(key);
        update(file, { status: "queued", loaded: file.size });
        onSaved();
      } catch (e) {
        const paused = e instanceof Error && e.name === "AbortError";
        update(file, {
          status: paused ? "paused" : "failed",
          error: paused
            ? undefined
            : e instanceof Error
              ? e.message
              : "Upload failed",
        });
        if (paused) break;
      }
    }
    active.current = null;
    setBusy(false);
  };
  return (
    <Modal
      title="Upload your assets"
      detail="Drop in your files. We’ll keep them organized and protected."
      onClose={() => {
        active.current?.abort();
        onClose();
      }}
    >
      <label className="upload-project">
        Project
        <select
          aria-label="Upload project"
          disabled={busy || transfers.length > 0}
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
        >
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </label>
      <div
        className={`upload-drop ${dragging ? "dragging" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          add(Array.from(e.dataTransfer.files));
        }}
      >
        <CloudUpload size={34} />
        <strong>Drag files here</strong>
        <span>Images, videos, documents and archives · up to 512 MB</span>
        <label className="button">
          Choose files
          <input
            type="file"
            multiple
            disabled={busy || !selected}
            onChange={(e) => {
              add(Array.from(e.target.files ?? []));
              e.target.value = "";
            }}
          />
        </label>
      </div>
      <ErrorNotice message={error} />
      <div className="transfer-list" aria-live="polite">
        {transfers.map((v, index) => (
          <div className="transfer" key={`${v.file.name}:${index}`}>
            <div className="transfer-top">
              <strong>{v.file.name}</strong>
              <Badge tone={v.status === "queued" ? "mint" : "blue"}>
                {v.status}
              </Badge>
            </div>
            <progress
              max={v.file.size}
              value={v.loaded}
              aria-label={`${v.file.name} transfer progress`}
            />
            <span>
              {bytes(v.loaded)} / {bytes(v.file.size)}
              {v.status === "queued" ? " · awaiting verified storage" : ""}
            </span>
            <ErrorNotice message={v.error ?? ""} />
          </div>
        ))}
      </div>
      <div className="modal-actions">
        {busy ? (
          <button className="button" onClick={() => active.current?.abort()}>
            <Pause size={16} />
            Pause transfer
          </button>
        ) : (
          <button
            className="button primary"
            disabled={
              !transfers.some((v) => v.status !== "queued") || !selected
            }
            onClick={() => void start()}
          >
            <Play size={16} />
            Start / resume uploads
          </button>
        )}
        <button
          className="button"
          disabled={busy}
          onClick={() => setTransfers([])}
        >
          Clear selection
        </button>
      </div>
      <p className="upload-hint">
        Transfer progress tracks your browser upload. Storage is complete only
        when the asset is marked ready. To resume after closing this window,
        select the same files and project.
      </p>
    </Modal>
  );
}

export function UploadHistory({
  enabled,
  onUpload,
}: {
  enabled: boolean;
  onUpload: () => void;
}) {
  const [items, setItems] = useState<Upload[]>([]);
  const [error, setError] = useState("");
  const refresh = async () => {
    try {
      const data = await request<Items<Upload>>("/uploads");
      setItems(data.items);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load uploads");
    }
  };
  useEffect(() => {
    let live = true;
    const controller = new AbortController();
    const load = async () => {
      try {
        const data = await request<Items<Upload>>("/uploads", {
          signal: controller.signal,
        });
        if (live) {
          setItems(data.items);
          setError("");
        }
      } catch (e) {
        if (live)
          setError(e instanceof Error ? e.message : "Unable to load uploads");
      }
    };
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => {
      live = false;
      controller.abort();
      clearInterval(timer);
    };
  }, []);
  const action = async (u: Upload) => {
    try {
      if (u.asset_status === "failed")
        await mutate(`/assets/${u.asset_id}/retry`);
      else await mutate(`/uploads/${u.id}`, undefined, "DELETE");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Action failed");
    }
  };
  return (
    <section className="panel upload-history">
      <div className="panel-heading">
        <div>
          <h2>Upload history</h2>
          <p>Browser transfers and durable storage jobs</p>
        </div>
        <button
          className="icon-button"
          aria-label="Refresh uploads"
          onClick={() => void refresh()}
        >
          <RefreshCw size={17} />
        </button>
      </div>
      <ErrorNotice message={error} />
      {!items.length ? (
        <Empty
          title="Your files begin here"
          detail={
            enabled
              ? "Upload to a project and follow each file as it becomes ready."
              : "Configure storage credentials, encryption and an independent backup to enable uploads."
          }
          action={
            enabled ? (
              <button className="button primary" onClick={onUpload}>
                Upload files
              </button>
            ) : undefined
          }
        />
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>File</th>
                <th>Size</th>
                <th>Status</th>
                <th>Storage progress</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((u) => {
                const status = u.asset_status || u.status;
                return (
                  <tr key={u.id}>
                    <td>
                      <strong>{u.name}</strong>
                      {u.error_code && (
                        <small className="job-error">
                          {u.error_code.replaceAll("_", " ")}
                        </small>
                      )}
                    </td>
                    <td>{bytes(u.size_bytes)}</td>
                    <td>
                      <Badge
                        tone={
                          status === "ready"
                            ? "mint"
                            : status === "failed"
                              ? "peach"
                              : "blue"
                        }
                      >
                        {status}
                      </Badge>
                    </td>
                    <td>
                      {u.asset_id
                        ? `${u.uploaded_chunks} / ${Math.ceil(u.size_bytes / (16 * 1024 ** 2))} chunks`
                        : "Awaiting browser transfer"}
                    </td>
                    <td>
                      {status === "failed" ? (
                        <button
                          className="button small"
                          onClick={() => void action(u)}
                        >
                          <RefreshCw size={14} />
                          Retry
                        </button>
                      ) : u.status === "receiving" ? (
                        <button
                          className="icon-button"
                          aria-label={`Cancel ${u.name}`}
                          onClick={() => void action(u)}
                        >
                          <X size={16} />
                        </button>
                      ) : null}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
