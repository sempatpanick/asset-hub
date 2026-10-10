"use client";
import { createContext, useContext, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  ArrowRight,
  BookOpen,
  ExternalLink,
  ChevronRight,
  CloudUpload,
  FolderOpen,
  Grid2X2,
  HardDrive,
  KeyRound,
  Layers3,
  LayoutDashboard,
  LogOut,
  Menu,
  Settings2,
  ShieldCheck,
  Activity as ActivityIcon,
} from "lucide-react";
import type {
  Health,
  Items,
  Project,
  Settings,
  User,
} from "@assethub/api-client";
import { mutate, request, setCsrf } from "@/infrastructure/api/client";
import { bytes, useAction, useResource } from "@/features/hooks";
import { HeaderSearch } from "./header-search";
import { ErrorNotice, Loading } from "./ui";

export type Section =
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
const initials = (name: string) =>
  name
    .split(/[\s@]+/)
    .slice(0, 2)
    .map((n) => n[0])
    .join("")
    .toUpperCase();

type Resource<T> = ReturnType<typeof useResource<T>>;
const WorkspaceContext = createContext<{
  user: User;
  projects: Resource<Items<Project>>;
  settings: Resource<Settings>;
  health: Resource<Items<Health>>;
  searchRevision: number;
} | null>(null);

export function useWorkspace() {
  const workspace = useContext(WorkspaceContext);
  if (!workspace)
    throw new Error("Dashboard content must be inside WorkspaceShell");
  return workspace;
}

export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const section =
    navigation.find(
      (n) => n.href.replace(/\/$/, "") === pathname.replace(/\/$/, ""),
    )?.id ?? "overview";
  const [user, setUser] = useState<User>();
  const [authError, setAuthError] = useState("");
  const [mobile, setMobile] = useState(false);
  const [searchRevision, setSearchRevision] = useState(0);
  const action = useAction();
  useEffect(() => {
    const controller = new AbortController();
    request<{ user: User; csrf_token: string }>("/auth/me", {
      signal: controller.signal,
    })
      .then((r) => {
        if (controller.signal.aborted) return;
        setCsrf(r.csrf_token);
        setUser(r.user);
      })
      .catch((e) => {
        if (controller.signal.aborted) return;
        if (e.status === 401) router.replace("/login/");
        else setAuthError(e.message);
      });
    return () => controller.abort();
  }, [router]);
  const projects = useResource<Items<Project>>(user ? "/projects" : null);
  const settings = useResource<Settings>(user ? "/settings" : null, 30000);
  const health = useResource<Items<Health>>(
    user?.role === "admin" ? "/storage/health" : null,
  );
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
  const allProjects = projects.data?.items ?? [];
  const totalQuota = allProjects.reduce((sum, p) => sum + p.quota_bytes, 0);
  const totalUsed = allProjects.reduce((sum, p) => sum + p.used_bytes, 0);
  return (
    <WorkspaceContext.Provider
      value={{ user, projects, settings, health, searchRevision }}
    >
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
            <a
              href="/api-docs/"
              target="_blank"
              rel="noopener noreferrer"
              className="nav-item"
            >
              <BookOpen size={19} strokeWidth={1.8} />
              <span>API Docs</span>
              <ExternalLink size={13} />
            </a>
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
                onSearchSubmitted={() => setSearchRevision((v) => v + 1)}
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
            <ErrorNotice message={action.error} />
            {children}
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
      </div>
    </WorkspaceContext.Provider>
  );
}
