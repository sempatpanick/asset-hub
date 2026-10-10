"use client";
import {
  Children,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { Check, ChevronDown, Search } from "lucide-react";

function optionsFrom(children: ReactNode): { value: string; label: string }[] {
  return Children.toArray(children).flatMap((child) => {
    if (!isValidElement<{ value?: string; children?: ReactNode }>(child))
      return [];
    if (child.type !== "option") return optionsFrom(child.props.children);
    return [
      {
        value: String(child.props.value ?? ""),
        label: String(child.props.children ?? ""),
      },
    ];
  });
}

// The popover lives in the browser's top layer, including inside modal dialogs.
export function SearchSelect({
  children,
  value,
  onValueChange,
  disabled,
  "aria-label": label,
}: {
  children: ReactNode;
  value: string;
  onValueChange: (value: string) => void;
  disabled?: boolean;
  "aria-label": string;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [limit, setLimit] = useState(30);
  const [active, setActive] = useState(0);
  const options = optionsFrom(children);
  const matches = options.filter((o) =>
    o.label.toLowerCase().includes(query.toLowerCase().trim()),
  );
  const visible = matches.slice(0, limit);
  const choose = (next: string) => {
    onValueChange(next);
    popup.current?.hidePopover();
    trigger.current?.focus();
  };
  useEffect(() => {
    if (!open) return;
    const position = () => {
      const rect = trigger.current?.getBoundingClientRect();
      const el = popup.current;
      if (!rect || !el) return;
      const width = Math.min(Math.max(rect.width, 260), window.innerWidth - 24);
      const height = Math.min(320, window.innerHeight - 24);
      el.style.width = `${width}px`;
      el.style.left = `${Math.max(12, Math.min(rect.left, window.innerWidth - width - 12))}px`;
      el.style.top = `${Math.max(12, rect.bottom + height > window.innerHeight ? rect.top - height - 6 : rect.bottom + 6)}px`;
      el.style.maxHeight = `${height}px`;
    };
    position();
    input.current?.focus();
    window.addEventListener("resize", position);
    window.addEventListener("scroll", position, true);
    return () => {
      window.removeEventListener("resize", position);
      window.removeEventListener("scroll", position, true);
    };
  }, [open]);
  useEffect(() => {
    if (open)
      document
        .getElementById(`${id}-option-${active}`)
        ?.scrollIntoView({ block: "nearest" });
  }, [active, id, open]);
  return (
    <div className="search-select">
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-label={label}
        aria-expanded={open}
        aria-controls={`${id}-list`}
        aria-haspopup="listbox"
        disabled={disabled}
        onClick={() => {
          setQuery("");
          setLimit(30);
          setActive(0);
          popup.current?.togglePopover();
        }}
        onKeyDown={(e) => {
          if (["ArrowDown", "ArrowUp"].includes(e.key)) {
            e.preventDefault();
            setQuery("");
            setLimit(30);
            setActive(0);
            popup.current?.showPopover();
          }
        }}
      >
        <span>
          {options.find((o) => o.value === value)?.label ?? "Choose an option"}
        </span>
        <ChevronDown size={15} />
      </button>
      <div
        ref={popup}
        popover="auto"
        className="select-popup"
        onToggle={(e) => {
          setOpen(e.newState === "open");
          if (e.newState === "open") list.current?.scrollTo(0, 0);
        }}
      >
        <div className="select-search">
          <Search size={16} />
          <input
            ref={input}
            aria-label={`Search ${label.toLowerCase()}`}
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-list`}
            aria-autocomplete="list"
            aria-activedescendant={
              visible[active] ? `${id}-option-${active}` : undefined
            }
            placeholder="Search options…"
            value={query}
            onChange={(e) => {
              list.current?.scrollTo(0, 0);
              setQuery(e.target.value);
              setLimit(30);
              setActive(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                const next = Math.max(
                  0,
                  Math.min(
                    matches.length - 1,
                    active + (e.key === "ArrowDown" ? 1 : -1),
                  ),
                );
                setActive(next);
                setLimit((n) => Math.max(n, next + 1));
              } else if (e.key === "Enter") {
                e.preventDefault();
                if (matches[active]) choose(matches[active].value);
              } else if (e.key === "Escape") {
                e.preventDefault();
                e.stopPropagation();
                popup.current?.hidePopover();
                trigger.current?.focus();
              } else if (e.key === "Tab") popup.current?.hidePopover();
            }}
          />
        </div>
        <div
          ref={list}
          id={`${id}-list`}
          role="listbox"
          aria-label={label}
          className="select-options"
          onScroll={(e) => {
            const el = e.currentTarget;
            if (el.scrollHeight - el.scrollTop - el.clientHeight < 60)
              setLimit((n) => Math.min(n + 30, matches.length));
          }}
        >
          {visible.map((o, i) => (
            <button
              type="button"
              role="option"
              aria-selected={value === o.value}
              tabIndex={-1}
              id={`${id}-option-${i}`}
              key={o.value}
              className={active === i ? "active" : ""}
              onClick={() => choose(o.value)}
            >
              <span>{o.label}</span>
              {value === o.value && <Check size={16} />}
            </button>
          ))}
          {!matches.length && (
            <p className="select-empty">No matching options</p>
          )}
        </div>
        {visible.length < matches.length && (
          <div className="select-more">
            Scroll for more · {visible.length} of {matches.length}
          </div>
        )}
      </div>
    </div>
  );
}
