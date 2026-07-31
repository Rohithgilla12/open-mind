import type { ReactNode } from "react";
import { createElement, Fragment } from "react";

/**
 * Flatten enrichment/feed markdown to plain text for contexts that cannot
 * render markup (aria labels, plain Text, clipboard). Same recipe as
 * apps/mobile/lib/text.ts — keep the two in sync.
 */
export function stripMarkdown(text: string | undefined): string {
  if (!text) return "";
  return text
    .replace(/```[\s\S]*?```/g, (m) => m.replace(/```/g, ""))
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/(\*\*\*|___)([^*_]+)\1/g, "$2")
    .replace(/(\*\*|__)([^*_]+)\1/g, "$2")
    .replace(/(?<![\w*])\*([^*\n]+)\*(?!\w)/g, "$1")
    .replace(/(?<![\w_])_([^_\n]+)_(?!\w)/g, "$1")
    .replace(/^>\s?/gm, "")
    .replace(/^[-*+]\s+/gm, "")
    .trim();
}

/** Drop structural markdown but leave emphasis markers for inline rendering. */
function prepareForInline(text: string): string {
  return text
    .replace(/```[\s\S]*?```/g, (m) => m.replace(/```/g, ""))
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^>\s?/gm, "")
    .replace(/^[-*+]\s+/gm, "")
    .trim();
}

const EMPHASIS =
  /(\*\*\*[^*]+?\*\*\*|\*\*[^*]+?\*\*|___[^_]+?___|__[^_]+?__|(?<![\w*])\*[^*\n]+?\*(?!\w)|(?<![\w_])_[^_\n]+?_(?!\w))/g;

/**
 * Render a short enrichment/feed summary with bold/italic emphasis.
 * Structural markdown (links, headings, lists) is flattened to text; no HTML
 * is ever interpreted — safe for untrusted AI/feed content.
 */
export function renderInlineMarkdown(text: string | undefined): ReactNode {
  if (!text) return null;
  const src = prepareForInline(text);
  if (!src) return null;

  const nodes: ReactNode[] = [];
  let last = 0;
  let match: RegExpExecArray | null;
  EMPHASIS.lastIndex = 0;
  while ((match = EMPHASIS.exec(src)) !== null) {
    if (match.index > last) {
      nodes.push(src.slice(last, match.index));
    }
    const token = match[0];
    let inner: string;
    let el: "strong" | "em" | "both";
    if (token.startsWith("***") || token.startsWith("___")) {
      inner = token.slice(3, -3);
      el = "both";
    } else if (token.startsWith("**") || token.startsWith("__")) {
      inner = token.slice(2, -2);
      el = "strong";
    } else {
      inner = token.slice(1, -1);
      el = "em";
    }
    const key = `em-${match.index}`;
    nodes.push(
      el === "both"
        ? createElement("strong", { key }, createElement("em", null, inner))
        : createElement(el, { key }, inner),
    );
    last = match.index + token.length;
  }
  if (last < src.length) {
    nodes.push(src.slice(last));
  }
  if (nodes.length === 0) return src;
  if (nodes.length === 1) return nodes[0];
  return createElement(Fragment, null, ...nodes);
}
