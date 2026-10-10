import { CodeBlock } from "./request-console";

export function ApiGuide() {
  return (
    <div className="docs-guide">
      <h2>Connect your other project</h2>
      <p>
        AssetHub stores files inside projects. A project groups files, sets a
        storage quota and controls who can use them. An asset is a stored file;
        an upload is the temporary transfer used to create it.
      </p>
      <ol>
        <li>
          Sign in to AssetHub, create a project and copy its id from the
          Projects page or GET /v1/projects.
        </li>
        <li>
          Open API Keys and create a key for that project. Choose only the
          scopes your application needs and save the secret when shown.
        </li>
        <li>
          Store that secret in your application’s server environment as
          ASSETHUB_API_KEY. Send it as Authorization: Bearer YOUR_KEY.
        </li>
        <li>
          Use your AssetHub website origin followed by /v1 as the API base URL.
          Choose an endpoint in this page to see fields, examples and send a
          real request.
        </li>
      </ol>
      <CodeBlock
        title="Server-side JavaScript: list your project’s files"
        code={`const baseUrl = "https://assets.example.com/v1";
const response = await fetch(baseUrl + "/assets?limit=25&offset=0", {
  headers: { Authorization: "Bearer " + process.env.ASSETHUB_API_KEY },
});
const result = await response.json();
if (!response.ok) throw new Error(result.error?.message ?? "Request failed");
console.log(result.items);`}
      />
      <h2>Authentication and permissions</h2>
      <p>
        For another application, call AssetHub from your server using a project
        key. Keys work for that single project and cannot manage users,
        memberships, keys, settings or the workspace overview. GET /projects can
        identify the key’s project. Authorization takes precedence over a
        session cookie.
      </p>
      <div className="docs-table-wrap">
        <table>
          <thead>
            <tr>
              <th>Key scope</th>
              <th>What it allows</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>assets:read</td>
              <td>
                List files and uploads, inspect metadata and download private
                content.
              </td>
            </tr>
            <tr>
              <td>assets:write</td>
              <td>
                Upload parts/files, complete/cancel transfers, rename files and
                retry processing.
              </td>
            </tr>
            <tr>
              <td>assets:delete</td>
              <td>Delete files.</td>
            </tr>
            <tr>
              <td>assets:sign</td>
              <td>Create temporary content links for ready files.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p>
        Scopes are independent: write does not imply read. Use both if your
        application needs to inspect uploads or poll file readiness. API keys
        cannot change public/private visibility or restore deleted files.
        Expired and revoked keys return 401.
      </p>
      <p>
        Session users have workspace roles admin or member, and project roles
        manager, editor or viewer. A viewer reads; an editor reads, uploads,
        renames, deletes and signs; a manager also changes project settings,
        visibility, membership and keys. A workspace admin can access projects
        as a manager, invite accounts, view storage health and restore backups.
        Members can create projects and become their manager.
      </p>
      <p>
        Cookie sessions last 12 hours. POST /auth/login sets an HttpOnly cookie:
        __Host-ah_session on HTTPS, ah_session in development. Save csrf_token
        from login or GET /auth/me. Session POST, PUT, PATCH and DELETE requests
        require X-CSRF-Token and an Origin exactly matching the installation’s
        PUBLIC_ORIGIN. Login and invitation acceptance require that Origin too.
        API-key mutations do not require CSRF or Origin.
      </p>
      <CodeBlock
        title="cURL: establish a session (replace the origin and credentials)"
        code={`curl -i -c cookies.txt 'https://assets.example.com/v1/auth/login' \\
  -H 'Origin: https://assets.example.com' \\
  -H 'Content-Type: application/json' \\
  --data '{"email":"you@example.com","password":"your-account-password"}'

# Copy csrf_token from the response. Use -b cookies.txt for later calls.
curl -b cookies.txt 'https://assets.example.com/v1/projects'`}
      />
      <p>
        The API does not enable cross-origin browser CORS. Keep a project secret
        on your server; your frontend can call your own backend, which calls
        AssetHub. For direct browser file previews, return a public content URL
        or a short-lived signed link. Do not expose the project key in frontend
        bundles or URLs. This console calls the same installation and keeps its
        typed key in memory only.
      </p>
      <h2>Uploading a file</h2>
      <p>
        For files up to 8 MiB (8388608 bytes), use POST /assets with multipart
        fields project_id and file. Do not manually set Content-Type for
        FormData; the browser or library supplies its boundary. The response is
        202 with an asset id. MIME type is detected from the bytes rather than
        trusted from the filename.
      </p>
      <CodeBlock
        title="cURL: small file upload using a project key"
        code={`curl 'https://assets.example.com/v1/assets' \\
  -H "Authorization: Bearer $ASSETHUB_API_KEY" \\
  -F 'project_id=YOUR_PROJECT_ID' \\
  -F 'file=@./hello.txt'`}
      />
      <p>
        For resumable transfers, files may contain 1–536870912 bytes (512 MiB).
        Follow these steps:
      </p>
      <ol>
        <li>
          POST /uploads with project_id, name and size_bytes. Optionally add a
          lowercase full-file SHA-256. Save the returned upload id, part_bytes
          and expires_at (24 hours).
        </li>
        <li>
          Split the file into part_bytes chunks (currently 8 MiB). Number them
          from 0. The last part has only the remaining bytes.
        </li>
        <li>
          PUT each raw part to /uploads/UPLOAD_ID/parts/INDEX with
          X-Content-SHA256 set to its lowercase 64-character SHA-256. In this
          console, selecting a part file calculates that header automatically.
        </li>
        <li>
          If interrupted, GET /uploads/UPLOAD_ID. Compare received parts to your
          local file and send only missing parts. Identical part retries are
          accepted; different bytes for an existing part conflict.
        </li>
        <li>
          POST /uploads/UPLOAD_ID/complete with no body. It verifies the file
          and returns 202 plus the asset. Missing parts return 409; a length or
          checksum mismatch returns 422.
        </li>
        <li>
          Poll GET /assets/ASSET_ID every 2–5 seconds. queued → uploading →
          verifying → ready means it can be downloaded. failed includes
          error_code; retry eligible failures with POST /assets/ASSET_ID/retry.
        </li>
      </ol>
      <p>
        Completion retries return the same asset, so retry a timed-out
        completion before starting a new transfer. Creating a new upload,
        project or key is not generally idempotent. Cancel unfinished transfers
        with DELETE /uploads/ID; delete the resulting asset after completion.
        Storage chunks (16 MiB) are internal encrypted storage units and differ
        from upload parts. The worker can retry transient failures; retained
        staging availability determines whether a later retry is possible.
      </p>
      <h2>Download, share, delete and restore</h2>
      <p>
        GET /assets/ID/content returns bytes. HEAD returns only headers. Private
        files need a reading session/key or a valid token; public files need no
        authentication. A file must be ready, or content returns 404. download=1
        forces an attachment; supported safe images and videos otherwise display
        inline. Other types download as attachments.
      </p>
      <p>
        A single Range such as bytes=0-1023 returns 206 and Content-Range.
        Open-ended and suffix ranges are supported. Multiple ranges fall back to
        a full 200 response. An invalid range returns 416 with Content-Range:
        bytes */TOTAL and {`{"error":"range_not_satisfiable"}`}. If-Range
        compares the previous ETag; a mismatch sends the full file. Check
        Content-Length and your checksum after interrupted downloads; a stream
        failure cannot append a JSON error to bytes already sent.
      </p>
      <p>
        POST /assets/ID/signed-url creates a content-only link. expires_in
        defaults to 900 seconds; 0 also means 900, and 1–86400 is valid. Treat
        the URL as a secret: its holder can download until expiry. Renaming,
        changing visibility, deleting or restoring revokes old signed links. It
        does not grant file-list or metadata access.
      </p>
      <p>
        Deletion revokes access immediately, while remote cleanup runs
        asynchronously. If backups are enabled, the encrypted backup remains for
        30 days and a workspace admin may restore it when quota allows. Without
        backups, retention is 0 and restoration returns 503. Keep the deleted
        asset id; workspace admins can use GET /assets?status=deleted to list
        deleted metadata. GET /settings reports actual feature availability; it
        is read-only.
      </p>
      <h2>Pagination, search and updates</h2>
      <p>
        Files, uploads and activity use limit (1–100, default 50) and offset
        (0–100000, default 0). Missing or invalid values fall back to those
        defaults. Responses include items, limit and offset, without a total
        count. Fetch the next page with offset + limit and stop when
        items.length is smaller than limit. Sorting is newest first; concurrent
        changes can shift offset pages, so deduplicate by id when collecting a
        changing list.
      </p>
      <p>
        q is a case-insensitive search. Files also accept project_id, status and
        a MIME prefix type. Uploads accept project_id and effective status;
        activity accepts project_id and an action prefix. Projects, members and
        keys return their full authorized list without pagination. Lists remain
        scoped to your permissions. There is no WebSocket or event-stream API;
        poll active transfers every few seconds and use slower polling for idle
        lists.
      </p>
      <h2>Errors, limits and retries</h2>
      <p>
        Check the HTTP status before treating a response as successful. Most
        failures return error.code, error.message and error.request_id;
        X-Request-ID is also available in response headers. Keep the request id
        when reporting a problem. Rate-limit errors may omit the body request
        id. Content 416 and readiness 503 use the special shapes shown in their
        endpoint examples. HEAD has no response body.
      </p>
      <div className="docs-table-wrap">
        <table>
          <thead>
            <tr>
              <th>Status / code</th>
              <th>What to do</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>400 invalid_input</td>
              <td>
                Check field names, formats and required fields. JSON is limited
                to 64 KiB, one object, and unknown fields are rejected.
              </td>
            </tr>
            <tr>
              <td>401 unauthorized</td>
              <td>
                Sign in again or replace an expired/revoked key or signed URL.
              </td>
            </tr>
            <tr>
              <td>403 forbidden</td>
              <td>Check role, key scope and session Origin/CSRF headers.</td>
            </tr>
            <tr>
              <td>404 not_found</td>
              <td>
                Check the id, project access and file readiness. Another
                project’s resource can appear missing.
              </td>
            </tr>
            <tr>
              <td>409 conflict</td>
              <td>
                Inspect current state, missing parts, expired transfer, quota
                reduction or existing record.
              </td>
            </tr>
            <tr>
              <td>413 capacity_exceeded</td>
              <td>
                Check project quota, reserved transfers, file size and available
                staging/disk capacity.
              </td>
            </tr>
            <tr>
              <td>416 range_not_satisfiable</td>
              <td>Use a range within the Content-Range total size.</td>
            </tr>
            <tr>
              <td>422 integrity_failed</td>
              <td>Recalculate checksums and verify exact part/file length.</td>
            </tr>
            <tr>
              <td>429 rate_limited / busy</td>
              <td>
                Wait for Retry-After seconds. Login/invitation acceptance share
                10 attempts/minute per client address and 2 concurrent requests;
                upload transfers and content streams each allow 4 concurrent
                requests per API process.
              </td>
            </tr>
            <tr>
              <td>500 internal_error</td>
              <td>
                Record the request id. Retry reads cautiously; inspect state
                before repeating writes.
              </td>
            </tr>
            <tr>
              <td>503 storage_unavailable / feature_unavailable</td>
              <td>
                Check effective settings and configured dependencies. Restore
                requires backups and signing requires signing configuration.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p>
        Use bounded retries with exponential backoff and jitter for transient
        429/503 failures. Do not automatically retry an invalid request,
        permission failure or checksum mismatch. Names must be nonempty safe
        names up to 180 characters after trimming, without NUL, carriage return
        or newline characters. Timestamps use RFC3339; sizes and quotas are
        integer bytes. Examples use illustrative ids and credentials: replace
        them with your installation’s values.
      </p>
      <h2>OpenAPI and operations</h2>
      <p>
        Download the OpenAPI 3.1 contract as JSON or YAML from this page to
        import into Postman, generate a client, or share the API definition. It
        contains every route, including /healthz (HTTP process alive) and
        /readyz (dependency readiness), which live outside /v1. Endpoint
        examples describe application responses; a reverse proxy or network
        failure may return a different body. The console never follows HTTP
        redirects with a key and limits previews to 64 KiB; use your application
        or cURL to save complete large downloads.
      </p>
    </div>
  );
}
