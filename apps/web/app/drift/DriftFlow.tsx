"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, useTransition } from "react";
import { Palette } from "../../components/Palette";
import { assetSrc } from "../../lib/assets";
import { cardKind, typeGradient } from "../../lib/cards";
import { derivedPalette } from "../../lib/palette";
import type { Item } from "../../lib/types";

const { color, font } = tokens;

// Drift is the ONE deliberately-dark screen (the rest of the app is warm paper),
// so its canvas is an intentional exception to tokens-only styling. Accent
// colours (gold, cobalt) still come from tokens; the dark values are literal.
const DRIFT_BG = "radial-gradient(circle at 50% 38%, #242019, #161410)";
const LIGHT = "#F4F0E6"; // paper-toned text on the dark canvas
const LIGHT_MUTED = "rgba(244,240,230,.68)";
const LIGHT_FAINT = "rgba(244,240,230,.42)";
const CARD_BG = "rgba(255,255,255,.045)";
const CARD_BORDER = "rgba(244,240,230,.12)";
const DOT_FAINT = "rgba(244,240,230,.22)";

/** "Saved N months ago" (approx), from an ISO createdAt. Coarse by design. */
function savedAge(createdAt: string): string {
  const then = new Date(createdAt).getTime();
  if (Number.isNaN(then)) return "Saved a while ago";
  const days = Math.floor((Date.now() - then) / 86_400_000);
  if (days <= 0) return "Saved today";
  if (days === 1) return "Saved yesterday";
  if (days < 30) return `Saved ${days} days ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `Saved ${months} month${months === 1 ? "" : "s"} ago`;
  const years = Math.floor(days / 365);
  return `Saved ${years} year${years === 1 ? "" : "s"} ago`;
}

export function DriftFlow({ items, total }: { items: Item[]; total: number }) {
  const router = useRouter();
  const [index, setIndex] = useState(0);
  const [kept, setKept] = useState(0);
  const [letGo, setLetGo] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const close = useCallback(() => router.push("/"), [router]);

  const done = index >= items.length;
  const current = done ? undefined : items[index];

  const act = useCallback(
    (keep: boolean) => {
      if (!current || pending) return;
      setError(null);
      const id = current.id;
      startTransition(async () => {
        try {
          const res = await fetch(`/api/drift/${id}`, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({ keep }),
          });
          if (!res.ok) {
            setError("Couldn't save that. Please try again.");
            return;
          }
          if (keep) setKept((n) => n + 1);
          else setLetGo((n) => n + 1);
          setIndex((i) => i + 1);
        } catch {
          setError("Couldn't save that. Please try again.");
        }
      });
    },
    [current, pending],
  );

  // Esc closes; ← lets go / → keeps (a nice-to-have keyboard path).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        close();
        return;
      }
      if (done || pending) return;
      if (e.key === "ArrowLeft") act(false);
      else if (e.key === "ArrowRight") act(true);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [act, close, done, pending]);

  return (
    <main
      style={{
        position: "fixed",
        inset: 0,
        background: DRIFT_BG,
        color: LIGHT,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        overflow: "auto",
        padding: "28px 24px 40px",
      }}
    >
      <CloseButton onClick={close} />

      {items.length === 0 ? (
        <EmptyState onBack={close} />
      ) : done ? (
        <Completion kept={kept} letGo={letGo} onBack={close} />
      ) : current ? (
        <>
          <Progress index={index} total={total} count={items.length} />
          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              width: "100%",
              maxWidth: 460,
              gap: 26,
            }}
          >
            <DriftCard item={current} />
            {error ? (
              <p
                aria-live="polite"
                style={{
                  fontFamily: font.mono,
                  fontSize: 11,
                  letterSpacing: ".04em",
                  color: color.danger,
                  margin: 0,
                }}
              >
                {error}
              </p>
            ) : null}
            <div style={{ display: "flex", gap: 12, width: "100%", maxWidth: 380 }}>
              <button
                type="button"
                onClick={() => act(false)}
                disabled={pending}
                style={{
                  flex: 1,
                  fontFamily: font.sans,
                  fontSize: 13,
                  fontWeight: 500,
                  color: LIGHT,
                  background: "transparent",
                  border: `1px solid ${CARD_BORDER}`,
                  borderRadius: 10,
                  padding: "12px 16px",
                  cursor: pending ? "default" : "pointer",
                  opacity: pending ? 0.55 : 1,
                }}
              >
                Let go
              </button>
              <button
                type="button"
                onClick={() => act(true)}
                disabled={pending}
                style={{
                  flex: 1,
                  fontFamily: font.sans,
                  fontSize: 13,
                  fontWeight: 600,
                  color: color.ink,
                  background: color.gold,
                  border: "none",
                  borderRadius: 10,
                  padding: "12px 16px",
                  cursor: pending ? "default" : "pointer",
                  opacity: pending ? 0.55 : 1,
                  boxShadow: "0 6px 18px -8px rgba(224,178,58,.7)",
                }}
              >
                Keep on my desk
              </button>
            </div>
            <p
              style={{
                fontFamily: font.mono,
                fontSize: 9.5,
                letterSpacing: ".08em",
                textTransform: "uppercase",
                color: LIGHT_FAINT,
                margin: 0,
              }}
            >
              ← let go · keep → · esc to close
            </p>
          </div>
        </>
      ) : null}
    </main>
  );
}

function Progress({ index, total, count }: { index: number; total: number; count: number }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 12 }}>
      <div
        style={{
          fontFamily: font.mono,
          fontSize: 11,
          letterSpacing: ".14em",
          textTransform: "uppercase",
          color: LIGHT_MUTED,
        }}
      >
        Drift · {index + 1} of {total} today
      </div>
      <div style={{ display: "flex", gap: 7 }}>
        {Array.from({ length: count }).map((_, i) => (
          <span
            key={i}
            style={{
              width: 6,
              height: 6,
              borderRadius: "50%",
              background: i <= index ? color.gold : DOT_FAINT,
            }}
          />
        ))}
      </div>
    </div>
  );
}

function DriftCard({ item }: { item: Item }) {
  const kind = cardKind(item.cardType);
  const gradient = typeGradient[kind];
  const img = assetSrc(item.leadImageUrl);
  const tags = item.tags ?? [];
  const dots =
    item.palette && item.palette.length > 0
      ? item.palette
      : derivedPalette(`${item.title ?? ""} ${tags.join(" ")}`.trim() || kind);

  return (
    <article
      className="drift-card"
      style={{
        width: "100%",
        background: CARD_BG,
        border: `1px solid ${CARD_BORDER}`,
        borderRadius: 16,
        overflow: "hidden",
        boxShadow: "0 40px 90px -30px rgba(0,0,0,.7)",
      }}
    >
      {/* Lead image over the type gradient (background-image, never <img>): a
          missing/broken image reveals the gradient with no broken-image glyph. */}
      <div style={{ position: "relative", height: 190, background: gradient }}>
        {img ? (
          <div
            role="img"
            aria-label={item.title ?? "saved image"}
            style={{
              position: "absolute",
              inset: 0,
              backgroundImage: `url(${img}), ${gradient}`,
              backgroundSize: "cover",
              backgroundPosition: "center",
              backgroundRepeat: "no-repeat",
            }}
          />
        ) : null}
      </div>
      <div style={{ padding: "20px 22px 22px" }}>
        {item.title ? (
          <h1
            style={{
              fontFamily: font.quote,
              fontStyle: "italic",
              fontSize: 22,
              fontWeight: 500,
              lineHeight: 1.25,
              letterSpacing: "-.01em",
              color: LIGHT,
              margin: 0,
            }}
          >
            {item.title}
          </h1>
        ) : null}
        {item.summary ? (
          <p
            style={{
              fontFamily: font.sans,
              fontSize: 13,
              lineHeight: 1.5,
              color: LIGHT_MUTED,
              margin: "10px 0 0",
              display: "-webkit-box",
              WebkitLineClamp: 4,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {item.summary}
          </p>
        ) : null}
        <div style={{ display: "flex", gap: 5, marginTop: 16 }}>
          <Palette colors={dots} />
        </div>
        <p
          style={{
            fontFamily: font.mono,
            fontSize: 10.5,
            letterSpacing: ".05em",
            color: LIGHT_MUTED,
            margin: "12px 0 0",
          }}
        >
          {savedAge(item.createdAt)} · never revisited
        </p>
      </div>
    </article>
  );
}

function Completion({ kept, letGo, onBack }: { kept: number; letGo: number; onBack: () => void }) {
  return (
    <CenteredMessage>
      <div style={{ fontSize: 46, lineHeight: 1, color: color.gold }} aria-hidden>
        ❍
      </div>
      <p
        style={{
          fontFamily: font.quote,
          fontStyle: "italic",
          fontSize: 24,
          color: LIGHT,
          margin: 0,
        }}
      >
        That&apos;s your drift for today.
      </p>
      <p
        style={{
          fontFamily: font.mono,
          fontSize: 11,
          letterSpacing: ".1em",
          textTransform: "uppercase",
          color: LIGHT_MUTED,
          margin: 0,
        }}
      >
        kept {kept} · let {letGo} go
      </p>
      <BackButton onClick={onBack} />
    </CenteredMessage>
  );
}

function EmptyState({ onBack }: { onBack: () => void }) {
  return (
    <CenteredMessage>
      <div style={{ fontSize: 46, lineHeight: 1, color: color.gold }} aria-hidden>
        ❍
      </div>
      <p
        style={{
          fontFamily: font.quote,
          fontStyle: "italic",
          fontSize: 22,
          color: LIGHT,
          margin: 0,
          textAlign: "center",
          maxWidth: "32ch",
        }}
      >
        Nothing to drift — your mind is all caught up.
      </p>
      <BackButton onClick={onBack} />
    </CenteredMessage>
  );
}

function CenteredMessage({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        flex: 1,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 18,
      }}
    >
      {children}
    </div>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        marginTop: 8,
        fontFamily: font.sans,
        fontSize: 13,
        fontWeight: 600,
        color: color.ink,
        background: color.gold,
        border: "none",
        borderRadius: 10,
        padding: "11px 20px",
        cursor: "pointer",
        boxShadow: "0 6px 18px -8px rgba(224,178,58,.7)",
      }}
    >
      Back to The Mind
    </button>
  );
}

function CloseButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label="Close Drift"
      style={{
        position: "fixed",
        top: 22,
        right: 24,
        width: 34,
        height: 34,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        fontSize: 18,
        lineHeight: 1,
        color: LIGHT_MUTED,
        background: "transparent",
        border: `1px solid ${CARD_BORDER}`,
        borderRadius: "50%",
        cursor: "pointer",
      }}
    >
      ✕
    </button>
  );
}
