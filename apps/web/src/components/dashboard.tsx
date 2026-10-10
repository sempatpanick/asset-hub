"use client";
import { SearchSelect } from "./select";
import { useEffect, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  Check,
  ChevronRight,
  CloudUpload,
  Copy,
  Database,
  File,
  FileImage,
  FileText,
  Folder,
  FolderOpen,
  Grid2X2,
  HardDrive,
  KeyRound,
  Layers3,
  LayoutDashboard,
  List,
  LogOut,
  Menu,
  Plus,
  Search,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Video,
  Activity as ActivityIcon,
  X,
} from "lucide-react";
import type {
  Activity,
  ApiKey,
  Asset,
  Health,
  Items,
  Member,
  Overview,
  Project,
  Settings,
  User,
} from "@assethub/api-client";
import { mutate, request, setCsrf } from "@/infrastructure/api/client";
import {
  bytes,
  useAction,
  useResource,
  useDebounced,
  useLocationQuery,
} from "@/features/hooks";
import { HeaderSearch } from "./header-search";
import { Pagination, TableSearch } from "./table-controls";
import { Badge, Empty, ErrorNotice, LinkAction, Loading, Modal } from "./ui";
import { UploadDialog, UploadHistory } from "./uploads";
import { AssetDetails, safeImage, assetContent } from "./asset-details";

type Section =
  | "overview"
  | "files"
  | "projects"
  | "uploads"
  | "api-keys"
  | "activity"
  | "settings";
const navigation = [
  { id: "overview", title: "Overview", icon: LayoutDashboard, href: "/" },
  { id: "files", title: "File Manager", icon: FolderOpen, href: "/files/" },
  { id: "projects", title: "Projects", icon: Grid2X2, href: "/projects/" },
  { id: "uploads", title: "Uploads", icon: CloudUpload, href: "/uploads/" },
  { id: "api-keys", title: "API Keys", icon: KeyRound, href: "/api-keys/" },
  {
    id: "activity",
    title: "Activity Logs",
    icon: ActivityIcon,
    href: "/activity/",
  },
  { id: "settings", title: "Settings", icon: Settings2, href: "/settings/" },
];
const headings: Record<Section, [string, string]> = {
  overview: [
    "Workspace overview",
    "A little clarity for everything you create.",
  ],
  files: ["File Manager", "All your assets, right where you need them."],
  projects: ["Projects", "Give every application a space of its own."],
  uploads: ["Uploads", "Follow your files from upload to ready."],
  "api-keys": ["API Keys", "Connect your applications securely."],
  activity: [
    "Activity Logs",
    "A clear record of what happens in your workspace.",
  ],
  settings: ["Settings", "Your workspace, configured your way."],
};
const initials = (name: string) =>
  name
    .split(/[\s@]+/)
    .slice(0, 2)
    .map((n) => n[0])
    .join("")
    .toUpperCase();
const date = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
function FileIcon({ mime }: { mime: string }) {
  const Icon = mime.startsWith("image/")
    ? FileImage
    : mime.startsWith("video/")
      ? Video
      : FileText;
  return (
    <span
      className={`file-icon ${mime.startsWith("image/") ? "violet" : mime.startsWith("video/") ? "peach" : "blue"}`}
    >
      <Icon size={21} />
    </span>
  );
}
async function copy(value: string) {
  if (!navigator.clipboard)
    throw new Error("Copy requires HTTPS. Select and copy the value manually.");
  await navigator.clipboard.writeText(value);
}

export default function Dashboard({ section }: { section: Section }) {
  const router = useRouter();
  const [user, setUser] = useState<User>();
  const [authError, setAuthError] = useState("");
  const [mobile, setMobile] = useState(false);
  const [selectedProject, setProject] = useState("");
  const [modal, setModal] = useState<
    "project" | "key" | "invite" | "upload" | null
  >(null);
  const [editing, setEditing] = useState<Project>();
  const [selectedAsset, setSelectedAsset] = useState<Asset>();
  const [members, setMembers] = useState<Project>();
  const locationQuery = useLocationQuery();
  const [searchInput, setSearch] = useState<string>();
  const search =
    searchInput ??
    (section === "files"
      ? (new URLSearchParams(locationQuery).get("q") ?? "")
      : "");
  const query = useDebounced(search);
  const [filter, setFilter] = useState("");
  const [fileType, setFileType] = useState("");
  const [grid, setGrid] = useState(false);
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const resizePage = (size: number) => {
    setPageSize(size);
    setOffset(0);
  };
  const searchTable = (value: string) => {
    setSearch(value);
    setOffset(0);
  };
  const filterTable = (value: string) => {
    setFilter(value);
    setOffset(0);
  };
  const action = useAction();
  useEffect(() => {
    const controller = new AbortController();
    request<{ user: User; csrf_token: string }>("/auth/me", {
      signal: controller.signal,
    })
      .then((r) => {
        setCsrf(r.csrf_token);
        setUser(r.user);
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          if (e.status === 401) router.replace("/login/");
          else setAuthError(e.message);
        }
      });
    return () => controller.abort();
  }, [router]);
  const projects = useResource<Items<Project>>(user ? "/projects" : null);
  const allProjects = projects.data?.items ?? [];
  const managedProjects = allProjects.filter((p) => p.role === "manager");
  const project =
    selectedProject ||
    (section === "api-keys" ? (managedProjects[0]?.id ?? "") : "");
  const settings = useResource<Settings>(user ? "/settings" : null, 30000);
  const overview = useResource<Overview>(
    user && section === "overview" ? "/overview" : null,
  );
  const health = useResource<Items<Health>>(
    user?.role === "admin" ? "/storage/health" : null,
  );
  const assets = useResource<Items<Asset>>(
    user && section === "files"
      ? `/assets?project_id=${project}&q=${encodeURIComponent(query)}&status=${filter}&type=${encodeURIComponent(fileType)}&offset=${offset}&limit=${pageSize}`
      : null,
  );
  const activity = useResource<Items<Activity>>(
    user && section === "activity"
      ? `/activity?offset=${offset}&limit=${pageSize}&q=${encodeURIComponent(query)}&action=${encodeURIComponent(filter)}&project_id=${project}`
      : null,
  );
  const keys = useResource<Items<ApiKey>>(
    user && section === "api-keys" && project
      ? `/api-keys?project_id=${project}`
      : null,
  );
  const refresh = () => {
    projects.refresh();
    overview.refresh();
    keys.refresh();
    activity.refresh();
    assets.refresh();
  };
  const filteredProjects = allProjects.filter(
    (p) =>
      `${p.name} ${p.description}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (!filter || p.role === filter),
  );
  const filteredKeys = (keys.data?.items ?? []).filter((k) => {
    const status = k.revoked_at
      ? "revoked"
      : k.expires_at && new Date(k.expires_at) <= new Date()
        ? "expired"
        : "active";
    return (
      `${k.name} ${k.prefix} ${k.scopes.join(" ")}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (!filter || status === filter)
    );
  });
  if (!user)
    return (
      <div className="boot-screen">
        <span className="brand-mark">
          <Layers3 />
        </span>
        {authError ? (
          <>
            <ErrorNotice message={authError} />
            <button className="button" onClick={() => window.location.reload()}>
              Try again
            </button>
          </>
        ) : (
          <Loading />
        )}
      </div>
    );
  const totalQuota = allProjects.reduce((sum, p) => sum + p.quota_bytes, 0);
  const totalUsed = allProjects.reduce((sum, p) => sum + p.used_bytes, 0);
  const title = headings[section];
  return (
    <div className="app-shell">
      {mobile && (
        <button
          className="sidebar-backdrop"
          aria-label="Close navigation"
          onClick={() => setMobile(false)}
        />
      )}
      <aside className={`sidebar ${mobile ? "open" : ""}`}>
        <Link href="/" className="brand">
          <span className="brand-mark">
            <Layers3 size={22} />
          </span>
          AssetHub
          <span className="brand-dot" />
        </Link>
        <div className="workspace-label">
          <span className="workspace-icon">
            <HardDrive size={17} />
          </span>
          <div>
            <strong>Your workspace</strong>
            <span>Self-hosted storage</span>
          </div>
          <ShieldCheck size={15} />
        </div>
        <p className="nav-label">WORKSPACE</p>
        <nav aria-label="Main navigation">
          {navigation.map((n) => (
            <Link
              key={n.id}
              href={n.href}
              className={`nav-item ${section === n.id ? "active" : ""}`}
              aria-current={section === n.id ? "page" : undefined}
              onClick={() => setMobile(false)}
            >
              <n.icon size={19} strokeWidth={1.8} />
              <span>{n.title}</span>
              {section === n.id && <span className="active-dot" />}
            </Link>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="storage-card">
            <div className="storage-card-title">
              <HardDrive size={16} />
              <strong>Workspace storage</strong>
            </div>
            <div className="progress-track">
              <div
                style={{
                  width: totalQuota
                    ? `${Math.min((totalUsed / totalQuota) * 100, 100)}%`
                    : "0%",
                }}
              />
            </div>
            <p>
              {totalQuota ? (
                <>
                  <strong>{bytes(totalUsed)}</strong> of {bytes(totalQuota)}{" "}
                  allocated
                </>
              ) : (
                "Set capacity with your first project"
              )}
            </p>
            <Link href="/projects/">
              Manage projects
              <ArrowRight size={13} />
            </Link>
          </div>
          <div className="sidebar-version">
            <span className="tiny-dot" />
            Self-hosted workspace
            <span>v{settings.data?.version ?? "0.1.0"}</span>
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <div className="breadcrumb">
            <button
              className="icon-button mobile-toggle"
              aria-label="Open navigation"
              onClick={() => setMobile(true)}
            >
              <Menu size={21} />
            </button>
            <span>Workspace</span>
            <ChevronRight size={14} />
            <strong>{navigation.find((n) => n.id === section)?.title}</strong>
          </div>
          <div className="topbar-right">
            <HeaderSearch
              onSearch={section === "files" ? searchTable : undefined}
            />
            <span className="top-divider" />
            <div className="user-menu">
              <span className="avatar">{initials(user.name)}</span>
              <div>
                <strong>{user.name}</strong>
                <span>
                  {user.role === "admin" ? "Administrator" : "Team member"}
                </span>
              </div>
              <button
                className="icon-button"
                aria-label="Sign out"
                title="Sign out"
                disabled={action.busy}
                onClick={() =>
                  void action.run(async () => {
                    await mutate("/auth/logout");
                    router.replace("/login/");
                  })
                }
              >
                <LogOut size={16} />
              </button>
            </div>
          </div>
        </header>
        <main className="main-content">
          <div className="page-heading">
            <div>
              <div className="eyebrow">YOUR WORKSPACE, AT A GLANCE</div>
              <h1>{title[0]}</h1>
              <p>{title[1]}</p>
            </div>
            <div className="heading-actions">
              {["files", "api-keys", "activity"].includes(section) && (
                <label className="select-label">
                  <Folder size={15} />
                  <SearchSelect
                    aria-label="Project filter"
                    value={project}
                    onValueChange={(value) => {
                      setProject(value);
                      setOffset(0);
                    }}
                  >
                    {section !== "api-keys" && (
                      <option value="">All projects</option>
                    )}
                    {(section === "api-keys"
                      ? managedProjects
                      : allProjects
                    ).map((p) => (
                      <option value={p.id} key={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </SearchSelect>
                </label>
              )}
              {section === "projects" || section === "overview" ? (
                <button
                  className="button primary"
                  onClick={() => {
                    setEditing(undefined);
                    setModal("project");
                  }}
                >
                  <Plus size={17} />
                  New project
                </button>
              ) : section === "api-keys" ? (
                <button
                  className="button primary"
                  disabled={!project}
                  onClick={() => setModal("key")}
                >
                  <Plus size={17} />
                  Create API key
                </button>
              ) : section === "files" || section === "uploads" ? (
                <button
                  className="button primary"
                  disabled={
                    !settings.data?.features.uploads ||
                    !allProjects.some((p) => p.role !== "viewer")
                  }
                  title={
                    settings.data?.features.uploads
                      ? "Upload to a project"
                      : "Configure storage and encryption to enable uploads"
                  }
                  onClick={() => setModal("upload")}
                >
                  <CloudUpload size={17} />
                  Upload files
                </button>
              ) : null}
            </div>
          </div>
          <ErrorNotice
            message={action.error || projects.error || settings.error}
          />
          {section === "overview" && (
            <>
              <ErrorNotice message={overview.error} />
              {overview.loading ? (
                <Loading />
              ) : (
                overview.data && (
                  <OverviewView
                    backupEnabled={settings.data?.features.backups !== false}
                    data={overview.data}
                    health={health.data?.items}
                    onProject={() => setModal("project")}
                  />
                )
              )}
            </>
          )}
          {section === "projects" && (
            <>
              <section className="panel collection-toolbar">
                <div className="file-toolbar">
                  <TableSearch
                    label="Search projects"
                    value={search}
                    onChange={searchTable}
                  />
                  <SearchSelect
                    aria-label="Project role filter"
                    value={filter}
                    onValueChange={filterTable}
                  >
                    <option value="">All roles</option>
                    {["viewer", "editor", "manager"].map((r) => (
                      <option key={r} value={r}>
                        {r}
                      </option>
                    ))}
                  </SearchSelect>
                </div>
              </section>
              {projects.loading ? (
                <Loading />
              ) : filteredProjects.length ? (
                <div className="project-grid">
                  {filteredProjects
                    .slice(offset, offset + pageSize)
                    .map((p, i) => (
                      <article className="panel project-card" key={p.id}>
                        <div className="project-top">
                          <span
                            className={`project-symbol ${["violet", "blue", "peach", "mint"][i % 4]}`}
                          >
                            <Folder size={25} />
                          </span>
                          <Badge tone="neutral">{p.role}</Badge>
                        </div>
                        <h3>{p.name}</h3>
                        <p>
                          {p.description ||
                            "A dedicated space for your application’s assets."}
                        </p>
                        <div className="project-stats">
                          <span>
                            <File size={14} />
                            {p.files} files
                          </span>
                          <span>{bytes(p.used_bytes)}</span>
                        </div>
                        <div className="progress-track">
                          <div
                            style={{
                              width: `${Math.min((p.used_bytes / p.quota_bytes) * 100, 100)}%`,
                            }}
                          />
                        </div>
                        <div className="project-capacity">
                          {bytes(p.quota_bytes)} allocated
                        </div>
                        <div className="project-footer">
                          <button
                            className="text-link"
                            onClick={() => setMembers(p)}
                          >
                            Members
                            <ArrowRight size={14} />
                          </button>
                          {p.role === "manager" && (
                            <button
                              className="icon-button"
                              aria-label={`Edit ${p.name}`}
                              onClick={() => {
                                setEditing(p);
                                setModal("project");
                              }}
                            >
                              <Settings2 size={17} />
                            </button>
                          )}
                        </div>
                      </article>
                    ))}
                </div>
              ) : (
                <section className="panel">
                  <Empty
                    title={
                      search || filter
                        ? "No matching projects"
                        : "Make room for your first project"
                    }
                    detail="Projects keep each application’s assets and API permissions together."
                    action={
                      <button
                        className="button primary"
                        onClick={() => setModal("project")}
                      >
                        <Plus size={16} />
                        Create a project
                      </button>
                    }
                  />
                </section>
              )}
              <Pagination
                offset={offset}
                count={filteredProjects.slice(offset, offset + pageSize).length}
                pageSize={pageSize}
                onChange={setOffset}
                onPageSizeChange={resizePage}
                hasMore={offset + pageSize < filteredProjects.length}
              />
            </>
          )}
          {section === "files" && (
            <section className="panel">
              <div className="file-toolbar">
                <div className="search-field">
                  <Search size={17} />
                  <input
                    aria-label="Search files"
                    placeholder="Search by file name…"
                    value={search}
                    onChange={(e) => {
                      setSearch(e.target.value);
                      setOffset(0);
                    }}
                  />
                  {search && (
                    <button
                      className="icon-button"
                      onClick={() => searchTable("")}
                      aria-label="Clear search"
                    >
                      <X size={15} />
                    </button>
                  )}
                </div>
                <div className="file-controls">
                  <SlidersHorizontal size={16} />
                  <SearchSelect
                    aria-label="Status filter"
                    value={filter}
                    onValueChange={(value) => {
                      setFilter(value);
                      setOffset(0);
                    }}
                  >
                    <option value="">All statuses</option>
                    {[
                      "ready",
                      "queued",
                      "uploading",
                      "verifying",
                      "failed",
                      ...(user.role === "admin" ? ["deleted"] : []),
                    ].map((v) => (
                      <option key={v} value={v}>
                        {v}
                      </option>
                    ))}
                  </SearchSelect>
                  <SearchSelect
                    aria-label="File type filter"
                    value={fileType}
                    onValueChange={(value) => {
                      setFileType(value);
                      setOffset(0);
                    }}
                  >
                    <option value="">All types</option>
                    <option value="image/">Images</option>
                    <option value="video/">Videos</option>
                    <option value="audio/">Audio</option>
                    <option value="text/">Text</option>
                    <option value="application/">Documents & archives</option>
                  </SearchSelect>
                  <div className="segmented">
                    <button
                      aria-label="List view"
                      className={!grid ? "selected" : ""}
                      onClick={() => setGrid(false)}
                    >
                      <List size={18} />
                    </button>
                    <button
                      aria-label="Grid view"
                      className={grid ? "selected" : ""}
                      onClick={() => setGrid(true)}
                    >
                      <Grid2X2 size={18} />
                    </button>
                  </div>
                </div>
              </div>
              <ErrorNotice message={assets.error} />
              {assets.loading ? (
                <Loading />
              ) : assets.data?.items.length ? (
                <>
                  <AssetList
                    assets={assets.data.items}
                    grid={grid}
                    onSelect={setSelectedAsset}
                  />
                </>
              ) : (
                <Empty
                  title={
                    search || filter || fileType || offset
                      ? "No matching assets"
                      : "A fresh start for your files"
                  }
                  detail={
                    search || filter || fileType || offset
                      ? "Try another name, status, or project."
                      : "Choose a project and upload your first files."
                  }
                />
              )}
              <Pagination
                offset={offset}
                count={assets.data?.items.length ?? 0}
                pageSize={pageSize}
                onChange={setOffset}
                onPageSizeChange={resizePage}
              />
              <div className="panel-note">
                <ShieldCheck size={15} />
                {settings.data?.features.backups === false
                  ? "Files become ready after encrypted primary storage verification. Backups are disabled."
                  : "Files become ready after encrypted storage and backup verification."}{" "}
                Select a file to preview, download or manage its access.
              </div>
            </section>
          )}
          {section === "uploads" && (
            <>
              <div className="stats-row two">
                <Stat
                  label="Transfer limit"
                  value="512 MB"
                  icon={HardDrive}
                  tone="violet"
                  note="Maximum per asset"
                />
                <Stat
                  label="Request parts"
                  value="8 MB"
                  icon={Layers3}
                  tone="peach"
                  note="Resumable transfer size"
                />
              </div>
              <UploadHistory
                projects={allProjects}
                enabled={!!settings.data?.features.uploads}
                onUpload={() => setModal("upload")}
              />
            </>
          )}
          {section === "api-keys" && (
            <section className="panel">
              <div className="panel-heading">
                <div>
                  <h2>Application access</h2>
                  <p>
                    Project-scoped credentials. Only the permissions you choose.
                  </p>
                </div>
                <Badge tone="blue">
                  {keys.data?.items.filter(
                    (k) =>
                      !k.revoked_at &&
                      (!k.expires_at || new Date(k.expires_at) > new Date()),
                  ).length ?? 0}{" "}
                  active
                </Badge>
              </div>
              <ErrorNotice message={keys.error} />
              <div className="file-toolbar">
                <TableSearch
                  label="Search API keys"
                  value={search}
                  onChange={searchTable}
                />
                <SearchSelect
                  aria-label="Key status filter"
                  value={filter}
                  onValueChange={filterTable}
                >
                  <option value="">All statuses</option>
                  {["active", "revoked", "expired"].map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </SearchSelect>
              </div>
              {!project ? (
                <Empty
                  title="Create a project to connect an application"
                  detail="Only project managers can issue and revoke application keys."
                  action={
                    <LinkAction href="/projects/">Go to projects</LinkAction>
                  }
                />
              ) : keys.loading ? (
                <Loading />
              ) : filteredKeys.length ? (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Key name</th>
                        <th>Permissions</th>
                        <th>Status</th>
                        <th>Created</th>
                        <th />
                      </tr>
                    </thead>
                    <tbody>
                      {filteredKeys
                        .slice(offset, offset + pageSize)
                        .map((k) => (
                          <tr key={k.id}>
                            <td>
                              <div className="key-name">
                                <span className="file-icon blue">
                                  <KeyRound size={19} />
                                </span>
                                <div>
                                  <strong>{k.name}</strong>
                                  <code>{k.prefix}••••••</code>
                                </div>
                              </div>
                            </td>
                            <td>
                              <div className="scope-list">
                                {k.scopes.map((s) => (
                                  <Badge key={s}>{s.split(":")[1]}</Badge>
                                ))}
                              </div>
                            </td>
                            <td>
                              <Badge
                                tone={
                                  k.revoked_at
                                    ? "neutral"
                                    : k.expires_at &&
                                        new Date(k.expires_at) < new Date()
                                      ? "peach"
                                      : "mint"
                                }
                              >
                                {k.revoked_at
                                  ? "Revoked"
                                  : k.expires_at &&
                                      new Date(k.expires_at) < new Date()
                                    ? "Expired"
                                    : "Active"}
                              </Badge>
                            </td>
                            <td className="muted">{date(k.created_at)}</td>
                            <td>
                              {!k.revoked_at && (
                                <button
                                  className="button small danger"
                                  disabled={action.busy}
                                  onClick={() =>
                                    void action.run(async () => {
                                      await mutate(
                                        `/api-keys/${k.id}?project_id=${project}`,
                                        undefined,
                                        "DELETE",
                                      );
                                      keys.refresh();
                                    })
                                  }
                                >
                                  Revoke
                                </button>
                              )}
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty
                  title={
                    search || filter
                      ? "No matching API keys"
                      : "Your applications are ready to connect"
                  }
                  detail="Create a key for this project. You’ll see its secret once."
                  action={
                    <button
                      className="button primary"
                      onClick={() => setModal("key")}
                    >
                      <Plus size={16} />
                      Create API key
                    </button>
                  }
                />
              )}
              <Pagination
                offset={offset}
                count={filteredKeys.slice(offset, offset + pageSize).length}
                pageSize={pageSize}
                onChange={setOffset}
                onPageSizeChange={resizePage}
                hasMore={offset + pageSize < filteredKeys.length}
              />
              <div className="panel-note">
                <LockNote />
                Keep API keys on your application server. Never include them in
                frontend code.
              </div>
            </section>
          )}
          {section === "activity" && (
            <section className="panel">
              <div className="panel-heading">
                <div>
                  <h2>Workspace activity</h2>
                  <p>Persistent audit events from your authorized projects.</p>
                </div>
                <Badge>Audit trail</Badge>
              </div>
              <ErrorNotice message={activity.error} />
              <div className="file-toolbar">
                <TableSearch
                  label="Search activity"
                  value={search}
                  onChange={searchTable}
                />
                <SearchSelect
                  aria-label="Activity action filter"
                  value={filter}
                  onValueChange={filterTable}
                >
                  <option value="">All actions</option>
                  {[
                    "project.",
                    "member.",
                    "key.",
                    "upload.",
                    "asset.",
                    "auth.",
                    "user.",
                    "admin.",
                  ].map((s) => (
                    <option key={s} value={s}>
                      {s.slice(0, -1)}
                    </option>
                  ))}
                </SearchSelect>
              </div>
              {activity.loading ? (
                <Loading />
              ) : activity.data?.items.length ? (
                <>
                  <div className="activity-list">
                    {activity.data.items.map((a) => (
                      <div className="activity-row" key={a.id}>
                        <span className="activity-symbol">
                          <ActivityIcon size={18} />
                        </span>
                        <div className="activity-description">
                          <strong>{a.action.replaceAll(".", " · ")}</strong>
                          <p>{a.detail}</p>
                          <span>{a.actor}</span>
                        </div>
                        <time>{date(a.created_at)}</time>
                      </div>
                    ))}
                  </div>
                </>
              ) : (
                <Empty
                  title={
                    search || filter || project || offset
                      ? "No matching activity"
                      : "Your workspace history starts here"
                  }
                  detail="Project, membership, and API key changes appear automatically."
                />
              )}
              <Pagination
                offset={offset}
                count={activity.data?.items.length ?? 0}
                pageSize={pageSize}
                onChange={setOffset}
                onPageSizeChange={resizePage}
              />
            </section>
          )}
          {section === "settings" && (
            <>
              <div className="settings-grid">
                <section className="panel">
                  <div className="panel-heading">
                    <div>
                      <h2>Workspace</h2>
                      <p>Your installation and signed-in account.</p>
                    </div>
                    <span className="file-icon blue">
                      <Settings2 size={20} />
                    </span>
                  </div>
                  <SettingsRow label="Workspace" value="AssetHub" />
                  <SettingsRow label="Account" value={user.email} />
                  <SettingsRow label="Role" value={user.role} />
                  <SettingsRow
                    label="Release"
                    value={`${settings.data?.version ?? "0.1.0"} · ${settings.data?.phase ?? "foundation"}`}
                  />
                  <div className="panel-note">
                    Deployment configuration is managed through environment
                    variables.
                  </div>
                </section>
                <section className="panel">
                  <div className="panel-heading">
                    <div>
                      <h2>Storage design</h2>
                      <p>Planned defaults for the ingestion phase.</p>
                    </div>
                    <span className="file-icon violet">
                      <ShieldCheck size={20} />
                    </span>
                  </div>
                  <SettingsRow
                    label="Primary provider"
                    value="Telegram Bot API"
                  />
                  <SettingsRow
                    label="Encryption"
                    value="AES-256-GCM per chunk"
                  />
                  <SettingsRow
                    label="Maximum asset size"
                    value={bytes(
                      settings.data?.asset_limit_bytes ?? 512 * 1024 ** 2,
                    )}
                  />
                  <SettingsRow
                    label="Deleted backup retention"
                    value={
                      settings.data?.features.backups === false
                        ? "Disabled"
                        : `${settings.data?.backup_retention_days ?? 30} days`
                    }
                  />
                  <div className="panel-note">
                    {settings.data?.features.backups === false
                      ? "Backups are disabled. Ready assets rely on primary storage. Deleted assets cannot be restored."
                      : settings.data?.features.uploads
                        ? "Uploads are enabled. New assets require verified encrypted primary storage and backup."
                        : "Configure Telegram credentials and an encryption key to enable uploads. An independent backup is required."}
                  </div>
                </section>
              </div>
              {user.role === "admin" && (
                <section className="panel team-panel">
                  <div>
                    <span className="file-icon mint">
                      <ShieldCheck size={21} />
                    </span>
                    <div>
                      <h2>Bring your team together</h2>
                      <p>
                        Invite an account, then grant access to individual
                        projects.
                      </p>
                    </div>
                  </div>
                  <button className="button" onClick={() => setModal("invite")}>
                    <Plus size={16} />
                    Invite member
                  </button>
                </section>
              )}
              <section className="panel provider-panel">
                <div className="panel-heading">
                  <div>
                    <h2>Service connections</h2>
                    <p>
                      Live dependency checks and explicit implementation status.
                    </p>
                  </div>
                  <button
                    className="text-link"
                    onClick={() => health.refresh()}
                    disabled={user.role !== "admin"}
                  >
                    Refresh
                    <ArrowRight size={14} />
                  </button>
                </div>
                {user.role === "admin" ? (
                  <>
                    <ErrorNotice message={health.error} />
                    {health.loading ? (
                      <Loading />
                    ) : (
                      health.data?.items.map((h) => (
                        <div className="health-row" key={h.component}>
                          <span className="health-icon">
                            <Database size={19} />
                          </span>
                          <div>
                            <strong>
                              {h.component === "local"
                                ? "Local development storage"
                                : h.component === "telegram"
                                  ? "Telegram storage"
                                  : h.component === "redis"
                                    ? "Redis queue"
                                    : h.component === "backup"
                                      ? "Independent backup"
                                      : h.component === "worker"
                                        ? "Background worker"
                                        : h.component === "jobs"
                                          ? "Durable jobs"
                                          : "Staging filesystem"}
                            </strong>
                            <p>{h.detail}</p>
                          </div>
                          <Badge
                            tone={
                              h.status === "healthy"
                                ? "mint"
                                : h.status === "configured"
                                  ? "blue"
                                  : "peach"
                            }
                          >
                            {h.status.replaceAll("_", " ")}
                          </Badge>
                        </div>
                      ))
                    )}
                  </>
                ) : (
                  <div className="panel-note">
                    Detailed service health is available to installation
                    administrators.
                  </div>
                )}
              </section>
            </>
          )}
          <footer className="dashboard-footer">
            <span>
              AssetHub <span>·</span> A home for every asset.
            </span>
            <span>
              <ShieldCheck size={13} />
              Your workspace. Your control.
            </span>
          </footer>
        </main>
      </div>
      {modal === "project" && (
        <ProjectDialog
          project={editing}
          onClose={() => setModal(null)}
          onSaved={() => {
            setModal(null);
            refresh();
          }}
        />
      )}
      {modal === "key" && (
        <KeyDialog
          project={project}
          onClose={() => setModal(null)}
          onSaved={() => keys.refresh()}
        />
      )}
      {modal === "invite" && <InviteDialog onClose={() => setModal(null)} />}
      {modal === "upload" && (
        <UploadDialog
          projects={allProjects.filter((p) => p.role !== "viewer")}
          project={
            allProjects.some((p) => p.id === project && p.role !== "viewer")
              ? project
              : ""
          }
          userID={user.id}
          onClose={() => setModal(null)}
          onSaved={refresh}
        />
      )}
      {selectedAsset && (
        <AssetDetails
          asset={selectedAsset}
          backupEnabled={settings.data?.features.backups !== false}
          canWrite={allProjects.some(
            (p) => p.id === selectedAsset.project_id && p.role !== "viewer",
          )}
          canManage={allProjects.some(
            (p) => p.id === selectedAsset.project_id && p.role === "manager",
          )}
          isAdmin={user.role === "admin"}
          onClose={() => setSelectedAsset(undefined)}
          onSaved={refresh}
        />
      )}
      {members && (
        <MembersDialog
          project={members}
          onClose={() => setMembers(undefined)}
        />
      )}
    </div>
  );
}
function LockNote() {
  return <ShieldCheck size={15} />;
}
function Stat({
  label,
  value,
  icon: Icon,
  tone,
  note,
}: {
  label: string;
  value: string;
  icon: typeof File;
  tone: string;
  note: string;
}) {
  return (
    <article className="panel stat-card">
      <div className="stat-top">
        <span>{label}</span>
        <span className={`stat-icon ${tone}`}>
          <Icon size={19} />
        </span>
      </div>
      <strong className="stat-value">{value}</strong>
      <div className="stat-note">{note}</div>
    </article>
  );
}
function OverviewView({
  backupEnabled,
  data,
  health,
  onProject,
}: {
  backupEnabled: boolean;
  data: Overview;
  health?: Health[];
  onProject: () => void;
}) {
  const max = Math.max(4, ...data.activity.map((a) => a.count));
  const total = data.activity.reduce((sum, a) => sum + a.count, 0);
  return (
    <>
      <div className="stats-row">
        <Stat
          label="Total files"
          value={data.files.toLocaleString()}
          icon={File}
          tone="blue"
          note="Across your authorized projects"
        />
        <Stat
          label="Logical storage"
          value={bytes(data.bytes)}
          icon={HardDrive}
          tone="violet"
          note="Original asset bytes"
        />
        <Stat
          label="Projects"
          value={data.projects.toLocaleString()}
          icon={Grid2X2}
          tone="peach"
          note="Organized, with isolated access"
        />
        <Stat
          label="Queued assets"
          value={data.queued.toLocaleString()}
          icon={CloudUpload}
          tone="mint"
          note="Waiting or processing in background"
        />
      </div>
      <div className="overview-middle">
        <section className="panel chart-panel">
          <div className="panel-heading">
            <div>
              <h2>Upload activity</h2>
              <p>Your assets over the last 7 days</p>
            </div>
            <Badge>
              <span className="tiny-dot blue-dot" />
              {total} uploads
            </Badge>
          </div>
          <div className="chart">
            <div className="chart-axis">
              {[max, max * 0.75, max * 0.5, max * 0.25, 0].map((n, i) => (
                <span key={i}>
                  {n.toLocaleString(undefined, { maximumFractionDigits: 1 })}
                </span>
              ))}
            </div>
            <div className="chart-plot">
              <div className="chart-lines">
                {[0, 1, 2, 3, 4].map((n) => (
                  <div key={n} />
                ))}
              </div>
              <div className="chart-bars">
                {data.activity.map((d) => (
                  <div className="chart-column" key={d.day}>
                    <div
                      className="chart-bar"
                      style={{ height: `${(d.count / max) * 100}%` }}
                      title={`${d.day}: ${d.count} uploads`}
                    />
                    <span>
                      {new Date(`${d.day}T12:00:00`).toLocaleDateString(
                        undefined,
                        { weekday: "short" },
                      )}
                    </span>
                  </div>
                ))}
              </div>
              {total === 0 && (
                <span className="chart-empty-label">
                  Your next chapter starts with your first upload.
                </span>
              )}
            </div>
          </div>
        </section>
        <section className="welcome-card">
          <div className="welcome-decoration">
            <Layers3 size={46} strokeWidth={1.3} />
            <span className="spark">+</span>
            <span className="spark second">+</span>
          </div>
          <Badge tone="white">ONE HOME. EVERY ASSET.</Badge>
          <h2>
            Less searching.
            <br />
            More creating.
          </h2>
          <p>
            Keep your applications’ assets together, with a space for every
            project.
          </p>
          <button className="button white" onClick={onProject}>
            Create a project
            <ArrowRight size={16} />
          </button>
        </section>
      </div>
      <div className="overview-bottom">
        <section className="panel recent-panel">
          <div className="panel-heading">
            <div>
              <h2>Recent assets</h2>
              <p>The latest additions to your workspace</p>
            </div>
            <LinkAction href="/files/">View all files</LinkAction>
          </div>
          {data.recent.length ? (
            <AssetList assets={data.recent} controls />
          ) : (
            <Empty
              title="A home for your first asset"
              detail="Start with a project. Your files will appear here once uploads are enabled."
              action={
                <LinkAction href="/projects/">Set up a project</LinkAction>
              }
            />
          )}
        </section>
        <section className="panel status-panel">
          <div className="panel-heading">
            <div>
              <h2>Workspace health</h2>
              <p>A clear view of your services</p>
            </div>
            <ShieldCheck size={20} className="muted" />
          </div>
          <div className="status-item">
            <span className="file-icon blue">
              <Database size={18} />
            </span>
            <div>
              <strong>Metadata API</strong>
              <span>PostgreSQL connected</span>
            </div>
            <span className="status-dot" />
          </div>
          <div className="status-item">
            <span className="file-icon violet">
              <HardDrive size={18} />
            </span>
            <div>
              <strong>Telegram storage</strong>
              <span>
                {health?.find(
                  (h) => h.component === "telegram" || h.component === "local",
                )?.status ?? "Administrator can check provider status"}
              </span>
            </div>
            <span className="status-dot amber" />
          </div>
          <div className="status-item">
            <span className="file-icon peach">
              <CloudUpload size={18} />
            </span>
            <div>
              <strong>Upload queue</strong>
              <span>
                {health?.find((h) => h.component === "redis")?.status ===
                "healthy"
                  ? "Redis reachable"
                  : "Administrator can check queue status"}
              </span>
            </div>
            <span className="status-dot amber" />
          </div>
          <div className="status-divider" />
          <div className="backup-note">
            <ShieldCheck size={20} />
            <div>
              <strong>
                {backupEnabled ? "Designed for recovery" : "Backups disabled"}
              </strong>
              <p>
                {backupEnabled
                  ? "New assets require a verified encrypted backup before becoming ready."
                  : "Primary storage holds the only permanent copy. Deleted assets cannot be restored."}
              </p>
            </div>
          </div>
          <LinkAction href="/settings/">View service settings</LinkAction>
        </section>
      </div>
    </>
  );
}
function AssetList({
  assets,
  grid = false,
  onSelect,
  controls = false,
}: {
  assets: Asset[];
  grid?: boolean;
  onSelect?: (asset: Asset) => void;
  controls?: boolean;
}) {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [kind, setKind] = useState("");
  const shown = controls
    ? assets.filter(
        (a) =>
          a.name.toLowerCase().includes(search.toLowerCase()) &&
          (!status || a.status === status) &&
          (!kind || a.mime.startsWith(kind)),
      )
    : assets;
  if (grid)
    return (
      <div className="asset-grid">
        {shown.map((a) => (
          <article className="asset-tile" key={a.id}>
            <div className="asset-preview">
              {a.status === "ready" && safeImage(a.mime) ? (
                <Image
                  src={assetContent(a.id)}
                  alt={a.name}
                  fill
                  unoptimized
                  sizes="(max-width: 720px) 50vw, 240px"
                />
              ) : (
                <FileIcon mime={a.mime} />
              )}
            </div>
            <button
              className="asset-name-button"
              onClick={() => onSelect?.(a)}
              disabled={!onSelect}
            >
              {a.name}
            </button>
            <div>
              <span>{bytes(a.size_bytes)}</span>
              <Badge>{a.status}</Badge>
            </div>
          </article>
        ))}
      </div>
    );
  return (
    <>
      {controls && (
        <div className="file-toolbar">
          <TableSearch
            label="Search recent assets"
            value={search}
            onChange={setSearch}
          />
          <div className="file-controls">
            <SearchSelect
              aria-label="Recent asset status filter"
              value={status}
              onValueChange={setStatus}
            >
              <option value="">All statuses</option>
              {Array.from(new Set(assets.map((a) => a.status))).map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </SearchSelect>
            <SearchSelect
              aria-label="Recent asset type filter"
              value={kind}
              onValueChange={setKind}
            >
              <option value="">All types</option>
              {["image/", "video/", "audio/", "text/", "application/"].map(
                (s) => (
                  <option key={s} value={s}>
                    {s.slice(0, -1)}
                  </option>
                ),
              )}
            </SearchSelect>
          </div>
        </div>
      )}
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>File name</th>
              <th>Size</th>
              <th>Status</th>
              <th>Added</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((a) => (
              <tr key={a.id}>
                <td>
                  <div className="file-name">
                    <FileIcon mime={a.mime} />
                    <div>
                      {onSelect ? (
                        <button
                          className="asset-name-button"
                          onClick={() => onSelect(a)}
                        >
                          {a.name}
                        </button>
                      ) : (
                        <strong>{a.name}</strong>
                      )}
                      <span>{a.mime}</span>
                    </div>
                  </div>
                </td>
                <td>{bytes(a.size_bytes)}</td>
                <td>
                  <Badge tone={a.status === "ready" ? "mint" : "neutral"}>
                    {a.status}
                  </Badge>
                </td>
                <td className="muted">{date(a.created_at)}</td>
              </tr>
            ))}
            {!shown.length && (
              <tr>
                <td colSpan={4}>No matching assets</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
function SettingsRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="settings-row">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
function ProjectDialog({
  project,
  onClose,
  onSaved,
}: {
  project?: Project;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(project?.name ?? "");
  const [description, setDescription] = useState(project?.description ?? "");
  const [quota, setQuota] = useState(
    String((project?.quota_bytes ?? 10 * 1024 ** 3) / 1024 ** 3),
  );
  const action = useAction();
  return (
    <Modal
      title={project ? "Project settings" : "Create a project"}
      detail="A dedicated space with its own access and storage quota."
      onClose={onClose}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const value = Number(quota) * 1024 ** 3;
            if (!Number.isSafeInteger(value) || value < 1024 ** 2)
              throw new Error("Enter a valid quota in GiB.");
            await mutate(
              project ? `/projects/${project.id}` : "/projects",
              { name, description, quota_bytes: value },
              project ? "PATCH" : "POST",
            );
            onSaved();
          });
        }}
      >
        <ErrorNotice message={action.error} />
        <label>
          Project name
          <input
            autoFocus
            required
            maxLength={180}
            placeholder="e.g. Marketing website"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label>
          Description
          <textarea
            maxLength={2000}
            placeholder="What is this project for?"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          Logical storage quota (GiB)
          <input
            type="number"
            min="0.01"
            max="100000"
            step="0.01"
            required
            value={quota}
            onChange={(e) => setQuota(e.target.value)}
          />
        </label>
        <p className="form-help">
          A quota is a limit for original asset bytes, not reserved physical
          disk space.
        </p>
        <div className="modal-actions">
          <button type="button" className="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button primary" disabled={action.busy}>
            {action.busy
              ? "Saving…"
              : project
                ? "Save changes"
                : "Create project"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
function KeyDialog({
  project,
  onClose,
  onSaved,
}: {
  project: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState(["assets:read"]);
  const [days, setDays] = useState("90");
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);
  const action = useAction();
  return (
    <Modal
      title={secret ? "Save your API key" : "Create API key"}
      detail={
        secret
          ? "This secret is shown once. Store it securely."
          : "Restrict application access to this project."
      }
      onClose={onClose}
    >
      {secret ? (
        <>
          <div className="secret-box">
            <code>{secret}</code>
          </div>
          <ErrorNotice message={action.error} />
          <div className="modal-actions">
            <button
              className="button"
              onClick={() =>
                void action.run(async () => {
                  await copy(secret);
                  setCopied(true);
                })
              }
            >
              {copied ? <Check size={16} /> : <Copy size={16} />}Copy secret
            </button>
            <button className="button primary" onClick={onClose}>
              I’ve saved it
            </button>
          </div>
        </>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              const r = await mutate<{ secret: string }>("/api-keys", {
                project_id: project,
                name,
                scopes,
                expires_at: new Date(
                  Date.now() + Number(days) * 86400000,
                ).toISOString(),
              });
              setSecret(r.secret);
              onSaved();
            });
          }}
        >
          <ErrorNotice message={action.error} />
          <label>
            Key name
            <input
              autoFocus
              required
              maxLength={180}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Production API"
            />
          </label>
          <fieldset>
            <legend>Permissions</legend>
            {[
              "assets:read",
              "assets:write",
              "assets:delete",
              "assets:sign",
            ].map((s) => (
              <label className="check-label" key={s}>
                <input
                  type="checkbox"
                  checked={scopes.includes(s)}
                  onChange={(e) =>
                    setScopes(
                      e.target.checked
                        ? [...scopes, s]
                        : scopes.filter((v) => v !== s),
                    )
                  }
                />
                <span>{s.split(":")[1]} assets</span>
              </label>
            ))}
          </fieldset>
          <label>
            Expires after
            <SearchSelect
              aria-label="Expires after"
              value={days}
              onValueChange={(value) => setDays(value)}
            >
              <option value="7">7 days</option>
              <option value="30">30 days</option>
              <option value="90">90 days</option>
              <option value="365">1 year</option>
            </SearchSelect>
          </label>
          <div className="modal-actions">
            <button type="button" className="button" onClick={onClose}>
              Cancel
            </button>
            <button
              className="button primary"
              disabled={action.busy || !scopes.length}
            >
              {action.busy ? "Creating…" : "Create key"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
function InviteDialog({ onClose }: { onClose: () => void }) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [url, setUrl] = useState("");
  const [copied, setCopied] = useState(false);
  const action = useAction();
  return (
    <Modal
      title={url ? "Your invitation is ready" : "Invite a team member"}
      detail={
        url
          ? "Share this single-use link directly. It expires in 48 hours."
          : "No email is sent automatically."
      }
      onClose={onClose}
    >
      {url ? (
        <>
          <div className="secret-box">
            <code>{url}</code>
          </div>
          <ErrorNotice message={action.error} />
          <div className="modal-actions">
            <button
              className="button"
              onClick={() =>
                void action.run(async () => {
                  await copy(url);
                  setCopied(true);
                })
              }
            >
              {copied ? <Check size={16} /> : <Copy size={16} />}Copy invitation
            </button>
            <button className="button primary" onClick={onClose}>
              Done
            </button>
          </div>
        </>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              const r = await mutate<{ url: string }>("/invitations", {
                email,
                role,
              });
              setUrl(r.url);
            });
          }}
        >
          <ErrorNotice message={action.error} />
          <label>
            Email address
            <input
              autoFocus
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="teammate@company.com"
            />
          </label>
          <label>
            Installation role
            <SearchSelect
              aria-label="Installation role"
              value={role}
              onValueChange={(value) => setRole(value)}
            >
              <option value="member">
                Member — project access assigned separately
              </option>
              <option value="admin">
                Administrator — full installation access
              </option>
            </SearchSelect>
          </label>
          <div className="modal-actions">
            <button className="button" type="button" onClick={onClose}>
              Cancel
            </button>
            <button className="button primary" disabled={action.busy}>
              {action.busy ? "Creating…" : "Create invitation"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
function MembersDialog({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const data = useResource<Items<Member>>(`/projects/${project.id}/members`);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("viewer");
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState("");
  const action = useAction();
  return (
    <Modal
      title={`${project.name} members`}
      detail="Grant access to accounts that have accepted an invitation."
      onClose={onClose}
    >
      <ErrorNotice message={data.error || action.error} />
      <div className="file-toolbar">
        <TableSearch
          label="Search members"
          value={search}
          onChange={setSearch}
        />
        <SearchSelect
          aria-label="Member role filter"
          value={roleFilter}
          onValueChange={setRoleFilter}
        >
          <option value="">All roles</option>
          {["viewer", "editor", "manager"].map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </SearchSelect>
      </div>
      {data.loading ? (
        <Loading />
      ) : (
        <div className="member-list">
          {data.data?.items
            .filter(
              (m) =>
                `${m.name} ${m.email}`
                  .toLowerCase()
                  .includes(search.toLowerCase()) &&
                (!roleFilter || m.role === roleFilter),
            )
            .map((m) => (
              <div className="member-row" key={m.user_id}>
                <span className="avatar">{initials(m.name)}</span>
                <div>
                  <strong>{m.name}</strong>
                  <span>{m.email}</span>
                </div>
                <Badge>{m.role}</Badge>
              </div>
            ))}
        </div>
      )}
      {project.role === "manager" && (
        <form
          className="member-form"
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              await mutate(`/projects/${project.id}/members`, { email, role });
              setEmail("");
              data.refresh();
            });
          }}
        >
          <label>
            Existing account email
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="teammate@company.com"
            />
          </label>
          <label>
            Project role
            <SearchSelect
              aria-label="Project role"
              value={role}
              onValueChange={(value) => setRole(value)}
            >
              <option value="viewer">Viewer</option>
              <option value="editor">Editor</option>
              <option value="manager">Manager</option>
            </SearchSelect>
          </label>
          <button className="button primary" disabled={action.busy}>
            {action.busy ? "Saving…" : "Add or update member"}
          </button>
        </form>
      )}
    </Modal>
  );
}
