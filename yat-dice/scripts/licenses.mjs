// Uygulamayla dağıtılan açık kaynak paketlerin lisans metinlerini public/licenses.txt dosyasına yazar.
// "npm run build" sırasında otomatik çalışır.
import { readFileSync, readdirSync, writeFileSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const lock = JSON.parse(readFileSync(join(root, "package-lock.json"), "utf8"));

const packages = Object.entries(lock.packages)
  .filter(([path, info]) => path.startsWith("node_modules/") && !info.dev && !info.devOptional)
  .map(([path, info]) => ({ dir: join(root, path), name: path.replace(/^.*node_modules\//, ""), version: info.version, license: info.license || "Bilinmiyor" }))
  .sort((a, b) => a.name.localeCompare(b.name));

function licenseText(dir) {
  if (!existsSync(dir)) return null;
  const file = readdirSync(dir).find(f => /^(licen[cs]e|copying)(\.(md|txt))?$/i.test(f));
  return file ? readFileSync(join(dir, file), "utf8").trim() : null;
}

const missing = [];
const sections = packages.map(pkg => {
  const text = licenseText(pkg.dir);
  if (!text) missing.push(pkg.name);
  return `${pkg.name} ${pkg.version} (${pkg.license})\n${"-".repeat(60)}\n${text || `Lisans: ${pkg.license}`}`;
});

const header = `Yat Dice — Açık Kaynak Lisansları

Bu uygulama aşağıdaki açık kaynak yazılımları içerir. Her paketin telif hakkı sahiplerine teşekkür ederiz.

@capgo/capacitor-social-login Mozilla Public License 2.0 ile lisanslıdır. Kaynak kodu:
https://github.com/Cap-go/capacitor-social-login

Android sürümü ayrıca Apache License 2.0 ile lisanslı AndroidX, Google Play Services ve
Credential Manager kütüphanelerini içerir: https://www.apache.org/licenses/LICENSE-2.0
`;

writeFileSync(join(root, "public", "licenses.txt"), [header, ...sections].join("\n\n" + "=".repeat(60) + "\n\n") + "\n");
console.log(`licenses.txt: ${packages.length} paket` + (missing.length ? `, lisans dosyası bulunamayan: ${missing.join(", ")}` : ""));
