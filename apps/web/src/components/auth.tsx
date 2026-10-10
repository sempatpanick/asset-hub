"use client";
import { useMemo, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import { Layers3, ArrowRight, LockKeyhole, ShieldCheck } from "lucide-react";
import { mutate, setCsrf } from "@/infrastructure/api/client";
import { useAction } from "@/features/hooks";
import { ErrorNotice } from "./ui";
function createInvitationStore(enabled: boolean) {
  let token = "";
  return {
    subscribe: (notify: () => void) => {
      if (enabled) {
        const value = new URLSearchParams(window.location.hash.slice(1)).get(
          "token",
        );
        if (value) {
          token = value;
          // Keep the secret in component memory after removing it from the URL.
          window.history.replaceState(null, "", window.location.pathname);
          notify();
        }
      }
      return () => {};
    },
    getSnapshot: () => token,
    getServerSnapshot: () => "",
  };
}
export default function Auth({ invite = false }: { invite?: boolean }) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const invitation = useMemo(() => createInvitationStore(invite), [invite]);
  const token = useSyncExternalStore(
    invitation.subscribe,
    invitation.getSnapshot,
    invitation.getServerSnapshot,
  );
  const [accepted, setAccepted] = useState(false);
  const action = useAction();
  return (
    <div className="auth-page">
      <div className="auth-brand">
        <span className="brand-mark">
          <Layers3 size={23} />
        </span>
        AssetHub
      </div>
      <div className="auth-layout">
        <section className="auth-intro">
          <span className="eyebrow">YOUR ASSETS. ONE HOME.</span>
          <h1>
            Everything you create,
            <br />
            beautifully organized.
          </h1>
          <p>
            A private workspace for the files that power your applications.
            Connected, organized, and under your control.
          </p>
          <div className="auth-visual">
            <div className="visual-file violet">
              IMG<span>Images</span>
            </div>
            <div className="visual-file blue">
              DOC<span>Documents</span>
            </div>
            <div className="visual-file peach">
              VID<span>Video</span>
            </div>
          </div>
          <div className="auth-note">
            <ShieldCheck size={19} />
            Self-hosted. Project-scoped. Built for your team.
          </div>
        </section>
        <section className="auth-card">
          <div className="lock-symbol">
            <LockKeyhole size={23} />
          </div>
          <h2>
            {accepted
              ? "You’re all set"
              : invite
                ? "Join your workspace"
                : "Welcome back"}
          </h2>
          <p>
            {accepted
              ? "Your account is ready. Sign in to get started."
              : invite
                ? "Set up your account using your invitation."
                : "Sign in to your AssetHub workspace."}
          </p>
          {accepted ? (
            <a className="button primary full" href="/login/">
              Go to sign in
              <ArrowRight size={17} />
            </a>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void action.run(async () => {
                  if (invite) {
                    await mutate("/auth/accept-invitation", {
                      token,
                      name,
                      password,
                    });
                    setAccepted(true);
                  } else {
                    const result = await mutate<{ csrf_token: string }>(
                      "/auth/login",
                      { email, password },
                    );
                    setCsrf(result.csrf_token);
                    router.replace("/");
                  }
                });
              }}
            >
              <ErrorNotice message={action.error} />
              {invite ? (
                <label>
                  Display name
                  <input
                    autoComplete="name"
                    required
                    maxLength={180}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Your name"
                  />
                </label>
              ) : (
                <label>
                  Email address
                  <input
                    type="email"
                    autoComplete="username"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@company.com"
                  />
                </label>
              )}
              <label>
                Password
                <input
                  type="password"
                  autoComplete={invite ? "new-password" : "current-password"}
                  minLength={invite ? 12 : undefined}
                  maxLength={256}
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={
                    invite ? "At least 12 characters" : "Enter your password"
                  }
                />
              </label>
              <button
                className="button primary full"
                disabled={action.busy || (invite && !token)}
              >
                {action.busy
                  ? "Please wait…"
                  : invite
                    ? "Create account"
                    : "Sign in"}
                <ArrowRight size={17} />
              </button>
              {invite && !token && (
                <ErrorNotice message="This invitation link has no token. Open the full link from your administrator." />
              )}
            </form>
          )}
          <div className="auth-footer">
            <LockKeyhole size={13} />
            {invite
              ? "Invitation-only access"
              : "Need access? Contact your workspace administrator."}
          </div>
        </section>
      </div>
      <footer className="page-footer">
        AssetHub · A home for every asset.
      </footer>
    </div>
  );
}
