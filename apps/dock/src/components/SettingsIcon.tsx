import type { CSSProperties, ReactNode } from "react";
import { tokens } from "@openmind/ui";

export function SettingsIcon({
  size = 18,
  color = tokens.color.inkFaint,
}: {
  size?: number;
  color?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M12 15.2a3.2 3.2 0 1 0 0-6.4 3.2 3.2 0 0 0 0 6.4Z"
        stroke={color}
        strokeWidth="1.6"
      />
      <path
        d="M19.4 13.5a7.4 7.4 0 0 0 .1-3l1.7-1.1-1.6-2.8-2 .5a7.5 7.5 0 0 0-2.6-1.5l-.3-2h-3.2l-.3 2a7.5 7.5 0 0 0-2.6 1.5l-2-.5-1.6 2.8 1.7 1.1a7.4 7.4 0 0 0 .1 3l-1.7 1.1 1.6 2.8 2-.5a7.5 7.5 0 0 0 2.6 1.5l.3 2h3.2l.3-2a7.5 7.5 0 0 0 2.6-1.5l2 .5 1.6-2.8-1.7-1.1Z"
        stroke={color}
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function IconButton({
  label,
  onClick,
  children,
  style,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
  style?: CSSProperties;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      style={{
        border: `1px solid ${tokens.color.hairline}`,
        background: tokens.color.cardSurface,
        borderRadius: 8,
        width: 32,
        height: 32,
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        cursor: "pointer",
        padding: 0,
        flex: "none",
        ...style,
      }}
    >
      {children}
    </button>
  );
}
