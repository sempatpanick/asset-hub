"use client";
import { ChevronLeft, ChevronRight, Search, X } from "lucide-react";
import { SearchSelect } from "./select";

export function TableSearch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="search-field">
      <Search size={17} />
      <input
        aria-label={label}
        placeholder={`${label}…`}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {value && (
        <button
          type="button"
          className="icon-button"
          aria-label={`Clear ${label.toLowerCase()}`}
          onClick={() => onChange("")}
        >
          <X size={15} />
        </button>
      )}
    </div>
  );
}

export function Pagination({
  offset,
  count,
  pageSize,
  onChange,
  onPageSizeChange,
  hasMore = count === pageSize,
}: {
  offset: number;
  count: number;
  pageSize: number;
  onChange: (n: number) => void;
  onPageSizeChange: (n: number) => void;
  hasMore?: boolean;
}) {
  return (
    <div className="pagination">
      <span>{count ? `${offset + 1}–${offset + count}` : "No results"}</span>
      <div className="pagination-controls">
        <span>Rows per page</span>
        <SearchSelect
          aria-label="Page size"
          value={String(pageSize)}
          onValueChange={(v) => onPageSizeChange(Number(v))}
        >
          {[10, 25, 50, 100].map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </SearchSelect>
        <button
          type="button"
          className="icon-button"
          disabled={!offset}
          onClick={() => onChange(Math.max(0, offset - pageSize))}
          aria-label="Previous page"
        >
          <ChevronLeft size={17} />
        </button>
        <button
          type="button"
          className="icon-button"
          disabled={!hasMore}
          onClick={() => onChange(offset + pageSize)}
          aria-label="Next page"
        >
          <ChevronRight size={17} />
        </button>
      </div>
    </div>
  );
}
