"use client";
import { useState } from "react";
import { CloudUpload } from "lucide-react";
import type { Settings } from "@assethub/api-client";
import { mutate } from "@/infrastructure/api/client";
import { ErrorNotice } from "./ui";

export function UploadSettings({ settings }: { settings: Settings }) {
  const [draft, setDraft] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const value = draft ?? String(settings.upload_concurrency);
  const limit = Number(value);
  const valid = Number.isInteger(limit) && limit >= 1 && limit <= 20;
  return (
    <section className="panel upload-settings">
      <div className="panel-heading">
        <div>
          <h2>Uploads</h2>
          <p>Control how many uploads can run at the same time.</p>
        </div>
        <span className="file-icon blue">
          <CloudUpload size={20} />
        </span>
      </div>
      <form
        noValidate
        onSubmit={async (event) => {
          event.preventDefault();
          if (busy || !settings.editable) return;
          setSaved(false);
          if (!valid) {
            setError("Enter a whole number from 1 to 20.");
            return;
          }
          setBusy(true);
          setError("");
          try {
            const updated = await mutate<Settings>(
              "/settings",
              { upload_concurrency: limit },
              "PATCH",
            );
            setDraft(String(updated.upload_concurrency));
            setSaved(true);
          } catch (e) {
            setError(
              e instanceof Error ? e.message : "Unable to save settings.",
            );
          } finally {
            setBusy(false);
          }
        }}
      >
        <label htmlFor="upload-concurrency">Maximum concurrent uploads</label>
        <p id="upload-concurrency-help">
          Choose 1–20. Applies to API and browser uploads on each server. Active
          transfers finish when you lower the limit.
        </p>
        <div className="upload-settings-controls">
          <input
            id="upload-concurrency"
            type="number"
            min={1}
            max={20}
            step={1}
            required
            value={value}
            disabled={busy || !settings.editable}
            aria-describedby="upload-concurrency-help upload-concurrency-feedback"
            aria-invalid={!!error && !valid}
            onChange={(event) => {
              setDraft(event.target.value);
              setSaved(false);
              setError("");
            }}
            onBlur={() => {
              if (!valid) setError("Enter a whole number from 1 to 20.");
            }}
          />
          {settings.editable && (
            <button className="button primary" disabled={busy} type="submit">
              {busy ? "Saving…" : "Save upload settings"}
            </button>
          )}
        </div>
        <div id="upload-concurrency-feedback" aria-live="polite">
          <ErrorNotice message={error} />
          {saved && <p role="status">Upload settings saved.</p>}
          {!settings.editable && (
            <p>Only administrators can change this limit.</p>
          )}
        </div>
      </form>
    </section>
  );
}
