"use client";
import { useEffect, useRef, useState } from "react";
import type { Endpoint, Parameter } from "./contract";
import { fields, resolve } from "./contract";

export function CodeBlock({ title, code }: { title: string; code: string }) {
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");
  return (
    <div className="docs-code">
      <div>
        <strong>{title}</strong>
        <button
          type="button"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(code);
              setCopied(true);
              setError("");
            } catch {
              setError("Select and copy the text below.");
            }
          }}
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {error && <p role="status">{error}</p>}
      <pre>
        <code>{code}</code>
      </pre>
    </div>
  );
}
type AuthMode = "session" | "key" | "anonymous";
export type ConsoleAuth = { mode: AuthMode; key: string };
const shellQuote = (text: string) => `'${text.replaceAll("'", "'\\''")}'`;
const editableParameters = (endpoint: Endpoint) =>
  endpoint.parameters.filter(
    (p) => !["Origin", "X-CSRF-Token"].includes(p.name),
  );
const paramKey = (parameter: Parameter) => `${parameter.in}:${parameter.name}`;
function initialValues(endpoint: Endpoint) {
  return Object.fromEntries(
    editableParameters(endpoint).map((p) => [
      paramKey(p),
      String(
        p.example ??
          (p.required
            ? (p.schema.default ?? (p.name === "index" ? 0 : ""))
            : ""),
      ),
    ]),
  );
}
async function preview(response: globalThis.Response) {
  if (!response.body) return { text: "(no response body)", truncated: false };
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  let truncated = false;
  const limit = 64 * 1024;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      const remaining = limit - size;
      chunks.push(value.slice(0, remaining));
      size += Math.min(value.byteLength, remaining);
      if (value.byteLength > remaining || size === limit) {
        truncated = true;
        await reader.cancel();
        break;
      }
    }
  } finally {
    reader.releaseLock();
  }
  const buffer = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    buffer.set(chunk, offset);
    offset += chunk.byteLength;
  }
  const contentType = response.headers.get("Content-Type") ?? "";
  if (!contentType.includes("json") && !contentType.startsWith("text/")) {
    return {
      text: `${size} bytes in preview. First 64 bytes (hex):\n${Array.from(buffer.slice(0, 64), (byte) => byte.toString(16).padStart(2, "0")).join(" ")}`,
      truncated,
    };
  }
  const text = new TextDecoder().decode(buffer);
  try {
    return { text: JSON.stringify(JSON.parse(text), null, 2), truncated };
  } catch {
    return { text, truncated };
  }
}
export function RequestConsole({
  endpoint,
  auth,
  origin,
}: {
  endpoint: Endpoint;
  auth: ConsoleAuth;
  origin: string;
}) {
  const requestBody = endpoint.requestBody
    ? resolve(endpoint.requestBody)
    : undefined;
  const mediaType = Object.keys(requestBody?.content ?? {})[0];
  const media = requestBody?.content[mediaType];
  const [values, setValues] = useState(() => initialValues(endpoint));
  const [body, setBody] = useState(
    mediaType === "application/json"
      ? JSON.stringify(media?.example ?? {}, null, 2)
      : "",
  );
  const [formValues, setFormValues] = useState<Record<string, string>>(() => {
    const example = media?.example as Record<string, string> | undefined;
    return Object.fromEntries(
      Object.entries(media ? fields(media.schema) : {})
        .filter(([, value]) => value.format !== "binary")
        .map(([name]) => [name, example?.[name] ?? ""]),
    );
  });
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    status: number;
    headers: string;
    text: string;
    truncated: boolean;
    elapsed: number;
  }>();
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  let path = endpoint.path;
  const query = new URLSearchParams();
  for (const p of editableParameters(endpoint)) {
    const value = values[paramKey(p)] ?? "";
    if (p.in === "path")
      path = path.replace(
        `{${p.name}}`,
        value ? encodeURIComponent(value) : `{${p.name}}`,
      );
    if (p.in === "query" && value) query.set(p.name, value);
  }
  const url = `${origin}${endpoint.servers?.[0]?.url === "/" ? "" : "/v1"}${path}${query.size ? `?${query}` : ""}`;
  const mutation = !["GET", "HEAD"].includes(endpoint.method);
  const headers: Record<string, string> = {};
  for (const p of editableParameters(endpoint))
    if (p.in === "header" && values[paramKey(p)])
      headers[p.name] = values[paramKey(p)];
  if (auth.mode === "key")
    headers.Authorization = "Bearer YOUR_PROJECT_API_KEY";
  if (auth.mode === "session" && mutation) {
    headers.Origin = origin || "https://assets.example.com";
    if (endpoint.security?.length !== 0)
      headers["X-CSRF-Token"] = "YOUR_SESSION_CSRF_TOKEN";
  }
  if (mediaType && mediaType !== "multipart/form-data")
    headers["Content-Type"] = mediaType;
  if (mediaType === "application/octet-stream")
    headers["X-Content-SHA256"] ||= "SHA256_OF_SELECTED_PART";
  const curl = [
    `curl -i -X ${endpoint.method} ${shellQuote(url || endpoint.path)}`,
    ...(auth.mode === "session" ? ["  -b cookies.txt"] : []),
    ...Object.entries(headers).map(
      ([name, value]) => `  -H ${shellQuote(`${name}: ${value}`)}`,
    ),
    ...(mediaType === "application/json"
      ? [`  --data ${shellQuote(body)}`]
      : []),
    ...(mediaType === "application/octet-stream"
      ? [`  --data-binary ${shellQuote(`@./${file?.name ?? "part.bin"}`)}`]
      : []),
    ...(mediaType === "multipart/form-data"
      ? [
          ...Object.entries(formValues).map(
            ([name, value]) => `  -F ${shellQuote(`${name}=${value}`)}`,
          ),
          `  -F ${shellQuote(`file=@./${file?.name ?? "hello.txt"}`)}`,
        ]
      : []),
  ].join(" \\\n");
  const send = async (event: React.FormEvent) => {
    event.preventDefault();
    const active = new AbortController();
    controller.current = active;
    const timeout = window.setTimeout(
      () => active.abort("Request timed out after 150 seconds"),
      150000,
    );
    setBusy(true);
    setError("");
    setResult(undefined);
    const started = performance.now();
    try {
      for (const p of editableParameters(endpoint)) {
        const value = values[paramKey(p)] ?? "";
        if (p.required && !value && !(p.name === "X-Content-SHA256" && file))
          throw new Error(`Enter ${p.in} parameter ${p.name}.`);
        if (
          p.in === "path" &&
          p.schema.format === "uuid" &&
          !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
            value,
          )
        )
          throw new Error(
            `${p.name} must be a UUID copied from an API response.`,
          );
        if (
          p.in === "path" &&
          p.schema.type === "integer" &&
          !/^\d+$/.test(value)
        )
          throw new Error(`${p.name} must be a nonnegative whole number.`);
      }
      const liveHeaders: Record<string, string> = { ...headers };
      delete liveHeaders.Origin; // Browsers set Origin themselves; it cannot be manually overridden.
      delete liveHeaders["X-CSRF-Token"];
      if (auth.mode === "key") {
        if (!auth.key.trim())
          throw new Error("Enter a project API key in Authentication.");
        liveHeaders.Authorization = `Bearer ${auth.key.trim()}`;
      }
      if (
        auth.mode === "session" &&
        mutation &&
        endpoint.security?.length !== 0
      ) {
        const session = await fetch("/v1/auth/me", {
          credentials: "same-origin",
          cache: "no-store",
          signal: active.signal,
          redirect: "error",
        });
        if (!session.ok)
          throw new Error(
            "Sign in to the dashboard first, or choose a project API key. Your session could not be loaded.",
          );
        liveHeaders["X-CSRF-Token"] = (await session.json()).csrf_token;
      }
      let payload: BodyInit | undefined;
      if (mediaType === "application/json") {
        JSON.parse(body);
        payload = body;
      }
      if (
        mediaType === "multipart/form-data" ||
        mediaType === "application/octet-stream"
      ) {
        if (!file) throw new Error("Select a file to send.");
        if (file.size < 1 || file.size > 8388608)
          throw new Error(
            "Select a nonempty file or part up to 8 MiB. Use resumable uploads for larger files.",
          );
        if (mediaType === "multipart/form-data") {
          const form = new FormData();
          for (const [name, value] of Object.entries(formValues))
            form.append(name, value);
          form.append("file", file);
          payload = form;
        } else {
          if (!values["header:X-Content-SHA256"]) {
            if (!crypto.subtle)
              throw new Error(
                "Automatic checksums require HTTPS or localhost. Enter the part’s SHA-256 header manually, or use HTTPS.",
              );
            const hash = await crypto.subtle.digest(
              "SHA-256",
              await file.arrayBuffer(),
            );
            liveHeaders["X-Content-SHA256"] = Array.from(
              new Uint8Array(hash),
              (byte) => byte.toString(16).padStart(2, "0"),
            ).join("");
          }
          payload = file;
        }
      }
      const response = await fetch(url, {
        method: endpoint.method,
        headers: liveHeaders,
        body: payload,
        credentials: auth.mode === "session" ? "same-origin" : "omit",
        cache: "no-store",
        redirect: "error",
        signal: active.signal,
      });
      const responseHeaders = Array.from(
        response.headers,
        ([name, value]) => `${name}: ${value}`,
      ).join("\n");
      const responseBody =
        endpoint.method === "HEAD"
          ? { text: "(no response body)", truncated: false }
          : await preview(response);
      if (active.signal.aborted) throw new Error("Request cancelled");
      setResult({
        status: response.status,
        headers: responseHeaders,
        ...responseBody,
        elapsed: Math.round(performance.now() - started),
      });
    } catch (failure) {
      if (controller.current === active)
        setError(
          active.signal.aborted
            ? "Request cancelled or timed out. Check the resource’s state before retrying a write."
            : failure instanceof Error
              ? failure.message
              : "The request could not be sent.",
        );
    } finally {
      window.clearTimeout(timeout);
      if (controller.current === active) {
        setBusy(false);
        controller.current = null;
      }
    }
  };
  return (
    <section className="docs-console" aria-label="Request console">
      <h3>Try this request</h3>
      <p>
        {mutation
          ? "Sending this request changes real data on this installation. Check the ids and body before sending."
          : "Send a real request to this installation. Examples below are separate from the live result."}
      </p>
      <form onSubmit={(event) => void send(event)}>
        <fieldset disabled={busy}>
          {editableParameters(endpoint).map((parameter) => (
            <label key={paramKey(parameter)}>
              {parameter.in === "path"
                ? "Path"
                : parameter.in === "query"
                  ? "Query"
                  : "Header"}
              : {parameter.name}
              {parameter.required ? " *" : ""}
              <input
                aria-label={`${parameter.in}: ${parameter.name}`}
                value={values[paramKey(parameter)] ?? ""}
                onChange={(event) =>
                  setValues({
                    ...values,
                    [paramKey(parameter)]: event.target.value,
                  })
                }
                placeholder={
                  parameter.name === "X-Content-SHA256"
                    ? "Automatically calculated from selected part"
                    : parameter.required
                      ? "Required"
                      : "Optional; blank omits it"
                }
              />
            </label>
          ))}
          {mediaType === "application/json" && (
            <label>
              JSON request body
              <textarea
                aria-label="JSON request body"
                rows={9}
                spellCheck={false}
                value={body}
                onChange={(event) => setBody(event.target.value)}
              />
            </label>
          )}
          {mediaType === "multipart/form-data" &&
            Object.keys(formValues).map((name) => (
              <label key={name}>
                Form: {name}
                <input
                  aria-label={`form: ${name}`}
                  value={formValues[name]}
                  onChange={(event) =>
                    setFormValues({ ...formValues, [name]: event.target.value })
                  }
                />
              </label>
            ))}
          {["multipart/form-data", "application/octet-stream"].includes(
            mediaType,
          ) && (
            <label>
              {mediaType === "application/octet-stream"
                ? "Part file (raw bytes)"
                : "File"}
              <input
                aria-label="Request file"
                type="file"
                onChange={(event) => setFile(event.target.files?.[0])}
              />
            </label>
          )}
          <p className="docs-url">
            <code>
              {endpoint.method} {url}
            </code>
          </p>
          <button className="button primary" type="submit" disabled={!origin}>
            {busy ? "Sending…" : "Send request"}
          </button>
        </fieldset>
        {busy && (
          <button
            className="button"
            type="button"
            onClick={() => controller.current?.abort()}
          >
            Cancel request
          </button>
        )}
      </form>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {result && (
        <div className="docs-live" role="status">
          <h4>
            Live response: HTTP {result.status} · {result.elapsed} ms
          </h4>
          <CodeBlock title="Response headers" code={result.headers} />
          <CodeBlock
            title="Response body"
            code={result.text || "(empty body)"}
          />
          {result.truncated && (
            <p>
              Preview stopped at 64 KiB. Use the cURL example to save the
              complete response.
            </p>
          )}
        </div>
      )}
      <CodeBlock
        title="cURL request (credentials are placeholders)"
        code={curl}
      />
      {auth.mode === "session" && (
        <p>
          cURL needs cookies.txt from the login example and the returned CSRF
          token for writes. Browser requests use your current session
          automatically.
        </p>
      )}
    </section>
  );
}
