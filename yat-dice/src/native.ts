import { App } from "@capacitor/app";
import { Browser } from "@capacitor/browser";
import { Capacitor } from "@capacitor/core";
import { Haptics, ImpactStyle } from "@capacitor/haptics";
import { StatusBar, Style } from "@capacitor/status-bar";
import { SocialLogin } from "@capgo/capacitor-social-login";
import { t } from "./i18n";

export const IS_NATIVE = Capacitor.isNativePlatform();
export const PLATFORM = Capacitor.getPlatform();

const GOOGLE_WEB_CLIENT_ID: string = import.meta.env.VITE_GOOGLE_CLIENT_ID || "";
const GOOGLE_IOS_CLIENT_ID: string = import.meta.env.VITE_GOOGLE_IOS_CLIENT_ID || "";

export const GOOGLE_AVAILABLE = PLATFORM === "ios" ? !!GOOGLE_IOS_CLIENT_ID : !!GOOGLE_WEB_CLIENT_ID;
export const APPLE_AVAILABLE = PLATFORM === "ios";

let socialLoginReady: Promise<void> | null = null;

function initSocialLogin(): Promise<void> {
  if (!socialLoginReady) {
    socialLoginReady = SocialLogin.initialize({
      google: { webClientId: GOOGLE_WEB_CLIENT_ID, iOSClientId: GOOGLE_IOS_CLIENT_ID, iOSServerClientId: GOOGLE_WEB_CLIENT_ID },
      apple: { useProperTokenExchange: true },
    });
  }
  return socialLoginReady;
}

export async function nativeGoogleLogin(): Promise<string> {
  await initSocialLogin();
  const { result } = await SocialLogin.login({ provider: "google", options: { scopes: ["email", "profile"] } });
  const idToken = "idToken" in result ? result.idToken : null;
  if (!idToken) throw new Error(t("Google girişi tamamlanamadı"));
  return idToken;
}

export async function nativeAppleLogin(): Promise<{ credential: string; authorizationCode?: string; nonce: string }> {
  await initSocialLogin();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  const nonce = Array.from(bytes, value => value.toString(16).padStart(2, "0")).join("");
  const { result } = await SocialLogin.login({ provider: "apple", options: { scopes: ["email"], nonce } });
  if (!result.idToken) throw new Error(t("Apple girişi tamamlanamadı"));
  return { credential: result.idToken, authorizationCode: result.authorizationCode, nonce };
}

export function openExternal(url: string): void {
  if (IS_NATIVE) {
    void Browser.open({ url });
  } else {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

export function hapticsEnabled(): boolean {
  try { return localStorage.getItem("yatzy_haptics") !== "off"; } catch { return true; }
}

export function setHapticsEnabled(on: boolean): void {
  try { localStorage.setItem("yatzy_haptics", on ? "on" : "off"); } catch { /* yoksay */ }
}

export function haptic(style: "light" | "medium" = "light"): void {
  if (!IS_NATIVE || !hapticsEnabled()) return;
  void Haptics.impact({ style: style === "medium" ? ImpactStyle.Medium : ImpactStyle.Light }).catch(() => {});
}

/** Telefonun durum çubuğunu açık veya koyu temaya uydurur. */
export function applyStatusBarTheme(dark: boolean, background: string): void {
  if (!IS_NATIVE) return;
  void StatusBar.setStyle({ style: dark ? Style.Dark : Style.Light }).catch(() => {});
  if (PLATFORM === "android") void StatusBar.setBackgroundColor({ color: background }).catch(() => {});
}

export function minimizeApp(): void {
  void App.minimizeApp();
}

// Android geri tuşu ve uygulamanın ön plana dönüşü
export function registerNativeHandlers(onBack: () => void, onResume: () => void): () => void {
  if (!IS_NATIVE) return () => {};
  const handles = [
    App.addListener("backButton", onBack),
    App.addListener("appStateChange", state => {
      if (state.isActive) onResume();
    }),
  ];
  return () => {
    handles.forEach(handle => void handle.then(listener => listener.remove()));
  };
}
