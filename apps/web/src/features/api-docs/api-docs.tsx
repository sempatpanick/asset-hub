"use client";
import { useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { BookOpen, ChevronLeft, Download, Layers3, Search } from "lucide-react";
import { SearchSelect } from "@/components/select";
import {
  contract,
  endpoints,
  fields,
  requiredFields,
  resolve,
  schemaLabel,
  type Schema,
} from "./contract";
import { ApiGuide } from "./guide";
import { CodeBlock, RequestConsole, type ConsoleAuth } from "./request-console";

function subscribeLocation(listener: () => void) {
  window.addEventListener("hashchange", listener);
  return () => window.removeEventListener("hashchange", listener);
}
function currentSelection() {
  const id = window.location.hash.slice(1);
  return endpoints.some((operation) => operation.operationId === id)
    ? id
    : "guide";
}

function SchemaFields({
  schema,
  depth = 0,
}: {
  schema: Schema;
  depth?: number;
}) {
  const resolved = resolve(schema);
  const properties = fields(resolved);
  const required = requiredFields(resolved);
  if (!Object.keys(properties).length)
    return (
      <div>
        <p>
          <code>{schemaLabel(resolved)}</code>
        </p>
        {resolved.items && depth < 4 && (
          <SchemaFields schema={resolved.items} depth={depth + 1} />
        )}
      </div>
    );
  return (
    <div className="docs-fields">
      {Object.entries(properties).map(([name, field]) => {
        const value = resolve(field);
        const nested = value.items ?? value;
        return (
          <div key={name}>
            <strong>
              <code>{name}</code>{" "}
              {required.includes(name) && <small>required</small>}
            </strong>
            <p>{schemaLabel(value)}</p>
            {value.description && <p>{value.description}</p>}
            {depth < 4 && Object.keys(fields(nested)).length > 0 && (
              <details>
                <summary>
                  {value.items ? "Item fields" : "Nested fields"}
                </summary>
                <SchemaFields schema={nested} depth={depth + 1} />
              </details>
            )}
          </div>
        );
      })}
    </div>
  );
}
export function ApiDocs() {
  const selected = useSyncExternalStore(
    subscribeLocation,
    currentSelection,
    () => "guide",
  );
  const [search, setSearch] = useState("");
  const origin = useSyncExternalStore(
    subscribeLocation,
    () => window.location.origin,
    () => "",
  );
  const [auth, setAuth] = useState<ConsoleAuth>({ mode: "session", key: "" });
  const endpoint = endpoints.find((op) => op.operationId === selected);
  const matches = endpoints.filter((op) =>
    `${op.method} ${op.path} ${op.summary} ${op.tags.join(" ")}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  const groups = [...new Set(matches.map((op) => op.tags[0]))];
  const body = endpoint?.requestBody
    ? resolve(endpoint.requestBody)
    : undefined;
  return (
    <div className="api-docs">
      <header className="docs-header">
        <Link href="/" className="brand">
          <span className="brand-mark">
            <Layers3 size={22} />
          </span>
          AssetHub
        </Link>
        <span>
          <BookOpen size={18} /> API Docs{" "}
          <small>v{contract.info.version}</small>
        </span>
        <Link href="/">
          <ChevronLeft size={15} /> Workspace
        </Link>
      </header>
      <div className="docs-layout">
        <aside className="docs-sidebar">
          <a href="#guide" className={selected === "guide" ? "selected" : ""}>
            Start here: integration guide
          </a>
          <label className="docs-search">
            <Search size={16} />
            <input
              aria-label="Search API endpoints"
              placeholder="Find an endpoint…"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <p className="docs-count">
            {matches.length} of {endpoints.length} endpoints
          </p>
          <nav aria-label="API endpoints">
            {groups.map((group) => (
              <section key={group}>
                <h2>{group}</h2>
                {matches
                  .filter((op) => op.tags[0] === group)
                  .map((op) => (
                    <a
                      key={op.operationId}
                      href={`#${op.operationId}`}
                      aria-current={
                        selected === op.operationId ? "page" : undefined
                      }
                      className={selected === op.operationId ? "selected" : ""}
                    >
                      <span
                        className={`docs-method method-${op.method.toLowerCase()}`}
                      >
                        {op.method}
                      </span>
                      <span>
                        {op.summary}
                        <small>{op.path}</small>
                      </span>
                    </a>
                  ))}
              </section>
            ))}
          </nav>
          {!matches.length && (
            <p>No endpoints match. Try a path, method or name.</p>
          )}
        </aside>
        <main className="docs-main">
          <div className="docs-intro">
            <p className="docs-eyebrow">DEVELOPER REFERENCE</p>
            <h1>AssetHub API Docs</h1>
            <p>
              Build with your files. Understand each request, see example
              responses and try the API here.
            </p>
            <div className="docs-downloads">
              <a className="button" href="/api-docs/openapi.json" download>
                <Download size={15} /> OpenAPI JSON
              </a>
              <a className="button" href="/api-docs/openapi.yaml" download>
                <Download size={15} /> OpenAPI YAML
              </a>
              <code>Base URL: {origin || "your website origin"}/v1</code>
            </div>
          </div>
          <section className="docs-auth" aria-label="Authentication">
            <div>
              <h2>Authentication for the console</h2>
              <p>
                Uses real data. Choose credentials before sending. No key is
                saved to browser storage.
              </p>
            </div>
            <SearchSelect
              aria-label="Console authentication"
              value={auth.mode}
              onValueChange={(mode) =>
                setAuth({ mode: mode as ConsoleAuth["mode"], key: "" })
              }
            >
              <option value="session">Dashboard session</option>
              <option value="key">Project API key</option>
              <option value="anonymous">Anonymous (no credentials)</option>
            </SearchSelect>
            {auth.mode === "key" && (
              <label>
                Project API key
                <input
                  aria-label="Project API key"
                  type="password"
                  autoComplete="off"
                  placeholder="ah_…"
                  value={auth.key}
                  onChange={(event) =>
                    setAuth({ ...auth, key: event.target.value })
                  }
                />
              </label>
            )}
            {auth.mode === "session" && (
              <p>
                Sign in in the Workspace tab first. Public login and invitation
                endpoints can be sent without an existing session.
              </p>
            )}
          </section>
          {!endpoint ? (
            <ApiGuide />
          ) : (
            <article className="docs-operation" aria-label="Endpoint reference">
              <div className="docs-operation-title">
                <span
                  className={`docs-method method-${endpoint.method.toLowerCase()}`}
                >
                  {endpoint.method}
                </span>
                <code>
                  {endpoint.servers?.[0]?.url === "/" ? "" : "/v1"}
                  {endpoint.path}
                </code>
              </div>
              <h2>{endpoint.summary}</h2>
              <p>{endpoint.description}</p>
              <p>
                <strong>Access:</strong> {endpoint["x-access"]}
              </p>
              <h3>Parameters</h3>
              {!endpoint.parameters.length ? (
                <p>No path, query or custom header parameters.</p>
              ) : (
                <div className="docs-fields">
                  {endpoint.parameters.map((parameter) => (
                    <div key={`${parameter.in}:${parameter.name}`}>
                      <strong>
                        <code>{parameter.name}</code>{" "}
                        <small>
                          {parameter.in}
                          {parameter.required ? " · required" : " · optional"}
                        </small>
                      </strong>
                      <p>
                        {parameter.description ??
                          parameter.schema.description ??
                          "See the field type and constraints below."}
                      </p>
                      <p>{schemaLabel(parameter.schema)}</p>
                    </div>
                  ))}
                </div>
              )}
              {body && (
                <section>
                  <h3>
                    Request body {body.required ? "(required)" : "(optional)"}
                  </h3>
                  {Object.entries(body.content).map(([type, media]) => (
                    <div key={type}>
                      <p>
                        <code>{type}</code>
                      </p>
                      <SchemaFields schema={media.schema} />
                      {media.example !== undefined && (
                        <CodeBlock
                          title="Example request body"
                          code={JSON.stringify(media.example, null, 2)}
                        />
                      )}
                    </div>
                  ))}
                </section>
              )}
              <RequestConsole
                key={`${endpoint.operationId}:${auth.mode}`}
                endpoint={endpoint}
                auth={auth}
                origin={origin}
              />
              <section aria-label="Response examples">
                <h3>Success and error response examples</h3>
                <p>
                  These are illustrative samples, separate from live responses
                  above. Replace example ids and credentials. Each status below
                  includes its response shape.
                </p>
                {Object.entries(endpoint.responses).map(
                  ([status, response]) => (
                    <details key={status} open={Number(status) < 300}>
                      <summary>
                        <strong
                          className={
                            Number(status) < 300
                              ? "docs-success"
                              : "docs-failure"
                          }
                        >
                          HTTP {status}
                        </strong>{" "}
                        {response.description}
                      </summary>
                      {response.headers && (
                        <CodeBlock
                          title="Example response headers"
                          code={Object.entries(response.headers)
                            .map(
                              ([name, value]) =>
                                `${name}: ${value.example ?? value.description ?? schemaLabel(value.schema)}`,
                            )
                            .join("\n")}
                        />
                      )}
                      {response["x-response-example"] ? (
                        <CodeBlock
                          title="Example HTTP response"
                          code={response["x-response-example"]}
                        />
                      ) : (
                        Object.entries(response.content ?? {}).map(
                          ([type, media]) => (
                            <div key={type}>
                              <CodeBlock
                                title={`Example response (${type})`}
                                code={JSON.stringify(media.example, null, 2)}
                              />
                              <details>
                                <summary>Response fields</summary>
                                <SchemaFields schema={media.schema} />
                              </details>
                            </div>
                          ),
                        )
                      )}
                      {endpoint.method === "HEAD" && Number(status) >= 300 && (
                        <p>
                          HEAD sends status and headers only. The example
                          documents the corresponding GET error shape.
                        </p>
                      )}
                    </details>
                  ),
                )}
              </section>
            </article>
          )}
          <footer className="docs-footer">
            AssetHub API v{contract.info.version} · OpenAPI 3.1 · Examples are
            illustrative; the console sends real requests.
          </footer>
        </main>
      </div>
    </div>
  );
}
