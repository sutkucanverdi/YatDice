# 🎲 Yat Dice — 3D Gerçek Zamanlı Çevrimiçi Zar & Strateji Oyunu

**Yat Dice**, **Go (WebSocket + REST API)**, **React 19 (TypeScript + Motion)** ve **PostgreSQL 15** kullanılarak geliştirilmiş, sunucu otoriteli (*server-authoritative*), gerçek zamanlı çevrimiçi ve yapay zeka destekli bir 5 zar strateji oyunudur.

![Go](https://img.shields.io/badge/Backend-Go_1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![React](https://img.shields.io/badge/Frontend-React_19_+_TypeScript-61DAFB?style=for-the-badge&logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/Database-PostgreSQL_15-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![WebSocket](https://img.shields.io/badge/Realtime-WebSocket-010101?style=for-the-badge)

---

## ✨ Öne Çıkan Özellikler

### 🎮 Oyun Modları, Turnuva Ödül Havuzu ve Eşleşme
- **⚡ Hızlı Maç (Quick Match):** Çevrimiçi gerçek oyuncularla anında 1v1 eşleşme.
- **🤖 Yapay Zeka Bot Modu (`YatBot`):** Çevrimiçi rakip beklemeden, akıllı zar tutma/kategori seçme stratejisine ve canlı sohbet yanıtlarına sahip bota karşı anında oynama.
- **🔥 Ustalar Arenası & 👑 VIP Kraliyet Masası:** Maça girerken `Ücretsiz`, `50`, `100`, `250` veya `500 🪙` sanal masa giriş jetonu seçebilme; kazanan oyuncu `2x` sanal ödül havuzunu alır, beraberlik (`tie`) durumunda her iki oyuncunun giriş jetonları atomik olarak iade edilir.
- **🔑 Özel Oda Kurma & Kodla Katılma:** 5 haneli çakışmasız (*collision-free*) özel oda kodu oluşturup arkadaşlarınla özel maç yapma.
- **👥 Arkadaşlık, Çevrimiçi Bildirimi & Canlı Oyun Daveti:** Kullanıcı adıyla arkadaş ekleme, arkadaşların **🟢 Çevrimiçi** veya **🎮 Oyunda** durumlarını gerçek zamanlı görme, arkadaş oyuna girdiğinde anlık üst bildirim alma ve tek tıkla maça davet etme.
- **💬 Canlı Maç Sohbeti, Küfür Filtresi & Anti-Spam Koruması:** Maç sırasında hazır mesajlar veya serbest metin ile gerçek zamanlı sohbet etme; hem emoji hem de sohbet mesajları için sunucu ve istemci tabanlı **Anti-Spam bekleme süresi (Cooldown)**, otomatik küfür/argo filtreleme ve tek tıkla **🚫 Sustur / Engelle** veya **🚩 Raporla** kontrolleri.
- **🔊 Web Audio API Prosedürel Ses Motoru:** Harici ses dosyası (`mp3/wav`) gerektirmeden tarayıcı/mobil Web Audio sentezleyicisi ile zar atma, zar kilitleme, puan yazma, YAT fanfarı ve galibiyet ses efektleri (üst barda kalıcı `🔊 / 🔇` açma/kapatma desteğiyle).

### 🏆 Küresel Liderlik Tablosu, Görev Modu (Quests) ve 3D Koleksiyon
- **🏆 Küresel Liderlik Tablosu (`/leaderboard`):** En çok galibiyet, XP ve yüksek skor elde eden ilk 20 oyuncuyu altın/gümüş/bronz madalyaları, ülke bayrakları ve seviyeleriyle canlı görüntüleme.
- **📖 Oyun Kuralları & Puan Rehberi:** Üst bölüm `+35 Bonus` şartı, *Full House*, *Küçük/Büyük Sıra* ve *YAT (50 Puan)* kombinasyonlarını anlatan oyun içi interaktif rehber.
- **🎯 Görev Modu (Normal & Zor Görevler):**
  - *Normal Görevler:* İlk maçı tamamlama, Üst Toplamdan +35 Bonus alma, ilk YAT atma, 3 galibiyet serisi (XP ve Sanal Jeton ödüllü).
  - *Zor & Efsanevi Görevler:* Tek maçta 220+ skor, 3 kez Üst Toplam Bonusu, **Tek Maçta 2 YAT** ve **Tek Maçta 3 YAT** görevleri — yüksek XP ve Sanal Jeton yanında **Sürpriz Kasa (`📦`)** kazandırır ve anında kasa açma animasyonunu tetikler!
- **🖼️ Profil Resmi & Ülke Bayrağı:** Cihazdan fotoğraf yükleme (otomatik `128x128` kırpma ve sıkıştırma), 12 hazır avatar ikonu ve ülke/bayrak seçimi (`🇹🇷`, `🇦🇿`, `🇩🇪`, `🇬🇧`, `🇺🇸` vb.).
- **🔍 3D Büyük Zar Önizleme & Koleksiyon Marketi:** Koleksiyondan zar alırken zara tıklandığında açılan büyük 3D etkileşimli önizleme sahnesi (*zarı test ederek döndürme* ve *"Satın Al / Vazgeç"* onayı). **Kemik, Ahşap, Mermer, Obsidyen, Kızıl Yakut ve Altın Kraliyet** zar setleri.

### 🔐 Kimlik Doğrulama & Kurumsal Güvenlik Önlemleri
- **🌐 Google Hesabıyla Kayıt & Giriş (`/auth/google`):** Google Identity Services ile giriş; sunucu ID token'ı, istemci kimliğini ve doğrulanmış e-postayı denetler.
- **🎲 Cihazda Kalıcı Misafir Hesabı (`/auth/guest`):** Misafir olarak giriş yapan oyuncular uygulamadan çıkıp tekrar girse bile cihaz kimliği (`deviceId`) sayesinde aynı misafir hesabında ve ilerlemesinde kalır.
- **🔑 Şifre Sıfırlama (`/auth/forgot-password` & `/auth/reset-password`):** 6 haneli tek kullanımlık doğrulama kodu ve sabit zamanlı (`subtle.ConstantTimeCompare`) karşılaştırma ile güvenli şifre kurtarma.
- **🛡️ Rate Limiting, Brute-Force Koruması & Güvenlik Başlıkları:** IP bazlı Token-Bucket hız sınırlandırma, ardışık 5 hatalı girişte 5 dakika geçici hesap kilitleme, zamanlama saldırısı (`timing attack`) kalkanı, `MaxBytesReader` (256 KB payload sınırı), `http.Server` Slowloris zaman aşımları ve güvenlik başlıkları (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`).

---

## 📱 App Store & Google Play Store Uyumluluk (Compliance) Özellikleri

Aşağıdaki özellikler mağaza başvurusu için hazırlanmıştır. Başvurudan önce gerçek cihaz testleri, Google ve Apple giriş kimlikleri, imzalı build ve mağaza formları tamamlanmalıdır:

1. **Uygulama İçi Kalıcı Hesap ve Veri Silme (Apple Guideline 5.1.1(v) & Google Play Data Safety):**
   - Kullanıcılar **Anasayfa &rarr; Profili Özelleştir &rarr; 🗑️ Hesabımı ve Verilerimi Kalıcı Olarak Sil** adımıyla (`POST /profile/delete`) veya uygulamayı kaldırdıysa `/account/delete` web sayfasından hesabını silebilir.
2. **Kullanıcı İçeriği (UGC) Güvenliği (Apple Guideline 1.2 & Google Play UGC Policy):**
   - Kayıt ve sohbette otomatik küfür filtresi, sunucu tarafında çalışan engelleme (eşleşme, oda, davet, arkadaşlık ve sohbet), son sohbet kayıtlarıyla şikâyet ve `/admin` moderasyon paneli bulunur. Şikâyetler panelden düzenli olarak incelenmelidir.
3. **Kasa (`📦 Loot Box`) Olasılık Bildirimi (Apple Guideline 3.1.1):**
   - Kasa açma butonunun altında ve Yasal Beyan ekranında kazanma olasılıkları açıkça belirtilir: `%100 Sanal Jeton (50–99 🪙: %33.3 • 100–149 🪙: %33.3 • 150–199 🪙: %33.4)`.
4. **Sanal Jeton & Kumar Yasağı Beyanı (Apple Guideline 5.3 & Google Play Gambling Policy):**
   - Oyun içi tüm jetonlar (`🪙`) tamamen eğlence amaçlı sanal puandır; gerçek parayla satın alınamaz ve gerçek paraya/maddi değere dönüştürülemez (*No Real-Money Gambling / No Cash-Out*).
5. **Hazır Gizlilik Politikası ve EULA Web Sayfaları:**
   - Mağaza panellerinde (*App Store Connect* ve *Google Play Console*) doğrudan URL olarak verilebilen canlı uç noktalar:
     - **Privacy Policy URL:** `https://<sunucu-adresiniz>/privacy`
     - **Terms of Use (EULA) URL:** `https://<sunucu-adresiniz>/terms`
     - **Hesap silme URL'si:** `https://<sunucu-adresiniz>/account/delete`
   - Bu metinler taslaktır; yayın öncesi `LEGAL_*` değerlerini girin ve bir hukuk uzmanına kontrol ettirin.

---

## 🗂️ Proje Dizin Yapısı

```text
YatDice/
├── docker-compose.yml       # PostgreSQL 15 veritabanı konteyner yapılandırması
├── .env.example             # Örnek ortam değişkenleri şablonu
├── .gitignore               # Git güvenlik ve derleme dosyası filtreleri
├── yat-dice-backend/        # Go Backend & WebSocket Oyun Sunucusu
│   ├── main.go              # Auth, REST API, WebSocket, Oyun & Bot Motoru, UGC & Mağaza Uyumluluğu
│   ├── go.mod
│   └── go.sum
└── yat-dice/                # React 19 + TypeScript + Vite Frontend
    ├── public/
    │   ├── favicon.svg
    │   └── manifest.json    # Mobil PWA & Tam Ekran Uygulama Manifestosu
    ├── src/
    │   ├── App.tsx          # Tüm oyun ekranları, 3D zar motoru, Web Audio SFX, WebSocket istemcisi
    │   └── main.tsx
    ├── index.html
    └── package.json
```

---

## 🚀 Kurulum ve Çalıştırma Rehberi

Projeyi kendi bilgisayarınızda çalıştırmak için **Docker Desktop**, **Go (1.26.10+)** ve **Node.js (20.19+ veya 22.12+)** kurulu olmalıdır.

### 1. Veritabanını Başlatın (PostgreSQL)
Proje ana dizininde (`YatDice/`) terminali açın ve Docker ile PostgreSQL'i ayağa kaldırın:

```bash
docker compose up -d
# (Eski Docker Compose sürümlerinde: docker-compose up -d)
```

### 2. Go Backend Sunucusunu Başlatın
Proje ana dizininde örnek ortam dosyasını kopyalayın. `APP_ENV=development` tanımlı değilse sunucu üretim kurallarıyla çalışır ve güçlü `JWT_SECRET` ile `DATABASE_URL` olmadan açılmaz:

```bash
cp .env.example .env
```

Yeni bir terminal açıp backend klasörüne gidin ve paketin tamamını çalıştırarak sunucuyu başlatın (tablolar, şema güncellemeleri ve `.env` değişkenleri ilk çalışmada otomatik yüklenir):

```bash
cd yat-dice-backend
go run .
```
> **Not:** Paketteki tüm Go kaynak dosyalarının birlikte derlenmesi için `go run main.go` yerine her zaman **`go run .`** komutunu kullanın.
> Çıktıda `✅ PostgreSQL veritabanına başarıyla bağlanıldı!`, `📦 Veritabanı tabloları ve şema güncellemeleri hazırlandı!` ve `🚀 Yat Dice Sunucusu 8080 portunda aktif...` mesajlarını görmelisiniz.

### 3. React Frontend Uygulamasını Başlatın
Üçüncü bir terminal açıp frontend klasörüne gidin, bağımlılıkları yükleyin ve geliştirme sunucusunu başlatın:

```bash
cd yat-dice
npm install
npm run dev
```
Tarayıcınızdan terminalde belirtilen adrese (genellikle `http://localhost:5173`) giderek oyuna başlayabilirsiniz.

### Sık Karşılaşılan Sorunlar
| Belirti | Çözüm |
| :--- | :--- |
| Backend `Güvenli olmayan yapılandırma` diyerek kapanıyor | Kök dizinde `.env` yok. `cp .env.example .env` komutunu çalıştırın. |
| `Veritabanına ulaşılamıyor` | Docker Desktop'ı açın ve `docker compose up -d` çalıştırın. |
| `password authentication failed` | 5432 portunu başka bir PostgreSQL kullanıyor olabilir; onu durdurun. |
| Uygulamada `Sunucuya ulaşılamadı` | Backend çalışmıyor. `http://localhost:8080/health` adresini kontrol edin. |
| Google butonu görünmüyor | `VITE_GOOGLE_CLIENT_ID` tanımlı değil; yerelde misafir veya kayıt ile giriş yapabilirsiniz. |

---

## ⚙️ Ortam Değişkenleri (`.env`)

Kök dizindeki [`.env.example`](.env.example) dosyasını `.env` olarak kopyalayın. Üretimde `APP_ENV=production`, güçlü bir `JWT_SECRET` ve `DATABASE_URL` zorunludur; eksikse sunucu açılmaz. `VITE_*` değişkenleri `yat-dice/.env` dosyasına yazılır:

| Değişken | Açıklama | Varsayılan (Yerel) |
| :--- | :--- | :--- |
| `PORT` | Go backend sunucu portu | `8080` |
| `APP_ENV` | Yalnızca `development` değeri geliştirme modunu açar; boşsa üretim kuralları geçerlidir | *(Boş = üretim)* |
| `DATABASE_URL` | PostgreSQL bağlantı cümlesi (Üretimde zorunlu) | Geliştirmede `host=localhost port=5432 user=yatzy_user password=yatzy_password dbname=yatzy_gamedb sslmode=disable` |
| `JWT_SECRET` | JWT imzalama anahtarı (Üretimde en az 32 karakter, zorunlu) | Geliştirmede boşsa geçici rastgele anahtar |
| `GOOGLE_CLIENT_ID` | Virgülle ayrılmış Google istemci kimlikleri (web, Android, iOS) | *(Boşsa Google girişi kapalı)* |
| `TRUSTED_PROXY_COUNT` | Sunucunun önündeki güvenilir proxy sayısı; `0` ise `X-Forwarded-For` yok sayılır | `0` |
| `ALLOWED_ORIGIN` | CORS ve WebSocket izin verilen origin (`*` veya domain) | `*` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | Şifre sıfırlama e-postası için SMTP (STARTTLS, 587) | *(Boşsa e-posta gönderilmez)* |
| `REPORT_NOTIFY_EMAIL` | Yeni şikâyet bildirim adresi | `yatdice@gmail.com` *(SMTP ayarlıysa)* |
| `APPLE_CLIENT_IDS` | Sign in with Apple için bundle id listesi | *(Boşsa Apple girişi kapalı)* |
| `APPLE_TEAM_ID`, `APPLE_KEY_ID`, `APPLE_PRIVATE_KEY` | Hesap silinince Apple yetkisini geri almak için | *(Apple girişi açıksa gerekli)* |
| `ADMIN_TOKEN` | `/admin` paneli anahtarı, en az 32 karakter | *(Boşsa panel kapalı)* |
| `LEGAL_OWNER_NAME`, `LEGAL_CONTACT_EMAIL` | Hukuk metinlerindeki sahip ve iletişim bilgisi | Destek adresi varsayılan olarak `yatdice@gmail.com` |
| `VITE_API_URL` | Frontend'in bağlanacağı HTTP API adresi | Geliştirmede boş (sayfanın adresi kullanılır), üretimde `https://backend-adresin` |
| `VITE_WS_URL` | Frontend'in bağlanacağı WebSocket adresi | Geliştirmede boş (sayfanın adresi kullanılır), üretimde `wss://backend-adresin` |
| `VITE_GOOGLE_CLIENT_ID` | Web ve Android Google girişi için web istemci kimliği | *(Boşsa buton gizli)* |
| `VITE_GOOGLE_IOS_CLIENT_ID` | iOS Google girişi için istemci kimliği | *(Boşsa iOS'ta buton gizli)* |

---

## 📱 Mobil Uygulama (Capacitor)

Android ve iOS projeleri `yat-dice/android` ve `yat-dice/ios` klasörlerindedir. Uygulama kimliği `com.sutkucanverdi.yatdice` olup mağazada yayınlandıktan sonra değiştirilemez.

Yayın build'i için `yat-dice/.env` dosyasında HTTPS adresleri tanımlayın, ardından web dosyalarını yerel projelere kopyalayın:

```bash
cd yat-dice
npm run build
npx cap sync
```

- **Android:** `npx cap open android` ile Android Studio'yu açın. Yayın için **Build > Generate Signed App Bundle** kullanın. Anahtar dosyanızı kaybetmeyin ve repoya eklemeyin.
- **iOS:** Mac üzerinde `npx cap open ios` ile Xcode'u açın. **Signing & Capabilities** bölümüne **Sign in with Apple** ekleyin. Google girişi için `Info.plist` içine ters çevrilmiş iOS istemci kimliğini URL scheme olarak ekleyin.
- **Sunucu:** `ALLOWED_ORIGIN` değerine web adresinizle birlikte `capacitor://localhost,https://localhost` ekleyin. `GOOGLE_CLIENT_ID` web ve iOS kimliklerini, `APPLE_CLIENT_IDS` ise bundle id'yi içermelidir.
- **Yerel HTTP testi:** Yalnızca geliştirme için `CAP_DEV_HTTP=1 npx cap sync android` kullanın; mağaza build'inde kullanmayın.
- **İnceleme demo hesapları:** Sunucu canlıya alındıktan sonra `cd yat-dice && node scripts/create-demo-accounts.mjs https://api.alanadiniz.com` çalıştırın. Komut iki hesap açar, onları arkadaş yapar ve bilgileri `review-demo-accounts.local` dosyasına yazar (git'e girmez). Bu bilgileri App Store Connect'te **App Review Information → Sign-in required** alanına ve Google Play'de **App access** bölümüne girin.
- **Şifre sıfırlama:** Kodlar e-postayla gider; canlı sunucuda `SMTP_*` değişkenleri tanımlı değilse istek, destek adresini gösteren bir hata döner.

---

## 🔌 REST API ve WebSocket Özeti

### REST API Uç Noktaları
| Metot | Uç Nokta | Açıklama |
| :--- | :--- | :--- |
| `POST` | `/register` & `/login` | Kullanıcı kaydı (küfür filtreli) ve brute-force korumalı JWT giriş |
| `POST` | `/auth/guest` | Cihaz kimliği (`deviceId`) ile kalıcı misafir hesabı oluşturma/giriş |
| `POST` | `/auth/google` | Google hesabıyla kayıt ve giriş |
| `POST` | `/auth/apple` | Sign in with Apple ile kayıt ve giriş |
| `POST` | `/auth/forgot-password` & `/auth/reset-password` | 6 haneli kod ile güvenli şifre sıfırlama |
| `GET` | `/profile` | Kullanıcının güncel profil, istatistik, görev ve zar koleksiyonu |
| `POST` | `/profile/update` | Kullanıcı adı, profil fotoğrafı/avatar, ülke bayrağı ve seçili zar güncelleme |
| `POST / DELETE` | `/profile/delete` | Hesabı ve ilişkili tüm verileri kalıcı olarak silme (App Store / Play Store zorunluluğu) |
| `POST` | `/profile/claim-daily` | Sunucu saati doğrulamalı günlük giriş serisi ödülü |
| `POST` | `/profile/open-case` | Sunucu tarafında atomik kasa açma (`50–199 🪙` eşit olasılıklı) |
| `POST` | `/quests/claim` | Tamamlanan normal ve zor görevlerin XP, Jeton ve Kasa ödüllerini alma |
| `POST` | `/shop/buy` | Sunucu doğrulamalı 3D zar seti satın alma |
| `GET` | `/friends` | Arkadaş listesi, çevrimiçi/oyunda durumları, istekler ve oyun davetleri |
| `POST` | `/friends/request` & `/friends/respond` & `/friends/invite` | Arkadaş ekleme, kabul/red ve özel odaya davet gönderme |
| `POST` | `/ugc/report`, `/ugc/block`, `/ugc/unblock` | Oyuncuyu şikâyet etme, engelleme ve engeli kaldırma |
| `GET` | `/ugc/blocks` | Engellenen oyuncuların listesi |
| `GET / POST` | `/account/delete` | Web'den hesap silme veya silme talebi |
| `GET / POST` | `/admin`, `/admin/queue`, `/admin/action` | Şikâyet ve silme taleplerinin moderasyonu (`ADMIN_TOKEN`) |
| `GET` | `/leaderboard` | En çok galibiyet ve XP toplayan ilk 20 oyuncu listesi |
| `GET` | `/privacy` & `/terms` | Mağaza başvuruları için resmi Gizlilik Politikası ve Kullanım Koşulları (HTML) |
| `GET` | `/health` | Sunucu ve PostgreSQL veritabanı sağlık durumu kontrolü |

### WebSocket (`ws://localhost:8080/play?token=<JWT>`) Eylemleri
Geçerli JWT olmadan bağlantı `401` ile reddedilir; oyuncu adı istemciden değil veritabanından alınır.

- **Eşleşme Komutları:** `QUICK_MATCH`, `BOT_MATCH`, `CREATE_ROOM`, `JOIN_ROOM` (sanal giriş jetonu doğrulamalı), `RESUME` (kopan maça dönüş; 60 saniye içinde)
- **Oyun İçi Komutlar:** `ROLL_DICE` (Zar at), `TOGGLE_LOCK` (Zar kilitle/aç), `SCORE_SELECTED` (Kategori seç), `EMOJI` (Hızlı emoji), `CHAT` (Canlı sohbet mesajı), `LEAVE` (Maçtan bilerek çıkış, hemen hükmen yenilgi)
- **Sunucu Yayını:** Her hamlede güncel zarlar, kilitler, iki oyuncunun skor tabloları, aktif tur, 30 saniyelik tur sayacı ve maç sonu ödülleri tüm odaya senkronize edilir.

---

## 📄 Lisans

Bu proje açık kaynaklı olarak geliştirilmiştir. Dilediğiniz gibi inceleyebilir, derleyebilir ve katkıda bulunabilirsiniz.
