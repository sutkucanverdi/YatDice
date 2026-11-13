// App Store / Google Play incelemesi için iki demo hesap açar, birbirine arkadaş yapar ve bir mesaj bırakır.
// İnceleme ekibi iki cihazla (ya da iki simülatörle) online maç, arkadaş listesi ve mesajlaşmayı deneyebilir.
//
// Kullanım:  node scripts/create-demo-accounts.mjs https://api.alanadiniz.com
// Bilgiler review-demo-accounts.local dosyasına yazılır (git'e girmez). Şifreler ekrana basılmaz.
import { randomInt } from "node:crypto";
import { writeFileSync, existsSync } from "node:fs";

const api = (process.argv[2] || "").replace(/\/+$/, "");
if (!/^https?:\/\//.test(api)) {
  console.error("Sunucu adresini verin: node scripts/create-demo-accounts.mjs https://api.alanadiniz.com");
  process.exit(1);
}
const OUT = new URL("../review-demo-accounts.local", import.meta.url);
if (existsSync(OUT)) {
  console.error("review-demo-accounts.local zaten var; mevcut hesapları korumak için durdu. Yeniden üretmek için dosyayı silin.");
  process.exit(1);
}

// Elle yazması kolay olsun diye karışan karakterler (0/O, 1/l/I) yok
const ALPHABET = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
const password = () => Array.from({ length: 14 }, () => ALPHABET[randomInt(ALPHABET.length)]).join("");

async function call(path, body, token) {
  const res = await fetch(api + path, {
    method: body ? "POST" : "GET",
    headers: { "Content-Type": "application/json", "Accept-Language": "en", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  if (!res.ok) throw new Error(`${path}: ${res.status} ${text.trim()}`);
  return text ? JSON.parse(text) : {};
}

async function createAccount(name) {
  const pass = password();
  const { username } = await call("/register", { username: name, password: pass, email: "", acceptedTerms: true });
  const { token } = await call("/login", { username, password: pass });
  return { username, password: pass, token };
}

const a = await createAccount("ReviewerOne");
const b = await createAccount("ReviewerTwo");

await call("/friends/request", { username: b.username }, a.token);
const { incoming = [], friends: bFriends = [] } = await call("/friends", null, b.token);
const request = incoming.find(item => item.username === a.username);
if (request) await call("/friends/respond", { friendshipId: request.friendshipId, action: "accept" }, b.token);

const { friends = [] } = await call("/friends", null, a.token);
const friend = friends.find(item => item.username === b.username) || bFriends.find(item => item.username === a.username);
if (!friend) throw new Error("Hesaplar arkadaş yapılamadı");
await call("/messages", { to: friend.userId, text: "Hi! This is a demo friend message." }, a.token);

writeFileSync(OUT, [
  `Sunucu: ${api}`,
  "",
  "Demo hesap 1",
  `  Kullanıcı adı: ${a.username}`,
  `  Şifre: ${a.password}`,
  "",
  "Demo hesap 2",
  `  Kullanıcı adı: ${b.username}`,
  `  Şifre: ${b.password}`,
  "",
  "İki hesap birbirinin arkadaşı; 1. hesaptan 2. hesaba bir mesaj gönderildi.",
  "",
].join("\n"), { encoding: "utf8" });

console.log(`Demo hesaplar hazır: ${a.username}, ${b.username}`);
console.log("Şifreler review-demo-accounts.local dosyasında.");
