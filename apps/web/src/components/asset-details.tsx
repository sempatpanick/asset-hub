"use client";
import { SearchSelect } from "./select";
import { useState } from "react";
import Image from "next/image";
import {
  Copy,
  Download,
  Link as LinkIcon,
  Trash2,
  RotateCcw,
} from "lucide-react";
import type { Asset } from "@assethub/api-client";
import { mutate } from "@/infrastructure/api/client";
import { bytes, useAction, useResource } from "@/features/hooks";
import { Badge, ErrorNotice, Modal } from "./ui";
export function safeImage(mime: string) {
  return [
    "image/png",
    "image/jpeg",
    "image/gif",
    "image/webp",
    "image/avif",
  ].includes(mime);
}
export function safeVideo(mime: string) {
  return ["video/mp4", "video/webm", "video/ogg"].includes(mime);
}
export function safeImageName(name: string) {
  return /\.(png|jpe?g|gif|webp|avif)$/i.test(name);
}
export function assetContent(id: string) {
  return `/v1/assets/${encodeURIComponent(id)}/content`;
}
export function AssetDetails({
  asset: initialAsset,
  canWrite,
  canManage,
  isAdmin,
  backupEnabled,
  onClose,
  onSaved,
}: {
  asset: Asset;
  canWrite: boolean;
  canManage: boolean;
  isAdmin: boolean;
  backupEnabled: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const metadata = useResource<Asset>(`/assets/${initialAsset.id}`);
  const asset = metadata.data ?? initialAsset;
  const [name, setName] = useState(asset.name);
  const [visibility, setVisibility] = useState(asset.visibility);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [signed, setSigned] = useState("");
  const [copied, setCopied] = useState(false);
  const action = useAction();
  const content = assetContent(asset.id);
  const copy = async (value: string) => {
    if (!navigator.clipboard)
      throw new Error("Copy requires HTTPS or localhost.");
    await navigator.clipboard.writeText(value);
    setCopied(true);
  };
  return (
    <Modal
      title="File details"
      detail="Metadata, access and sharing for this asset."
      onClose={onClose}
    >
      {asset.status === "ready" && (
        <div className="details-preview">
          {safeImage(asset.mime) ? (
            <Image
              src={content}
              alt={asset.name}
              width={640}
              height={280}
              unoptimized
              style={{ width: "100%", height: "auto" }}
            />
          ) : safeVideo(asset.mime) ? (
            <video src={content} controls preload="metadata" />
          ) : (
            <div className="document-preview">
              {asset.mime}
              <span>Download to open this file.</span>
            </div>
          )}
        </div>
      )}
      <div className="details-meta">
        <Badge tone={asset.status === "ready" ? "mint" : "peach"}>
          {asset.status}
        </Badge>
        <span>{bytes(asset.size_bytes)}</span>
        <span>{asset.mime}</span>
      </div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            await mutate(
              `/assets/${asset.id}`,
              { name, ...(canManage ? { visibility } : {}) },
              "PATCH",
            );
            onSaved();
            onClose();
          });
        }}
      >
        <label className="upload-project">
          File name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={!canWrite || asset.status === "deleted"}
            maxLength={180}
          />
        </label>
        <label className="upload-project">
          Visibility
          <SearchSelect
            aria-label="Visibility"
            value={visibility}
            onValueChange={(value) =>
              setVisibility(value === "public" ? "public" : "private")
            }
            disabled={!canManage || asset.status === "deleted"}
          >
            <option value="private">Private — project access required</option>
            <option value="public">Public — anyone with the URL</option>
          </SearchSelect>
        </label>
        <div className="details-checksum">
          <span>SHA-256</span>
          <code>{asset.sha256 || "Awaiting validation"}</code>
          <span>Asset ID</span>
          <code>{asset.id}</code>
        </div>
        <ErrorNotice message={action.error || metadata.error} />
        {asset.status !== "deleted" && (
          <div className="modal-actions">
            <button
              className="button primary"
              disabled={
                !canWrite ||
                action.busy ||
                (name === asset.name && visibility === asset.visibility)
              }
            >
              Save changes
            </button>
            {asset.status === "ready" && (
              <a className="button" href={`${content}?download=1`}>
                <Download size={16} />
                Download
              </a>
            )}
          </div>
        )}
      </form>
      {asset.status === "ready" && (
        <div className="details-sharing">
          <button
            className="button"
            disabled={action.busy}
            onClick={() =>
              void action.run(() =>
                copy(new URL(content, location.origin).href),
              )
            }
          >
            <Copy size={15} />
            {copied ? "Copied" : "Copy asset URL"}
          </button>
          <button
            className="button"
            disabled={action.busy}
            onClick={() =>
              void action.run(async () => {
                const r = await mutate<{ url: string }>(
                  `/assets/${asset.id}/signed-url`,
                  { expires_in: 900 },
                );
                setSigned(r.url);
                setCopied(false);
              })
            }
          >
            <LinkIcon size={15} />
            Create 15-minute link
          </button>
          {signed && (
            <label className="upload-project">
              Signed URL
              <input
                readOnly
                aria-label="Signed URL"
                value={signed}
                onFocus={(e) => e.target.select()}
              />
              <button
                className="button small"
                onClick={() => void action.run(() => copy(signed))}
              >
                Copy signed URL
              </button>
            </label>
          )}
          <p>
            Private asset URLs require authorization. A signed URL grants
            temporary content access.
          </p>
        </div>
      )}
      {asset.status === "deleted" && !backupEnabled && (
        <p>Backups are disabled. This deleted asset cannot be restored.</p>
      )}
      {asset.status === "deleted" && isAdmin && backupEnabled ? (
        <button
          className="button"
          disabled={action.busy}
          onClick={() =>
            void action.run(async () => {
              await mutate(`/assets/${asset.id}/restore`);
              onSaved();
              onClose();
            })
          }
        >
          <RotateCcw size={15} />
          Restore retained backup
        </button>
      ) : (
        canWrite &&
        asset.status !== "deleted" && (
          <div className="details-delete">
            {confirmDelete ? (
              <>
                <p>
                  {backupEnabled
                    ? "Delete this asset? Access stops immediately. Administrators can restore a verified retained backup for 30 days if one exists."
                    : "Delete this asset? Access stops immediately. Backups are disabled, so this asset cannot be restored."}
                </p>
                <button
                  className="button danger"
                  disabled={action.busy}
                  onClick={() =>
                    void action.run(async () => {
                      await mutate(`/assets/${asset.id}`, undefined, "DELETE");
                      onSaved();
                      onClose();
                    })
                  }
                >
                  Confirm deletion
                </button>
                <button
                  className="button"
                  onClick={() => setConfirmDelete(false)}
                >
                  Keep asset
                </button>
              </>
            ) : (
              <button
                className="button danger"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 size={15} />
                Delete asset
              </button>
            )}
          </div>
        )
      )}
    </Modal>
  );
}
