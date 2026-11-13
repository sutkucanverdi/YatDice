// Basit iki dilli metin sistemi. Türkçe metnin kendisi anahtardır; İngilizce karşılığı en.ts içindedir.
// Çevirisi olmayan metin Türkçe gösterilir, böylece eksik bir çeviri arayüzü bozmaz.
import { EN } from "./locales/en";

export type Lang = "tr" | "en";

const STORAGE_KEY = "yatzy_lang";

function detectLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "tr" || saved === "en") return saved;
  } catch { /* depolama kapalı olabilir */ }
  const device = (typeof navigator !== "undefined" && (navigator.languages?.[0] || navigator.language)) || "tr";
  return device.toLowerCase().startsWith("tr") ? "tr" : "en";
}

let current: Lang = detectLang();
applyDocumentLang(current);

function applyDocumentLang(lang: Lang) {
  // CSS büyük harf dönüşümü (i/İ) dile göre doğru çalışsın
  if (typeof document === "undefined") return;
  document.documentElement.lang = lang;
  document.title = lang === "en" ? "Yat Dice — 3D Online Dice & Strategy Game" : "Yat Dice — 3D Çevrimiçi Zar & Strateji Oyunu";
}

export function getLang(): Lang {
  return current;
}

export function setLang(lang: Lang) {
  current = lang;
  applyDocumentLang(lang);
  try { localStorage.setItem(STORAGE_KEY, lang); } catch { /* yoksay */ }
}

/** Metni seçili dile çevirir. {0}, {1} yer tutucuları sırasıyla args ile doldurulur. */
export function t(text: string, ...args: (string | number)[]): string {
  const base = current === "en" ? (EN[text] ?? text) : text;
  return args.length ? base.replace(/\{(\d+)\}/g, (m, i) => (args[+i] !== undefined ? String(args[+i]) : m)) : base;
}

/** Sayıları seçili dilin biçiminde yazar (1.000 / 1,000). */
export function formatNumber(n: number): string {
  return n.toLocaleString(current === "en" ? "en-US" : "tr-TR");
}

/** Backend isteklerinde dil başlığı. */
export function langHeader(): Record<string, string> {
  return { "Accept-Language": current };
}
