"use client";
import { useState } from "react";
import { CloudUpload } from "lucide-react";
import type { Settings } from "@assethub/api-client";
import { mutate } from "@/infrastructure/api/client";
import { ErrorNotice } from "./ui";

const fields = [
  {
    key: "upload_concurrency",
    label: "Maximum concurrent uploads",
    min: 1,
    max: 20,
    help: "Choose 1–20. Applies to API and browser uploads on each server. Active transfers finish when you lower the limit.",
  },
  {
    key: "upload_request_timeout_seconds",
    label: "Upload request timeout (seconds)",
    min: 30,
    max: 1800,
    help: "Time allowed to receive one file or 8 MiB part. Choose 30–1800 seconds. Default: 120.",
  },
  {
    key: "upload_response_timeout_seconds",
    label: "Upload response timeout (seconds)",
    min: 60,
    max: 1830,
    help: "Total time for one upload request, including validation. Choose 60–1830 seconds, at least 30 seconds longer than the request timeout. Default: 150.",
  },
  {
    key: "upload_session_lifetime_hours",
    label: "Resumable session lifetime (hours)",
    min: 1,
    max: 168,
    help: "Time to finish a newly created resumable upload session. Choose 1–168 hours. Default: 24.",
  },
] as const;
type Field = (typeof fields)[number]["key"];
type Values = Partial<Record<Field, string>>;

export function UploadSettings({ settings }: { settings: Settings }) {
  const [draft, setDraft] = useState<Values>({});
  const [errors, setErrors] = useState<Values>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const value = (key: Field) => draft[key] ?? String(settings[key]);
  const validate = () => {
    const result: Values = {};
    for (const field of fields) {
      const n = Number(value(field.key));
      if (!Number.isInteger(n) || n < field.min || n > field.max)
        result[field.key] =
          `Enter a whole number from ${field.min} to ${field.max}.`;
    }
    if (
      !result.upload_request_timeout_seconds &&
      !result.upload_response_timeout_seconds &&
      Number(value("upload_response_timeout_seconds")) <
        Number(value("upload_request_timeout_seconds")) + 30
    )
      result.upload_response_timeout_seconds =
        "Response timeout must be at least 30 seconds longer than request timeout.";
    return result;
  };
  return (
    <section className="panel upload-settings">
      <div className="panel-heading">
        <div>
          <h2>Uploads</h2>
          <p>
            Manage parallel uploads, request timeouts and resumable sessions.
          </p>
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
          const invalid = validate();
          setErrors(invalid);
          const first = fields.find((field) => invalid[field.key]);
          if (first) {
            document.getElementById(first.key)?.focus();
            return;
          }
          setBusy(true);
          setError("");
          try {
            const updated = await mutate<Settings>(
              "/settings",
              Object.fromEntries(
                fields.map((field) => [field.key, Number(value(field.key))]),
              ),
              "PATCH",
            );
            setDraft(
              Object.fromEntries(
                fields.map((field) => [field.key, String(updated[field.key])]),
              ),
            );
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
        <div className="upload-settings-fields">
          {fields.map((field) => (
            <div key={field.key}>
              <label htmlFor={field.key}>{field.label}</label>
              <input
                id={field.key}
                type="number"
                min={field.min}
                max={field.max}
                step={1}
                required
                value={value(field.key)}
                disabled={busy || !settings.editable}
                aria-describedby={`${field.key}-help${errors[field.key] ? ` ${field.key}-error` : ""}`}
                aria-invalid={!!errors[field.key]}
                onChange={(event) => {
                  setDraft((previous) => ({
                    ...previous,
                    [field.key]: event.target.value,
                  }));
                  setSaved(false);
                  setErrors({});
                  setError("");
                }}
                onBlur={() => {
                  const message = validate()[field.key];
                  setErrors((previous) => ({
                    ...previous,
                    [field.key]: message,
                  }));
                }}
              />
              <p id={`${field.key}-help`}>{field.help}</p>
              {errors[field.key] && (
                <p
                  id={`${field.key}-error`}
                  className="upload-settings-error"
                  role="alert"
                >
                  {errors[field.key]}
                </p>
              )}
            </div>
          ))}
        </div>
        <div className="upload-settings-controls">
          {settings.editable && (
            <button className="button primary" disabled={busy} type="submit">
              {busy ? "Saving…" : "Save upload settings"}
            </button>
          )}
        </div>
        <div aria-live="polite">
          <ErrorNotice message={error} />
          {saved && <p role="status">Upload settings saved.</p>}
          {!settings.editable && (
            <p>Only administrators can change upload settings.</p>
          )}
        </div>
        <p>
          Saved timeouts apply to new requests. Session lifetime is set when a
          session is created.
        </p>
      </form>
    </section>
  );
}
