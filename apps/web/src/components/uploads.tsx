"use client";
import { SearchSelect } from "./select";
import { useEffect, useRef, useState } from "react";
import { CloudUpload, Pause, Play, RefreshCw, X } from "lucide-react";
import type { Asset, Items, Project, Upload } from "@assethub/api-client";
import { mutate, request, uploadPart } from "@/infrastructure/api/client";
import { bytes, useResource, useDebounced } from "@/features/hooks";
import { Badge, Empty, ErrorNotice, Loading, Modal } from "./ui";
import { Pagination, TableSearch } from "./table-controls";
import Image from "next/image";
import { FileIcon } from "./dashboard";
import { safeImage, safeImageName, assetContent } from "./asset-details";

function TransferPreview({ file }: { file: File }) {
  const [url] = useState(() =>
    safeImage(file.type) ? URL.createObjectURL(file) : undefined,
  );
  useEffect(() => {
    return () => {
      if (url) URL.revokeObjectURL(url);
    };
  }, [url]);
  if (url) {
    return (
      <span className="file-thumbnail">
        {/* ponytail: unoptimized preview for local blob URL */}
        <Image src={url} alt="" width={34} height={36} unoptimized />
      </span>
    );
  }
  return <FileIcon mime={file.type || "application/octet-stream"} />;
}

type Transfer = { file: File; status: string; loaded: number; error?: string };
export function UploadDialog({
  projects,
  project,
  userID,
  concurrency,
  onClose,
  onSaved,
}: {
  projects: Project[];
  project: string;
  userID: string;
  concurrency: number;
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
    const pending = transfers.filter((v) => v.status !== "queued");
    const uploadFile = async (transfer: Transfer) => {
      const file = transfer.file;
      const key = `assethub:upload:${userID}:${selected}:${encodeURIComponent(file.name)}:${file.size}:${file.lastModified}`;
      try {
        if (controller.signal.aborted) return;
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
      }
    };
    let next = 0;
    const worker = async () => {
      while (!controller.signal.aborted && next < pending.length) {
        const transfer = pending[next++];
        await uploadFile(transfer);
      }
    };
    await Promise.all(
      Array.from(
        {
          length: Math.min(
            pending.length,
            Math.max(1, Math.min(20, concurrency)),
          ),
        },
        worker,
      ),
    );
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
        <SearchSelect
          aria-label="Upload project"
          disabled={busy || transfers.length > 0}
          value={selected}
          onValueChange={setSelected}
        >
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </SearchSelect>
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
              <div className="file-name">
                <TransferPreview file={v.file} />
                <strong>{v.file.name}</strong>
              </div>
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
        Up to {concurrency} files upload at the same time. Transfer progress
        tracks your browser upload. Storage is complete only when the asset is
        marked ready. To resume after closing this window, select the same files
        and project.
      </p>
    </Modal>
  );
}

export function UploadHistory({
  enabled,
  onUpload,
  projects,
  onSelectAsset,
}: {
  enabled: boolean;
  onUpload: () => void;
  projects: Project[];
  onSelectAsset?: (asset: Asset) => void;
}) {
  const [search, setSearch] = useState("");
  const query = useDebounced(search);
  const [status, setStatus] = useState("");
  const [project, setProject] = useState("");
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const history = useResource<Items<Upload>>(
    `/uploads?q=${encodeURIComponent(query)}&status=${status}&project_id=${project}&offset=${offset}&limit=${pageSize}`,
    3000,
  );
  const items = history.data?.items ?? [];
  const [error, setError] = useState("");
  const action = async (u: Upload) => {
    try {
      if (u.asset_status === "failed")
        await mutate(`/assets/${u.asset_id}/retry`);
      else await mutate(`/uploads/${u.id}`, undefined, "DELETE");
      setError("");
      history.refresh();
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
          onClick={history.refresh}
        >
          <RefreshCw size={17} />
        </button>
      </div>
      <div className="file-toolbar">
        <TableSearch
          label="Search uploads"
          value={search}
          onChange={(v) => {
            setSearch(v);
            setOffset(0);
          }}
        />
        <div className="file-controls">
          <SearchSelect
            aria-label="Upload status filter"
            value={status}
            onValueChange={(v) => {
              setStatus(v);
              setOffset(0);
            }}
          >
            <option value="">All statuses</option>
            {[
              "receiving",
              "completing",
              "queued",
              "uploading",
              "verifying",
              "ready",
              "failed",
              "cancelled",
              "expired",
              "deleting",
              "deleted",
            ].map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </SearchSelect>
          <SearchSelect
            aria-label="Upload project filter"
            value={project}
            onValueChange={(v) => {
              setProject(v);
              setOffset(0);
            }}
          >
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </SearchSelect>
        </div>
      </div>
      <ErrorNotice message={error || history.error} />
      {history.loading ? (
        <Loading />
      ) : !items.length ? (
        <Empty
          title={
            search || status || project || offset
              ? "No matching uploads"
              : "Your files begin here"
          }
          detail={
            enabled
              ? "Upload to a project and follow each file as it becomes ready."
              : "Configure storage credentials and encryption to enable uploads."
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
                      <div className="file-name">
                        {u.asset_id &&
                        (status === "ready" || u.asset_status === "ready") &&
                        safeImageName(u.name) ? (
                          <span
                            className="file-thumbnail"
                            onClick={() =>
                              onSelectAsset?.({
                                id: u.asset_id,
                                project_id: u.project_id,
                                name: u.name,
                                mime: "image/png",
                                size_bytes: u.size_bytes,
                                sha256: "",
                                status: u.asset_status || status,
                                visibility: "private",
                                created_at: u.created_at,
                              })
                            }
                            style={{
                              cursor:
                                u.asset_id && onSelectAsset
                                  ? "pointer"
                                  : "default",
                            }}
                          >
                            {/* ponytail: small unoptimized thumbnail in upload table; responsive srcset when scaling */}
                            <Image
                              src={assetContent(u.asset_id)}
                              alt=""
                              width={34}
                              height={36}
                              unoptimized
                            />
                          </span>
                        ) : (
                          <span
                            onClick={() =>
                              u.asset_id &&
                              onSelectAsset?.({
                                id: u.asset_id,
                                project_id: u.project_id,
                                name: u.name,
                                mime: safeImageName(u.name)
                                  ? "image/png"
                                  : "application/octet-stream",
                                size_bytes: u.size_bytes,
                                sha256: "",
                                status: u.asset_status || status,
                                visibility: "private",
                                created_at: u.created_at,
                              })
                            }
                            style={{
                              cursor:
                                u.asset_id && onSelectAsset
                                  ? "pointer"
                                  : "default",
                            }}
                          >
                            <FileIcon
                              mime={
                                safeImageName(u.name)
                                  ? "image/png"
                                  : "application/octet-stream"
                              }
                            />
                          </span>
                        )}
                        <div>
                          {u.asset_id && onSelectAsset ? (
                            <button
                              className="asset-name-button"
                              onClick={() =>
                                onSelectAsset({
                                  id: u.asset_id,
                                  project_id: u.project_id,
                                  name: u.name,
                                  mime: safeImageName(u.name)
                                    ? "image/png"
                                    : "application/octet-stream",
                                  size_bytes: u.size_bytes,
                                  sha256: "",
                                  status: u.asset_status || status,
                                  visibility: "private",
                                  created_at: u.created_at,
                                })
                              }
                            >
                              {u.name}
                            </button>
                          ) : (
                            <strong>{u.name}</strong>
                          )}
                          {u.error_code && (
                            <small className="job-error">
                              {u.error_code.replaceAll("_", " ")}
                            </small>
                          )}
                        </div>
                      </div>
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
      <Pagination
        offset={offset}
        count={items.length}
        pageSize={pageSize}
        onChange={setOffset}
        onPageSizeChange={(n) => {
          setPageSize(n);
          setOffset(0);
        }}
      />
    </section>
  );
}
