import { Instrument_Sans, JetBrains_Mono, Newsreader } from "next/font/google";

// Newsreader — serif titles, quotes, editorial voice; italics used heavily.
export const newsreader = Newsreader({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  style: ["normal", "italic"],
  display: "swap",
  variable: "--font-newsreader",
});

// Instrument Sans — UI text, buttons, body.
export const instrumentSans = Instrument_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  display: "swap",
  variable: "--font-instrument-sans",
});

// JetBrains Mono — all metadata, tags, chrome labels.
export const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  display: "swap",
  variable: "--font-jetbrains-mono",
});

// Space-joined class list that exposes all three CSS variables on <body>.
export const fontVariables = [
  newsreader.variable,
  instrumentSans.variable,
  jetbrainsMono.variable,
].join(" ");
