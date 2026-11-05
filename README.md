# 🎲 Yat Dice — Yahtzee | Gerçek Zamanlı Çevrimiçi Zar Oyunu

**Yat Dice**, **Go (WebSocket)**, **React (TypeScript + Motion)** ve **PostgreSQL** kullanılarak geliştirilmiş, sunucu otoriteli (server-authoritative), gerçek zamanlı çevrimiçi ve yapay zeka destekli bir Yatzy (5 zar) strateji oyunudur.

![Go](https://img.shields.io/badge/Backend-Go_1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![React](https://img.shields.io/badge/Frontend-React_19_+_TypeScript-61DAFB?style=for-the-badge&logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/Database-PostgreSQL_15-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![WebSocket](https://img.shields.io/badge/Realtime-WebSocket-010101?style=for-the-badge)

---

## ✨ Öne Çıkan Özellikler

### 🎮 Oyun Modları, Bahis Sistemi ve Eşleşme
- **⚡ Hızlı Maç (Quick Match):** Çevrimiçi gerçek oyuncularla anında 1v1 eşleşme.
- **🤖 Yapay Zeka Bot Modu (`YatBot`):** Çevrimiçi rakip beklemeden, akıllı zar tutma/kategori seçme stratejisine ve canlı sohbet yanıtlarına sahip bota karşı anında oynama.
- **💰 Masa Bahsi (Wager / Ödül Havuzu):** Maça girerken `Ücretsiz`, `50`, `100`, `250` veya `500 🪙` bahis seçebilme; kazanan oyuncu `2x` ödül havuzunun tamamını alır!
- **🔑 Özel Oda Kurma & Kodla Katılma:** 5 haneli özel oda kodu oluşturup arkadaşlarınla özel maç yapma.
- **👥 Arkadaşlık, Çevrimiçi Bildirimi & Canlı Oyun Daveti:** Kullanıcı adıyla arkadaş ekleme, arkadaşların **🟢 Çevrimiçi** veya **🎮 Oyunda** durumlarını gerçek zamanlı görme, arkadaş oyuna girdiğinde anlık üst bildirim alma ve tek tıkla maça davet etme.
- **💬 Canlı Maç Sohbeti & Anti-Spam Koruması:** Maç sırasında hazır mesajlar veya serbest metin ile gerçek zamanlı sohbet etme; hem emoji hem de sohbet mesajları için sunucu ve istemci tabanlı **Anti-Spam bekleme süresi (Cooldown)** ve içerik filtreleme.

### 🔐 Kimlik Doğrulama & Kurumsal Güvenlik Önlemleri
- **🌐 Google Hesabıyla Kayıt & Giriş (`/auth/google`):** Tek tıkla Google e-posta hesabıyla kayıt olma veya mevcut hesaba bağlanma.
- **🎲 Cihazda Kalıcı Misafir Hesabı (`/auth/guest`):** Misafir olarak giriş yapan oyuncular uygulamadan çıkıp tekrar girse bile cihaz kimliği (`deviceId`) sayesinde aynı misafir hesabında ve ilerlemesinde kalır.
- **🔑 Şifre Sıfırlama (`/auth/forgot-password` & `/auth/reset-password`):** 6 haneli tek kullanımlık doğrulama kodu ve sabit zamanlı (`ConstantTimeCompare`) karşılaştırma ile güvenli şifre kurtarma.
- **🛡️ Rate Limiting, Brute-Force Koruması & Güvenlik Başlıkları:** IP bazlı hız sınırlandırma, ardışık hatalı girişlerde 5 dakika geçici hesap kilitleme, `MaxBytesReader` (256KB payload sınırı), `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` ve sunucu otoriteli (`Server-Authoritative`) oyun motoru.

### 🏆 Görev Modu (Quests), Koleksiyon ve Profil Özelleştirme
- **🎯 Görev Modu (Normal & Zor Görevler):**
  - *Normal Görevler:* İlk maçı tamamlama, Üst Toplamdan +35 Bonus alma, ilk YAT atma, 3 galibiyet serisi (XP ve Altın ödüllü).
  - *Zor & Efsanevi Görevler:* Tek maçta 220+ skor, 3 kez Üst Toplam Bonusu, **Tek Maçta 2 YAT** ve **Tek Maçta 3 YAT** görevleri — yüksek XP ve Altın yanında **Sürpriz Kasa (`📦`)** kazandırır ve anında kasa açma ekranını tetikler!
- **🖼️ Profil Resmi & Ülke Bayrağı:** Cihazdan fotoğraf yükleme (otomatik `128x128` kırpma/boyutlandırma), 12 hazır avatar ikonu ve ülke/bayrak seçimi (`🇹🇷`, `🇦🇿`, `🇩🇪`, `🇬🇧`, `🇺🇸` vb.).
- **🔍 3D Büyük Zar Önizleme & Koleksiyon Marketi:** Koleksiyondan zar alırken zara tıklandığında açılan büyük 3D etkileşimli önizleme sahnesi (*zarı test ederek döndürme* ve *"Satın Al / Alma (Vazgeç)"* onayı). **Kemik, Ahşap, Mermer, Obsidyen, Kızıl Yakut ve Altın Kraliyet** zar setleri.

---

## 🗂️ Proje Dizin Yapısı

```text
YatDice/
├── docker-compose.yml       # PostgreSQL 15 veritabanı konteyner yapılandırması
├── .env.example             # Örnek ortam değişkenleri şablonu
├── .gitignore               # Git güvenlik ve derleme dosyası filtreleri
├── yat-dice-backend/        # Go Backend & WebSocket Oyun Sunucusu
│   ├── main.go              # Kimlik doğrulama, REST API, WebSocket, Oyun & Bot Motoru
│   ├── go.mod
│   └── go.sum
└── yat-dice/                # React + TypeScript + Vite Frontend
    ├── src/
    │   ├── App.tsx          # Oyun ekranları, 3D zar bileşenleri, WebSocket istemcisi
    │   └── main.tsx
    ├── index.html
    └── package.json
```

---

## 🚀 Kurulum ve Çalıştırma Rehberi

Projeyi kendi bilgisayarınızda çalıştırmak için **Docker Desktop**, **Go (1.22+)** ve **Node.js (18+)** kurulu olmalıdır.

### 1. Veritabanını Başlatın (PostgreSQL)
Proje ana dizininde (`YatDice/`) terminali açın ve Docker ile PostgreSQL'i ayağa kaldırın:

```bash
docker-compose up -d
```

### 2. Go Backend Sunucusunu Başlatın
Yeni bir terminal açıp backend klasörüne gidin ve sunucuyu başlatın (tablolar ilk çalışmada otomatik oluşturulur):

```bash
cd yat-dice-backend
go run main.go
```
> Çıktıda `✅ Veritabanı bağlantısı başarılı ve tablolar hazır!` ve `🚀 Sunucu 8080 portunda çalışıyor...` mesajlarını görmelisiniz.

### 3. React Frontend Uygulamasını Başlatın
Üçüncü bir terminal açıp frontend klasörüne gidin, bağımlılıkları yükleyin ve geliştirme sunucusunu başlatın:

```bash
cd yat-dice
npm install
npm run dev
```
Tarayıcınızdan terminalde belirtilen adrese (genellikle `http://localhost:5173`) giderek oyuna başlayabilirsiniz!

---

## ⚙️ Ortam Değişkenleri (`.env`)

Yerel geliştirmede hiçbir ek ayar yapmadan varsayılan değerlerle çalıştırabilirsiniz. Üretim (Production) ortamına çıkarken kök dizindeki [`.env.example`](.env.example) dosyasını `.env` olarak kopyalayıp kendi değerlerinizi tanımlayabilirsiniz:

| Değişken | Açıklama | Varsayılan (Yerel) |
| :--- | :--- | :--- |
| `PORT` | Go backend sunucu portu | `8080` |
| `DATABASE_URL` | PostgreSQL bağlantı cümlesi | `postgres://yatzy_user:yatzy_password@localhost:5432/yatzy_gamedb?sslmode=disable` |
| `JWT_SECRET` | JWT token imzalama gizli anahtarı | `dev-secret-change-in-production` |
| `ALLOWED_ORIGIN` | CORS ve WebSocket izin verilen origin (`*` veya domain) | `*` |
| `VITE_API_URL` | Frontend'in bağlanacağı HTTP API adresi | `http://localhost:8080` |
| `VITE_WS_URL` | Frontend'in bağlanacağı WebSocket adresi | `ws://localhost:8080` |

---

## 🔌 API ve WebSocket Özeti

### REST API Uç Noktaları
- `POST /register` & `POST /login` — Kullanıcı kaydı ve JWT giriş işlemi
- `GET /profile` & `POST /profile/update` — Kullanıcı profili ve seçili zar görünümü
- `POST /profile/claim-daily` — Sunucu doğrulamalı günlük ödül talebi
- `POST /profile/open-case` — Sunucu tarafında kasa açma ve ödül üretimi
- `POST /shop/buy` — Sunucu doğrulamalı zar görünümü satın alma
- `GET /friends` — Arkadaş listesi, bekleyen istekler ve aktif oyun davetleri
- `POST /friends/request` — Kullanıcı adıyla arkadaşlık isteği gönderme
- `POST /friends/respond` — Arkadaşlık isteğini kabul/reddetme veya silme
- `POST /friends/invite` — Arkadaşı özel odaya davet etme

### WebSocket (`ws://localhost:8080/play`) Eylemleri
- **Eşleşme Komutları:** `QUICK_MATCH`, `BOT_MATCH`, `CREATE_ROOM`, `JOIN_ROOM`
- **Oyun İçi Komutlar:** `ROLL` (Zar at), `TOGGLE_LOCK` (Zar kilitle/aç), `SELECT_SCORE` (Kategori seç)
- **Sunucu Yayını (`GAME_STATE`):** Her hamlede güncel zarlar, kilitler, iki oyuncunun skor tabloları, aktif tur ve kalan süre tüm odaya senkronize edilir.

---

## 📄 Lisans

Bu proje eğitim ve portfolyo amaçlı geliştirilmiştir. Dilediğiniz gibi inceleyebilir ve katkıda bulunabilirsiniz.
