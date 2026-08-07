"use client";

import { useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";
import { isUploadable, UPLOAD_ACCEPT, UPLOAD_LABEL, UPLOAD_PROMPT } from "../lib/uploads";

type FileStatus = { name: string; state: "uploading" | "done" | "error"; message?: string };

export function ImageDrop() {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [statuses, setStatuses] = useState<FileStatus[]>([]);
  const [, startTransition] = useTransition();

  async function uploadOne(file: File) {
    setStatuses((prev) => [...prev, { name: file.name, state: "uploading" }]);
    try {
      const fd = new FormData();
      fd.append("file", file);
      const res = await fetch("/api/assets", { method: "POST", body: fd });
      if (res.status === 201) {
        setStatuses((prev) =>
          prev.map((s) => (s.name === file.name && s.state === "uploading" ? { ...s, state: "done" } : s)),
        );
        startTransition(() => router.refresh());
        return;
      }
      throw new Error(`status ${res.status}`);
    } catch {
      setStatuses((prev) =>
        prev.map((s) =>
          s.name === file.name && s.state === "uploading"
            ? { ...s, state: "error", message: "upload failed" }
            : s,
        ),
      );
    }
  }

  function handleFiles(files: FileList | null) {
    if (!files) return;
    for (const file of Array.from(files)) {
      if (isUploadable(file)) void uploadOne(file);
    }
  }

  return (
    <div style={{ height: "100%" }}>
      <div
        role="button"
        tabIndex={0}
        aria-label={UPLOAD_LABEL}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            inputRef.current?.click();
          }
        }}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          handleFiles(e.dataTransfer.files);
        }}
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          minHeight: 46,
          padding: "11px 14px",
          fontFamily: tokens.font.sans,
          fontSize: 13,
          color: dragging ? tokens.color.cobalt : tokens.color.inkMuted,
          border: `1px dashed ${dragging ? tokens.color.cobalt : tokens.color.hairline}`,
          borderRadius: 10,
          backgroundColor: dragging ? "rgba(27,63,209,.05)" : tokens.color.cardSurface,
          cursor: "pointer",
          textAlign: "center",
          transition: ".15s",
        }}
      >
        {UPLOAD_PROMPT}
      </div>
      <input
        ref={inputRef}
        type="file"
        accept={UPLOAD_ACCEPT}
        multiple
        onChange={(e) => {
          handleFiles(e.target.files);
          e.target.value = "";
        }}
        style={{ display: "none" }}
      />
      {statuses.length > 0 ? (
        <ul style={{ listStyle: "none", margin: "0.5rem 0 0", padding: 0 }}>
          {statuses.map((s, i) => (
            <li
              key={`${s.name}-${i}`}
              style={{
                fontFamily: tokens.font.mono,
                fontSize: "0.72rem",
                color:
                  s.state === "error"
                    ? tokens.color.danger
                    : s.state === "done"
                      ? tokens.color.ink
                      : tokens.color.cobalt,
                opacity: s.state === "done" ? 0.6 : 1,
                margin: "2px 0",
              }}
            >
              {s.name} —{" "}
              {s.state === "uploading" ? "uploading…" : s.state === "done" ? "saved" : (s.message ?? "failed")}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
