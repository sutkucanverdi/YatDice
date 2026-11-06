import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.sutkucanverdi.yatdice',
  appName: 'Yat Dice',
  webDir: 'dist',
  backgroundColor: '#111318',
  // Yalnızca yerel HTTP sunucusuyla telefon testi için. Mağaza build'inde kullanmayın.
  ...(process.env.CAP_DEV_HTTP === '1' ? { server: { androidScheme: 'http', cleartext: true } } : {}),
  plugins: {
    SplashScreen: {
      launchShowDuration: 800,
      backgroundColor: '#111318',
      showSpinner: false,
    },
    StatusBar: {
      style: 'DARK',
      backgroundColor: '#111318',
      overlaysWebView: false,
    },
    SocialLogin: {
      providers: { google: true, apple: true, facebook: false, twitter: false },
    },
  },
};

export default config;
