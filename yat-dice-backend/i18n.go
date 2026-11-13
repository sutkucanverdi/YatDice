package main

import (
	"fmt"
	"net/http"
	"strings"
)

// İstemciye giden mesajlar Türkçe yazılır; İngilizce isteyen istemciye bu tablodaki karşılığı gönderilir.
// Dil, uygulamanın gönderdiği Accept-Language başlığından veya ?lang= parametresinden okunur.

const (
	langTR = "tr"
	langEN = "en"
)

// Dil belirtilmemişse (ör. curl) Türkçe; Türkçe dışındaki her dil için İngilizce kullanılır.
func parseLang(query, acceptLanguage string) string {
	if query == langTR || query == langEN {
		return query
	}
	al := strings.ToLower(strings.TrimSpace(acceptLanguage))
	if al == "" || strings.HasPrefix(al, "tr") {
		return langTR
	}
	return langEN
}

func requestLang(r *http.Request) string {
	if r == nil {
		return langTR
	}
	return parseLang(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
}

// localize mesajı verilen dile çevirir; çevirisi yoksa olduğu gibi döner.
func localize(lang, msg string) string {
	if lang == langEN {
		if en, ok := enMessages[msg]; ok {
			return en
		}
	}
	return msg
}

// L isteğin diline göre çevirir.
func L(r *http.Request, msg string) string {
	return localize(requestLang(r), msg)
}

// Lf biçimli mesajlar içindir; biçim metni çevrilir, değerler sonra yerleştirilir.
func Lf(r *http.Request, format string, args ...interface{}) string {
	return fmt.Sprintf(L(r, format), args...)
}

// httpError, http.Error'ın isteğin diline göre çeviren hâlidir.
func httpError(w http.ResponseWriter, r *http.Request, msg string, code int) {
	http.Error(w, L(r, msg), code)
}

var enMessages = map[string]string{
	// Giriş ve hesap
	"Apple hesabı oluşturulamadı":              "Could not create the Apple account",
	"Apple ile giriş bu sunucuda etkin değil":  "Sign in with Apple is not enabled on this server",
	"Apple kimlik bilgisi gerekli":             "Apple credential required",
	"Apple oturumu doğrulanamadı":              "Could not verify the Apple sign-in",
	"Google hesabı oluşturulamadı":             "Could not create the Google account",
	"Google ile giriş bu sunucuda etkin değil": "Google sign-in is not enabled on this server",
	"Google kimlik bilgisi gerekli":            "Google credential required",
	"Google oturumu doğrulanamadı":             "Could not verify the Google sign-in",
	"Bu e-posta ile kayıtlı bir hesap var. Lütfen diğer giriş yönteminizi kullanın.":        "An account with this email already exists. Please use your other sign-in method.",
	"Bu e-posta ile kayıtlı bir hesap var. Lütfen kullanıcı adı ve şifrenizle giriş yapın.": "An account with this email already exists. Please sign in with your username and password.",
	"Bu kullanıcı adı zaten alınmış":                                                        "This username is already taken",
	"Bu kullanıcı adı zaten kullanılıyor":                                                   "This username is already in use",
	"Kullanıcı adı (2-30 karakter) ve şifre (en az 6 karakter) olmalıdır":                   "Username must be 2-30 characters and password at least 6 characters",
	"Kullanıcı adı boş olamaz":                                                              "Username cannot be empty",
	"Kullanıcı adı giriniz":                                                                 "Enter a username",
	"Kullanıcı adı topluluk kurallarına aykırı ifadeler içeremez":                           "Username cannot contain words that break the community rules",
	"Kullanıcı bulunamadı veya şifre yanlış":                                                "User not found or wrong password",
	"Misafir": "Guest",
	"Bu adla birden fazla hesap var. Etiketinle birlikte gir (ör. Ad#1234).":          "More than one account uses this name. Sign in with your tag (e.g. Name#1234).",
	"Bu adı kullanan birden fazla oyuncu var. Etiketiyle birlikte yaz (ör. Ad#1234).": "More than one player uses this name. Add their tag (e.g. Name#1234).",
	"Yalnızca arkadaşlarınla mesajlaşabilirsin":                                       "You can only message your friends",
	"Mesajlar alınamadı": "Could not load messages",
	"Mesaj boş olamaz":   "Message cannot be empty",
	"Mesajın topluluk kurallarına aykırı ifadeler içeriyor":                   "Your message contains words that break the community rules",
	"Çok hızlı yazıyorsun, biraz bekle":                                       "You're typing too fast, wait a moment",
	"Mesaj gönderilemedi":                                                     "Could not send the message",
	"Misafir hesabı oluşturulamadı":                                           "Could not create a guest account",
	"Geçersiz cihaz kimliği":                                                  "Invalid device ID",
	"Kayıt başarılı!":                                                         "Account created!",
	"Token oluşturulamadı":                                                    "Could not create a session",
	"Şifre en fazla 72 karakter olabilir":                                     "Password can be at most 72 characters",
	"Çok fazla hatalı giriş denemesi. Lütfen %d saniye sonra tekrar deneyin.": "Too many failed sign-in attempts. Please try again in %d seconds.",
	"Devam etmek için Kullanım Koşulları ve Gizlilik Politikası'nı kabul etmelisiniz.": "You must accept the Terms of Use and Privacy Policy to continue.",
	"Bu hesap topluluk kuralları ihlali nedeniyle kapatılmıştır.":                      "This account was closed for breaking the community rules.",
	"Geçerli bir e-posta adresi girin":                                                 "Enter a valid email address",
	"Hesabınız kalıcı olarak silindi.":                                                 "Your account was permanently deleted.",
	"Hesap silinirken sunucu hatası oluştu":                                            "A server error occurred while deleting the account",

	// Şifre sıfırlama
	"Şifre sıfırlama e-postası şu an gönderilemiyor. Lütfen %s adresinden bize yazın.": "Password reset emails can't be sent right now. Please email us at %s.",
	"Bilgiler kayıtlı hesapla eşleşiyorsa doğrulama kodu e-postanıza gönderildi.":      "If the details match an account, a verification code has been sent to your email.",
	"Geçersiz kullanıcı veya sıfırlama kodu":                                           "Invalid user or reset code",
	"Sıfırlama kodu hatalı!":                                      "Wrong reset code!",
	"Sıfırlama kodunun süresi dolmuş veya geçersiz":               "The reset code has expired or is invalid",
	"Çok fazla hatalı kod denemesi. Yeni kod talep edin.":         "Too many wrong codes. Please request a new one.",
	"Yeni şifre en az 6 karakter olmalıdır":                       "New password must be at least 6 characters",
	"Şifre güncellenemedi":                                        "Could not update the password",
	"Şifreniz başarıyla güncellendi! Şimdi giriş yapabilirsiniz.": "Your password was updated! You can sign in now.",
	"Yat Dice şifre sıfırlama kodu":                               "Your Yat Dice password reset code",
	"Yat Dice şifre sıfırlama kodunuz: %s\n\nKod 10 dakika geçerlidir. Bu isteği siz yapmadıysanız e-postayı yok sayın.": "Your Yat Dice password reset code: %s\n\nThe code is valid for 10 minutes. If you did not request this, you can ignore this email.",

	// Profil, ödüller, mağaza
	"Profil bulunamadı":                 "Profile not found",
	"Profil okunamadı":                  "Could not read the profile",
	"Profil oluşturulamadı":             "Could not create the profile",
	"Bugünkü günlük ödül zaten alındı":  "Today's daily reward was already claimed",
	"Ödül kaydedilemedi":                "Could not save the reward",
	"Açılacak kasa yok":                 "No cases to open",
	"Bu görev ödülü zaten alındı":       "This quest reward was already claimed",
	"Geçersiz görev":                    "Invalid quest",
	"Görev ödülü kaydedilemedi":         "Could not save the quest reward",
	"Görev şartları henüz tamamlanmadı": "Quest requirements are not complete yet",
	"Geçersiz zar tipi":                 "Invalid dice type",
	"Satın alma başarısız":              "Purchase failed",
	"Yetersiz jeton!":                   "Not enough coins!",
	"Liderlik tablosu alınamadı":        "Could not load the leaderboard",

	// Arkadaşlar ve moderasyon
	"Arkadaş listesi alınamadı":                    "Could not load the friend list",
	"Arkadaşlık isteği gönderildi!":                "Friend request sent!",
	"Arkadaşlık isteği gönderilemedi":              "Could not send the friend request",
	"Bu isimde bir oyuncu bulunamadı!":             "No player found with that name!",
	"Bu oyuncu zaten arkadaş listenizde!":          "This player is already your friend!",
	"Kendinizi arkadaş olarak ekleyemezsiniz!":     "You can't add yourself as a friend!",
	"Zaten bekleyen bir arkadaşlık isteği var!":    "There is already a pending friend request!",
	"Yalnızca arkadaşlarınızı davet edebilirsiniz": "You can only invite your friends",
	"Oyun daveti gönderildi!":                      "Game invite sent!",
	"Arkadaşın":                                    "Your friend",
	"Oyuncu bulunamadı":                            "Player not found",
	"Oyuncu engellenemedi":                         "Could not block the player",
	"Engel kaldırılamadı":                          "Could not unblock",
	"Engel listesi alınamadı":                      "Could not load the block list",
	"Kendinizi engelleyemezsiniz":                  "You can't block yourself",
	"Kendinizi bildiremezsiniz":                    "You can't report yourself",
	"Bildirim nedeni zorunludur":                   "A reason is required",
	"Bildirim kaydedilemedi":                       "Could not save the report",
	"Bildiriminiz moderasyon ekibine iletildi.":    "Your report was sent to the moderation team.",

	// Genel
	"Geçersiz istek":      "Invalid request",
	"Geçersiz işlem":      "Invalid action",
	"Geçersiz veri":       "Invalid data",
	"Sunucu hatası":       "Server error",
	"İşlem başarısız":     "Action failed",
	"İşlem tamamlanamadı": "Could not complete the action",
	"Yetkisiz":            "Unauthorized",
	"Kuyruk alınamadı":    "Could not load the queue",
	"Talep kaydedilemedi": "Could not save the request",
	"Çok fazla istek gönderdiniz. Lütfen kısa bir süre bekleyin.": "Too many requests. Please wait a moment.",

	// Maç (WebSocket)
	"Seçilen masa için yeterli jetonunuz yok!":            "You don't have enough coins for this table!",
	"Bu hesap zaten eşleşme kuyruğunda!":                  "This account is already in the matchmaking queue!",
	"Geçersiz oda kodu formatı!":                          "Invalid room code format!",
	"Oda bulunamadı veya kendi odanıza katılamazsınız!":   "Room not found, or you can't join your own room!",
	"Özel oda bekleme süresi dolduğu için kapatıldı.":     "The private room was closed because the waiting time ran out.",
	"Emoji spam koruması: Lütfen 2 saniye bekleyin.":      "Emoji spam protection: please wait 2 seconds.",
	"Çok hızlı yazıyorsunuz! Lütfen 2.5 saniye bekleyin.": "You're typing too fast! Please wait 2.5 seconds.",
	"Bol şans! Zarlar bugün güzel dönüyor 🎲":              "Good luck! The dice are rolling nicely today 🎲",
	"İyi hamle! Bakalım YAT kimden gelecek 🔥":             "Nice move! Let's see who gets the YAT 🔥",
	"Güzel maç oluyor, başarılar! 😎":                      "Great match so far, good luck! 😎",
	"Hesaplamalarıma göre bu tur çok kritik 🤖":            "By my calculations, this round is crucial 🤖",
	"Harika gidiyorsun! 👏":                                "You're doing great! 👏",
}
