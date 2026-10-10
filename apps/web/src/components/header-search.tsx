"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";

export function HeaderSearch({
  onSearchSubmitted,
}: {
  onSearchSubmitted?: () => void;
}) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const shortcut = (e: KeyboardEvent) => {
      const target = e.target;
      if (
        e.key === "/" &&
        !e.ctrlKey &&
        !e.metaKey &&
        !e.altKey &&
        !(
          target instanceof HTMLElement &&
          (target.isContentEditable ||
            target.closest("input, textarea, select, [role=combobox]"))
        )
      ) {
        e.preventDefault();
        input.current?.focus();
      }
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, []);
  return (
    <form
      className="top-search"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearchSubmitted?.();
        router.push(`/files/?q=${encodeURIComponent(query.trim())}`);
      }}
    >
      <input
        ref={input}
        aria-label="Search workspace assets"
        placeholder="Find an asset…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      <button type="submit" className="icon-button" aria-label="Search assets">
        <Search size={16} />
      </button>
      <kbd>/</kbd>
    </form>
  );
}
