import { useState, useEffect, useRef, useCallback } from "react";
import React from "react";
import { motion, AnimatePresence } from "motion/react";
import {
  Sun, Moon, ShoppingBag, Users, Zap, Trophy,
  RotateCcw, Crown, Check, Lock,
  Home, TrendingUp, Edit3, X, ChevronLeft
} from "lucide-react";

const API_BASE = import.meta.env.VITE_API_URL || "http://localhost:8080";
const WS_BASE  = import.meta.env.VITE_WS_URL  || "ws://localhost:8080";
const GOOGLE_CLIENT_ID: string = import.meta.env.VITE_GOOGLE_CLIENT_ID || "";
const APP_FONT = 'system-ui,-apple-system,"Segoe UI",Roboto,sans-serif';

// ─── Web Audio API Ses Efekt Motoru (Harici MP3 Gerektirmez) ──────────────────
let sharedAudioCtx: AudioContext | null = null;
function getAudioCtx(): AudioContext | null {
  if (typeof window === "undefined") return null;
  if (localStorage.getItem("yatzy_sound") === "off") return null;
  const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
  if (!Ctx) return null;
  if (!sharedAudioCtx) sharedAudioCtx = new Ctx();
  if (sharedAudioCtx.state === "suspended") sharedAudioCtx.resume().catch(() => {});
  return sharedAudioCtx;
}

const sfx = {
  // Gerçekçi 3D Zar Atma Sesi (Bandpass Filtreli Darbe Gürültüsü + Ahşap/Fildişi Rezonansı)
  roll() {
    const ctx = getAudioCtx();
    if (!ctx) return;
    const bounces = [0, 0.065, 0.13, 0.20, 0.28, 0.38, 0.50];
    bounces.forEach((delay, idx) => {
      const t0 = ctx.currentTime + delay;
      const vol = Math.max(0.03, 0.13 - idx * 0.014);

      // 1) Kısa darbe gürültüsü (Zarların birbirine ve masaya çarpması)
      const bufSize = Math.floor(ctx.sampleRate * 0.045);
      const noiseBuf = ctx.createBuffer(1, bufSize, ctx.sampleRate);
      const data = noiseBuf.getChannelData(0);
      for (let i = 0; i < bufSize; i++) {
        data[i] = (Math.random() * 2 - 1) * Math.exp(-i / (bufSize * 0.28));
      }
      const noise = ctx.createBufferSource();
      noise.buffer = noiseBuf;

      const filter = ctx.createBiquadFilter();
      filter.type = "bandpass";
      filter.frequency.setValueAtTime(950 + idx * 140 + Math.random() * 250, t0);
      filter.Q.setValueAtTime(3.2, t0);

      const nGain = ctx.createGain();
      nGain.gain.setValueAtTime(vol, t0);
      nGain.gain.exponentialRampToValueAtTime(0.001, t0 + 0.045);

      noise.connect(filter);
      filter.connect(nGain);
      nGain.connect(ctx.destination);
      noise.start(t0);

      // 2) Gövde tınısı (Kemik/Ahşap zar tıkırtısı)
      const osc = ctx.createOscillator();
      const oGain = ctx.createGain();
      osc.type = "triangle";
      osc.frequency.setValueAtTime(240 + idx * 35 + Math.random() * 60, t0);
      osc.frequency.exponentialRampToValueAtTime(110, t0 + 0.04);
      oGain.gain.setValueAtTime(vol * 0.65, t0);
      oGain.gain.exponentialRampToValueAtTime(0.001, t0 + 0.042);
      osc.connect(oGain);
      oGain.connect(ctx.destination);
      osc.start(t0);
      osc.stop(t0 + 0.045);
    });
  },
  // Zar Kilitleme / Kilit Açma Sesi
  lock(locked = true) {
    const ctx = getAudioCtx();
    if (!ctx) return;
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.setValueAtTime(locked ? 520 : 680, ctx.currentTime);
    osc.frequency.exponentialRampToValueAtTime(locked ? 780 : 440, ctx.currentTime + 0.055);
    gain.gain.setValueAtTime(0.08, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.06);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.065);
  },
  // Puan Yazma & 50 Puan YAT Fanfarı
  score(pts: number) {
    const ctx = getAudioCtx();
    if (!ctx) return;
    const notes = pts >= 50 ? [523.25, 659.25, 783.99, 1046.5, 1318.5] : pts > 0 ? [440, 554.37, 659.25] : [220, 174.61];
    notes.forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = pts >= 50 ? "triangle" : "sine";
      osc.frequency.setValueAtTime(freq, ctx.currentTime + i * 0.065);
      gain.gain.setValueAtTime(0.095, ctx.currentTime + i * 0.065);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + i * 0.065 + 0.18);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(ctx.currentTime + i * 0.065);
      osc.stop(ctx.currentTime + i * 0.065 + 0.19);
    });
  },
  // Kasa Sallanma Efekt Sesi
  caseShake() {
    const ctx = getAudioCtx();
    if (!ctx) return;
    [0, 0.18, 0.36, 0.54, 0.72, 0.90, 1.08, 1.26].forEach((d, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = "triangle";
      osc.frequency.setValueAtTime(180 + i * 30, ctx.currentTime + d);
      gain.gain.setValueAtTime(0.07, ctx.currentTime + d);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + d + 0.08);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(ctx.currentTime + d);
      osc.stop(ctx.currentTime + d + 0.09);
    });
  },
  // Son 5 Saniye Süre Uyarı Tıkırtısı
  tick() {
    const ctx = getAudioCtx();
    if (!ctx) return;
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.setValueAtTime(880, ctx.currentTime);
    gain.gain.setValueAtTime(0.04, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.035);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.04);
  },
  // Sohbet / Bildirim / Eşleşme Bulundu Sesi
  pop() {
    const ctx = getAudioCtx();
    if (!ctx) return;
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.setValueAtTime(587.33, ctx.currentTime);
    osc.frequency.exponentialRampToValueAtTime(880, ctx.currentTime + 0.08);
    gain.gain.setValueAtTime(0.07, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.09);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.095);
  },
  // Galibiyet & Ödül Fanfarı
  win() {
    const ctx = getAudioCtx();
    if (!ctx) return;
    [523.25, 659.25, 783.99, 1046.5].forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = "triangle";
      osc.frequency.setValueAtTime(freq, ctx.currentTime + i * 0.09);
      gain.gain.setValueAtTime(0.11, ctx.currentTime + i * 0.09);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + i * 0.09 + 0.24);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(ctx.currentTime + i * 0.09);
      osc.stop(ctx.currentTime + i * 0.09 + 0.25);
    });
  },
};

// ─── Global Bildirim (Native alert() yerine modern oyun-içi Toast) ────────────
let toastListener: ((msg: string) => void) | null = null;
function emitToast(msg: string) {
  if (toastListener) toastListener(msg);
}

interface LeaderboardEntry {
  rank: number;
  username: string;
  avatar?: string;
  country?: string;
  xp: number;
  wins: number;
  highScore: number;
}

// ─── Types ────────────────────────────────────────────────────────────────────
type Screen  = "auth" | "home" | "shop" | "game" | "result" | "modes" | "friends" | "quests";
type DieVal  = 1 | 2 | 3 | 4 | 5 | 6;
type DieMat  = "ivory" | "wood" | "marble" | "obsidian" | "ruby" | "gold";
type BetOption = 0 | 50 | 100 | 250 | 500;
const BET_OPTIONS: BetOption[] = [0, 50, 100, 250, 500];

interface FriendItem {
  friendshipId: number;
  userId: number;
  username: string;
  avatar?: string;
  country?: string;
  xp: number;
  wins: number;
  isOnline?: boolean;
  inGame?: boolean;
}

interface GameInvite {
  id: number;
  roomCode: string;
  fromUserId: number;
  fromName: string;
  createdAt: string;
}

interface DieState { value: DieVal; rx: number; ry: number; px: number; py: number; }
interface UserProfile { 
  name: string; 
  seed: string; 
  avatar?: string;
  country?: string;
  level: number; 
  xp: number; 
  tokens: number;           
  cases: number;            
  unlockedDice: DieMat[];   
  lastDailyReward: string;  
  loginStreak: number;
  upperBonusesWon?: number;
  totalYachts?: number;
  maxYachtsInMatch?: number;
  claimedQuests?: string[];
  stats: {
    wins: number;
    gamesPlayed: number;
    highScore: number;
  };
}

interface QuestItemDef {
  id: string;
  title: string;
  desc: string;
  icon: string;
  difficulty: "normal" | "hard";
  xp: number;
  tokens: number;
  cases: number;
  getProgress: (p: UserProfile) => { current: number; target: number };
}

const QUEST_LIST: QuestItemDef[] = [
  {
    id: "first_match",
    title: "İlk Zar, İlk Heyecan",
    desc: "Herhangi bir modda ilk maçını tamamla.",
    icon: "🎲",
    difficulty: "normal",
    xp: 150,
    tokens: 75,
    cases: 0,
    getProgress: p => ({ current: Math.min(p.stats.gamesPlayed, 1), target: 1 }),
  },
  {
    id: "upper_bonus_1",
    title: "Üst Toplam Ustası",
    desc: "Bir maçta Üst Toplamdan 63+ puana ulaşıp +35 Bonus kazan.",
    icon: "📈",
    difficulty: "normal",
    xp: 300,
    tokens: 150,
    cases: 0,
    getProgress: p => ({ current: Math.min(p.upperBonusesWon || 0, 1), target: 1 }),
  },
  {
    id: "first_yacht",
    title: "Beşi Bir Yerde (YAT)",
    desc: "Maç sırasında 5 zarı da aynı getirerek YAT (50 puan) yap.",
    icon: "⚡",
    difficulty: "normal",
    xp: 350,
    tokens: 200,
    cases: 0,
    getProgress: p => ({ current: Math.min(p.totalYachts || 0, 1), target: 1 }),
  },
  {
    id: "wins_3",
    title: "Galibiyet Serisi",
    desc: "Bota veya gerçek rakiplere karşı toplam 3 maç kazan.",
    icon: "🏆",
    difficulty: "normal",
    xp: 400,
    tokens: 250,
    cases: 0,
    getProgress: p => ({ current: Math.min(p.stats.wins, 3), target: 3 }),
  },
  {
    id: "score_220",
    title: "Rekor Kıran (Zor Görev)",
    desc: "Tek bir maçta 220 veya üzeri toplam puana ulaş.",
    icon: "🔥",
    difficulty: "hard",
    xp: 600,
    tokens: 400,
    cases: 1,
    getProgress: p => ({ current: Math.min(p.stats.highScore, 220), target: 220 }),
  },
  {
    id: "upper_bonus_3",
    title: "Kusursuz Stratejist (Zor Görev)",
    desc: "Toplam 3 farklı maçta Üst Toplam (+35) Bonusu al.",
    icon: "🧠",
    difficulty: "hard",
    xp: 750,
    tokens: 500,
    cases: 1,
    getProgress: p => ({ current: Math.min(p.upperBonusesWon || 0, 3), target: 3 }),
  },
  {
    id: "double_yacht",
    title: "Çifte YAT Efsanesi (Zor Görev)",
    desc: "Tek bir maç içinde 2 kez YAT (5 aynı zar) at!",
    icon: "💎",
    difficulty: "hard",
    xp: 1000,
    tokens: 750,
    cases: 2,
    getProgress: p => ({ current: Math.min(p.maxYachtsInMatch || 0, 2), target: 2 }),
  },
  {
    id: "triple_yacht",
    title: "İmkansız Üçleme: 3 YAT (Efsanevi)",
    desc: "Tek bir maç içinde tam 3 kez YAT (5 aynı zar) at!",
    icon: "👑",
    difficulty: "hard",
    xp: 2000,
    tokens: 1500,
    cases: 3,
    getProgress: p => ({ current: Math.min(p.maxYachtsInMatch || 0, 3), target: 3 }),
  },
];

function getClaimableQuestCount(p: UserProfile): number {
  const claimed = new Set(p.claimedQuests || []);
  return QUEST_LIST.filter(q => {
    if (claimed.has(q.id)) return false;
    const prog = q.getProgress(p);
    return prog.current >= prog.target;
  }).length;
}

const COUNTRIES: { code: string; flag: string; name: string }[] = [
  { code: "TR", flag: "🇹🇷", name: "Türkiye" },
  { code: "AZ", flag: "🇦🇿", name: "Azerbaycan" },
  { code: "DE", flag: "🇩🇪", name: "Almanya" },
  { code: "GB", flag: "🇬🇧", name: "Birleşik Krallık" },
  { code: "US", flag: "🇺🇸", name: "ABD" },
  { code: "NL", flag: "🇳🇱", name: "Hollanda" },
  { code: "FR", flag: "🇫🇷", name: "Fransa" },
  { code: "ES", flag: "🇪🇸", name: "İspanya" },
  { code: "IT", flag: "🇮🇹", name: "İtalya" },
  { code: "JP", flag: "🇯🇵", name: "Japonya" },
  { code: "KR", flag: "🇰🇷", name: "Güney Kore" },
  { code: "BR", flag: "🇧🇷", name: "Brezilya" },
];

const PRESET_AVATARS = ["🎲", "👑", "🦁", "🐉", "🦊", "🐼", "⚡", "🔥", "💎", "🤖", "🎯", "🧙‍♂️"];

function getCountryFlag(code?: string): string {
  const found = COUNTRIES.find(c => c.code === (code || "TR").toUpperCase());
  return found ? found.flag : "🇹🇷";
}

export interface MatchSummary {
  myName: string;
  mySeed: string;
  oppName: string;
  oppSeed: string;
  myScores: Record<string, number>;
  oppScores: Record<string, number>;
  myBonus: number;
  oppBonus: number;
  myTotal: number;
  oppTotal: number;
  rewardXp: number;
  rewardTokens: number;
  rewardCases: number;
}

// ─── Scorecard (Turkish) ──────────────────────────────────────────────────────
const CATS = [
  { key:"ones",   label:"Birler",       desc:"1'lerin toplamı",    upper:true  },
  { key:"twos",   label:"İkiler",       desc:"2'lerin toplamı",    upper:true  },
  { key:"threes", label:"Üçlüler",      desc:"3'lerin toplamı",    upper:true  },
  { key:"fours",  label:"Dörtlüler",    desc:"4'lerin toplamı",    upper:true  },
  { key:"fives",  label:"Beşliler",     desc:"5'lerin toplamı",    upper:true  },
  { key:"sixes",  label:"Altılar",      desc:"6'ların toplamı",    upper:true  },
  { key:"three",  label:"Üç Aynı",      desc:"Tüm zarların top.",  upper:false },
  { key:"four",   label:"Dört Aynı",    desc:"Tüm zarların top.",  upper:false },
  { key:"full",   label:"Full Ev",      desc:"25 puan",            upper:false },
  { key:"small",  label:"Küçük Sıralı", desc:"30 puan",            upper:false },
  { key:"large",  label:"Büyük Sıralı", desc:"40 puan",            upper:false },
  { key:"yacht",  label:"YAT",          desc:"50 puan",            upper:false },
  { key:"chance", label:"Şans",         desc:"Tüm zarların top.",  upper:false },
];

const SCORE_GRID: ({ k: string; l: string; spec?: boolean } | null)[][] = [
  [{k:'ones', l:'Birler'}, {k:'twos', l:'İkiler'}],
  [{k:'threes', l:'Üçlüler'}, {k:'fours', l:'Dörtlüler'}],
  [{k:'fives', l:'Beşliler'}, {k:'sixes', l:'Altılar'}],
  [{k:'upper_total', l:'Üst Top. (63)', spec:true}, {k:'bonus', l:'Bonus (+35)', spec:true}],
  [{k:'three', l:"3'lü grup"}, {k:'four', l:"4'lü grup"}],
  [{k:'full', l:'Full Ev'}, {k:'small', l:'Küç. Sıralı'}],
  [{k:'large', l:'Büy. Sıralı'}, {k:'yacht', l:'YAT'}],
  [{k:'chance', l:'Şans'}, null]
];

function calcScore(key: string, dice: DieVal[]): number {
  const sum = dice.reduce((a,b)=>a+b,0);
  const cnt = (n:number) => dice.filter(d=>d===n).length;
  if (key==="ones")   return cnt(1);
  if (key==="twos")   return cnt(2)*2;
  if (key==="threes") return cnt(3)*3;
  if (key==="fours")  return cnt(4)*4;
  if (key==="fives")  return cnt(5)*5;
  if (key==="sixes")  return cnt(6)*6;
  if (key==="chance") return sum;
  if (key==="three")  return [1,2,3,4,5,6].some(n=>cnt(n)>=3) ? sum : 0;
  if (key==="four")   return [1,2,3,4,5,6].some(n=>cnt(n)>=4) ? sum : 0;
  if (key==="full") {
    const h3=[1,2,3,4,5,6].some(n=>cnt(n)===3);
    const h2=[1,2,3,4,5,6].some(n=>cnt(n)===2);
    const h5=[1,2,3,4,5,6].some(n=>cnt(n)===5);
    return (h3&&h2)||h5 ? 25 : 0;
  }
  if (key==="small") {
    const v=new Set(dice); const h=(n:number)=>v.has(n as DieVal);
    return (h(1)&&h(2)&&h(3)&&h(4))||(h(2)&&h(3)&&h(4)&&h(5))||(h(3)&&h(4)&&h(5)&&h(6))?30:0;
  }
  if (key==="large") {
    const s=[...new Set(dice)].sort().join("");
    return s==="12345"||s==="23456"?40:0;
  }
  if (key==="yacht") return [1,2,3,4,5,6].some(n=>cnt(n)===5)?50:0;
  return 0;
}

function computeLevel(xp: number): number {
  return Math.max(1, Math.floor(xp / 1000) + 1);
}

// ─── Colors ───────────────────────────────────────────────────────────────────
function C(dark: boolean) {
  return dark ? {
    bg:"#1c1f26", bgDeep:"#16181e", panel:"#252830", panelAlt:"#2e3140",
    border:"rgba(255,255,255,0.07)", borderMid:"rgba(255,255,255,0.12)",
    text:"#e4e2dc", textSub:"#9a9da8", textFaint:"#4e5160",
    accent:"#4aada6", accentBg:"rgba(74,173,166,0.1)", accentRing:"rgba(74,173,166,0.22)",
    gold:"#c9a84c", goldBg:"rgba(201,168,76,0.1)",
    danger:"#c0392b", shadow:"rgba(0,0,0,0.45)",
    trayOuter: "#1b2124", 
    trayOuterBorder: "#2a3539",
    trayFelt: "radial-gradient(ellipse at center, #2e7d32 0%, #144216 100%)",
  } : {
    bg:"#f3f0ea", bgDeep:"#e8e4dc", panel:"#ffffff", panelAlt:"#f0ede6",
    border:"rgba(0,0,0,0.08)", borderMid:"rgba(0,0,0,0.14)",
    text:"#28261f", textSub:"#887f72", textFaint:"#c8c2b8",
    accent:"#3d8b85", accentBg:"rgba(61,139,133,0.08)", accentRing:"rgba(61,139,133,0.2)",
    gold:"#a87c2a", goldBg:"rgba(168,124,42,0.09)",
    danger:"#b03020", shadow:"rgba(0,0,0,0.12)",
    trayOuter: "#334145",
    trayOuterBorder: "#45565a",
    trayFelt: "radial-gradient(ellipse at center, #4caf50 0%, #2e7d32 100%)",
  };
}

// ─── 3D Dice System ───────────────────────────────────────────────────────────
const DOTS: Record<DieVal,[number,number][]> = {
  1: [[50,50]],
  2: [[27,27],[73,73]],
  3: [[27,27],[50,50],[73,73]],
  4: [[27,27],[73,27],[27,73],[73,73]],
  5: [[27,27],[73,27],[50,50],[27,73],[73,73]],
  6: [[27,19],[73,19],[27,50],[73,50],[27,81],[73,81]],
};

const FACE_DEFS: { v:DieVal; t:(h:number)=>string }[] = [
  { v:1, t:h=>`translateZ(${h}px)` },                        
  { v:6, t:h=>`rotateY(180deg) translateZ(${h}px)` },        
  { v:2, t:h=>`rotateX(-90deg) translateZ(${h}px)` },        
  { v:5, t:h=>`rotateX(90deg) translateZ(${h}px)` },         
  { v:3, t:h=>`rotateY(90deg) translateZ(${h}px)` },         
  { v:4, t:h=>`rotateY(-90deg) translateZ(${h}px)` },        
];

const SHOW: Record<DieVal,[number,number]> = {
  1: [0,    0  ],
  2: [90,   0  ],
  3: [0,   -90 ],
  4: [0,    90 ],
  5: [-90,  0  ],
  6: [0,    180],
};

function targetRot(v:DieVal, cx:number, cy:number): {rx:number,ry:number} {
  const [tx,ty] = SHOW[v];
  const spinsX = 360 * (1 + Math.floor(Math.random() * 2));
  const spinsY = 360 * (1 + Math.floor(Math.random() * 2));
  
  const nX = ((cx%360)+360)%360, nY = ((cy%360)+360)%360;
  const tX = ((tx%360)+360)%360, tY = ((ty%360)+360)%360;
  const dX = tX>=nX ? tX-nX : tX+360-nX;
  const dY = tY>=nY ? tY-nY : tY+360-nY;
  
  return { rx: cx + spinsX + dX, ry: cy + spinsY + dY };
}

function initDie(v: DieVal, index: number = 0): DieState {
  const [rx,ry] = SHOW[v];
  const angle = (index / 5) * Math.PI * 2 - Math.PI / 2;
  const r = 45;
  return { value:v, rx, ry, px: Math.cos(angle)*r, py: Math.sin(angle)*r };
}

const FIXED_POSITIONS: {x:number,y:number}[] = [
  { x: -42, y: -35 },
  { x:  38, y: -40 },
  { x: -45, y:  28 },
  { x:  42, y:  25 },
  { x:   0, y:   5 },
];

function generateRandomPositions() {
  const shuffled = [...FIXED_POSITIONS].sort(() => Math.random() - 0.5);
  return shuffled.map(p => ({
    x: p.x + (Math.random() - 0.5) * 12,
    y: p.y + (Math.random() - 0.5) * 12,
  }));
}

const MAT: Record<DieMat, {
  face:string; edge:string; dot:string; spec:string; lockColor:string; name:string; desc:string;
}> = {
  ivory: {
    face: "linear-gradient(145deg, #fefdf8 0%, #f8f3e8 45%, #ede5d2 100%)",
    edge: "rgba(160,135,90,0.35)", dot: "#241c10", spec: "rgba(255,255,255,0.72)",
    lockColor: "#4aada6", name: "Kemik", desc: "Klasik fildişi dokulu dengeli zar seti.",
  },
  wood: {
    face: "linear-gradient(145deg, #d9b06c 0%, #c09050 45%, #9e7030 100%)",
    edge: "rgba(50,24,8,0.5)", dot: "#231006", spec: "rgba(255,210,130,0.45)",
    lockColor: "#4aada6", name: "Ahşap", desc: "Sıcak ceviz ağacından oyulmuş zarif zar.",
  },
  marble: {
    face: "linear-gradient(145deg, #f6f4f8 0%, #e8e4f0 45%, #d4cee0 100%)",
    edge: "rgba(80,60,120,0.25)", dot: "#22183c", spec: "rgba(255,255,255,0.65)",
    lockColor: "#7b5ea7", name: "Mermer", desc: "Soğuk ve pürüzsüz İtalyan mermeri.",
  },
  obsidian: {
    face: "linear-gradient(145deg, #3e3c46 0%, #2a2830 45%, #1c1a22 100%)",
    edge: "rgba(255,255,255,0.06)", dot: "#ece8ff", spec: "rgba(160,145,220,0.3)",
    lockColor: "#c9a84c", name: "Obsidyen", desc: "Volkanik siyah camdan keskin ve asil görünüm.",
  },
  ruby: {
    face: "linear-gradient(145deg, #e74c3c 0%, #b02318 48%, #6e0f09 100%)",
    edge: "rgba(255,140,140,0.35)", dot: "#fff5f5", spec: "rgba(255,190,190,0.5)",
    lockColor: "#f5c842", name: "Kızıl Yakut", desc: "Parlak yakut kristali. Ustalar arenasının favorisi.",
  },
  gold: {
    face: "linear-gradient(145deg, #ffe680 0%, #d4af37 45%, #997312 100%)",
    edge: "rgba(255,245,190,0.55)", dot: "#2a1c02", spec: "rgba(255,255,230,0.78)",
    lockColor: "#4aada6", name: "Altın Kraliyet", desc: "24 ayar saf altın kaplama efsanevi koleksiyon zarı.",
  },
};

function DieFace({ v, size, mat, locked }: { v:DieVal; size:number; mat:DieMat; locked:boolean }) {
  const m = MAT[mat];
  const r = size * 0.18;
  const ds = size * 0.135;
  return (
    <div style={{
      width:size, height:size, borderRadius:r, background: m.face,
      border: locked ? `2.5px solid ${m.lockColor}` : `1px solid ${m.edge}`,
      boxShadow: `inset 0 1px 4px rgba(255,255,255,0.55), inset 0 -1px 3px ${m.edge}`,
      position:"relative", overflow:"hidden",
      backfaceVisibility:"hidden", WebkitBackfaceVisibility:"hidden",
    }}>
      <div style={{
        position:"absolute", top:"7%", left:"9%", width:"40%", height:"28%",
        borderRadius:"50%", background:m.spec, filter:"blur(4px)", pointerEvents:"none",
      }}/>
      {DOTS[v].map(([x,y],i)=>(
        <div key={i} style={{
          position:"absolute", left:`${x}%`, top:`${y}%`, width:ds, height:ds, borderRadius:"50%",
          background:m.dot, transform:"translate(-50%,-50%)",
          boxShadow:`inset 0 1.5px 3px rgba(0,0,0,0.4), inset 0 -0.5px 1px rgba(255,255,255,0.08)`,
        }}/>
      ))}
    </div>
  );
}

// ─── 3D Die Component (FLIP Layout Bozulması Olmadan Saf GPU 3D Dönüş) ─────────
function Die3D({
  state, size, locked, mat, onClick, isRolling = false, index = 0, style, isIdle = false
}: {
  state:DieState; size:number; locked:boolean; mat:DieMat; onClick?:()=>void; isRolling?:boolean; index?:number; layoutId?:string; style?:React.CSSProperties; isIdle?:boolean;
}) {
  const h = size / 2;
  const m = MAT[mat];

  const showIdle = isIdle && !isRolling;
  const bounceY = isRolling ? [0, -18, 0, -6, 0] : 0;
  const tumbleZ = isRolling ? [0, (index % 2 === 0 ? 1 : -1) * 16, 0] : 0;

  return (
    <div
      onClick={onClick}
      style={{
        width:size, height:size, flexShrink:0, perspective:`${size*8}px`,
        cursor: onClick?"pointer":"default", position:"relative", zIndex: locked ? 10 : 1,
        willChange: "transform",
        ...style
      }}
    >
      <motion.div
        animate={
          showIdle
            ? { rotateX: [0, 360], rotateY: [0, 360], rotateZ: 0, y: 0 }
            : { rotateX: state.rx, rotateY: state.ry, rotateZ: tumbleZ, y: bounceY }
        }
        transition={
          showIdle
            ? { duration: 7 + index * 0.6, ease: "linear", repeat: Infinity }
            : { duration: 0.72, ease: [0.22, 1, 0.36, 1], delay: isRolling ? index * 0.03 : 0 }
        }
        style={{ width:"100%", height:"100%", transformStyle:"preserve-3d", position:"relative", willChange:"transform" }}
      >
        {FACE_DEFS.map(({ v, t })=>(
          <div key={v} style={{ position:"absolute", width:size, height:size, transform: t(h), backfaceVisibility:"hidden", WebkitBackfaceVisibility:"hidden" }}>
            <DieFace v={v} size={size} mat={mat} locked={locked}/>
          </div>
        ))}
      </motion.div>

      <motion.div
        animate={{ 
          opacity: locked ? 0.18 : (isRolling ? [0.26, 0.1, 0.26, 0.16, 0.26] : 0.26),
          scale: isRolling ? [1, 0.68, 1, 0.85, 1] : 1
        }}
        transition={{ duration: 0.72, delay: isRolling ? index * 0.03 : 0 }}
        style={{
          position:"absolute", bottom:-10, left:"12%", right:"12%", height:8, borderRadius:"50%",
          background:"rgba(0,0,0,1)", filter:"blur(5px)", pointerEvents:"none",
        }}
      />

      {locked && (
        <div style={{
          position:"absolute", top:-9, right:-9, width:20, height:20,
          borderRadius:"50%", background:m.lockColor, zIndex:20,
          display:"flex", alignItems:"center", justifyContent:"center",
          boxShadow:`0 2px 8px rgba(0,0,0,0.4)`,
        }}>
          <Lock size={10} color="#fff" strokeWidth={2.5}/>
        </div>
      )}
    </div>
  );
}

// ─── Avatar (Profil Resmi, Emoji Avatar ve Ülke Bayrağı Destekli) ─────────────
function Av({
  seed, size=40, active=false, dark, avatar, country
}: {
  seed:string; size?:number; active?:boolean; dark:boolean; avatar?:string; country?:string;
}) {
  const c = C(dark);
  const pal=[["#7a9e8a","#3d6050"],["#8b7855","#5e4a2c"],["#7a8fa8","#486888"],["#a07868","#6e4030"]];
  const [f,t]=pal[(seed.charCodeAt(0) || 0)%pal.length];
  const letters = seed.substring(0,2).toUpperCase() || "Ü";
  const isImg = avatar && avatar.startsWith("data:image/");
  const isEmoji = avatar && !isImg && avatar.length > 0;
  
  return (
    <div style={{
      width:size, height:size, borderRadius:"50%",
      background:`linear-gradient(145deg,${f},${t})`,
      border: active?`2px solid ${c.accent}`:`1.5px solid ${c.border}`,
      display:"flex", alignItems:"center", justifyContent:"center",
      fontSize: isEmoji ? size*0.48 : size*0.33, fontWeight:700, color:"#fff", flexShrink:0,
      boxShadow: active?`0 0 0 3px ${c.accentRing}`:undefined,
      fontFamily:APP_FONT, position:"relative", overflow:"visible"
    }}>
      {isImg ? (
        <img src={avatar} alt="avatar" style={{ width:"100%", height:"100%", borderRadius:"50%", objectFit:"cover" }} />
      ) : isEmoji ? (
        <span>{avatar}</span>
      ) : (
        letters
      )}
      {country && (
        <span style={{
          position:"absolute", top:-3, left:-4, fontSize: Math.max(10, size*0.28),
          lineHeight:1, filter:"drop-shadow(0 1px 2px rgba(0,0,0,0.5))"
        }}>
          {getCountryFlag(country)}
        </span>
      )}
    </div>
  );
}

// ─── Felt Texture & Tray Wrapper ──────────────────────────────────────────────
function FeltTexture() {
  return (
    <div className="bg-[#0c3c6461]" style={{
      position:"absolute", inset:0, borderRadius:16, pointerEvents:"none", zIndex:0,
      backgroundImage:`
        repeating-linear-gradient(0deg, transparent 0px, transparent 3px, rgba(0,0,0,0.035) 3px, rgba(0,0,0,0.035) 4px),
        repeating-linear-gradient(90deg, transparent 0px, transparent 3px, rgba(0,0,0,0.035) 3px, rgba(0,0,0,0.035) 4px)
      `,
    }}/>
  );
}

function PremiumTray({ children, dark }: { children:React.ReactNode; dark:boolean }) {
  const c = C(dark);
  return (
    <div style={{
      borderRadius: 24, background: c.trayOuter, padding: 10,
      border: `1px solid ${c.trayOuterBorder}`,
      boxShadow: `0 14px 34px rgba(0,0,0,0.5), inset 0 2px 4px rgba(255,255,255,0.08)`,
      position: "relative",
    }}>
      <div style={{
        borderRadius: 16, background: c.trayFelt, 
        border: `2px dashed rgba(255,255,255,0.18)`,
        padding: "24px 16px 18px", position: "relative", overflow: "hidden",
        boxShadow: "inset 0 12px 30px rgba(0,0,0,0.7)",
      }}>
        <FeltTexture />
        {children}
      </div>
    </div>
  );
}


// -----------------------------------------------------------
// KAYIT OL / GİRİŞ YAP / ŞİFRE SIFIRLAMA / GOOGLE & KALICI MİSAFİR EKRANI
// -----------------------------------------------------------
function getOrCreateDeviceId(): string {
  let devId = localStorage.getItem("yatzy_device_id");
  if (!devId) {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    devId = "dev_" + Array.from(bytes, x => x.toString(16).padStart(2, "0")).join("");
    localStorage.setItem("yatzy_device_id", devId);
  }
  return devId;
}

function LegalModal({ open, onClose, dark }: { open: boolean; onClose: () => void; dark: boolean }) {
  const c = C(dark);
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
          onClick={onClose}
          style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.78)", zIndex: 260, display: "flex", alignItems: "center", justifyContent: "center", padding: 18, backdropFilter: "blur(6px)" }}
        >
          <motion.div
            initial={{ scale: 0.92, y: 20 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.92, y: 20 }}
            onClick={e => e.stopPropagation()}
            style={{ background: c.panel, borderRadius: 24, width: "100%", maxWidth: 365, maxHeight: "84vh", display: "flex", flexDirection: "column", border: `1px solid ${c.borderMid}`, overflow: "hidden", boxShadow: "0 20px 60px rgba(0,0,0,0.55)" }}
          >
            <div style={{ padding: "15px 18px", borderBottom: `1px solid ${c.border}`, display: "flex", justifyContent: "space-between", alignItems: "center", background: c.panelAlt }}>
              <h3 style={{ color: c.text, fontSize: 15, fontWeight: 800, margin: 0 }}>📜 Gizlilik Politikası, EULA & Yasal Beyan</h3>
              <button onClick={onClose} style={{ background: "none", border: "none", color: c.textSub, cursor: "pointer" }}><X size={18} /></button>
            </div>
            <div style={{ padding: 16, overflowY: "auto", display: "flex", flexDirection: "column", gap: 10, fontSize: 11.5, color: c.textSub, lineHeight: 1.55 }}>
              <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                <p style={{ color: c.gold, fontWeight: 800, margin: "0 0 4px" }}>⚖️ Sanal Jeton & Kumar Yasağı Beyanı (No Gambling)</p>
                Yat Dice tamamen beceri ve stratejiye dayalı eğlence amaçlı bir oyundur. Oyun içindeki tüm <b>Jetonlar (🪙)</b> ve <b>Kasalar (📦)</b> yalnızca sanal oyun içi puandır; <b>gerçek parayla satın alınamaz, gerçek paraya, kripto varlığa veya maddi bir değere dönüştürülemez (No Real-Money Gambling / No Cash-Out).</b>
              </div>
              <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                <p style={{ color: c.accent, fontWeight: 800, margin: "0 0 4px" }}>📦 Kasa (Loot Box) Olasılık Bildirimi</p>
                Açılan her Kasa (📦) %100 olasılıkla <b>50 ile 199 arasında sanal jeton</b> kazandırır. Olasılık dağılımı tamamen eşittir (Uniform):<br />
                • <b>50 – 99 🪙 Sanal Jeton:</b> %33.3<br />
                • <b>100 – 149 🪙 Sanal Jeton:</b> %33.3<br />
                • <b>150 – 199 🪙 Sanal Jeton:</b> %33.4
              </div>
              <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                <p style={{ color: c.text, fontWeight: 800, margin: "0 0 4px" }}>🛡️ Topluluk Kuralları (UGC) & Sıfır Tolerans</p>
                Oyun içi sohbette küfür, hakaret, taciz ve nefret söylemine karşı <b>sıfır tolerans</b> uygulanır. Uygunsuz mesajlar otomatik filtrelenir; oyuncular maç esnasında rakiplerini anında <b>Susturup / Engelleyebilir (Block)</b> veya <b>Şikayet Edebilir (Report)</b>.
              </div>
              <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                <p style={{ color: c.text, fontWeight: 800, margin: "0 0 4px" }}>🔒 Gizlilik & Hesap Silme Hakkı (KVKK / GDPR)</p>
                Hesap, profil, oyun ve güvenlik verileri hizmeti sunmak için işlenir; satılmaz ve reklam takipçisi kullanılmaz. Hesabınızı <b>Profili Özelleştir &rarr; Hesabımı Kalıcı Olarak Sil</b> menüsünden silebilirsiniz.
                <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginTop: 8 }}>
                  <a href={`${API_BASE}/privacy`} target="_blank" rel="noreferrer" style={{ color: c.accent }}>Gizlilik Politikası</a>
                  <a href={`${API_BASE}/terms`} target="_blank" rel="noreferrer" style={{ color: c.accent }}>Kullanım Koşulları</a>
                  <a href={`${API_BASE}/account/delete`} target="_blank" rel="noreferrer" style={{ color: c.accent }}>Web'den Hesap Silme</a>
                </div>
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

// ─── Google Identity Services (Gerçek Google Girişi) ──────────────────────────
type GoogleIdentityApi = {
  initialize: (config: { client_id: string; callback: (response: { credential: string }) => void }) => void;
  renderButton: (parent: HTMLElement, options: Record<string, unknown>) => void;
};

let googleIdentityPromise: Promise<GoogleIdentityApi> | null = null;

function loadGoogleIdentity(): Promise<GoogleIdentityApi> {
  if (!googleIdentityPromise) {
    googleIdentityPromise = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "https://accounts.google.com/gsi/client";
      script.async = true;
      script.onload = () => {
        const api = (window as unknown as { google?: { accounts?: { id?: GoogleIdentityApi } } }).google?.accounts?.id;
        if (api) resolve(api);
        else reject(new Error("Google girişi yüklenemedi"));
      };
      script.onerror = () => {
        googleIdentityPromise = null;
        reject(new Error("Google girişi yüklenemedi"));
      };
      document.head.appendChild(script);
    });
  }
  return googleIdentityPromise;
}

function GoogleSignInButton({ dark, onCredential, onError }: {
  dark: boolean; onCredential: (credential: string) => void; onError: (message: string) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const handlersRef = useRef({ onCredential, onError });

  useEffect(() => {
    handlersRef.current = { onCredential, onError };
  });

  useEffect(() => {
    let active = true;
    loadGoogleIdentity()
      .then(api => {
        if (!active || !containerRef.current) return;
        api.initialize({
          client_id: GOOGLE_CLIENT_ID,
          callback: response => handlersRef.current.onCredential(response.credential)
        });
        api.renderButton(containerRef.current, {
          type: "standard", theme: dark ? "filled_black" : "outline", size: "large",
          text: "continue_with", shape: "rectangular", locale: "tr",
          width: Math.min(400, containerRef.current.offsetWidth)
        });
      })
      .catch((err: Error) => {
        if (active) handlersRef.current.onError(err.message);
      });
    return () => { active = false; };
  }, [dark]);

  return <div ref={containerRef} style={{ display:"flex", justifyContent:"center", minHeight:44 }} />;
}

type ResumeState = {
  dices: number[]; locked: boolean[]; rollsLeft: number; turn: "p1" | "p2"; round: number;
  p1Scores: Record<string, number>; p2Scores: Record<string, number>;
  p1Total: number; p2Total: number; secondsLeft: number;
};
type ResumedMatch = { playerIndex: number; opponentName: string; betAmount: number; state: ResumeState };
type BlockedUser = { userId: number; username: string };

const TERMS_REQUIRED_TEXT = "Devam etmek için kutuyu işaretleyerek koşulları kabul etmelisiniz.";

function AuthScreen({ onLogin, dark }: { onLogin: (username: string, token: string) => void, dark: boolean }) {
  const c = C(dark);
  const [authMode, setAuthMode] = useState<"login" | "register" | "forgot" | "reset">("login");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [resetCode, setResetCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState("");
  const [infoMsg, setInfoMsg] = useState("");
  const [loading, setLoading] = useState(false);
  const [showLegal, setShowLegal] = useState(false);
  const [acceptedTerms, setAcceptedTerms] = useState(() => localStorage.getItem("yatzy_terms_accepted") === "yes");

  const updateAcceptedTerms = (value: boolean) => {
    setAcceptedTerms(value);
    localStorage.setItem("yatzy_terms_accepted", value ? "yes" : "no");
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setInfoMsg("");
    if (authMode === "register" && !acceptedTerms) {
      setError(TERMS_REQUIRED_TEXT);
      return;
    }
    setLoading(true);

    try {
      if (authMode === "login" || authMode === "register") {
        const endpoint = authMode === "login" ? "/login" : "/register";
        const res = await fetch(`${API_BASE}${endpoint}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username, password, email, acceptedTerms })
        });
        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || "Bir hata oluştu");
        }
        if (authMode === "login") {
          const data = await res.json();
          onLogin(data.username, data.token);
        } else {
          setInfoMsg("🎉 Kayıt başarılı! Şimdi giriş yapabilirsiniz.");
          setAuthMode("login");
          setPassword("");
        }
      } else if (authMode === "forgot") {
        const res = await fetch(`${API_BASE}/auth/forgot-password`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username, email })
        });
        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || "Kod oluşturulamadı");
        }
        const data = await res.json();
        if (data.resetCode) {
          setResetCode(data.resetCode);
          setInfoMsg(`Doğrulama kodunuz oluşturuldu: ${data.resetCode}`);
        } else {
          setInfoMsg(data.message || "Bilgiler eşleşiyorsa doğrulama kodu e-postanıza gönderildi.");
        }
        setAuthMode("reset");
      } else if (authMode === "reset") {
        const res = await fetch(`${API_BASE}/auth/reset-password`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username, resetCode, newPassword })
        });
        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || "Şifre sıfırlanamadı");
        }
        setInfoMsg("✅ Şifreniz başarıyla sıfırlandı! Giriş yapabilirsiniz.");
        setPassword("");
        setNewPassword("");
        setAuthMode("login");
      }
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  // Aynı cihazda her zaman aynı Misafir hesabına bağlanır
  const handleGuestLogin = async () => {
    setError("");
    if (!acceptedTerms) {
      setError(TERMS_REQUIRED_TEXT);
      return;
    }
    setLoading(true);
    const deviceId = getOrCreateDeviceId();
    try {
      const res = await fetch(`${API_BASE}/auth/guest`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ deviceId, acceptedTerms })
      });
      if (res.ok) {
        const data = await res.json();
        localStorage.setItem("yatzy_guest_name", data.username);
        onLogin(data.username, data.token);
        return;
      }
      setError((await res.text()) || "Misafir girişi başarısız");
    } catch {
      setError("Sunucuya ulaşılamadı. Bağlantını kontrol edip tekrar dene.");
    } finally {
      setLoading(false);
    }
  };

  const handleGoogleCredential = async (credential: string) => {
    setError("");
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/auth/google`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ credential, acceptedTerms })
      });
      if (!res.ok) {
        const txt = await res.text();
        throw new Error(txt || "Google girişi başarısız");
      }
      const data = await res.json();
      onLogin(data.username, data.token);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ height:"100%", overflowY:"auto", display:"flex", flexDirection:"column", justifyContent:"center", padding:24, background:c.bg, color:c.text, fontFamily:APP_FONT }}>
      <div style={{ display:"flex", justifyContent:"center", gap:8, marginBottom: 24 }}>
        {([3,1,5,2,4] as DieVal[]).map((v,i)=>(
          <motion.div key={i} initial={{ y: -50, opacity: 0, rotate: -180 }} animate={{ y: 0, opacity: 1, rotate: 0 }} transition={{ delay: i * 0.1, type: "spring", stiffness: 200, damping: 15 }}>
            <Die3D index={i} state={initDie(v, i)} size={38} locked={false} mat="ivory"/>
          </motion.div>
        ))}
      </div>

      <div style={{ textAlign:"center", marginBottom:22 }}>
        <h1 style={{ fontSize:34, fontWeight:900, letterSpacing:"-0.02em", margin:0 }}>
          Yat <span style={{ color:c.accent }}>Dice</span>
        </h1>
        <p style={{ fontSize:13, color:c.textSub, marginTop:6 }}>
          {authMode === "login" && "Hesabınıza giriş yapın"}
          {authMode === "register" && "Yeni bir hesap oluşturun"}
          {authMode === "forgot" && "Şifre sıfırlama kodu alın"}
          {authMode === "reset" && "Yeni şifrenizi belirleyin"}
        </p>
      </div>

      <form onSubmit={handleSubmit} style={{ display:"flex", flexDirection:"column", gap:13, background:c.panel, padding:22, borderRadius:24, border:`1px solid ${c.border}`, boxShadow:`0 8px 32px ${c.shadow}` }}>
        <div>
          <label style={{ fontSize:12, fontWeight:700, color:c.textSub, marginBottom:6, display:"block" }}>Kullanıcı Adı</label>
          <input type="text" value={username} onChange={e => setUsername(e.target.value)} required placeholder="Örn: Utku" style={{ width:"100%", boxSizing:"border-box", padding:"13px 15px", borderRadius:12, border:`1px solid ${c.border}`, background:c.bg, color:c.text, fontSize:14, outline:"none", fontWeight:600 }} />
        </div>

        {(authMode === "register" || authMode === "forgot") && (
          <div>
            <label style={{ fontSize:12, fontWeight:700, color:c.textSub, marginBottom:6, display:"block" }}>
              E-posta {authMode === "register" ? "(Şifre kurtarma için gerekli)" : "(Hesabınızdaki adres)"}
            </label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="ornek@gmail.com" style={{ width:"100%", boxSizing:"border-box", padding:"13px 15px", borderRadius:12, border:`1px solid ${c.border}`, background:c.bg, color:c.text, fontSize:14, outline:"none", fontWeight:600 }} />
          </div>
        )}

        {(authMode === "login" || authMode === "register") && (
          <div>
            <div style={{ display:"flex", justifyContent:"space-between", alignItems:"center", marginBottom:6 }}>
              <label style={{ fontSize:12, fontWeight:700, color:c.textSub }}>Şifre</label>
              {authMode === "login" && (
                <button type="button" onClick={() => { setAuthMode("forgot"); setError(""); setInfoMsg(""); }} style={{ background:"none", border:"none", color:c.accent, fontSize:11, fontWeight:700, cursor:"pointer", padding:0 }}>
                  Şifremi Unuttum?
                </button>
              )}
            </div>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)} required placeholder="••••••••" style={{ width:"100%", boxSizing:"border-box", padding:"13px 15px", borderRadius:12, border:`1px solid ${c.border}`, background:c.bg, color:c.text, fontSize:14, outline:"none", fontWeight:600 }} />
          </div>
        )}

        {authMode === "reset" && (
          <>
            <div>
              <label style={{ fontSize:12, fontWeight:700, color:c.textSub, marginBottom:6, display:"block" }}>6 Haneli Sıfırlama Kodu</label>
              <input type="text" value={resetCode} onChange={e => setResetCode(e.target.value)} required placeholder="123456" maxLength={6} style={{ width:"100%", boxSizing:"border-box", padding:"13px 15px", borderRadius:12, border:`1px solid ${c.border}`, background:c.bg, color:c.accent, fontSize:16, letterSpacing:4, outline:"none", fontWeight:800 }} />
            </div>
            <div>
              <label style={{ fontSize:12, fontWeight:700, color:c.textSub, marginBottom:6, display:"block" }}>Yeni Şifre (En az 6 karakter)</label>
              <input type="password" value={newPassword} onChange={e => setNewPassword(e.target.value)} required placeholder="••••••••" style={{ width:"100%", boxSizing:"border-box", padding:"13px 15px", borderRadius:12, border:`1px solid ${c.border}`, background:c.bg, color:c.text, fontSize:14, outline:"none", fontWeight:600 }} />
            </div>
          </>
        )}

        {error && (
          <div style={{ color:"#ff4757", fontSize:12, fontWeight:700, textAlign:"center", background:"rgba(255, 71, 87, 0.1)", padding:"10px", borderRadius:8 }}>{error}</div>
        )}
        {infoMsg && (
          <div style={{ color:c.accent, fontSize:12, fontWeight:700, textAlign:"center", background:c.accentBg, padding:"10px", borderRadius:8, border:`1px solid ${c.accentRing}` }}>{infoMsg}</div>
        )}

        <motion.button whileTap={!loading ? {scale:0.97} : {}} disabled={loading} type="submit" style={{ padding:"15px", marginTop:4, borderRadius:12, background:c.accent, color:dark?c.bg:"#fff", border:"none", fontWeight:800, fontSize:14, cursor:loading?"not-allowed":"pointer", opacity:loading?0.7:1, display:"flex", justifyContent:"center" }}>
          {loading ? <motion.div animate={{ rotate: 360 }} transition={{ duration: 1, repeat: Infinity, ease: "linear" }}><RotateCcw size={18}/></motion.div> : (
            authMode === "login" ? "GİRİŞ YAP" :
            authMode === "register" ? "KAYIT OL" :
            authMode === "forgot" ? "SIFIRLAMA KODU AL" : "ŞİFREYİ GÜNCELLE"
          )}
        </motion.button>

        {(authMode === "login" || authMode === "register") && (
          <>
            <label style={{ display:"flex", alignItems:"flex-start", gap:8, fontSize:11, lineHeight:1.4, color:c.textSub, cursor:"pointer" }}>
              <input type="checkbox" checked={acceptedTerms} onChange={e => updateAcceptedTerms(e.target.checked)} style={{ marginTop:1, accentColor:c.accent }} />
              <span>
                13 yaşından büyüğüm; <a href={`${API_BASE}/terms`} target="_blank" rel="noreferrer" style={{ color:c.accent }}>Kullanım Koşulları</a>'nı ve <a href={`${API_BASE}/privacy`} target="_blank" rel="noreferrer" style={{ color:c.accent }}>Gizlilik Politikası</a>'nı kabul ediyorum.
              </span>
            </label>

            {GOOGLE_CLIENT_ID && acceptedTerms && (
              <GoogleSignInButton dark={dark} onCredential={handleGoogleCredential} onError={setError} />
            )}

            <button type="button" onClick={handleGuestLogin} style={{ padding:"13px", borderRadius:12, background:c.panelAlt, color:c.text, border:`1px solid ${c.border}`, fontWeight:700, fontSize:13, cursor:"pointer" }}>
              🎲 Misafir Olarak Devam Et
            </button>
          </>
        )}
      </form>

      <div style={{ textAlign:"center", marginTop:14, display:"flex", flexDirection:"column", alignItems:"center", gap:6 }}>
        {authMode === "login" ? (
          <button type="button" onClick={() => { setAuthMode("register"); setError(""); setInfoMsg(""); }} style={{ background:"none", border:"none", color:c.textSub, fontSize:13, fontWeight:600, cursor:"pointer", padding:4 }}>
            Hesabın yok mu? <span style={{ color:c.accent, fontWeight:800 }}>Kayıt Ol</span>
          </button>
        ) : (
          <button type="button" onClick={() => { setAuthMode("login"); setError(""); setInfoMsg(""); }} style={{ background:"none", border:"none", color:c.textSub, fontSize:13, fontWeight:600, cursor:"pointer", padding:4 }}>
            Giriş ekranına dön → <span style={{ color:c.accent, fontWeight:800 }}>Giriş Yap</span>
          </button>
        )}
        <button
          type="button"
          onClick={() => setShowLegal(true)}
          style={{ background: "none", border: "none", color: c.textSub, fontSize: 10.5, cursor: "pointer", textDecoration: "underline", padding: 2 }}
        >
          📜 Gizlilik Politikası, Kullanım Koşulları (EULA) ve Yasal Beyan
        </button>
      </div>
      <LegalModal open={showLegal} onClose={() => setShowLegal(false)} dark={dark} />
    </div>
  );
}


// ─── Screen 1: Home ───────────────────────────────────────────────────────────
function HomeScreen({
  dark, toggle, profile, setProfile, isSearching, connectToServer, createdRoomCode, cancelMatchmaking, onLogout, token, profileLoaded, friends, onSendInvite, betAmount, setBetAmount
}: {
  go:(s:Screen)=>void; dark:boolean; toggle:()=>void; profile:UserProfile; setProfile:React.Dispatch<React.SetStateAction<UserProfile>>;
  isSearching:boolean; connectToServer:(action:"QUICK_MATCH"|"BOT_MATCH"|"CREATE_ROOM"|"JOIN_ROOM", code?:string)=>void;
  createdRoomCode:string|null; cancelMatchmaking:()=>void; onLogout:()=>void; token:string|null; profileLoaded:boolean;
  friends:FriendItem[]; onSendInvite:(friendUserId:number, roomCode:string)=>void;
  betAmount:BetOption; setBetAmount:(b:BetOption)=>void;
}) {
  const c = C(dark);
  const [joinCode, setJoinCode] = useState("");
  const [isEditing, setIsEditing] = useState(false);
  const [tempName, setTempName] = useState(profile.name);
  const [tempAvatar, setTempAvatar] = useState(profile.avatar || "");
  const [tempCountry, setTempCountry] = useState(profile.country || "TR");
  
  const [showDailyPopup, setShowDailyPopup] = useState(false);
  const [pendingReward, setPendingReward] = useState({ xp: 0, tokens: 0, streak: 1 });
  const [caseAnim, setCaseAnim] = useState<{status: "shaking" | "revealed", reward: number} | null>(null);
  const [soundOn, setSoundOn] = useState(() => localStorage.getItem("yatzy_sound") !== "off");
  const [showLeaderboard, setShowLeaderboard] = useState(false);
  const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);
  const [loadingLb, setLoadingLb] = useState(false);
  const [showRulesModal, setShowRulesModal] = useState(false);
  const [showLegalModal, setShowLegalModal] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deletingAccount, setDeletingAccount] = useState(false);

  const handleDeleteMyAccount = async () => {
    if (deletingAccount) return;
    setDeletingAccount(true);
    try {
      if (token && !token.startsWith("guest_token_")) {
        const res = await fetch(`${API_BASE}/profile/delete`, {
          method: "POST",
          headers: { "Authorization": "Bearer " + token }
        });
        if (!res.ok) {
          const msg = await res.text();
          emitToast(msg || "Hesap silinemedi");
          setDeletingAccount(false);
          return;
        }
      }
      localStorage.removeItem("yatzy_jwt_token");
      localStorage.removeItem("yatzy_username");
      localStorage.removeItem("yatzy_guest_saved_profile");
      localStorage.removeItem("yatzy_guest_name");
      localStorage.removeItem("yatzy_device_id");
      setIsEditing(false);
      setShowDeleteConfirm(false);
      emitToast("Hesabınız ve tüm verileriniz kalıcı olarak silindi.");
      onLogout();
    } catch {
      emitToast("Sunucuya ulaşılamadı.");
    } finally {
      setDeletingAccount(false);
    }
  };

  const toggleSound = () => {
    const next = !soundOn;
    setSoundOn(next);
    localStorage.setItem("yatzy_sound", next ? "on" : "off");
    if (next) sfx.lock();
  };

  const openLeaderboard = async () => {
    setShowLeaderboard(true);
    setLoadingLb(true);
    try {
      const res = await fetch(`${API_BASE}/leaderboard`);
      if (res.ok) {
        const data = await res.json();
        setLeaderboard(data.leaders || []);
      }
    } catch {
    } finally {
      setLoadingLb(false);
    }
  };

  useEffect(() => {
    setTempName(profile.name);
    setTempAvatar(profile.avatar || "");
    setTempCountry(profile.country || "TR");
  }, [profile.name, profile.avatar, profile.country]);

  const handleAvatarFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const img = new Image();
      img.onload = () => {
        const canvas = document.createElement("canvas");
        canvas.width = 128;
        canvas.height = 128;
        const ctx = canvas.getContext("2d");
        if (ctx) {
          const minSide = Math.min(img.width, img.height);
          const sx = (img.width - minSide) / 2;
          const sy = (img.height - minSide) / 2;
          ctx.drawImage(img, sx, sy, minSide, minSide, 0, 0, 128, 128);
          setTempAvatar(canvas.toDataURL("image/jpeg", 0.82));
        }
      };
      img.src = reader.result as string;
    };
    reader.readAsDataURL(file);
  };

  // SAYFA YÜKLENDİĞİNDE GÜNLÜK ÖDÜLÜ KONTROL ET (Sadece profil veritabanından yüklendikten sonra!)
  useEffect(() => {
    if (!profileLoaded) return;
    const today = new Date().toISOString().split('T')[0];
    
    if (profile.lastDailyReward !== today) {
      let newStreak = 1;
      if (profile.lastDailyReward) {
        const lastDate = new Date(profile.lastDailyReward);
        const currentDate = new Date(today);
        const diffDays = Math.round(Math.abs(currentDate.getTime() - lastDate.getTime()) / (1000 * 60 * 60 * 24));
        if (diffDays === 1) {
          newStreak = profile.loginStreak + 1; 
        }
      }
      const rewardXp = 100 + (Math.min(newStreak, 30) * 20);
      const rewardTokens = 50 + (Math.min(newStreak, 30) * 10);
      
      setPendingReward({ xp: rewardXp, tokens: rewardTokens, streak: newStreak });
      setShowDailyPopup(true);
    }
  }, [profileLoaded, profile.lastDailyReward, profile.loginStreak]);

  const claimDailyReward = async () => {
    const today = new Date().toISOString().split('T')[0];
    if (token && !token.startsWith("guest_token_")) {
      try {
        const res = await fetch(`${API_BASE}/profile/claim-daily`, {
          method: "POST",
          headers: { "Authorization": "Bearer " + token }
        });
        if (res.ok) {
          const data = await res.json();
          const p = data.profile;
          setProfile(prev => ({
            ...prev,
            xp: p.xp,
            level: computeLevel(p.xp),
            tokens: p.tokens,
            loginStreak: p.loginStreak,
            lastDailyReward: p.lastDailyReward
          }));
        }
      } catch (e) {
        console.error("Günlük ödül hatası:", e);
      }
    } else {
      setProfile(prev => ({
        ...prev,
        xp: prev.xp + pendingReward.xp,
        level: computeLevel(prev.xp + pendingReward.xp),
        tokens: prev.tokens + pendingReward.tokens,
        lastDailyReward: today,
        loginStreak: pendingReward.streak
      }));
    }
    setShowDailyPopup(false);
  };

  const saveProfile = async () => {
    const trimmed = tempName.trim();
    if (trimmed.length >= 2) {
      if (token && !token.startsWith("guest_token_")) {
        const res = await fetch(`${API_BASE}/profile/update`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + token
          },
          body: JSON.stringify({
            username: trimmed,
            avatar: tempAvatar,
            country: tempCountry
          })
        });
        if (!res.ok) {
          const msg = await res.text();
          emitToast(msg || "Profil güncellenemedi");
          return;
        }
      }
      localStorage.setItem("yatzy_username", trimmed);
      setProfile(prev => ({
        ...prev,
        name: trimmed,
        seed: trimmed.substring(0,2).toUpperCase(),
        avatar: tempAvatar,
        country: tempCountry
      }));
    }
    setIsEditing(false);
  };

  const openCase = async () => {
    if (profile.cases <= 0 || caseAnim) return;
    sfx.caseShake();

    if (token && !token.startsWith("guest_token_")) {
      try {
        const res = await fetch(`${API_BASE}/profile/open-case`, {
          method: "POST",
          headers: { "Authorization": "Bearer " + token }
        });
        if (!res.ok) return;
        const data = await res.json();
        setCaseAnim({ status: "shaking", reward: data.reward });
        setTimeout(() => {
          sfx.win();
          setCaseAnim(prev => prev ? { ...prev, status: "revealed" } : null);
          setProfile(prev => ({
            ...prev,
            cases: data.profile.cases,
            tokens: data.profile.tokens
          }));
        }, 1600);
      } catch (e) {
        console.error("Kasa açma hatası:", e);
      }
    } else {
      const reward = Math.floor(Math.random() * 150) + 50;
      setCaseAnim({ status: "shaking", reward });
      setTimeout(() => {
        sfx.win();
        setCaseAnim(prev => prev ? { ...prev, status: "revealed" } : null);
        setProfile(prev => ({
          ...prev,
          cases: prev.cases - 1,
          tokens: prev.tokens + reward
        }));
      }, 1600);
    }
  };

  const claimCaseReward = () => {
    if (caseAnim && caseAnim.status === "revealed") {
      setCaseAnim(null);
    }
  };

  const winRate = profile.stats.gamesPlayed > 0 
    ? Math.round((profile.stats.wins / profile.stats.gamesPlayed) * 100) 
    : 0;

  const xpInLevel = profile.xp % 1000;

  return (
    <div style={{ minHeight:"100%", background:c.bg, fontFamily:APP_FONT, display:"flex", flexDirection:"column", paddingBottom: 32 }}>
      {/* Topbar */}
      <div style={{ display:"flex", alignItems:"center", justifyContent:"space-between", padding:"28px 20px 14px" }}>
        <div onClick={()=>setIsEditing(true)} style={{ display:"flex", alignItems:"center", gap:12, cursor:"pointer", padding: "4px 8px 4px 0", borderRadius: 12, transition:"background 0.2s" }}>
          <div style={{ position:"relative" }}>
            <Av seed={profile.seed} size={46} dark={dark} avatar={profile.avatar} country={profile.country}/>
            <div style={{ position:"absolute", bottom:-2, right:-4, background:c.gold, color:"#fff", fontSize:9, fontWeight:700, borderRadius:6, padding:"1px 5px", border:`1.5px solid ${c.bg}` }}>{profile.level}</div>
          </div>
          <div>
            <p style={{ fontSize:11, color:c.textSub, display:"flex", alignItems:"center", gap:4 }}>Hoşgeldin <Edit3 size={10}/></p>
            <p style={{ fontSize:16, color:c.text, fontWeight:700 }}>{profile.name}</p>
          </div>
        </div>
        
        <div style={{ display:"flex", alignItems:"center", gap:8 }}>
          <div style={{ display:"flex", alignItems:"center", gap:5, background:c.goldBg, border:`1px solid ${c.gold}30`, borderRadius:20, padding:"5px 11px" }}>
            <span style={{ fontSize:14 }}>🪙</span>
            <span style={{ fontSize:13, fontWeight:800, color:c.gold }}>{profile.tokens}</span>
          </div>
          <button onClick={toggleSound} title={soundOn ? "Sesi Kapat" : "Sesi Aç"} style={{ background:c.panelAlt, border:`1px solid ${c.border}`, borderRadius:10, padding:"7px 9px", cursor:"pointer", display:"flex", fontSize:14 }}>
            {soundOn ? "🔊" : "🔇"}
          </button>
          <button onClick={toggle} style={{ background:c.panelAlt, border:`1px solid ${c.border}`, borderRadius:10, padding:8, cursor:"pointer", display:"flex" }}>
            {dark ? <Sun size={16} color={c.accent}/> : <Moon size={16} color={c.accent}/>}
          </button>
        </div>
      </div>

      <div style={{ display:"flex", justifyContent:"center", gap:8, padding:"4px 20px 20px" }}>
        {([3,1,5,2,4] as DieVal[]).map((v,i)=>(
          <Die3D key={i} index={i} state={initDie(v, i)} size={44} locked={false} mat="ivory"/>
        ))}
      </div>

      <div style={{ textAlign:"center", paddingBottom:24 }}>
        <h1 style={{ fontSize:24, fontWeight:800, color:c.text, letterSpacing:"-0.02em" }}>
          Yat <span style={{ color:c.accent }}>Dice</span>
        </h1>
        <p style={{ fontSize:11, color:c.textSub, marginTop:2 }}>Zar at. Puan yap. Kazan.</p>
      </div>

      {/* XP ÇUBUĞU */}
      <div style={{ padding:"0 20px 18px" }}>
        <div style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:14, padding:"13px 16px", boxShadow:`0 2px 8px ${c.shadow}` }}>
          <div style={{ display:"flex", justifyContent:"space-between", marginBottom:7 }}>
            <span style={{ fontSize:11, color:c.textSub }}>Seviye {profile.level}</span>
            <span style={{ fontSize:11, color:c.accent, fontWeight:600 }}>{xpInLevel} / 1.000 XP (Top: {profile.xp})</span>
          </div>
          <div style={{ height:5, background:c.panelAlt, borderRadius:99, overflow:"hidden" }}>
            <motion.div initial={{ width:0 }} animate={{ width:`${(xpInLevel/1000)*100}%` }} transition={{ duration:1.2, ease:"easeOut", delay:0.3 }}
              style={{ height:"100%", background:c.accent, borderRadius:99 }}/>
          </div>
        </div>
      </div>

      {/* KASA AÇ BUTONU + OLASILIK BİLDİRİMİ (Apple Guideline 3.1.1 Uyumlu) */}
      <div style={{ padding:"0 20px 14px", display:"flex", flexDirection:"column", gap:5 }}>
        <motion.button whileTap={profile.cases>0 ? {scale:0.96} : {}} onClick={openCase} style={{
          width:"100%", padding:"15px", background: profile.cases>0 ? c.goldBg : c.panelAlt, 
          border:`1px solid ${profile.cases>0 ? c.gold : c.border}`, borderRadius:14, cursor: profile.cases>0 ? "pointer" : "not-allowed",
          display:"flex", alignItems:"center", justifyContent:"center", gap:10
        }}>
           <span style={{ fontSize:24, filter: profile.cases===0?"grayscale(100%) opacity(0.5)":"" }}>📦</span>
           <span style={{ fontSize:14, fontWeight:700, color: profile.cases>0 ? c.gold : c.textSub }}>
             {profile.cases > 0 ? `${profile.cases} Kasa Açılmayı Bekliyor` : "Kasa Yok"}
           </span>
        </motion.button>
        <span style={{ fontSize:9.5, color:c.textSub, textAlign:"center" }}>
          Kasa olasılığı (sanal jeton): 50–99 %33,3 · 100–149 %33,3 · 150–199 %33,4
        </span>
      </div>

      {/* MASA GİRİŞ JETONU SEÇİCİ (Mağaza Politikalarına Uygun Sanal Turnuva Girişi) */}
      <div style={{ padding:"0 20px 14px" }}>
        <div style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:14, padding:"11px 14px" }}>
          <div style={{ display:"flex", justifyContent:"space-between", alignItems:"center", marginBottom:8 }}>
            <span style={{ fontSize:10, fontWeight:800, color:c.textSub, textTransform:"uppercase", letterSpacing:"0.06em", whiteSpace:"nowrap" }}>
              Masa Giriş Jetonu
            </span>
            <span style={{ fontSize:11, fontWeight:800, color:c.gold, whiteSpace:"nowrap" }}>
              {betAmount === 0 ? "Dostluk Maçı" : `Ödül: ${betAmount * 2} 🪙`}
            </span>
          </div>
          <div style={{ display:"grid", gridTemplateColumns:"repeat(5, 1fr)", gap:6 }}>
            {BET_OPTIONS.map(b => {
              const active = betAmount === b;
              const canAfford = profile.tokens >= b;
              return (
                <button
                  key={b}
                  type="button"
                  disabled={!canAfford || isSearching || createdRoomCode !== null}
                  onClick={() => setBetAmount(b)}
                  style={{
                    padding:"7px 4px", borderRadius:10, fontSize:11, fontWeight:800, whiteSpace:"nowrap",
                    cursor: (!canAfford || isSearching || createdRoomCode !== null) ? "not-allowed" : "pointer",
                    background: active ? c.gold : c.panelAlt,
                    color: active ? "#1a1405" : (canAfford ? c.text : c.textFaint),
                    border: active ? `1.5px solid ${c.gold}` : `1px solid ${c.border}`,
                    opacity: canAfford ? 1 : 0.45,
                    transition: "0.15s"
                  }}
                >
                  {b === 0 ? "Ücretsiz" : `🪙 ${b}`}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      {/* OYUN MODLARI */}
      <div style={{ padding:"0 20px", display:"flex", flexDirection:"column", gap:10 }}>
        <div style={{ display: "flex", gap: 8 }}>
          <motion.button 
            whileTap={!isSearching && !createdRoomCode ? { scale:0.97 } : {}} 
            onClick={() => connectToServer("QUICK_MATCH")} 
            disabled={isSearching || createdRoomCode !== null}
            style={{ flex:1, padding:"16px", background:c.accent, color:dark?c.bg:"#fff", fontSize:14, fontWeight:800, borderRadius:14, border:"none", cursor:isSearching||createdRoomCode ? "not-allowed" : "pointer", display:"flex", alignItems:"center", justifyContent:"center", gap:8, opacity: isSearching||createdRoomCode ? 0.6 : 1 }}>
            {isSearching && !createdRoomCode ? (
              <motion.div animate={{ rotate: 360 }} transition={{ duration: 1, repeat: Infinity, ease: "linear" }} style={{ display:"flex" }}><RotateCcw size={17} strokeWidth={2.5}/></motion.div>
            ) : <Zap size={17} strokeWidth={2.5}/>}
            {isSearching && !createdRoomCode ? "ARANIYOR..." : "HIZLI MAÇ"}
          </motion.button>

          {!isSearching && !createdRoomCode && (
            <motion.button
              whileTap={{ scale: 0.97 }}
              onClick={() => connectToServer("BOT_MATCH")}
              style={{
                flex: 1, padding: "16px", background: c.panel, color: c.text,
                fontSize: 14, fontWeight: 800, borderRadius: 14,
                border: `1.5px solid ${c.accent}`, cursor: "pointer",
                display: "flex", alignItems: "center", justifyContent: "center", gap: 7
              }}
            >
              <span>🤖</span> BOTA KARŞI
            </motion.button>
          )}

          {isSearching && !createdRoomCode && (
            <motion.button
               initial={{ opacity: 0, scale: 0.8 }} animate={{ opacity: 1, scale: 1 }} whileTap={{ scale: 0.9 }}
               onClick={cancelMatchmaking}
               style={{ width: 56, background: c.panelAlt, border: `1px solid ${c.border}`, borderRadius: 14, color: c.text, display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer" }}
            >
               <X size={20} />
            </motion.button>
          )}
        </div>

        <div style={{ display:"flex", alignItems:"center", gap:8, margin:"16px 0" }}>
            <div style={{flex:1, height:1, background:c.border}}></div>
            <span style={{fontSize:10, color:c.textFaint, fontWeight:"bold", letterSpacing:"0.1em"}}>ÖZEL ODA</span>
            <div style={{flex:1, height:1, background:c.border}}></div>
        </div>
        
        {createdRoomCode ? (
           <div style={{ background: c.panelAlt, border: `1px dashed ${c.accent}`, borderRadius: 14, padding: "16px", textAlign: "center", position: "relative" }}>
              <button onClick={cancelMatchmaking} style={{ position: "absolute", top: 12, right: 12, background: "none", border: "none", color: c.textSub, cursor: "pointer", display: "flex" }}>
                  <X size={18} />
              </button>
              
              <p style={{ fontSize: 11, color: c.textSub, marginBottom: 4, fontWeight: 600 }}>Oda Kodunuz (Arkadaşınıza Gönderin):</p>
              <h2 style={{ letterSpacing: 8, color: c.accent, margin:"8px 0" }}>{createdRoomCode}</h2>
              <div style={{ display:"flex", justifyContent:"center", gap: 8, marginTop: 10, alignItems:"center" }}>
                 <motion.div animate={{ rotate: 360 }} transition={{ duration: 1, repeat: Infinity, ease: "linear" }} style={{ display:"flex" }}>
                   <RotateCcw size={14} color={c.accent}/>
                 </motion.div>
                 <span style={{ fontSize:11, color:c.accent, fontWeight:600 }}>Arkadaşınız bekleniyor...</span>
              </div>

              {friends.length > 0 && (
                <div style={{ marginTop: 14, borderTop: `1px solid ${c.border}`, paddingTop: 10, textAlign: "left" }}>
                  <p style={{ fontSize: 10, color: c.textSub, fontWeight: 700, marginBottom: 6, textTransform: "uppercase" }}>Arkadaşını Odaya Davet Et:</p>
                  <div style={{ display: "flex", flexDirection: "column", gap: 6, maxHeight: 110, overflowY: "auto" }}>
                    {friends.map(f => (
                      <div key={f.userId} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", background: c.panel, padding: "6px 10px", borderRadius: 10, border: `1px solid ${c.border}` }}>
                        <span style={{ fontSize: 12, fontWeight: 700, color: c.text }}>{f.username}</span>
                        <button
                          onClick={() => onSendInvite(f.userId, createdRoomCode)}
                          style={{ background: c.accent, color: dark ? c.bg : "#fff", border: "none", borderRadius: 8, padding: "4px 10px", fontSize: 11, fontWeight: 700, cursor: "pointer" }}
                        >
                          Davet Et
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              )}
           </div>
        ) : (
           <div style={{ display:"flex", flexDirection:"column", gap:10 }}>
              <motion.button 
                whileTap={!isSearching ? { scale:0.97 } : {}} 
                onClick={() => connectToServer("CREATE_ROOM")} 
                disabled={isSearching}
                style={{ width:"100%", padding:"15px", background:c.panelAlt, color:c.text, fontSize:14, fontWeight:700, borderRadius:12, border:`1px solid ${c.border}`, cursor:isSearching?"not-allowed":"pointer", display:"flex", alignItems:"center", justifyContent:"center", gap:8, opacity: isSearching ? 0.6 : 1 }}>
                <Users size={17}/> ODA KUR
              </motion.button>

              <div style={{ display:"flex", gap:8 }}>
                  <input 
                    type="text" 
                    placeholder="KOD GİRİN (Örn: X7K9A)" 
                    maxLength={5}
                    value={joinCode}
                    onChange={(e) => setJoinCode(e.target.value.toUpperCase())}
                    style={{ flex:1, background:c.bg, border:`1px solid ${c.border}`, padding:"0 16px", borderRadius:12, color:c.text, fontSize:13, fontWeight:"bold", textTransform:"uppercase", outline:"none" }}
                  />
                  <motion.button 
                    whileTap={!isSearching && joinCode.length === 5 ? { scale:0.97 } : {}} 
                    onClick={() => joinCode.length === 5 && connectToServer("JOIN_ROOM", joinCode)}
                    disabled={isSearching || joinCode.length !== 5}
                    style={{ padding:"14px 20px", background: joinCode.length === 5 ? c.accent : c.panelAlt, color: joinCode.length === 5 ? (dark?c.bg:"#fff") : c.textFaint, borderRadius:12, border:`1px solid ${joinCode.length === 5 ? "transparent" : c.border}`, cursor: joinCode.length === 5 ? "pointer" : "not-allowed", fontWeight:800, fontSize:13 }}
                  >KATIL</motion.button>
              </div>
           </div>
        )}
      </div>

      {/* DİNAMİK İSTATİSTİKLER */}
      <div style={{ padding:"18px 20px 0" }}>
        <p style={{ fontSize:9, letterSpacing:"0.1em", textTransform:"uppercase", color:c.textFaint, fontWeight:600, marginBottom:10 }}>İstatistikler</p>
        <div style={{ display:"grid", gridTemplateColumns:"repeat(3,1fr)", gap:8 }}>
          {[
            ["Galibiyet", profile.stats.wins],
            ["Oran", `%${winRate}`],
            ["Rekor", profile.stats.highScore]
          ].map(([l,v])=>(
            <div key={l} style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:12, padding:"12px 10px", textAlign:"center" }}>
              <p style={{ fontSize:19, fontWeight:700, color:c.accent }}>{v}</p>
              <p style={{ fontSize:10, color:c.textSub, marginTop:2 }}>{l}</p>
            </div>
          ))}
        </div>

        {/* LİDERLİK TABLOSU VE OYUN KURALLARI HIZLI ERİŞİM BUTONLARI */}
        <div style={{ display:"grid", gridTemplateColumns:"1fr 1fr", gap:8, marginTop:10 }}>
          <motion.button
            whileTap={{ scale: 0.97 }}
            onClick={openLeaderboard}
            style={{
              background: c.panel, border: `1px solid ${c.border}`, borderRadius: 12,
              padding: "11px 12px", cursor: "pointer", display: "flex",
              alignItems: "center", justifyContent: "center", gap: 6,
              color: c.text, fontSize: 12, fontWeight: 700
            }}
          >
            <span>🏆</span> Liderlik Tablosu
          </motion.button>
          <motion.button
            whileTap={{ scale: 0.97 }}
            onClick={() => setShowRulesModal(true)}
            style={{
              background: c.panel, border: `1px solid ${c.border}`, borderRadius: 12,
              padding: "11px 12px", cursor: "pointer", display: "flex",
              alignItems: "center", justifyContent: "center", gap: 6,
              color: c.text, fontSize: 12, fontWeight: 700
            }}
          >
            <span>📖</span> Kurallar & Puanlar
          </motion.button>
        </div>
      </div>

      {/* LİDERLİK TABLOSU MODALI */}
      <AnimatePresence>
        {showLeaderboard && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={() => setShowLeaderboard(false)}
            style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.75)", zIndex: 180, display: "flex", alignItems: "center", justifyContent: "center", padding: 18, backdropFilter: "blur(6px)" }}
          >
            <motion.div
              initial={{ scale: 0.92, y: 20 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.92, y: 20 }}
              onClick={e => e.stopPropagation()}
              style={{ background: c.panel, borderRadius: 24, width: "100%", maxWidth: 365, maxHeight: "82vh", display: "flex", flexDirection: "column", border: `1px solid ${c.borderMid}`, overflow: "hidden", boxShadow: "0 20px 60px rgba(0,0,0,0.5)" }}
            >
              <div style={{ padding: "16px 18px", borderBottom: `1px solid ${c.border}`, display: "flex", justifyContent: "space-between", alignItems: "center", background: c.panelAlt }}>
                <div>
                  <h3 style={{ color: c.text, fontSize: 16, fontWeight: 800, margin: 0 }}>🏆 Küresel Liderlik Tablosu</h3>
                  <p style={{ color: c.textSub, fontSize: 11, margin: "2px 0 0" }}>En çok galibiyet ve XP toplayan ilk 20 oyuncu</p>
                </div>
                <button onClick={() => setShowLeaderboard(false)} style={{ background: "none", border: "none", color: c.textSub, cursor: "pointer" }}><X size={18} /></button>
              </div>
              <div style={{ padding: 14, overflowY: "auto", display: "flex", flexDirection: "column", gap: 8 }}>
                {loadingLb ? (
                  <p style={{ color: c.textSub, fontSize: 12, textAlign: "center", padding: "24px 0" }}>Sıralama yükleniyor...</p>
                ) : leaderboard.length === 0 ? (
                  <p style={{ color: c.textSub, fontSize: 12, textAlign: "center", padding: "24px 0" }}>Henüz sıralama verisi yok.</p>
                ) : (
                  leaderboard.map(item => {
                    const isMe = item.username === profile.name;
                    const medal = item.rank === 1 ? "🥇" : item.rank === 2 ? "🥈" : item.rank === 3 ? "🥉" : `#${item.rank}`;
                    return (
                      <div
                        key={item.username}
                        style={{
                          display: "flex", alignItems: "center", justifyContent: "space-between",
                          padding: "9px 12px", borderRadius: 12,
                          background: isMe ? c.accentBg : c.bg,
                          border: isMe ? `1.5px solid ${c.accent}` : `1px solid ${c.border}`
                        }}
                      >
                        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                          <span style={{ width: 26, fontSize: item.rank <= 3 ? 17 : 12, fontWeight: 800, color: c.gold, textAlign: "center" }}>{medal}</span>
                          <Av seed={item.username} size={32} dark={dark} avatar={item.avatar} country={item.country} active={isMe} />
                          <div>
                            <p style={{ fontSize: 13, fontWeight: 800, color: c.text, margin: 0 }}>
                              {item.username} {isMe && <span style={{ fontSize: 10, color: c.accent }}>(Sen)</span>}
                            </p>
                            <p style={{ fontSize: 10, color: c.textSub, margin: "1px 0 0" }}>
                              Sv.{computeLevel(item.xp)} • Rekor: {item.highScore}
                            </p>
                          </div>
                        </div>
                        <div style={{ textAlign: "right" }}>
                          <p style={{ fontSize: 13, fontWeight: 800, color: c.accent, margin: 0 }}>{item.wins} Galibiyet</p>
                          <p style={{ fontSize: 10, color: c.gold, fontWeight: 700, margin: 0 }}>{item.xp} XP</p>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* OYUN KURALLARI & PUAN REHBERİ MODALI */}
      <AnimatePresence>
        {showRulesModal && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={() => setShowRulesModal(false)}
            style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.75)", zIndex: 180, display: "flex", alignItems: "center", justifyContent: "center", padding: 18, backdropFilter: "blur(6px)" }}
          >
            <motion.div
              initial={{ scale: 0.92, y: 20 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.92, y: 20 }}
              onClick={e => e.stopPropagation()}
              style={{ background: c.panel, borderRadius: 24, width: "100%", maxWidth: 365, maxHeight: "82vh", display: "flex", flexDirection: "column", border: `1px solid ${c.borderMid}`, overflow: "hidden", boxShadow: "0 20px 60px rgba(0,0,0,0.5)" }}
            >
              <div style={{ padding: "16px 18px", borderBottom: `1px solid ${c.border}`, display: "flex", justifyContent: "space-between", alignItems: "center", background: c.panelAlt }}>
                <h3 style={{ color: c.text, fontSize: 16, fontWeight: 800, margin: 0 }}>📖 Nasıl Oynanır & Puan Tablosu</h3>
                <button onClick={() => setShowRulesModal(false)} style={{ background: "none", border: "none", color: c.textSub, cursor: "pointer" }}><X size={18} /></button>
              </div>
              <div style={{ padding: 16, overflowY: "auto", display: "flex", flexDirection: "column", gap: 10, fontSize: 12, color: c.textSub, lineHeight: 1.5 }}>
                <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                  <p style={{ color: c.accent, fontWeight: 800, margin: "0 0 4px" }}>🎲 Temel Akış (13 Tur)</p>
                  Her turda zarları en fazla <b>3 kez</b> atabilirsin. İstediğin zarların üzerine dokunarak onları <b>kilitleyebilir</b> ve kalan zarları tekrar atabilirsin. Tur sonunda tabloda boş bir kategoriye puanını yazarsın.
                </div>
                <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                  <p style={{ color: c.gold, fontWeight: 800, margin: "0 0 4px" }}>📈 Üst Bölüm & +35 Bonus</p>
                  <b>Birler – Altılar:</b> Sadece ilgili zarların toplamı yazılır.<br />
                  Üst bölüm toplamın <b>63 veya üzeri</b> olursa anında <b>+35 Bonus Puan</b> kazanırsın!
                </div>
                <div style={{ background: c.bg, padding: 12, borderRadius: 12, border: `1px solid ${c.border}` }}>
                  <p style={{ color: c.text, fontWeight: 800, margin: "0 0 4px" }}>🔥 Alt Bölüm Kombinasyonları</p>
                  • <b>Üçlü / Dörtlü:</b> En az 3 veya 4 aynı zar varsa tüm zarların toplamı.<br />
                  • <b>Full House (25 Puan):</b> 3 aynı + 2 aynı zar (Örn: 4-4-4-2-2).<br />
                  • <b>Küçük Sıra (30 Puan):</b> Ardışık 4 zar (1-2-3-4, 2-3-4-5 veya 3-4-5-6).<br />
                  • <b>Büyük Sıra (40 Puan):</b> Ardışık 5 zar (1-2-3-4-5 veya 2-3-4-5-6).<br />
                  • <b>YAT (50 Puan):</b> 5 zarın tamamı aynı! (Örn: 6-6-6-6-6).<br />
                  • <b>Şans:</b> Koşulsuz tüm 5 zarın toplamı.
                </div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ŞIK GÜNLÜK ÖDÜL POP-UP'I */}
      <AnimatePresence>
        {showDailyPopup && (
          <motion.div 
            initial={{ opacity:0 }} animate={{ opacity:1 }} exit={{ opacity:0 }}
            style={{ position:"fixed", inset:0, background:"rgba(0,0,0,0.85)", zIndex:200, display:"flex", alignItems:"center", justifyContent:"center", padding:20, backdropFilter:"blur(8px)" }}
          >
             <motion.div 
               initial={{ scale:0.8, y:50 }} animate={{ scale:1, y:0 }} exit={{ scale:0.8, y:50 }} transition={{ type:"spring", damping:18 }}
               style={{ background:c.panel, borderRadius:32, width:"100%", maxWidth:340, padding:"40px 24px 24px", display:"flex", flexDirection:"column", alignItems:"center", gap:16, border:`2px solid ${c.accent}`, boxShadow:`0 20px 60px ${c.accentBg}`, position:"relative" }}
             >
                <div style={{ position:"absolute", top:-40, background:c.accent, width:80, height:80, borderRadius:"50%", display:"flex", alignItems:"center", justifyContent:"center", border:`6px solid ${c.panel}`, boxShadow:`0 10px 20px rgba(0,0,0,0.3)` }}>
                   <span style={{ fontSize:40 }}>🎁</span>
                </div>
                
                <h3 style={{ color:c.text, fontSize:22, fontWeight:800, margin:"10px 0 0", textAlign:"center" }}>
                  <span style={{ color:c.accent }}>{pendingReward.streak}. Gün</span> Giriş Ödülü!
                </h3>
                
                <p style={{ fontSize:13, color:c.textSub, textAlign:"center", lineHeight:1.5 }}>
                  Her gün oyuna girerek serini bozma, <br/>ödüllerini katlayarak artır!
                </p>

                <div style={{ display:"flex", gap:12, width:"100%", marginTop:10 }}>
                   <div style={{ flex:1, background:c.accentBg, border:`1px solid ${c.accentRing}`, borderRadius:16, padding:"16px", display:"flex", flexDirection:"column", alignItems:"center", gap:6 }}>
                      <span style={{ fontSize:28 }}>✨</span>
                      <span style={{ fontSize:18, fontWeight:800, color:c.accent }}>+{pendingReward.xp}</span>
                      <span style={{ fontSize:10, fontWeight:700, color:c.textSub, textTransform:"uppercase" }}>XP Puanı</span>
                   </div>
                   <div style={{ flex:1, background:c.goldBg, border:`1px solid rgba(201,168,76,0.3)`, borderRadius:16, padding:"16px", display:"flex", flexDirection:"column", alignItems:"center", gap:6 }}>
                      <span style={{ fontSize:28 }}>🪙</span>
                      <span style={{ fontSize:18, fontWeight:800, color:c.gold }}>+{pendingReward.tokens}</span>
                      <span style={{ fontSize:10, fontWeight:700, color:c.textSub, textTransform:"uppercase" }}>Jeton</span>
                   </div>
                </div>

                <motion.button whileTap={{scale:0.95}} onClick={claimDailyReward} style={{ 
                  width:"100%", padding:"18px", marginTop:10, borderRadius:16, background:c.accent, color:dark?c.bg:"#fff", border:"none", cursor:"pointer", fontWeight:800, fontSize:16, letterSpacing:"0.05em", boxShadow:`0 8px 20px ${c.accentBg}` 
                }}>
                  Harika, Al ve Kapat
                </motion.button>
             </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* KASA AÇMA ANİMASYONU EKRANI */}
      <AnimatePresence>
        {caseAnim && (
          <motion.div 
            initial={{ opacity:0 }} animate={{ opacity:1 }} exit={{ opacity:0 }}
            style={{ position:"fixed", inset:0, background:"rgba(0,0,0,0.85)", zIndex:300, display:"flex", alignItems:"center", justifyContent:"center", backdropFilter:"blur(10px)" }}
          >
            {caseAnim.status === "shaking" ? (
              <motion.div
                animate={{ 
                  rotate: [0, -12, 12, -12, 12, -12, 12, 0],
                  scale: [1, 1.15, 1.15, 1.15, 1.15, 1.15, 1.15, 1] 
                }}
                transition={{ duration: 1.5, repeat: Infinity }}
                style={{ fontSize: 120 }}
              >
                📦
              </motion.div>
            ) : (
              <motion.div 
                initial={{ scale:0, opacity:0, rotate: -180 }} 
                animate={{ scale:1, opacity:1, rotate: 0 }} 
                transition={{ type:"spring", damping:12 }}
                style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:20, background:c.panel, padding:"40px 30px", borderRadius:32, border:`2px solid ${c.gold}`, boxShadow:`0 0 80px ${c.goldBg}` }}
              >
                <div style={{ fontSize:100, lineHeight:1 }}>🪙</div>
                <div style={{ textAlign:"center" }}>
                  <h2 style={{ color:c.gold, fontSize:36, fontWeight:900, margin:0 }}>+{caseAnim.reward}</h2>
                  <p style={{ color:c.textSub, fontSize:14, fontWeight:700, margin:"4px 0 0", textTransform:"uppercase", letterSpacing:"0.1em" }}>Jeton Kazandın</p>
                </div>
                <motion.button 
                  whileTap={{scale:0.95}} 
                  onClick={claimCaseReward} 
                  style={{ width:"100%", padding:"16px", borderRadius:16, background:c.gold, color:"#000", border:"none", cursor:"pointer", fontWeight:800, fontSize:16, marginTop:10 }}
                >
                  Topla
                </motion.button>
              </motion.div>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Profil Düzenleme Modalı (Avatar, Fotoğraf Yükleme ve Ülke Seçimi) */}
      <AnimatePresence>
        {isEditing && (
          <motion.div initial={{ opacity:0 }} animate={{ opacity:1 }} exit={{ opacity:0 }} style={{ position:"fixed", inset:0, background:"rgba(0,0,0,0.65)", zIndex:100, display:"flex", alignItems:"center", justifyContent:"center", padding:20, backdropFilter:"blur(6px)" }}>
             <motion.div initial={{ scale:0.95, y:20 }} animate={{ scale:1, y:0 }} exit={{ scale:0.95, y:20 }} style={{ background:c.panel, borderRadius:24, width:"100%", maxWidth:350, maxHeight:"88vh", overflowY:"auto", padding:22, display:"flex", flexDirection:"column", gap:14, border:`1px solid ${c.border}`, boxShadow:"0 20px 50px rgba(0,0,0,0.5)" }}>
                <div style={{ display:"flex", justifyContent:"space-between", alignItems:"center" }}>
                  <h3 style={{ color:c.text, fontSize:18, fontWeight:800, margin:0 }}>Profili Özelleştir</h3>
                  <button onClick={()=>setIsEditing(false)} style={{ background:"none", border:"none", color:c.textSub, cursor:"pointer" }}><X size={20}/></button>
                </div>

                <div style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:10, margin:"4px 0" }}>
                   <Av seed={tempName || "U"} size={72} dark={dark} avatar={tempAvatar} country={tempCountry} active={true} />
                   <div style={{ display:"flex", gap:8 }}>
                     <label style={{ background:c.accentBg, border:`1px solid ${c.accentRing}`, color:c.accent, padding:"6px 12px", borderRadius:10, fontSize:11, fontWeight:700, cursor:"pointer" }}>
                       📷 Fotoğraf Yükle
                       <input type="file" accept="image/*" onChange={handleAvatarFileChange} style={{ display:"none" }} />
                     </label>
                     {tempAvatar && (
                       <button type="button" onClick={() => setTempAvatar("")} style={{ background:c.panelAlt, border:`1px solid ${c.border}`, color:c.textSub, padding:"6px 10px", borderRadius:10, fontSize:11, fontWeight:600, cursor:"pointer" }}>
                         Sıfırla
                       </button>
                     )}
                   </div>
                </div>

                <div>
                  <p style={{ fontSize:11, color:c.textSub, marginBottom:6, fontWeight:700 }}>Hazır Avatar Seç</p>
                  <div style={{ display:"grid", gridTemplateColumns:"repeat(6, 1fr)", gap:6 }}>
                    {PRESET_AVATARS.map(em => (
                      <button
                        key={em}
                        type="button"
                        onClick={() => setTempAvatar(em)}
                        style={{
                          padding:"7px 0", borderRadius:10, fontSize:18, cursor:"pointer",
                          background: tempAvatar === em ? c.accentBg : c.bg,
                          border: tempAvatar === em ? `1.5px solid ${c.accent}` : `1px solid ${c.border}`
                        }}
                      >
                        {em}
                      </button>
                    ))}
                  </div>
                </div>

                <div>
                  <p style={{ fontSize:11, color:c.textSub, marginBottom:6, fontWeight:700 }}>Ülke / Bayrak</p>
                  <select
                    value={tempCountry}
                    onChange={e => setTempCountry(e.target.value)}
                    style={{ width:"100%", boxSizing:"border-box", background:c.bg, border:`1px solid ${c.border}`, color:c.text, padding:"11px 14px", borderRadius:12, fontSize:14, fontWeight:600, outline:"none" }}
                  >
                    {COUNTRIES.map(co => (
                      <option key={co.code} value={co.code}>{co.flag} {co.name} ({co.code})</option>
                    ))}
                  </select>
                </div>

                <div>
                  <p style={{ fontSize:11, color:c.textSub, marginBottom:6, fontWeight:700 }}>Kullanıcı Adı</p>
                  <input value={tempName} onChange={e=>setTempName(e.target.value)} style={{ width:"100%", boxSizing:"border-box", background:c.bg, border:`1px solid ${c.border}`, color:c.text, padding:"12px 16px", borderRadius:12, fontSize:15, outline:"none", fontFamily:APP_FONT, fontWeight:600 }} placeholder="Adınızı girin" />
                </div>

                <button onClick={saveProfile} style={{ width:"100%", padding:14, marginTop:4, borderRadius:12, background:c.accent, color:dark?c.bg:"#fff", border:"none", cursor:"pointer", fontWeight:800, fontSize:14 }}>Değişiklikleri Kaydet</button>
                <button type="button" onClick={() => setShowLegalModal(true)} style={{ width:"100%", padding:11, borderRadius:12, background:c.panelAlt, color:c.textSub, border:`1px solid ${c.border}`, cursor:"pointer", fontWeight:700, fontSize:12 }}>
                   📜 Gizlilik Politikası, EULA & Kasa Olasılıkları
                </button>
                <button onClick={onLogout} style={{ width:"100%", padding:12, borderRadius:12, background:"rgba(255, 71, 87, 0.1)", color:"#ff4757", border:`1px solid rgba(255, 71, 87, 0.3)`, cursor:"pointer", fontWeight:700, fontSize:13 }}>
                   Çıkış Yap
                </button>

                {/* Apple Guideline 5.1.1(v) & Google Play Zorunlu Hesap Silme Bölümü */}
                {!showDeleteConfirm ? (
                  <button
                    type="button"
                    onClick={() => setShowDeleteConfirm(true)}
                    style={{ width:"100%", padding:10, borderRadius:10, background:"transparent", color:c.textFaint, border:`1px dashed ${c.border}`, cursor:"pointer", fontWeight:600, fontSize:11 }}
                  >
                    🗑️ Hesabımı ve Verilerimi Kalıcı Olarak Sil
                  </button>
                ) : (
                  <div style={{ background:"rgba(192,57,43,0.12)", border:`1px solid ${c.danger}`, borderRadius:12, padding:12, display:"flex", flexDirection:"column", gap:8 }}>
                    <p style={{ fontSize:11, color:c.text, fontWeight:700, margin:0, lineHeight:1.4 }}>
                      ⚠️ Hesabınız, tüm istatistikleriniz, arkadaşlarınız ve koleksiyonunuz sunucudan <b>kalıcı olarak silinecektir</b>. Onaylıyor musunuz?
                    </p>
                    <div style={{ display:"flex", gap:8 }}>
                      <button type="button" onClick={() => setShowDeleteConfirm(false)} style={{ flex:1, padding:"9px", borderRadius:8, background:c.panelAlt, color:c.text, border:`1px solid ${c.border}`, fontSize:11, fontWeight:700, cursor:"pointer" }}>
                        Vazgeç
                      </button>
                      <button type="button" disabled={deletingAccount} onClick={handleDeleteMyAccount} style={{ flex:1, padding:"9px", borderRadius:8, background:c.danger, color:"#fff", border:"none", fontSize:11, fontWeight:800, cursor:"pointer" }}>
                        {deletingAccount ? "Siliniyor..." : "Evet, Kalıcı Sil"}
                      </button>
                    </div>
                  </div>
                )}
             </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
      <LegalModal open={showLegalModal} onClose={() => setShowLegalModal(false)} dark={dark} />
    </div>
  );
}

// ─── Screen 2: Shop ───────────────────────────────────────────────────────────
function ShopScreen({
  dark, sel, setSel, profile, setProfile, token
}: {
  dark:boolean; sel:DieMat; setSel:(d:DieMat)=>void; profile:UserProfile; setProfile:React.Dispatch<React.SetStateAction<UserProfile>>; token:string|null;
}) {
  const c = C(dark);
  const [rs, setRs] = useState<DieState[]>([2,5,3].map((v,i)=>initDie(v as DieVal, i)));
  const [rolling, setRolling] = useState(false);
  const [previewDie, setPreviewDie] = useState<DieMat | null>(null);
  const [previewState, setPreviewState] = useState<DieState>(initDie(6, 0));
  const [previewRolling, setPreviewRolling] = useState(false);
  const [buying, setBuying] = useState(false);
  
  const DICE_PRICES: Record<DieMat, number> = {
    ivory: 0,
    wood: 250,
    marble: 600,
    obsidian: 1500,
    ruby: 2500,
    gold: 4000,
  };

  const roll = () => {
    sfx.roll();
    setRolling(true);
    const newVals = Array.from({length:3},()=>(Math.floor(Math.random()*6)+1) as DieVal);
    const newStates = newVals.map((v,i)=>{
      const cur = rs[i];
      const t = targetRot(v, cur.rx, cur.ry);
      return { value:v, rx:t.rx, ry:t.ry, px: cur.px, py: cur.py };
    });
    setRs(newStates);
    setTimeout(()=>setRolling(false), 1000); 
  };

  const rollPreview = () => {
    if (previewRolling) return;
    sfx.roll();
    setPreviewRolling(true);
    const v = (Math.floor(Math.random() * 6) + 1) as DieVal;
    const t = targetRot(v, previewState.rx, previewState.ry);
    setPreviewState(prev => ({ ...prev, value: v, rx: t.rx, ry: t.ry }));
    setTimeout(() => setPreviewRolling(false), 850);
  };

  const equipDie = async (d: DieMat) => {
    setSel(d);
    if (token && !token.startsWith("guest_token_")) {
      await fetch(`${API_BASE}/profile/update`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + token
        },
        body: JSON.stringify({ selectedDice: d })
      }).catch(() => {});
    }
  };

  const handleDiceClick = (d: DieMat) => {
    setPreviewState(initDie(6, 0));
    setPreviewDie(d);
  };

  const confirmBuyDie = async (d: DieMat) => {
    const price = DICE_PRICES[d];
    if (profile.tokens < price || buying) return;
    setBuying(true);
    try {
      if (token && !token.startsWith("guest_token_")) {
        const res = await fetch(`${API_BASE}/shop/buy`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + token
          },
          body: JSON.stringify({ dice: d })
        });
        if (!res.ok) {
          const errTxt = await res.text();
          emitToast(errTxt || "Satın alma başarısız!");
          return;
        }
        sfx.win();
        const updated = await res.json();
        setProfile(prev => ({
          ...prev,
          tokens: updated.tokens,
          unlockedDice: updated.unlockedDice as DieMat[]
        }));
        await equipDie(d);
      } else {
        setProfile(prev => ({
          ...prev,
          tokens: prev.tokens - price,
          unlockedDice: [...prev.unlockedDice, d]
        }));
        setSel(d);
      }
      setPreviewDie(null);
    } finally {
      setBuying(false);
    }
  };

  return (
    <div style={{ minHeight:"100%", background:c.bg, fontFamily:APP_FONT, display:"flex", flexDirection:"column" }}>
      <div style={{ display:"flex", alignItems:"center", gap:12, padding:"28px 20px 18px", borderBottom:`1px solid ${c.border}` }}>
        <div>
          <h2 style={{ fontSize:20, fontWeight:800, color:c.text }}>Koleksiyon</h2>
          <p style={{ fontSize:12, color:c.textSub }}>Zara tıklayarak 3D incele ve koleksiyonuna kat</p>
        </div>
      </div>

      <div style={{ padding:"20px" }}>
        <PremiumTray dark={dark}>
          <div style={{ display:"flex", justifyContent:"center", gap:18, marginBottom:22, marginTop: 10, position:"relative", zIndex:1 }}>
            {rs.map((s,i)=>(
              <Die3D key={i} index={i} state={s} size={72} locked={false} mat={sel} isRolling={rolling}/>
            ))}
          </div>
          <button onClick={roll} disabled={rolling} style={{
            width:"100%", padding:"11px", background:dark?"rgba(255,255,255,0.08)":"rgba(255,255,255,0.45)",
            border:`1px solid ${c.borderMid}`, borderRadius:10,
            color:dark?"#fff":c.text, fontSize:13, fontWeight:600, fontFamily:APP_FONT, cursor:rolling?"not-allowed":"pointer", display:"flex", alignItems:"center", justifyContent:"center", gap:6, position:"relative", zIndex:1, backdropFilter:"blur(5px)",
          }}>
            <RotateCcw size={13}/> Seçili Zarı Dene ({MAT[sel].name})
          </button>
        </PremiumTray>
      </div>

      <div style={{ padding:"0 20px 32px" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
           <p style={{ fontSize:9, letterSpacing:"0.1em", textTransform:"uppercase", color:c.textFaint, fontWeight:600, margin:0 }}>Zar Marketi (Büyük Görmek İçin Tıkla)</p>
           <div style={{ display:"flex", alignItems:"center", gap:4, background:c.goldBg, padding:"4px 10px", borderRadius:12 }}>
             <span style={{ fontSize:16 }}>🪙</span>
             <p style={{ fontSize:13, color:c.gold, fontWeight:800, margin:0 }}>{profile.tokens}</p>
           </div>
        </div>
        
        <div style={{ display:"grid", gridTemplateColumns:"1fr 1fr", gap:10 }}>
          {(["ivory","wood","marble","obsidian","ruby","gold"] as DieMat[]).map(d=>{
            const isUnlocked = profile.unlockedDice.includes(d);
            const price = DICE_PRICES[d];
            const canAfford = profile.tokens >= price;
            const active = sel===d;
            
            return (
              <motion.div key={d} whileTap={{scale:0.97}} onClick={()=>handleDiceClick(d)} style={{
                background: active?c.accentBg:c.panel,
                border: active?`1.5px solid ${c.accent}`:`1px solid ${c.border}`,
                borderRadius:14, padding:"12px", cursor:"pointer",
                display:"flex", alignItems:"center", gap:10, position:"relative",
              }}>
                <Die3D state={initDie(5)} size={40} locked={false} mat={d}/>
                <div style={{ flex:1, minWidth:0, paddingRight:12 }}>
                  <p style={{ fontSize:13, fontWeight:700, color:c.text, margin:0 }}>{MAT[d].name}</p>
                  {isUnlocked ? (
                    <p style={{ fontSize:10, color:active?c.accent:c.textSub, marginTop:3, fontWeight:600 }}>
                      {active ? "✓ Kullanımda" : "Sahipsin (İncele)"}
                    </p>
                  ) : (
                    <p style={{ fontSize:11, color:canAfford?c.gold:c.danger, fontWeight:800, marginTop:4, whiteSpace:"nowrap" }}>🪙 {price}</p>
                  )}
                </div>
                {active&&<div style={{ position:"absolute", top:8, right:8, width:18, height:18, borderRadius:"50%", background:c.accent, display:"flex", alignItems:"center", justifyContent:"center" }}><Check size={10} color="#fff" strokeWidth={3}/></div>}
                {!isUnlocked&&<div style={{ position:"absolute", top:8, right:8, color:c.textFaint }}><Lock size={12}/></div>}
              </motion.div>
            );
          })}
        </div>
      </div>

      {/* BÜYÜK 3D ZAR ÖNİZLEME VE SATIN ALMA MODALI */}
      <AnimatePresence>
        {previewDie && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={() => setPreviewDie(null)}
            style={{
              position: "fixed", inset: 0, background: "rgba(0,0,0,0.82)", zIndex: 250,
              display: "flex", alignItems: "center", justifyContent: "center", padding: 20, backdropFilter: "blur(8px)"
            }}
          >
            <motion.div
              initial={{ scale: 0.85, y: 25 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.85, y: 25 }}
              onClick={e => e.stopPropagation()}
              style={{
                background: c.panel, borderRadius: 28, width: "100%", maxWidth: 350,
                padding: "24px 22px", border: `1.5px solid ${c.borderMid}`,
                boxShadow: `0 24px 64px rgba(0,0,0,0.6)`, display: "flex", flexDirection: "column", alignItems: "center", gap: 16
              }}
            >
              <div style={{ width: "100%", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ fontSize: 10, fontWeight: 800, color: c.accent, textTransform: "uppercase", letterSpacing: "0.08em" }}>
                  3D Zar İnceleme
                </span>
                <button onClick={() => setPreviewDie(null)} style={{ background: "none", border: "none", color: c.textSub, cursor: "pointer" }}>
                  <X size={20} />
                </button>
              </div>

              {/* Büyük 3D Dönen Zar Sahnesi */}
              <div style={{
                width: "100%", height: 185, borderRadius: 22,
                background: c.trayFelt, border: `6px solid ${c.trayOuter}`,
                boxShadow: "inset 0 10px 28px rgba(0,0,0,0.55)",
                display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
                position: "relative", overflow: "hidden"
              }}>
                <Die3D
                  state={previewState}
                  size={96}
                  locked={false}
                  mat={previewDie}
                  isRolling={previewRolling}
                  isIdle={!previewRolling}
                  onClick={rollPreview}
                />
                <button
                  type="button"
                  onClick={rollPreview}
                  style={{
                    position: "absolute", bottom: 10, right: 10,
                    background: "rgba(0,0,0,0.55)", color: "#fff",
                    border: "1px solid rgba(255,255,255,0.2)", borderRadius: 10,
                    padding: "5px 10px", fontSize: 11, fontWeight: 700, cursor: "pointer",
                    display: "flex", alignItems: "center", gap: 5
                  }}
                >
                  <RotateCcw size={12} /> Zarı At
                </button>
              </div>

              <div style={{ textAlign: "center" }}>
                <h3 style={{ fontSize: 22, fontWeight: 900, color: c.text, margin: 0 }}>{MAT[previewDie].name} Zarı</h3>
                <p style={{ fontSize: 12, color: c.textSub, margin: "6px 0 0", lineHeight: 1.45 }}>
                  {MAT[previewDie].desc}
                </p>
              </div>

              {profile.unlockedDice.includes(previewDie) ? (
                <div style={{ display: "flex", gap: 10, width: "100%", marginTop: 4 }}>
                  <button
                    type="button"
                    onClick={() => setPreviewDie(null)}
                    style={{
                      flex: 1, padding: "14px", borderRadius: 14,
                      background: c.panelAlt, color: c.textSub,
                      border: `1px solid ${c.border}`, fontWeight: 700, fontSize: 13, cursor: "pointer"
                    }}
                  >
                    Kapat
                  </button>
                  <button
                    type="button"
                    onClick={() => { equipDie(previewDie); setPreviewDie(null); }}
                    style={{
                      flex: 1.4, padding: "14px", borderRadius: 14,
                      background: c.accent, color: dark ? c.bg : "#fff",
                      border: "none", fontWeight: 800, fontSize: 14, cursor: "pointer"
                    }}
                  >
                    {sel === previewDie ? "✓ Seçili Zar" : "Kuşan / Seç"}
                  </button>
                </div>
              ) : (
                <>
                  <div style={{
                    width: "100%", background: c.goldBg, border: `1px solid ${c.gold}40`,
                    borderRadius: 14, padding: "10px 14px", display: "flex", justifyContent: "space-between", alignItems: "center"
                  }}>
                    <span style={{ fontSize: 12, fontWeight: 700, color: c.textSub }}>Satın Alma Fiyatı:</span>
                    <span style={{ fontSize: 16, fontWeight: 900, color: c.gold }}>🪙 {DICE_PRICES[previewDie]} Jeton</span>
                  </div>

                  {profile.tokens < DICE_PRICES[previewDie] && (
                    <p style={{ fontSize: 11, fontWeight: 700, color: c.danger, margin: "-4px 0 0" }}>
                      Bu zarı almak için {DICE_PRICES[previewDie] - profile.tokens} 🪙 daha gerekiyor.
                    </p>
                  )}

                  <div style={{ display: "flex", gap: 10, width: "100%", marginTop: 2 }}>
                    <button
                      type="button"
                      onClick={() => setPreviewDie(null)}
                      style={{
                        flex: 1, padding: "14px", borderRadius: 14,
                        background: c.panelAlt, color: c.text,
                        border: `1px solid ${c.border}`, fontWeight: 700, fontSize: 13, cursor: "pointer"
                      }}
                    >
                      Alma (Vazgeç)
                    </button>
                    <button
                      type="button"
                      disabled={profile.tokens < DICE_PRICES[previewDie] || buying}
                      onClick={() => confirmBuyDie(previewDie)}
                      style={{
                        flex: 1.4, padding: "14px", borderRadius: 14,
                        background: profile.tokens >= DICE_PRICES[previewDie] ? c.gold : c.panelAlt,
                        color: profile.tokens >= DICE_PRICES[previewDie] ? "#1a1405" : c.textFaint,
                        border: "none", fontWeight: 900, fontSize: 14,
                        cursor: profile.tokens >= DICE_PRICES[previewDie] ? "pointer" : "not-allowed"
                      }}
                    >
                      {buying ? "Alınıyor..." : `Satın Al (🪙 ${DICE_PRICES[previewDie]})`}
                    </button>
                  </div>
                </>
              )}
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}


// ─── Screen 3: Game (Server-Authoritative + Turn Timer + Emojis + Wager) ─────
function GameScreen({
  go, dark, diceMat, profile, setProfile, ws, myPlayerIndex, opponentName, onMatchEnd, activeBet,
  reconnecting, resumeState, onCloseSocket, onMatchResumed, onResumeFailed, onSessionReplaced
}: {
  go:(s:Screen)=>void; dark:boolean; diceMat:DieMat; profile:UserProfile;
  setProfile:React.Dispatch<React.SetStateAction<UserProfile>>;
  ws:WebSocket|null; myPlayerIndex:number; opponentName:string;
  onMatchEnd:(summary: MatchSummary)=>void; activeBet:number;
  reconnecting:boolean; resumeState:ResumeState|null; onCloseSocket:()=>void;
  onMatchResumed:(data:ResumedMatch)=>void; onResumeFailed:()=>void; onSessionReplaced:()=>void;
}) {
  const c = C(dark);

  const [opponentOffline, setOpponentOffline] = useState(false);
  const [showLeaveConfirm, setShowLeaveConfirm] = useState(false);
  const [dieStates, setDieStates] = useState<DieState[]>(()=>{
    const pos = generateRandomPositions();
    return ([3,1,5,2,4] as DieVal[]).map((v,i)=>({ ...initDie(v,i), px:pos[i].x, py:pos[i].y }));
  });
  const [locked, setLocked] = useState([false,false,false,false,false]);
  const [rollsLeft, setRollsLeft] = useState(3);
  const [rolling, setRolling] = useState(false);
  const [opponentLeft, setOpponentLeft] = useState(false);
  const [currentPlayer, setCurrentPlayer] = useState<"p1" | "p2">("p1");
  const [p1Scores, setP1Scores] = useState<Record<string,number>>({});
  const [p2Scores, setP2Scores] = useState<Record<string,number>>({});
  const [p1Total, setP1Total] = useState(0);
  const [p2Total, setP2Total] = useState(0);
  const [round, setRound] = useState(1);
  const [turnTimeLeft, setTurnTimeLeft] = useState(30);
  const [floatingEmoji, setFloatingEmoji] = useState<{emoji:string; fromMe:boolean; id:number}|null>(null);

  // Canlı Sohbet & Anti-Spam + UGC Engelleme/Raporlama State'leri (Apple Guideline 1.2)
  const [chatOpen, setChatOpen] = useState(false);
  const [chatMessages, setChatMessages] = useState<{ sender: string; senderName: string; text: string; time: string }[]>([]);
  const [chatInput, setChatInput] = useState("");
  const [unreadChat, setUnreadChat] = useState(0);
  const [chatPreview, setChatPreview] = useState<{ senderName: string; text: string } | null>(null);
  const [emojiCooldown, setEmojiCooldown] = useState(false);
  const [chatCooldown, setChatCooldown] = useState(false);
  const [spamWarn, setSpamWarn] = useState<string | null>(null);
  const [opponentMuted, setOpponentMuted] = useState(false);
  const [gameSoundOn, setGameSoundOn] = useState(() => localStorage.getItem("yatzy_sound") !== "off");
  const opponentMutedRef = React.useRef(false);
  const chatOpenRef = React.useRef(false);

  const toggleGameSound = () => {
    const next = !gameSoundOn;
    setGameSoundOn(next);
    localStorage.setItem("yatzy_sound", next ? "on" : "off");
    if (next) sfx.lock(true);
  };

  const unblockOpponent = async () => {
    const headers = { "Authorization": "Bearer " + (localStorage.getItem("yatzy_jwt_token") || "") };
    try {
      const res = await fetch(`${API_BASE}/ugc/blocks`, { headers });
      if (!res.ok) return;
      const data = await res.json();
      const target = (data.blocked as BlockedUser[]).find(item => item.username === opponentName);
      if (target) {
        await fetch(`${API_BASE}/ugc/unblock`, {
          method: "POST",
          headers: { ...headers, "Content-Type": "application/json" },
          body: JSON.stringify({ userId: target.userId })
        });
      }
    } catch {}
  };

  const toggleMuteBlockOpponent = async () => {
    const next = !opponentMuted;
    setOpponentMuted(next);
    opponentMutedRef.current = next;
    if (next) {
      setChatPreview(null);
      emitToast(`${opponentName || "Rakip"} susturuldu ve engellendi.`);
      try {
        await fetch(`${API_BASE}/ugc/block`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + (localStorage.getItem("yatzy_jwt_token") || "")
          },
          body: JSON.stringify({ username: opponentName || "Rakip" })
        });
      } catch {}
    } else {
      emitToast(`${opponentName || "Rakip"} için engel kaldırıldı.`);
      unblockOpponent();
    }
  };

  const reportOpponent = async () => {
    try {
      await fetch(`${API_BASE}/ugc/report`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + (localStorage.getItem("yatzy_jwt_token") || "")
        },
        body: JSON.stringify({
          username: opponentName || "Rakip",
          reason: "Oyun içi uygunsuz sohbet / davranış bildirimi"
        })
      });
    } catch {}
    setOpponentMuted(true);
    opponentMutedRef.current = true;
    emitToast("Şikayetiniz güvenlik ekibine iletildi ve oyuncu susturuldu.");
  };
  useEffect(() => {
    chatOpenRef.current = chatOpen;
    if (chatOpen) {
      setUnreadChat(0);
      setChatPreview(null);
    }
  }, [chatOpen]);

  const [scorePopup, setScorePopup] = useState<{pts:number; label:string; id:number}|null>(null);
  const popupIdRef = React.useRef(0);
  const emojiIdRef = React.useRef(0);

  const myPlayerId = myPlayerIndex === 1 ? "p1" : "p2"; 
  const isMyTurn = currentPlayer === myPlayerId;

  const myScores = myPlayerIndex === 1 ? p1Scores : p2Scores;
  const oppScores = myPlayerIndex === 1 ? p2Scores : p1Scores;

  const p1Upper = CATS.filter(cat=>cat.upper).reduce((a,cat)=>p1Scores[cat.key]!=null?a+p1Scores[cat.key]:a,0);
  const p1Bonus = p1Upper>=63?35:0;
  const p2Upper = CATS.filter(cat=>cat.upper).reduce((a,cat)=>p2Scores[cat.key]!=null?a+p2Scores[cat.key]:a,0);
  const p2Bonus = p2Upper>=63?35:0;

  const myUpper = myPlayerIndex === 1 ? p1Upper : p2Upper;
  const oppUpper = myPlayerIndex === 1 ? p2Upper : p1Upper;
  const myBonus = myPlayerIndex === 1 ? p1Bonus : p2Bonus;
  const oppBonus = myPlayerIndex === 1 ? p2Bonus : p1Bonus;

  // 30 Saniyelik Görsel Tur Sayacı + Son 5 Saniye Uyarı Sesi
  useEffect(() => {
    if (opponentLeft) return;
    const interval = setInterval(() => {
      setTurnTimeLeft(prev => {
        const next = prev > 0 ? prev - 1 : 0;
        if (next > 0 && next <= 5 && isMyTurn) {
          sfx.tick();
        }
        return next;
      });
    }, 1000);
    return () => clearInterval(interval);
  }, [opponentLeft, isMyTurn]);

  const handleLeave = () => setShowLeaveConfirm(true);

  const confirmLeave = () => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "LEAVE" }));
    onCloseSocket();
    go("home");
  };

  // Geri bağlanınca maç durumunu sunucudan gelen anlık görüntüyle değiştirir
  useEffect(() => {
    if (!resumeState) return;
    setP1Scores(resumeState.p1Scores || {});
    setP2Scores(resumeState.p2Scores || {});
    setP1Total(resumeState.p1Total);
    setP2Total(resumeState.p2Total);
    setCurrentPlayer(resumeState.turn);
    setRound(resumeState.round);
    setRollsLeft(resumeState.rollsLeft);
    setLocked(resumeState.locked);
    setTurnTimeLeft(resumeState.secondsLeft);
    setRolling(false);
    setDieStates(prev => prev.map((die, i) => {
      const value = resumeState.dices[i] as DieVal;
      const target = targetRot(value, die.rx, die.ry);
      return { ...die, value, rx: target.rx, ry: target.ry };
    }));
  }, [resumeState]);

  const sendGame = (payload: Record<string, unknown>) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
  };

  useEffect(() => {
    if (!ws) return;
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      
      if (data.type === "MATCH_FOUND" && data.resumed) {
        onMatchResumed(data);
      }
      else if (data.type === "RESUME_FAILED") {
        onResumeFailed();
      }
      else if (data.type === "SESSION_REPLACED") {
        onSessionReplaced();
      }
      else if (data.type === "OPPONENT_DISCONNECTED") {
        setOpponentOffline(true);
      }
      else if (data.type === "OPPONENT_RECONNECTED") {
        setOpponentOffline(false);
      }
      else if (data.type === "DICE_ROLLED") {
        if (!isMyTurn) sfx.roll();
        const syncedLocked: boolean[] = data.locked || [false,false,false,false,false];
        setLocked(syncedLocked);
        setRolling(true);
        setRollsLeft(typeof data.rollsLeft === "number" ? data.rollsLeft : 0);
        
        const newPos = generateRandomPositions();
        setDieStates(prev => prev.map((s,i)=>{
          if (syncedLocked[i]) return s;
          const v = data.dices[i] as DieVal; 
          const t = targetRot(v, s.rx, s.ry);
          return { value:v, rx:t.rx, ry:t.ry, px: newPos[i].x, py: newPos[i].y };
        }));
        setTimeout(()=>{
          setRolling(false);
        }, 760);
      }
      else if (data.type === "TOGGLE_LOCK") {
        setLocked(data.locked);
      }
      else if (data.type === "EMOJI") {
        if (opponentMutedRef.current && data.sender !== myPlayerId) return;
        sfx.pop();
        const id = ++emojiIdRef.current;
        setFloatingEmoji({ emoji: data.emoji, fromMe: data.sender === myPlayerId, id });
        setTimeout(() => setFloatingEmoji(prev => prev?.id === id ? null : prev), 2200);
      }
      else if (data.type === "CHAT") {
        if (opponentMutedRef.current && data.sender !== myPlayerId) return;
        sfx.pop();
        const newMsg = {
          sender: data.sender || "p2",
          senderName: data.senderName || "Oyuncu",
          text: data.text || "",
          time: data.time || ""
        };
        setChatMessages(prev => [...prev.slice(-49), newMsg]);
        if (!chatOpenRef.current && data.sender !== myPlayerId) {
          setUnreadChat(prev => prev + 1);
          setChatPreview({ senderName: newMsg.senderName, text: newMsg.text });
          setTimeout(() => setChatPreview(null), 3500);
        }
      }
      else if (data.type === "SPAM_WARN") {
        setSpamWarn(data.message || "Çok hızlı işlem yapıyorsunuz!");
        setTimeout(() => setSpamWarn(null), 2200);
      }
      else if (data.type === "OPPONENT_LEFT") {
        setOpponentLeft(true);
        setOpponentOffline(false);
        const rXp = data.reward?.xp ?? 150;
        const rTokens = data.reward?.tokens ?? 50;

        setProfile(prev => ({
           ...prev,
           xp: prev.xp + rXp,
           level: computeLevel(prev.xp + rXp),
           tokens: prev.tokens + rTokens,
           stats: {
              ...prev.stats,
              wins: prev.stats.wins + 1,
              gamesPlayed: prev.stats.gamesPlayed + 1
           }
        }));
        
        onCloseSocket();
        setTimeout(() => {
          go("home");
        }, 3500); 
      }
      else if (data.type === "SCORE_SELECTED") {
        const { key, pts, nextTurn, round: nextRound, p1Scores: s1, p2Scores: s2, p1Total: t1, p2Total: t2, gameOver, rewards } = data;
        sfx.score(pts);
        const catLabel = CATS.find(cat => cat.key === key)?.label ?? key;
        const id = ++popupIdRef.current;
        setScorePopup({ pts, label: data.autoSelected ? `${catLabel} (Süre Doldu)` : catLabel, id });
        setTimeout(() => setScorePopup(prev => prev?.id===id ? null : prev), 1800);

        if (s1) setP1Scores(s1);
        if (s2) setP2Scores(s2);
        if (typeof t1 === "number") setP1Total(t1);
        if (typeof t2 === "number") setP2Total(t2);
        if (nextTurn) setCurrentPlayer(nextTurn);
        if (nextRound) setRound(nextRound);
        setTurnTimeLeft(data.turnSeconds || 30);

        const pos = generateRandomPositions();
        setDieStates(([3,1,5,2,4] as DieVal[]).map((v,i)=>({ ...initDie(v,i), px:pos[i].x, py:pos[i].y })));
        setLocked([false,false,false,false,false]);
        setRollsLeft(3);

        // HER İKİ OYUNCU İÇİN OYUN BİTİŞ VE SONUÇ EKRANI SENKRONİZASYONU
        if (gameOver) {
          const myFinalScores = myPlayerIndex === 1 ? s1 : s2;
          const oppFinalScores = myPlayerIndex === 1 ? s2 : s1;
          const myFinalTotal = myPlayerIndex === 1 ? t1 : t2;
          const oppFinalTotal = myPlayerIndex === 1 ? t2 : t1;
          const myFinalBonus = myPlayerIndex === 1 ? (data.p1Bonus || 0) : (data.p2Bonus || 0);
          const oppFinalBonus = myPlayerIndex === 1 ? (data.p2Bonus || 0) : (data.p1Bonus || 0);
          const myYachtsInMatch = myPlayerIndex === 1 ? (data.p1Yachts || (myFinalScores?.yacht === 50 ? 1 : 0)) : (data.p2Yachts || (myFinalScores?.yacht === 50 ? 1 : 0));
          const myReward = rewards ? rewards[myPlayerId] : { xp: 150, tokens: 40, cases: 0 };
          const won = myFinalTotal > oppFinalTotal;
          if (won) sfx.win();

          setProfile(prev => {
            const newXp = prev.xp + (myReward?.xp || 0);
            return {
              ...prev,
              xp: newXp,
              level: computeLevel(newXp),
              tokens: prev.tokens + (myReward?.tokens || 0),
              cases: prev.cases + (myReward?.cases || 0),
              upperBonusesWon: (prev.upperBonusesWon || 0) + (myFinalBonus > 0 ? 1 : 0),
              totalYachts: (prev.totalYachts || 0) + myYachtsInMatch,
              maxYachtsInMatch: Math.max(prev.maxYachtsInMatch || 0, myYachtsInMatch),
              stats: {
                wins: prev.stats.wins + (won ? 1 : 0),
                gamesPlayed: prev.stats.gamesPlayed + 1,
                highScore: Math.max(prev.stats.highScore, myFinalTotal)
              }
            };
          });

          onMatchEnd({
            myName: profile.name,
            mySeed: profile.seed,
            oppName: opponentName || "Rakip",
            oppSeed: (opponentName || "RK").substring(0, 2).toUpperCase(),
            myScores: myFinalScores,
            oppScores: oppFinalScores,
            myBonus: myFinalBonus,
            oppBonus: oppFinalBonus,
            myTotal: myFinalTotal,
            oppTotal: oppFinalTotal,
            rewardXp: myReward?.xp || 150,
            rewardTokens: myReward?.tokens || 40,
            rewardCases: myReward?.cases || 0,
          });

          setTimeout(() => {
            onCloseSocket();
            go("result");
          }, 1600);
        }
      }
    };
  }, [ws, myPlayerIndex, myPlayerId, isMyTurn, opponentName, profile.name, profile.seed, go, onMatchEnd, setProfile, onCloseSocket, onMatchResumed, onResumeFailed, onSessionReplaced]);

  const doRoll = () => {
    if (!isMyTurn || rollsLeft===0 || rolling || !ws || reconnecting) return; 
    sfx.roll();
    setRolling(true);
    sendGame({ type: "ROLL_DICE", locked: locked });
  };

  const toggleLock = (i:number) => {
    if (rollsLeft===3 || rolling || !isMyTurn || !ws) return;
    const newLocked = [...locked];
    newLocked[i] = !newLocked[i];
    sfx.lock(newLocked[i]);
    setLocked(newLocked);
    sendGame({ type: "TOGGLE_LOCK", locked: newLocked });
  };

  const scoreIt = (key:string) => {
    if (!isMyTurn || rolling || rollsLeft===3 || !ws) return;
    if (myScores[key] != null) return;
    // Puanı sunucu hesaplayıp her iki oyuncuya SCORE_SELECTED olarak yayınlar
    sendGame({ type: "SCORE_SELECTED", key: key });
  };

  const sendEmoji = (emoji: string) => {
    if (!ws || emojiCooldown) {
      if (emojiCooldown) {
        setSpamWarn("Emoji spam koruması aktif (2sn).");
        setTimeout(() => setSpamWarn(null), 1800);
      }
      return;
    }
    setEmojiCooldown(true);
    setTimeout(() => setEmojiCooldown(false), 2000);
    sendGame({ type: "EMOJI", emoji });
  };

  const sendChat = (customText?: string) => {
    const txt = (customText ?? chatInput).trim();
    if (!ws || !txt) return;
    if (chatCooldown) {
      setSpamWarn("Sohbet spam koruması: 2.5 saniye bekleyin.");
      setTimeout(() => setSpamWarn(null), 2000);
      return;
    }
    setChatCooldown(true);
    setTimeout(() => setChatCooldown(false), 2500);
    sendGame({ type: "CHAT", text: txt.slice(0, 120) });
    if (!customText) setChatInput("");
  };

  const diceVals = dieStates.map(s=>s.value);
  const canScore = rollsLeft < 3 && !rolling;

  return (
    <div style={{ minHeight:"100%", background:c.bg, fontFamily:APP_FONT, display:"flex", flexDirection:"column", position:"relative" }}>
      <div style={{ padding: "12px 12px 0", background: c.panel, display:"flex", justifyContent:"space-between", alignItems:"center", gap: 6 }}>
         <button onClick={handleLeave} style={{ background: "transparent", border: "none", color: c.danger, fontSize: 12, fontWeight: 700, cursor: "pointer", display: "flex", alignItems: "center", gap: 2, padding: 0, whiteSpace: "nowrap", flexShrink: 0 }}>
           <ChevronLeft size={16} /> TERK ET
         </button>

         {/* Hızlı Emoji ve Canlı Sohbet Çubuğu */}
         <div style={{ display:"flex", alignItems:"center", gap:4 }}>
           {["🎲", "👏", "🔥", "😱", "😎"].map(em => (
             <button
               key={em}
               onClick={() => sendEmoji(em)}
               style={{
                 background:c.panelAlt, border:`1px solid ${c.border}`, borderRadius:8,
                 padding:"3px 5px", cursor: emojiCooldown ? "not-allowed" : "pointer",
                 fontSize:13, opacity: emojiCooldown ? 0.45 : 1, transition: "0.15s"
               }}
             >
               {em}
             </button>
           ))}
           <button
             onClick={toggleGameSound}
             title={gameSoundOn ? "Sesi Kapat" : "Sesi Aç"}
             style={{
               background: c.panelAlt, border: `1px solid ${c.border}`, borderRadius: 8,
               padding: "3px 5px", cursor: "pointer", fontSize: 12
             }}
           >
             {gameSoundOn ? "🔊" : "🔇"}
           </button>
           <button
             onClick={() => setChatOpen(true)}
             style={{
               position: "relative", background: c.accentBg, border: `1px solid ${c.accentRing}`,
               color: c.accent, borderRadius: 8, padding: "4px 7px", cursor: "pointer", whiteSpace: "nowrap",
               fontSize: 11, fontWeight: 800, display: "flex", alignItems: "center", gap: 4
             }}
           >
             💬 Sohbet
             {unreadChat > 0 && (
               <span style={{
                 position: "absolute", top: -5, right: -5, background: c.danger, color: "#fff",
                 fontSize: 9, fontWeight: 900, width: 16, height: 16, borderRadius: "50%",
                 display: "flex", alignItems: "center", justifyContent: "center"
               }}>
                 {unreadChat}
               </span>
             )}
           </button>
         </div>
      </div>

      <div style={{ background: c.panel, paddingTop: "12px", paddingBottom: "10px", boxShadow: `0 2px 10px ${c.shadow}`, zIndex: 10 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "0 20px" }}>
           {/* SOL OYUNCU (HER ZAMAN SEN) */}
           <div style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:2, opacity: isMyTurn?1:0.4, transition:"0.3s" }}>
              <Av seed={profile.seed} size={32} active={isMyTurn} dark={dark}/>
              <p style={{fontSize:10, fontWeight:"bold", color:isMyTurn?c.accent:c.text}}>{profile.name} ({myPlayerIndex === 1 ? p1Total : p2Total})</p>
           </div>
           {/* TUR, BAHİS VE SÜRE BİLGİSİ */}
           <div style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:2 }}>
              <span style={{ fontSize: 10, color: c.textSub, fontWeight: "bold", textTransform: "uppercase" }}>TUR {round}/13</span>
              {activeBet > 0 && (
                <span style={{ fontSize: 10, fontWeight: 800, color: c.gold, background: c.goldBg, padding: "2px 8px", borderRadius: 10, border: `1px solid ${c.gold}40` }}>
                  💰 Ödül: {activeBet * 2} 🪙
                </span>
              )}
              <span style={{ fontSize: 11, fontWeight: 800, color: turnTimeLeft <= 8 ? c.danger : c.accent }}>⏱ {turnTimeLeft}s</span>
           </div>
           {/* SAĞ OYUNCU (HER ZAMAN RAKİP) */}
           <div style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:2, opacity: !isMyTurn?1:0.4, transition:"0.3s" }}>
              <Av seed={opponentName ? opponentName.substring(0,2).toUpperCase() : "R"} size={32} active={!isMyTurn} dark={dark}/>
              <p style={{fontSize:10, fontWeight:"bold", color:!isMyTurn?c.accent:c.text}}>{opponentName || "Rakip"} ({myPlayerIndex === 1 ? p2Total : p1Total})</p>
           </div>
        </div>

        {/* Görsel Tur Süresi Çubuğu */}
        <div style={{ height: 3, background: c.panelAlt, margin: "8px 10px 0", borderRadius: 99, overflow: "hidden" }}>
          <div style={{
            height: "100%",
            width: `${(turnTimeLeft / 30) * 100}%`,
            background: turnTimeLeft <= 8 ? c.danger : c.accent,
            transition: "width 0.9s linear"
          }} />
        </div>

        {/* 2 Sütunlu Izgara Skorbord */}
        <div style={{ marginTop: 10, padding: "0 10px" }}>
           <div style={{ display: "flex", flexDirection: "column", gap: 1, background: c.borderMid, border: `1px solid ${c.borderMid}`, borderRadius: 8, overflow: "hidden" }}>
             {SCORE_GRID.map((row, rIdx) => (
               <div key={rIdx} style={{ display: "grid", gridTemplateColumns: "1fr 38px 38px 1fr 38px 38px", gap: 1 }}>
                  {row.map((cell, cIdx) => {
                     if (!cell) return <div key={cIdx} style={{ background: c.panel, gridColumn: "span 3" }} />;
                     
                     const isSpec = cell.spec;
                     const isHeader = isSpec;
                     const pot = !isSpec ? calcScore(cell.k, diceVals) : 0;

                     const myVal = isSpec ? (cell.k==='upper_total'?myUpper:(cell.k==='bonus'?myBonus:0)) : myScores[cell.k];
                     const myFilled = myVal != null;
                     const myCanScore = isMyTurn && canScore && !myFilled && !isSpec;
                     const myHighlight = myCanScore && pot > 0;

                     const oppVal = isSpec ? (cell.k==='upper_total'?oppUpper:(cell.k==='bonus'?oppBonus:0)) : oppScores[cell.k];
                     const oppFilled = oppVal != null;

                     return (
                         <React.Fragment key={cell.k}>
                           <div style={{ background: isHeader ? c.panelAlt : c.panel, padding: "5px 6px", display: "flex", alignItems: "center" }}>
                              <span style={{ fontSize: 11, fontWeight: isHeader?700:500, color: c.text }}>{cell.l}</span>
                           </div>
                           
                           {/* SOL SÜTUN (SEN) */}
                           <div onClick={() => myCanScore ? scoreIt(cell.k) : null} style={{ background: myHighlight ? c.accentBg : (isHeader ? c.panelAlt : c.panel), padding: "8px 0", display: "flex", alignItems: "center", justifyContent: "center", cursor: myCanScore?"pointer":"default" }}>
                              {myFilled || isSpec ? (
                                  <span style={{ fontSize: 13, fontWeight: 700, color: isHeader ? c.text : c.accent }}>{myVal ?? 0}</span>
                              ) : myCanScore ? (
                                  <span style={{ fontSize: 13, fontWeight: 600, color: pot>0 ? c.accent : c.textFaint }}>{pot}</span>
                              ) : (
                                  <span style={{ color: c.textFaint }}></span>
                              )}
                           </div>
                           
                           {/* SAĞ SÜTUN (RAKİP) */}
                           <div style={{ background: isHeader ? c.panelAlt : c.panel, padding: "8px 0", display: "flex", alignItems: "center", justifyContent: "center", cursor: "default" }}>
                              {oppFilled || isSpec ? (
                                  <span style={{ fontSize: 13, fontWeight: 700, color: isHeader ? c.text : c.accent }}>{oppVal ?? 0}</span>
                              ) : (
                                  <span style={{ color: c.textFaint }}></span>
                              )}
                           </div>
                         </React.Fragment>
                     );
                  })}
               </div>
             ))}
           </div>
        </div>
      </div>

      {/* Emoji Bildirimi */}
      <AnimatePresence>
        {floatingEmoji && (
          <motion.div
            key={floatingEmoji.id}
            initial={{ opacity: 0, y: 20, scale: 0.5 }}
            animate={{ opacity: 1, y: 0, scale: 1.2 }}
            exit={{ opacity: 0, y: -30, scale: 0.8 }}
            style={{
              position: "absolute",
              top: 90,
              left: floatingEmoji.fromMe ? 40 : undefined,
              right: !floatingEmoji.fromMe ? 40 : undefined,
              zIndex: 60,
              fontSize: 36,
              background: c.panel,
              borderRadius: "50%",
              padding: "6px 12px",
              boxShadow: `0 6px 20px ${c.shadow}`,
              border: `1px solid ${c.borderMid}`
            }}
          >
            {floatingEmoji.emoji}
          </motion.div>
        )}
      </AnimatePresence>

      <div style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "15px", gap: 15 }}>
        <div style={{ display: "flex", width: "100%", maxWidth: 360, height: 220, alignItems: "center", justifyContent: "space-between" }}>
           <div style={{ width: 44, height: 220, flexShrink: 0 }}></div>

           {/* Yuvarlak Zar Tepsisi */}
           <div style={{
              width: "220px", height: "220px", borderRadius: "50%", flexShrink: 0,
              background: "radial-gradient(circle at center, #27ae60 0%, #145a32 100%)",
              border: `12px solid ${dark?"#5c3a21":"#8e5a32"}`,
              boxShadow: "inset 0 10px 20px rgba(0,0,0,0.5), 0 8px 16px rgba(0,0,0,0.3)",
              position: "relative", display: "flex", alignItems: "center", justifyContent: "center",
              overflow: "hidden", transform: "translateZ(0)",
           }}>
              <div style={{ position:"absolute", inset:0, borderRadius:"50%", opacity:0.4, backgroundImage:`repeating-linear-gradient(45deg, transparent, transparent 2px, rgba(0,0,0,0.1) 2px, rgba(0,0,0,0.1) 4px)` }}/>
              <div style={{ position: "absolute", inset: 0, zIndex: 2 }}>
                {dieStates.map((s, i) => !locked[i] && (
                   <Die3D
                     key={`tray-die-${i}`} index={i} state={s} size={32}
                     locked={false} mat={diceMat} onClick={()=>toggleLock(i)} isRolling={rolling} isIdle={rollsLeft === 3 && !rolling}
                     style={{
                       position: "absolute",
                       left: "50%", top: "50%",
                       marginLeft: -16, marginTop: -16,
                       transform: `translate3d(${s.px}px, ${s.py}px, 0)`,
                       transition: "transform 0.55s cubic-bezier(0.22, 1, 0.36, 1)",
                     }}
                   />
                ))}
              </div>

              <AnimatePresence>
                {scorePopup && (
                  <motion.div
                    key={scorePopup.id} initial={{ opacity:0, y:0, scale:0.7 }} animate={{ opacity:1, y:-20, scale:1 }} exit={{ opacity:0, y:-50, scale:0.8 }} transition={{ duration:0.35 }}
                    style={{ position:"absolute", inset:0, zIndex:20, display:"flex", flexDirection:"column", alignItems:"center", justifyContent:"center", pointerEvents:"none" }}
                  >
                    <div style={{ background:"rgba(0,0,0,0.78)", borderRadius:16, padding:"10px 22px", display:"flex", flexDirection:"column", alignItems:"center", gap:2, backdropFilter:"blur(6px)", border:"1px solid rgba(255,255,255,0.12)" }}>
                      <span style={{ fontSize:10, color:"rgba(255,255,255,0.6)", fontWeight:600, letterSpacing:"0.06em", textTransform:"uppercase", textAlign:"center" }}>{scorePopup.label}</span>
                      <span style={{ fontSize: scorePopup.pts===0 ? 22 : 32, fontWeight:900, fontFamily:APP_FONT, lineHeight:1, color: scorePopup.pts===0 ? "#e74c3c" : scorePopup.pts>=40 ? "#f5c842" : "#4aada6" }}>
                        {scorePopup.pts===0 ? "Puan yok" : `+${scorePopup.pts}`}
                      </span>
                      {scorePopup.pts>0 && <span style={{ fontSize:11, color:"rgba(255,255,255,0.5)" }}>puan</span>}
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
           </div>

           <div style={{ display: "flex", flexDirection: "column", gap: 8, width: 44, height: 220, alignItems: "center", justifyContent: "center", flexShrink: 0, position: "relative" }}>
              {dieStates.map((s, i) => locked[i] && (
                 <Die3D key={`locked-die-${i}`} index={i} state={s} size={32} locked={true} mat={diceMat} onClick={()=>toggleLock(i)} isRolling={false} isIdle={false} />
              ))}
           </div>
        </div>

        <div style={{ width: "100%", maxWidth: 360, padding: "0 10px" }}>
          <motion.button
            whileTap={rollsLeft>0 && isMyTurn ? {scale:0.96} : {}}
            onClick={doRoll}
            disabled={rollsLeft===0 || rolling || !isMyTurn}
            style={{
              width:"100%", padding:"16px",
              background: rollsLeft>0 && isMyTurn ? c.accent : c.panelAlt,
              color: rollsLeft>0 && isMyTurn ? (dark?c.bg:"#fff") : c.textSub,
              fontSize:16, fontWeight:800, fontFamily:APP_FONT,
              borderRadius:12, border:"none", cursor:rollsLeft===0||rolling||!isMyTurn?"not-allowed":"pointer",
              boxShadow: rollsLeft>0 && isMyTurn ? `0 6px 0 ${dark?"#2f7a75":"#2c6e69"}` : "none",
              transform: rollsLeft===0||rolling||!isMyTurn ? "translateY(4px)" : "none",
              transition: "all 0.1s"
            }}
          >
            {isMyTurn 
              ? (rollsLeft===3 ? `Zar At` : (rollsLeft > 0 ? `Tekrar At (${rollsLeft}/3)` : `Tablodan Puan Seçin`)) 
              : "RAKİBİN HAMLESİ BEKLENİYOR..."}
          </motion.button>
        </div>
      </div>

      {/* RAKİP KAÇTI POP-UP'I */}
      <AnimatePresence>
        {opponentLeft && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }}
            style={{ 
              position: "absolute", inset: 0, background: "rgba(0,0,0,0.85)", zIndex: 100, 
              display: "flex", alignItems: "center", justifyContent: "center", padding: 20, backdropFilter: "blur(6px)" 
            }}
          >
            <motion.div 
              initial={{ scale: 0.8, y: 30 }} animate={{ scale: 1, y: 0 }}
              style={{ 
                background: c.panel, padding: "40px 32px", borderRadius: 24, textAlign: "center", 
                border: `2px solid ${c.accent}`, boxShadow: `0 15px 50px ${c.accentBg}` 
              }}
            >
              <div style={{ fontSize: 56, marginBottom: 16 }}>🏆</div>
              <h2 style={{ color: c.accent, fontSize: 26, fontWeight: 900, margin: 0, letterSpacing: "-0.02em" }}>HÜKMEN GALİP!</h2>
              <p style={{ color: c.textSub, fontSize: 14, marginTop: 8, fontWeight: 500 }}>Rakip oyundan ayrıldı.</p>
              
              <div style={{ display: "flex", justifyContent: "center", gap: 12, marginTop: 24 }}>
                <div style={{ background: c.accentBg, padding: "10px 16px", borderRadius: 12, border:`1px solid ${c.accentRing}` }}>
                  <span style={{ color: c.accent, fontWeight: 800, fontSize: 15 }}>+150 XP</span>
                </div>
                <div style={{ background: c.goldBg, padding: "10px 16px", borderRadius: 12, border:`1px solid rgba(201,168,76,0.3)` }}>
                  <span style={{ color: c.gold, fontWeight: 800, fontSize: 15 }}>+50 Jeton</span>
                </div>
              </div>
              
              <motion.button 
                whileTap={{ scale: 0.96 }}
                onClick={() => go("home")}
                style={{ 
                  width: "100%", marginTop: 24, padding: "16px", borderRadius: 14, 
                  background: c.accent, color: dark ? c.bg : "#fff", border: "none", 
                  fontWeight: 800, fontSize: 15, cursor: "pointer", boxShadow: `0 8px 20px ${c.accentBg}` 
                }}
              >
                Ana Sayfaya Dön
              </motion.button>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {showLeaveConfirm && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            style={{ 
              position: "absolute", inset: 0, background: "rgba(0,0,0,0.85)", zIndex: 100, 
              display: "flex", alignItems: "center", justifyContent: "center", padding: 20, backdropFilter: "blur(6px)" 
            }}
          >
            <motion.div 
              initial={{ scale: 0.8, y: 20 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.8, y: 20 }}
              style={{ 
                background: c.panel, padding: "32px 24px", borderRadius: 24, textAlign: "center", 
                width: "100%", maxWidth: 300, border: `1px solid ${c.border}`, boxShadow: `0 20px 40px ${c.shadow}` 
              }}
            >
              <div style={{ fontSize: 40, marginBottom: 12 }}>⚠️</div>
              <h3 style={{ color: c.text, fontSize: 18, fontWeight: 800, margin: 0 }}>Oyundan Ayrıl?</h3>
              <p style={{ color: c.textSub, fontSize: 13, marginTop: 8, lineHeight: 1.4 }}>
                Maçtan çıkarsan <span style={{ color: c.danger, fontWeight: 700 }}>hükmen mağlup</span> sayılacaksın. Emin misin?
              </p>
              
              <div style={{ display: "flex", gap: 10, marginTop: 24 }}>
                <button 
                  onClick={() => setShowLeaveConfirm(false)}
                  style={{ 
                    flex: 1, padding: "14px", borderRadius: 12, background: c.panelAlt, 
                    color: c.text, border: `1px solid ${c.border}`, fontWeight: 700, fontSize: 14, cursor: "pointer" 
                  }}
                >
                  Vazgeç
                </button>
                <button 
                  onClick={confirmLeave}
                  style={{ 
                    flex: 1, padding: "14px", borderRadius: 12, background: c.danger, 
                    color: "#fff", border: "none", fontWeight: 700, fontSize: 14, cursor: "pointer",
                    boxShadow: "0 4px 12px rgba(192,57,43,0.3)"
                  }}
                >
                  Evet, Çık
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Anti-Spam Uyarı Bildirimi */}
      <AnimatePresence>
        {spamWarn && (
          <motion.div
            initial={{ opacity: 0, y: -15 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -15 }}
            style={{
              position: "absolute", top: 44, left: 20, right: 20, zIndex: 95,
              background: "rgba(192, 57, 43, 0.95)", color: "#fff",
              padding: "8px 14px", borderRadius: 10, fontSize: 11, fontWeight: 800,
              textAlign: "center", boxShadow: "0 6px 20px rgba(0,0,0,0.35)"
            }}
          >
            🛡️ {spamWarn}
          </motion.div>
        )}
      </AnimatePresence>

      {(reconnecting || opponentOffline) && (
        <div role="status" style={{ position:"absolute", top:58, left:12, right:12, zIndex:90, background:c.panel, border:`1px solid ${c.gold}`, color:c.gold, borderRadius:10, padding:"8px 12px", fontSize:12, fontWeight:700, textAlign:"center", backdropFilter:"blur(6px)" }}>
          {reconnecting ? "Bağlantı koptu, maça geri dönülüyor..." : "Rakibin bağlantısı koptu, geri dönmesi bekleniyor..."}
        </div>
      )}

      {/* Rakip Sohbet Mesajı Önizleme Balonu (Sohbet kapalıyken) */}
      <AnimatePresence>
        {chatPreview && !chatOpen && (
          <motion.div
            initial={{ opacity: 0, y: -15 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -15 }}
            onClick={() => setChatOpen(true)}
            style={{
              position: "absolute", top: 48, right: 16, zIndex: 85,
              background: c.panel, border: `1.5px solid ${c.accent}`,
              padding: "8px 12px", borderRadius: 12, maxWidth: 240,
              cursor: "pointer", boxShadow: `0 8px 24px ${c.shadow}`
            }}
          >
            <p style={{ fontSize: 10, fontWeight: 800, color: c.accent, margin: 0 }}>💬 {chatPreview.senderName}</p>
            <p style={{ fontSize: 12, color: c.text, margin: "2px 0 0", wordBreak: "break-word" }}>{chatPreview.text}</p>
          </motion.div>
        )}
      </AnimatePresence>

      {/* OYUN İÇİ CANLI SOHBET PANELİ */}
      <AnimatePresence>
        {chatOpen && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={() => setChatOpen(false)}
            style={{
              position: "absolute", inset: 0, background: "rgba(0,0,0,0.65)", zIndex: 110,
              display: "flex", flexDirection: "column", justifyContent: "flex-end", backdropFilter: "blur(4px)"
            }}
          >
            <motion.div
              initial={{ y: 260 }} animate={{ y: 0 }} exit={{ y: 260 }}
              transition={{ type: "spring", damping: 22, stiffness: 260 }}
              onClick={e => e.stopPropagation()}
              style={{
                background: c.panel, borderTopLeftRadius: 24, borderTopRightRadius: 24,
                borderTop: `1.5px solid ${c.borderMid}`, padding: "16px 18px 20px",
                height: "62%", display: "flex", flexDirection: "column", gap: 10,
                boxShadow: "0 -12px 40px rgba(0,0,0,0.45)"
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: `1px solid ${c.border}`, paddingBottom: 10 }}>
                <div>
                  <h3 style={{ fontSize: 15, fontWeight: 800, color: c.text, margin: 0 }}>💬 Maç Sohbeti</h3>
                  <span style={{ fontSize: 10, color: c.textSub }}>Küfür ve spam koruması aktif</span>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <button
                    type="button"
                    onClick={toggleMuteBlockOpponent}
                    style={{
                      background: opponentMuted ? c.danger : c.panelAlt,
                      color: opponentMuted ? "#fff" : c.textSub,
                      border: `1px solid ${c.border}`, borderRadius: 8,
                      padding: "5px 8px", fontSize: 10, fontWeight: 700, cursor: "pointer"
                    }}
                  >
                    {opponentMuted ? "🔇 Engellendi" : "🚫 Sustur"}
                  </button>
                  <button
                    type="button"
                    onClick={reportOpponent}
                    style={{
                      background: "rgba(192,57,43,0.12)", color: c.danger,
                      border: `1px solid ${c.danger}40`, borderRadius: 8,
                      padding: "5px 8px", fontSize: 10, fontWeight: 700, cursor: "pointer"
                    }}
                  >
                    🚩 Raporla
                  </button>
                  <button onClick={() => setChatOpen(false)} style={{ background: c.panelAlt, border: `1px solid ${c.border}`, borderRadius: 8, padding: 6, color: c.textSub, cursor: "pointer", display: "flex" }}>
                    <X size={16} />
                  </button>
                </div>
              </div>

              {/* Hızlı Hazır Mesajlar */}
              <div style={{ display: "flex", gap: 6, overflowX: "auto", paddingBottom: 4 }}>
                {["Bol şans! 🍀", "İyi hamle! 👏", "Eyvah! 😅", "YAT geliyor! 🔥", "İyi oyundu! 🤝"].map(preset => (
                  <button
                    key={preset}
                    type="button"
                    disabled={chatCooldown}
                    onClick={() => sendChat(preset)}
                    style={{
                      flexShrink: 0, background: c.panelAlt, border: `1px solid ${c.border}`,
                      color: c.text, borderRadius: 14, padding: "5px 10px", fontSize: 11,
                      fontWeight: 600, cursor: chatCooldown ? "not-allowed" : "pointer",
                      opacity: chatCooldown ? 0.5 : 1
                    }}
                  >
                    {preset}
                  </button>
                ))}
              </div>

              {/* Mesaj Listesi */}
              <div style={{ flex: 1, overflowY: "auto", display: "flex", flexDirection: "column", gap: 8, padding: "4px 2px" }}>
                {chatMessages.length === 0 ? (
                  <div style={{ margin: "auto", textAlign: "center", color: c.textFaint, fontSize: 12 }}>
                    Henüz mesaj yok. Rakibine selam ver! 👋
                  </div>
                ) : (
                  chatMessages.map((m, idx) => {
                    const isMe = m.sender === myPlayerId;
                    return (
                      <div key={idx} style={{ display: "flex", flexDirection: "column", alignItems: isMe ? "flex-end" : "flex-start" }}>
                        <span style={{ fontSize: 9, color: c.textSub, marginBottom: 2, fontWeight: 600 }}>
                          {isMe ? "Sen" : m.senderName} • {m.time}
                        </span>
                        <div style={{
                          background: isMe ? c.accent : c.panelAlt,
                          color: isMe ? (dark ? c.bg : "#fff") : c.text,
                          padding: "8px 12px", borderRadius: 14,
                          borderBottomRightRadius: isMe ? 4 : 14,
                          borderBottomLeftRadius: isMe ? 14 : 4,
                          fontSize: 13, fontWeight: 600, maxWidth: "82%", wordBreak: "break-word"
                        }}>
                          {m.text}
                        </div>
                      </div>
                    );
                  })
                )}
              </div>

              {/* Mesaj Gönderme Formu */}
              <form
                onSubmit={e => { e.preventDefault(); sendChat(); }}
                style={{ display: "flex", gap: 8, marginTop: 4 }}
              >
                <input
                  type="text"
                  maxLength={120}
                  value={chatInput}
                  onChange={e => setChatInput(e.target.value)}
                  placeholder={chatCooldown ? "Spam koruması: Lütfen bekleyin..." : "Mesaj yaz (Maks 120 kr)..."}
                  style={{
                    flex: 1, background: c.bg, border: `1px solid ${c.border}`,
                    borderRadius: 12, padding: "11px 14px", color: c.text,
                    fontSize: 13, outline: "none", fontWeight: 600
                  }}
                />
                <button
                  type="submit"
                  disabled={chatCooldown || !chatInput.trim()}
                  style={{
                    background: chatCooldown ? c.panelAlt : c.accent,
                    color: chatCooldown ? c.textFaint : (dark ? c.bg : "#fff"),
                    border: "none", borderRadius: 12, padding: "0 16px",
                    fontWeight: 800, fontSize: 13,
                    cursor: (chatCooldown || !chatInput.trim()) ? "not-allowed" : "pointer"
                  }}
                >
                  {chatCooldown ? "⏳" : "Gönder"}
                </button>
              </form>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ─── Screen 4: Result (Gerçek Maç Skorları & Ödüller) ────────────────────────
function ResultScreen({
  go, dark, profile, summary, onPlayAgain
}: {
  go:(s:Screen)=>void; dark:boolean; profile:UserProfile; summary:MatchSummary|null; onPlayAgain:()=>void;
}) {
  const c = C(dark);

  const myTotal = summary ? summary.myTotal : 0;
  const oppTotal = summary ? summary.oppTotal : 0;
  const oppName = summary ? summary.oppName : "Rakip";
  const oppSeed = summary ? summary.oppSeed : "RK";
  const rewardXp = summary ? summary.rewardXp : 150;
  const rewardTokens = summary ? summary.rewardTokens : 40;
  const isDraw = myTotal === oppTotal;
  const won = myTotal > oppTotal;

  const rows = [
    ...CATS.slice(0, 6).map(cat => ({
      l: cat.label,
      m: summary?.myScores[cat.key] ?? 0,
      o: summary?.oppScores[cat.key] ?? 0
    })),
    { l: "Üst Bonus", m: summary?.myBonus ?? 0, o: summary?.oppBonus ?? 0 },
    ...CATS.slice(6).map(cat => ({
      l: cat.label,
      m: summary?.myScores[cat.key] ?? 0,
      o: summary?.oppScores[cat.key] ?? 0
    })),
  ];

  return (
    <div style={{ minHeight:"100%", background:c.bg, fontFamily:APP_FONT, display:"flex", flexDirection:"column" }}>
      <div style={{ background:c.panel, borderBottom:`1px solid ${c.border}`, padding:"28px 24px 20px", display:"flex", flexDirection:"column", alignItems:"center", gap:12 }}>
        <motion.div initial={{ scale:0.5,opacity:0 }} animate={{ scale:1,opacity:1 }} transition={{ type:"spring",damping:14,delay:0.05 }}
          style={{ width:64, height:64, borderRadius:"50%", background:isDraw?c.goldBg:(won?c.accentBg:"rgba(192,57,43,0.08)"), border:`1.5px solid ${isDraw?c.gold:(won?c.accentRing:"rgba(192,57,43,0.2)")}`, display:"flex", alignItems:"center", justifyContent:"center" }}>
          {isDraw ? <span style={{ fontSize: 28 }}>🤝</span> : won ? <Crown size={28} color={c.accent}/> : <Trophy size={28} color={c.danger}/>}
        </motion.div>
        <div style={{ textAlign:"center" }}>
          <h1 style={{ fontSize:24, fontWeight:800, color:isDraw?c.gold:(won?c.accent:c.danger) }}>{isDraw ? "Berabere!" : (won ? "Galibiyet!" : "Mağlubiyet")}</h1>
          <p style={{ fontSize:12, color:c.textSub, marginTop:2 }}>{isDraw ? "Baş başa mücadele! Giriş jetonları iade edildi." : (won ? "Harika oynandı!" : "Bir dahaki sefere!")}</p>
        </div>
        <div style={{ display:"flex", gap:28, alignItems:"flex-end" }}>
          {[{name:profile.name,seed:profile.seed,score:myTotal,you:true},{name:oppName,seed:oppSeed,score:oppTotal,you:false}].map((p, idx)=>(
            <div key={idx} style={{ display:"flex", flexDirection:"column", alignItems:"center", gap:5 }}>
              <Av seed={p.seed} size={40} active={p.you} dark={dark}/>
              <p style={{ fontSize:11, color:c.textSub }}>{p.name}</p>
              <p style={{ fontSize:26, fontWeight:800, color:p.you?c.accent:c.text, lineHeight:1 }}>{p.score}</p>
            </div>
          ))}
        </div>
      </div>

      <div style={{ padding:"14px 20px 0" }}>
        <div style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:14, padding:"13px 16px" }}>
          <div style={{ display:"flex", justifyContent:"space-between", marginBottom:7 }}>
            <div style={{ display:"flex", alignItems:"center", gap:6 }}>
              <TrendingUp size={13} color={c.gold}/>
              <span style={{ fontSize:13, fontWeight:600, color:c.text }}>+{rewardXp} XP & +{rewardTokens} 🪙 kazanıldı</span>
            </div>
            <span style={{ fontSize:11, color:c.textSub }}>Seviye {profile.level}</span>
          </div>
          <div style={{ height:5, background:c.panelAlt, borderRadius:99, overflow:"hidden" }}>
            <motion.div initial={{ width:"20%" }} animate={{ width:`${((profile.xp % 1000)/1000)*100}%` }} transition={{ duration:1.4, ease:"easeOut", delay:0.4 }}
              style={{ height:"100%", background:c.gold, borderRadius:99 }}/>
          </div>
        </div>
      </div>

      <div style={{ padding:"14px 20px 0" }}>
        <div style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:14, overflow:"hidden" }}>
          <div style={{ display:"grid", gridTemplateColumns:"1fr 54px 54px", padding:"8px 14px", background:c.panelAlt, borderBottom:`1px solid ${c.border}` }}>
            <span style={{ fontSize:9, color:c.textFaint, fontWeight:600, letterSpacing:"0.08em", textTransform:"uppercase" }}>Kategori</span>
            <span style={{ fontSize:9, color:c.accent, fontWeight:700, textAlign:"right", letterSpacing:"0.06em" }}>Sen</span>
            <span style={{ fontSize:9, color:c.textSub, fontWeight:600, textAlign:"right", letterSpacing:"0.06em" }}>{oppName.substring(0,8)}</span>
          </div>
          <div style={{ maxHeight:220, overflowY:"auto" }}>
            {rows.map((r,i)=>{
              const iW=r.m>r.o, tW=r.o>r.m;
              return (
                <div key={r.l} style={{ display:"grid", gridTemplateColumns:"1fr 54px 54px", padding:"8px 14px", borderBottom:i<rows.length-1?`1px solid ${c.border}`:"none", background:iW?c.accentBg:"transparent" }}>
                  <span style={{ fontSize:12, color:c.textSub }}>{r.l}</span>
                  <span style={{ fontSize:13, fontWeight:iW?700:500, color:iW?c.accent:c.textSub, textAlign:"right" }}>{r.m}</span>
                  <span style={{ fontSize:13, fontWeight:tW?600:400, color:tW?c.text:c.textFaint, textAlign:"right" }}>{r.o}</span>
                </div>
              );
            })}
          </div>
          <div style={{ display:"grid", gridTemplateColumns:"1fr 54px 54px", padding:"10px 14px", background:c.panelAlt, borderTop:`1.5px solid ${c.borderMid}` }}>
            <span style={{ fontSize:13, fontWeight:800, color:c.text }}>TOPLAM</span>
            <span style={{ fontSize:15, fontWeight:800, color:c.accent, textAlign:"right" }}>{myTotal}</span>
            <span style={{ fontSize:15, fontWeight:700, color:c.textSub, textAlign:"right" }}>{oppTotal}</span>
          </div>
        </div>
      </div>

      <div style={{ padding:"14px 20px 32px", display:"flex", flexDirection:"column", gap:10, marginTop:"auto" }}>
        <motion.button whileTap={{ scale:0.97 }} onClick={onPlayAgain} style={{
          width:"100%", padding:"15px", background:c.accent, color:dark?c.bg:"#fff",
          fontSize:15, fontWeight:700, fontFamily:APP_FONT, borderRadius:14, border:"none",
          cursor:"pointer", letterSpacing:"0.04em", boxShadow:dark?`0 4px 16px rgba(74,173,166,0.32)`:`0 4px 16px rgba(61,139,133,0.26)`,
        }}>Yeni Maç Ara</motion.button>
        <motion.button whileTap={{ scale:0.97 }} onClick={()=>go("home")} style={{
          width:"100%", padding:"15px", background:"transparent", color:c.text,
          fontSize:15, fontWeight:500, fontFamily:APP_FONT, borderRadius:14, border:`1.5px solid ${c.borderMid}`, cursor:"pointer",
        }}>Ana Sayfaya Dön</motion.button>
      </div>
    </div>
  );
}


// ─── Screen 5: Modes ──────────────────────────────────────────────────────────
function ModeScreen({
  dark, onSelectMode, betAmount, setBetAmount, tokens
}: {
  dark:boolean; onSelectMode:(mode: "QUICK_MATCH" | "BOT_MATCH", bet?: BetOption)=>void;
  betAmount:BetOption; setBetAmount:(b:BetOption)=>void; tokens:number;
}) {
  const c = C(dark);

  const modes: { key: string; label: string; sub: string; icon: string; available: boolean; action: "QUICK_MATCH" | "BOT_MATCH"; requiredBet?: BetOption }[] = [
    {
      key: "bot", label: "Bota Karşı (Yapay Zeka)", sub: "Beklemeden anında YatBot ile oyna",
      icon: "🤖", available: true, action: "BOT_MATCH",
    },
    {
      key: "classic", label: "Klasik Çevrimiçi Arena", sub: "Gerçek oyuncularla hızlı eşleşme",
      icon: "🎲", available: true, action: "QUICK_MATCH",
    },
    {
      key: "highroller", label: "Ustalar Arenası", sub: "Giriş 250 🪙 · Ödül 500 🪙",
      icon: "🔥", available: true, action: "QUICK_MATCH", requiredBet: 250,
    },
    {
      key: "viproyal", label: "VIP Kraliyet Masası", sub: "Giriş 500 🪙 · Ödül 1.000 🪙",
      icon: "👑", available: true, action: "QUICK_MATCH", requiredBet: 500,
    },
  ];

  return (
    <div style={{ minHeight:"100%", background:c.bg, fontFamily:APP_FONT, display:"flex", flexDirection:"column" }}>
      <div style={{ padding:"28px 20px 18px", borderBottom:`1px solid ${c.border}` }}>
        <h2 style={{ fontSize:22, fontWeight:800, color:c.text, marginBottom:4 }}>Mod Seç</h2>
        <p style={{ fontSize:12, color:c.textSub }}>Masa giriş jetonunu seç ve maça başla</p>
      </div>

      {/* Üstte dekoratif dönen 3D zarlar - uygun boşluklarla hizalandı */}
      <div style={{ padding:"18px 20px 10px", display:"flex", justifyContent:"center", gap:18 }}>
        {([2,4,6,1,3] as DieVal[]).map((v,i)=>(
          <Die3D key={i} index={i} state={initDie(v,i)} size={34} locked={false} mat="ivory" isIdle={true}/>
        ))}
      </div>

      {/* Mod Ekranı Masa Giriş Jetonu Seçici */}
      <div style={{ padding:"6px 20px 4px" }}>
        <div style={{ background:c.panel, border:`1px solid ${c.border}`, borderRadius:14, padding:"11px 14px" }}>
          <div style={{ display:"flex", justifyContent:"space-between", alignItems:"center", marginBottom:8 }}>
            <span style={{ fontSize:10, fontWeight:800, color:c.textSub, textTransform:"uppercase", letterSpacing:"0.06em", whiteSpace:"nowrap" }}>
              Masa Giriş Jetonu
            </span>
            <span style={{ fontSize:11, fontWeight:800, color:c.gold, whiteSpace:"nowrap" }}>
              {betAmount === 0 ? "Dostluk Maçı" : `Ödül: ${betAmount * 2} 🪙`}
            </span>
          </div>
          <div style={{ display:"grid", gridTemplateColumns:"repeat(5, 1fr)", gap:6 }}>
            {BET_OPTIONS.map(b => {
              const active = betAmount === b;
              const canAfford = tokens >= b;
              return (
                <button
                  key={b}
                  type="button"
                  disabled={!canAfford}
                  onClick={() => setBetAmount(b)}
                  style={{
                    padding:"7px 4px", borderRadius:10, fontSize:11, fontWeight:800, whiteSpace:"nowrap",
                    cursor: !canAfford ? "not-allowed" : "pointer",
                    background: active ? c.gold : c.panelAlt,
                    color: active ? "#1a1405" : (canAfford ? c.text : c.textFaint),
                    border: active ? `1.5px solid ${c.gold}` : `1px solid ${c.border}`,
                    opacity: canAfford ? 1 : 0.45,
                  }}
                >
                  {b === 0 ? "Ücretsiz" : `🪙 ${b}`}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div style={{ padding:"16px 20px 28px", display:"flex", flexDirection:"column", gap:12, flex:1 }}>
        {modes.map((m, i) => (
          <motion.div
            key={m.key}
            initial={{ opacity:0, y:16 }}
            animate={{ opacity:1, y:0 }}
            transition={{ delay: i*0.06 }}
            whileTap={m.available ? { scale:0.97 } : {}}
            onClick={() => {
              if (!m.available) return;
              if (m.requiredBet) {
                if (tokens < m.requiredBet) {
                  emitToast(`Bu masa için en az ${m.requiredBet} 🪙 jeton gerekli!`);
                  return;
                }
                setBetAmount(m.requiredBet);
              }
              onSelectMode(m.action, m.requiredBet);
            }}
            style={{
              background: m.available ? c.panel : c.panelAlt,
              border: m.available ? `1px solid ${c.borderMid}` : `1px solid ${c.border}`,
              borderRadius:18, padding:"16px 18px",
              cursor: m.available ? "pointer" : "default",
              display:"flex", alignItems:"center", gap:14,
              opacity: m.available ? 1 : 0.55,
              boxShadow: m.available ? `0 2px 12px ${c.shadow}` : "none",
            }}
          >
            <div style={{
              width:48, height:48, borderRadius:14,
              background: m.available ? c.accentBg : c.panelAlt,
              border: m.available ? `1px solid ${c.accentRing}` : `1px solid ${c.border}`,
              display:"flex", alignItems:"center", justifyContent:"center",
              fontSize:22, flexShrink:0,
            }}>
              {m.icon}
            </div>
            <div style={{ flex:1, minWidth:0 }}>
              <p style={{ fontSize:15, fontWeight:700, color: m.available ? c.text : c.textSub }}>{m.label}</p>
              <p style={{ fontSize:11, color: m.available ? c.accent : c.textFaint, marginTop:2, fontWeight: m.available?500:400 }}>{m.sub}</p>
            </div>
            {m.available && (
              <div style={{
                padding:"6px 14px", background:c.accent, borderRadius:20,
                fontSize:12, fontWeight:700, color: dark ? c.bg : "#fff",
                boxShadow: dark ? `0 3px 12px rgba(74,173,166,0.35)` : `0 3px 12px rgba(61,139,133,0.28)`,
              }}>
                Oyna
              </div>
            )}
            {!m.available && (
              <div style={{
                padding:"4px 10px", background:"transparent",
                border:`1px solid ${c.border}`,
                borderRadius:20, fontSize:11, fontWeight:500, color:c.textFaint,
              }}>
                Yakında
              </div>
            )}
          </motion.div>
        ))}
      </div>
    </div>
  );
}


const TABS = [
  { id:"home",    Icon:Home,        label:"Anasayfa"   },
  { id:"quests",  Icon:Trophy,      label:"Görevler"   },
  { id:"modes",   Icon:Zap,         label:"Oyna"       },
  { id:"friends", Icon:Users,       label:"Arkadaşlar" },
  { id:"shop",    Icon:ShoppingBag, label:"Koleksiyon" },
];

// ─── Screen 7: Quests (Görev Modu — Normal & Zor Kasa Ödüllü Görevler) ──────
function QuestsScreen({
  dark, profile, setProfile, token
}: {
  dark: boolean;
  profile: UserProfile;
  setProfile: React.Dispatch<React.SetStateAction<UserProfile>>;
  token: string | null;
}) {
  const c = C(dark);
  const [claimingId, setClaimingId] = useState<string | null>(null);
  const [questCasePopup, setQuestCasePopup] = useState<{ title: string; xp: number; tokens: number; cases: number; bonusGold?: number } | null>(null);

  const claimedSet = new Set(profile.claimedQuests || []);

  const claimQuest = async (q: QuestItemDef) => {
    if (claimingId || claimedSet.has(q.id)) return;
    setClaimingId(q.id);
    try {
      if (token && !token.startsWith("guest_token_")) {
        const res = await fetch(`${API_BASE}/quests/claim`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + token
          },
          body: JSON.stringify({ questId: q.id })
        });
        if (!res.ok) {
          const txt = await res.text();
          emitToast(txt || "Görev ödülü alınamadı");
          return;
        }
        sfx.win();
        const data = await res.json();
        const p = data.profile;
        setProfile(prev => ({
          ...prev,
          xp: p.xp,
          level: computeLevel(p.xp),
          tokens: p.tokens,
          cases: p.cases,
          claimedQuests: p.claimedQuests || [...(prev.claimedQuests || []), q.id]
        }));
      } else {
        setProfile(prev => {
          const nextXp = prev.xp + q.xp;
          return {
            ...prev,
            xp: nextXp,
            level: computeLevel(nextXp),
            tokens: prev.tokens + q.tokens,
            cases: prev.cases + q.cases,
            claimedQuests: [...(prev.claimedQuests || []), q.id]
          };
        });
      }

      if (q.cases > 0) {
        setQuestCasePopup({ title: q.title, xp: q.xp, tokens: q.tokens, cases: q.cases });
      }
    } finally {
      setClaimingId(null);
    }
  };

  const openBonusCaseNow = async () => {
    if (!questCasePopup || profile.cases <= 0) {
      setQuestCasePopup(null);
      return;
    }
    if (token && !token.startsWith("guest_token_")) {
      try {
        const res = await fetch(`${API_BASE}/profile/open-case`, {
          method: "POST",
          headers: { "Authorization": "Bearer " + token }
        });
        if (res.ok) {
          const data = await res.json();
          setProfile(prev => ({
            ...prev,
            cases: data.profile.cases,
            tokens: data.profile.tokens
          }));
          setQuestCasePopup(prev => prev ? { ...prev, bonusGold: data.reward } : null);
          return;
        }
      } catch {}
    }
    const reward = Math.floor(Math.random() * 150) + 75;
    setProfile(prev => ({
      ...prev,
      cases: Math.max(0, prev.cases - 1),
      tokens: prev.tokens + reward
    }));
    setQuestCasePopup(prev => prev ? { ...prev, bonusGold: reward } : null);
  };

  const renderQuestCard = (q: QuestItemDef) => {
    const prog = q.getProgress(profile);
    const pct = Math.min(100, Math.round((prog.current / prog.target) * 100));
    const isCompleted = prog.current >= prog.target;
    const isClaimed = claimedSet.has(q.id);
    const isHard = q.difficulty === "hard";

    return (
      <div
        key={q.id}
        style={{
          background: isClaimed ? c.panelAlt : c.panel,
          border: isHard
            ? `1.5px solid ${isCompleted && !isClaimed ? c.gold : "rgba(201,168,76,0.32)"}`
            : `1px solid ${isCompleted && !isClaimed ? c.accent : c.border}`,
          borderRadius: 16,
          padding: "14px 16px",
          opacity: isClaimed ? 0.65 : 1,
          display: "flex",
          flexDirection: "column",
          gap: 10,
          boxShadow: isCompleted && !isClaimed ? `0 6px 20px ${c.shadow}` : "none"
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 10 }}>
          <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
            <div style={{
              width: 44, height: 44, borderRadius: 12,
              background: isHard ? c.goldBg : c.accentBg,
              border: `1px solid ${isHard ? "rgba(201,168,76,0.4)" : c.accentRing}`,
              display: "flex", alignItems: "center", justifyContent: "center", fontSize: 22, flexShrink: 0
            }}>
              {q.icon}
            </div>
            <div>
              <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                <span style={{ fontSize: 14, fontWeight: 800, color: c.text }}>{q.title}</span>
                {isHard && (
                  <span style={{ fontSize: 9, fontWeight: 900, background: c.gold, color: "#1a1405", padding: "2px 6px", borderRadius: 6, textTransform: "uppercase" }}>
                    ZOR • KASA ÖDÜLLÜ
                  </span>
                )}
              </div>
              <p style={{ fontSize: 11, color: c.textSub, margin: "3px 0 0", lineHeight: 1.35 }}>{q.desc}</p>
            </div>
          </div>
        </div>

        {/* Ödül Rozetleri */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
            <span style={{ fontSize: 11, fontWeight: 800, color: c.accent, background: c.accentBg, padding: "4px 8px", borderRadius: 8 }}>
              ✨ +{q.xp} XP
            </span>
            <span style={{ fontSize: 11, fontWeight: 800, color: c.gold, background: c.goldBg, padding: "4px 8px", borderRadius: 8 }}>
              🪙 +{q.tokens} Jeton
            </span>
            {q.cases > 0 && (
              <span style={{ fontSize: 11, fontWeight: 800, color: "#f39c12", background: "rgba(243,156,18,0.14)", padding: "4px 8px", borderRadius: 8, border: "1px solid rgba(243,156,18,0.35)" }}>
                📦 +{q.cases} Kasa
              </span>
            )}
          </div>

          {isClaimed ? (
            <span style={{ fontSize: 11, fontWeight: 800, color: c.accent, display: "flex", alignItems: "center", gap: 4 }}>
              <Check size={14} /> Tamamlandı
            </span>
          ) : isCompleted ? (
            <motion.button
              whileTap={{ scale: 0.95 }}
              onClick={() => claimQuest(q)}
              disabled={claimingId === q.id}
              style={{
                background: isHard ? c.gold : c.accent,
                color: isHard ? "#1a1405" : (dark ? c.bg : "#fff"),
                border: "none", borderRadius: 10, padding: "8px 14px",
                fontSize: 12, fontWeight: 900, cursor: "pointer",
                boxShadow: `0 4px 14px ${isHard ? c.goldBg : c.accentBg}`
              }}
            >
              {claimingId === q.id ? "Alınıyor..." : (q.cases > 0 ? "🎁 Ödül & Kasa Al" : "🎁 Ödülü Al")}
            </motion.button>
          ) : (
            <span style={{ fontSize: 11, fontWeight: 700, color: c.textSub, whiteSpace: "nowrap" }}>
              {prog.current} / {prog.target}
            </span>
          )}
        </div>

        {/* İlerleme Çubuğu */}
        <div style={{ height: 5, background: c.bg, borderRadius: 99, overflow: "hidden" }}>
          <div style={{
            height: "100%",
            width: `${pct}%`,
            background: isHard ? c.gold : c.accent,
            borderRadius: 99,
            transition: "width 0.4s ease"
          }} />
        </div>
      </div>
    );
  };

  return (
    <div style={{ minHeight: "100%", background: c.bg, fontFamily: APP_FONT, display: "flex", flexDirection: "column", paddingBottom: 32 }}>
      <div style={{ padding: "28px 20px 18px", borderBottom: `1px solid ${c.border}`, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <h2 style={{ fontSize: 22, fontWeight: 800, color: c.text, marginBottom: 4 }}>Görev Modu</h2>
          <p style={{ fontSize: 12, color: c.textSub }}>Görevleri tamamla; XP, Jeton ve Kasa kazan!</p>
        </div>
        <div style={{ background: c.accentBg, border: `1px solid ${c.accentRing}`, borderRadius: 14, padding: "8px 12px", textAlign: "center" }}>
          <p style={{ fontSize: 14, fontWeight: 900, color: c.accent, margin: 0 }}>{claimedSet.size} / {QUEST_LIST.length}</p>
          <p style={{ fontSize: 9, color: c.textSub, fontWeight: 700, margin: 0, textTransform: "uppercase" }}>Tamamlanan</p>
        </div>
      </div>

      <div style={{ padding: "18px 20px", display: "flex", flexDirection: "column", gap: 18 }}>
        <div>
          <p style={{ fontSize: 10, fontWeight: 800, color: c.accent, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 10 }}>
            🎯 Normal Görevler
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {QUEST_LIST.filter(q => q.difficulty === "normal").map(renderQuestCard)}
          </div>
        </div>

        <div>
          <p style={{ fontSize: 10, fontWeight: 800, color: c.gold, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 10 }}>
            🔥 Zor & Efsanevi Görevler
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {QUEST_LIST.filter(q => q.difficulty === "hard").map(renderQuestCard)}
          </div>
        </div>
      </div>

      {/* Zor Görev Tamamlandığında Anında Kasa Açma Modalı */}
      <AnimatePresence>
        {questCasePopup && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.85)", zIndex: 260, display: "flex", alignItems: "center", justifyContent: "center", padding: 20, backdropFilter: "blur(8px)" }}
          >
            <motion.div
              initial={{ scale: 0.85, y: 20 }} animate={{ scale: 1, y: 0 }} exit={{ scale: 0.85, y: 20 }}
              style={{
                background: c.panel, borderRadius: 28, width: "100%", maxWidth: 340,
                padding: "28px 24px", border: `2px solid ${c.gold}`,
                boxShadow: `0 20px 60px ${c.goldBg}`, textAlign: "center",
                display: "flex", flexDirection: "column", alignItems: "center", gap: 14
              }}
            >
              <div style={{ fontSize: 68 }}>{questCasePopup.bonusGold ? "🪙" : "📦"}</div>
              <h3 style={{ fontSize: 20, fontWeight: 900, color: c.gold, margin: 0 }}>
                {questCasePopup.bonusGold ? `Kasadan +${questCasePopup.bonusGold} 🪙 Çıktı!` : "Zor Görev Kasası Kazandın!"}
              </h3>
              <p style={{ fontSize: 12, color: c.textSub, margin: 0, lineHeight: 1.45 }}>
                <strong>{questCasePopup.title}</strong> görevinden <strong>+{questCasePopup.xp} XP</strong>, <strong>+{questCasePopup.tokens} 🪙</strong> ve <strong>{questCasePopup.cases} Kasa</strong> hesabına eklendi!
              </p>

              {!questCasePopup.bonusGold ? (
                <div style={{ display: "flex", gap: 10, width: "100%", marginTop: 6 }}>
                  <button
                    onClick={() => setQuestCasePopup(null)}
                    style={{ flex: 1, padding: "13px", borderRadius: 12, background: c.panelAlt, color: c.textSub, border: `1px solid ${c.border}`, fontWeight: 700, fontSize: 12, cursor: "pointer" }}
                  >
                    Sonra Aç
                  </button>
                  <button
                    onClick={openBonusCaseNow}
                    style={{ flex: 1.4, padding: "13px", borderRadius: 12, background: c.gold, color: "#1a1405", border: "none", fontWeight: 900, fontSize: 13, cursor: "pointer" }}
                  >
                    📦 Kasayı Şimdi Aç!
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setQuestCasePopup(null)}
                  style={{ width: "100%", padding: "14px", borderRadius: 14, background: c.accent, color: dark ? c.bg : "#fff", border: "none", fontWeight: 800, fontSize: 14, cursor: "pointer", marginTop: 6 }}
                >
                  Harika, Devam Et
                </button>
              )}
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ─── Screen 6: Friends (Arkadaşlar & Oyun Davetleri) ─────────────────────────
function FriendsScreen({
  dark, token, friends, incoming, outgoing, onRefresh, onInviteToGame
}: {
  dark: boolean;
  token: string | null;
  friends: FriendItem[];
  incoming: FriendItem[];
  outgoing: FriendItem[];
  onRefresh: () => void;
  onInviteToGame: (friendUserId: number) => void;
}) {
  const c = C(dark);
  const [searchName, setSearchName] = useState("");
  const [blockedUsers, setBlockedUsers] = useState<BlockedUser[]>([]);
  const [statusMsg, setStatusMsg] = useState<{ text: string; isErr: boolean } | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!token) return;
    let active = true;
    fetch(`${API_BASE}/ugc/blocks`, { headers: { "Authorization": "Bearer " + token } })
      .then(res => (res.ok ? res.json() : null))
      .then(data => {
        if (active && data) setBlockedUsers(data.blocked || []);
      })
      .catch(() => {});
    return () => { active = false; };
  }, [token]);

  const unblockUser = async (userId: number) => {
    try {
      const res = await fetch(`${API_BASE}/ugc/unblock`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": "Bearer " + token },
        body: JSON.stringify({ userId })
      });
      if (res.ok) setBlockedUsers(prev => prev.filter(item => item.userId !== userId));
    } catch {
      setStatusMsg({ text: "Engel kaldırılamadı.", isErr: true });
    }
  };

  const isGuest = !token || token.startsWith("guest_token_");

  const sendRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchName.trim() || isGuest) return;
    setLoading(true);
    setStatusMsg(null);
    try {
      const res = await fetch(`${API_BASE}/friends/request`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + token
        },
        body: JSON.stringify({ username: searchName.trim() })
      });
      if (!res.ok) {
        const txt = await res.text();
        throw new Error(txt || "İstek gönderilemedi");
      }
      setStatusMsg({ text: "Arkadaşlık isteği gönderildi!", isErr: false });
      setSearchName("");
      onRefresh();
    } catch (err: any) {
      setStatusMsg({ text: err.message, isErr: true });
    } finally {
      setLoading(false);
    }
  };

  const respondRequest = async (friendshipId: number, action: "accept" | "reject" | "remove") => {
    if (isGuest) return;
    try {
      await fetch(`${API_BASE}/friends/respond`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + token
        },
        body: JSON.stringify({ friendshipId, action })
      });
      onRefresh();
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div style={{ minHeight: "100%", background: c.bg, fontFamily: APP_FONT, display: "flex", flexDirection: "column", paddingBottom: 32 }}>
      <div style={{ padding: "28px 20px 18px", borderBottom: `1px solid ${c.border}` }}>
        <h2 style={{ fontSize: 22, fontWeight: 800, color: c.text, marginBottom: 4 }}>Arkadaşlar</h2>
        <p style={{ fontSize: 12, color: c.textSub }}>Arkadaş ekle ve tek tıkla maça davet et</p>
      </div>

      {isGuest ? (
        <div style={{ padding: 24, margin: 20, background: c.panel, borderRadius: 18, border: `1px solid ${c.border}`, textAlign: "center" }}>
          <p style={{ fontSize: 28, margin: "0 0 8px" }}>🔒</p>
          <p style={{ fontSize: 14, fontWeight: 700, color: c.text }}>Misafir Hesabındasınız</p>
          <p style={{ fontSize: 12, color: c.textSub, marginTop: 6, lineHeight: 1.5 }}>
            Arkadaş eklemek ve arkadaşlarınıza oyun daveti göndermek için lütfen kayıtlı bir hesapla giriş yapın.
          </p>
        </div>
      ) : (
        <div style={{ padding: "18px 20px", display: "flex", flexDirection: "column", gap: 18 }}>
          {/* Arkadaş Ekleme Formu */}
          <form onSubmit={sendRequest} style={{ background: c.panel, padding: 16, borderRadius: 16, border: `1px solid ${c.border}`, display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ fontSize: 11, fontWeight: 700, color: c.textSub, textTransform: "uppercase", letterSpacing: "0.06em", margin: 0 }}>
              Kullanıcı Adı ile Arkadaş Ekle
            </p>
            <div style={{ display: "flex", gap: 8 }}>
              <input
                type="text"
                placeholder="Örn: Utku"
                value={searchName}
                onChange={e => setSearchName(e.target.value)}
                style={{ flex: 1, background: c.bg, border: `1px solid ${c.border}`, padding: "11px 14px", borderRadius: 12, color: c.text, fontSize: 13, fontWeight: 600, outline: "none" }}
              />
              <motion.button
                whileTap={{ scale: 0.96 }}
                type="submit"
                disabled={loading || !searchName.trim()}
                style={{ padding: "0 18px", background: c.accent, color: dark ? c.bg : "#fff", border: "none", borderRadius: 12, fontWeight: 800, fontSize: 13, cursor: "pointer" }}
              >
                Ekle
              </motion.button>
            </div>
            {statusMsg && (
              <p style={{ fontSize: 12, fontWeight: 600, color: statusMsg.isErr ? c.danger : c.accent, margin: 0 }}>
                {statusMsg.text}
              </p>
            )}
          </form>

          {/* Gelen Arkadaşlık İstekleri */}
          {incoming.length > 0 && (
            <div>
              <p style={{ fontSize: 10, fontWeight: 700, color: c.gold, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
                Gelen İstekler ({incoming.length})
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {incoming.map(req => (
                  <div key={req.friendshipId} style={{ background: c.panel, border: `1px solid ${c.gold}50`, borderRadius: 14, padding: "12px 14px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <Av seed={req.username.substring(0, 2).toUpperCase()} size={36} dark={dark} avatar={req.avatar} country={req.country} />
                      <div>
                        <p style={{ fontSize: 14, fontWeight: 700, color: c.text, margin: 0 }}>{req.username}</p>
                        <p style={{ fontSize: 11, color: c.textSub, margin: 0 }}>Seviye {computeLevel(req.xp)} • {req.wins} Galibiyet</p>
                      </div>
                    </div>
                    <div style={{ display: "flex", gap: 6 }}>
                      <button onClick={() => respondRequest(req.friendshipId, "accept")} style={{ background: c.accent, color: dark ? c.bg : "#fff", border: "none", borderRadius: 10, padding: "7px 12px", fontSize: 12, fontWeight: 700, cursor: "pointer" }}>
                        Kabul Et
                      </button>
                      <button onClick={() => respondRequest(req.friendshipId, "reject")} style={{ background: c.panelAlt, color: c.textSub, border: `1px solid ${c.border}`, borderRadius: 10, padding: "7px 10px", fontSize: 12, fontWeight: 600, cursor: "pointer" }}>
                        Reddet
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {blockedUsers.length > 0 && (
            <div>
              <p style={{ fontSize: 10, fontWeight: 700, color: c.textFaint, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
                Engellenenler ({blockedUsers.length})
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {blockedUsers.map(item => (
                  <div key={item.userId} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", background: c.panel, border: `1px solid ${c.border}`, borderRadius: 12, padding: "9px 12px" }}>
                    <span style={{ fontSize: 13, fontWeight: 700, color: c.text }}>{item.username}</span>
                    <button onClick={() => unblockUser(item.userId)} style={{ background: c.panelAlt, border: `1px solid ${c.border}`, color: c.textSub, borderRadius: 8, padding: "5px 10px", fontSize: 11, fontWeight: 700, cursor: "pointer" }}>
                      Engeli Kaldır
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Arkadaş Listesi */}
          <div>
            <p style={{ fontSize: 10, fontWeight: 700, color: c.textFaint, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
              Arkadaşlarım ({friends.length})
            </p>
            {friends.length === 0 ? (
              <div style={{ background: c.panel, border: `1px solid ${c.border}`, borderRadius: 14, padding: 20, textAlign: "center" }}>
                <p style={{ fontSize: 12, color: c.textSub, margin: 0 }}>Henüz arkadaşın yok. Yukarıdan kullanıcı adıyla arkadaş ekleyebilirsin!</p>
              </div>
            ) : (
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {friends.map(f => (
                  <div key={f.friendshipId} style={{ background: c.panel, border: `1px solid ${f.isOnline ? c.accentRing : c.border}`, borderRadius: 14, padding: "12px 14px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <div style={{ position: "relative" }}>
                        <Av seed={f.username.substring(0, 2).toUpperCase()} size={40} dark={dark} avatar={f.avatar} country={f.country} active={!!f.isOnline} />
                        <span style={{
                          position: "absolute", bottom: -1, right: -1, width: 11, height: 11,
                          borderRadius: "50%", border: `2px solid ${c.panel}`,
                          background: f.inGame ? "#f39c12" : (f.isOnline ? "#2ecc71" : "#7f8c8d")
                        }} />
                      </div>
                      <div>
                        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                          <p style={{ fontSize: 14, fontWeight: 700, color: c.text, margin: 0 }}>{f.username}</p>
                          <span style={{ fontSize: 10, fontWeight: 700, color: f.inGame ? "#f39c12" : (f.isOnline ? "#2ecc71" : c.textFaint) }}>
                            {f.inGame ? "🎮 Oyunda" : (f.isOnline ? "🟢 Çevrimiçi" : "⚪ Çevrimdışı")}
                          </span>
                        </div>
                        <p style={{ fontSize: 11, color: c.textSub, margin: "2px 0 0" }}>Seviye {computeLevel(f.xp)} • 🏆 {f.wins} Galibiyet</p>
                      </div>
                    </div>
                    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                      <motion.button
                        whileTap={{ scale: 0.95 }}
                        onClick={() => onInviteToGame(f.userId)}
                        style={{ background: c.accent, color: dark ? c.bg : "#fff", border: "none", borderRadius: 10, padding: "8px 12px", fontSize: 12, fontWeight: 700, cursor: "pointer" }}
                      >
                        🎲 Davet Et
                      </motion.button>
                      <button
                        onClick={() => respondRequest(f.friendshipId, "remove")}
                        title="Arkadaşlıktan Çıkar"
                        style={{ background: "transparent", border: "none", color: c.textFaint, cursor: "pointer", padding: 4 }}
                      >
                        <X size={16} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Gönderilen Bekleyen İstekler */}
          {outgoing.length > 0 && (
            <div>
              <p style={{ fontSize: 10, fontWeight: 700, color: c.textFaint, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>
                Gönderilen İstekler ({outgoing.length})
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {outgoing.map(req => (
                  <div key={req.friendshipId} style={{ background: c.panelAlt, border: `1px solid ${c.border}`, borderRadius: 12, padding: "10px 14px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                    <span style={{ fontSize: 13, fontWeight: 600, color: c.textSub }}>{req.username} (Onay Bekleniyor)</span>
                    <button onClick={() => respondRequest(req.friendshipId, "remove")} style={{ background: "none", border: "none", color: c.danger, fontSize: 11, fontWeight: 700, cursor: "pointer" }}>
                      İptal
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default function App() {
  const [token, setToken] = useState<string | null>(localStorage.getItem("yatzy_jwt_token"));
  const closedByAppRef = useRef(new WeakSet<WebSocket>());
  const resumeInFlightRef = useRef(false);
  const reconnectRef = useRef<(attempt?: number) => void>(() => {});
  const [reconnecting, setReconnecting] = useState(false);
  const [resumeState, setResumeState] = useState<ResumeState | null>(null);
  const [profileRefresh, setProfileRefresh] = useState(0);
  const contentRef = useRef<HTMLDivElement>(null);
  const savedUsername = localStorage.getItem("yatzy_username") || "Oyuncu 1";
  const [screen, setScreen] = useState<Screen>(localStorage.getItem("yatzy_jwt_token") ? "home" : "auth");
  const hasBottomNav = screen !== "auth" && screen !== "result" && screen !== "game";
  const screenRef = useRef(screen);
  useEffect(() => { screenRef.current = screen; }, [screen]);

  const [dark, setDark] = useState(true);
  const [dieMat, setDieMat] = useState<DieMat>("ivory");
  const [profileLoaded, setProfileLoaded] = useState(false);
  const [matchSummary, setMatchSummary] = useState<MatchSummary | null>(null);

  const [profile, setProfile] = useState<UserProfile>({
    name: savedUsername,
    seed: savedUsername.substring(0, 2).toUpperCase(),
    level: 1,
    xp: 120,
    tokens: 210,
    cases: 2,
    unlockedDice: ["ivory"],
    loginStreak: 0,
    lastDailyReward: new Date().toISOString().split('T')[0], // Yüklenmeden önce pop-up tetiklenmesini önler
    stats: { gamesPlayed: 0, wins: 0, highScore: 0 }
  });

  const [ws, setWs] = useState<WebSocket | null>(null);
  const [isSearching, setIsSearching] = useState(false);
  const [myPlayerIndex, setMyPlayerIndex] = useState<number>(1);
  const [createdRoomCode, setCreatedRoomCode] = useState<string | null>(null);
  const [opponentName, setOpponentName] = useState<string>("Rakip");
  const [betAmount, setBetAmount] = useState<BetOption>(50);
  const [activeBet, setActiveBet] = useState<number>(0);

  // Arkadaş ve Oyun Daveti State'leri
  const [friends, setFriends] = useState<FriendItem[]>([]);
  const [incomingRequests, setIncomingRequests] = useState<FriendItem[]>([]);
  const [outgoingRequests, setOutgoingRequests] = useState<FriendItem[]>([]);
  const [gameInvites, setGameInvites] = useState<GameInvite[]>([]);
  const [onlineToast, setOnlineToast] = useState<FriendItem | null>(null);
  const [appToast, setAppToast] = useState<string | null>(null);
  const prevOnlineIdsRef = React.useRef<Set<number>>(new Set());
  const friendsInitRef = React.useRef<boolean>(false);
  const c = C(dark);

  useEffect(() => {
    toastListener = (msg: string) => {
      setAppToast(msg);
      setTimeout(() => setAppToast(prev => (prev === msg ? null : prev)), 3600);
    };
    return () => {
      toastListener = null;
    };
  }, []);

  const fetchFriends = React.useCallback(async () => {
    if (!token || token.startsWith("guest_token_")) return;
    try {
      const res = await fetch(`${API_BASE}/friends`, {
        headers: { "Authorization": "Bearer " + token }
      });
      if (res.ok) {
        const data = await res.json();
        const nextFriends: FriendItem[] = data.friends || [];
        if (friendsInitRef.current) {
          for (const f of nextFriends) {
            if (f.isOnline && !prevOnlineIdsRef.current.has(f.userId)) {
              setOnlineToast(f);
              setTimeout(() => setOnlineToast(prev => prev?.userId === f.userId ? null : prev), 4500);
              break;
            }
          }
        }
        prevOnlineIdsRef.current = new Set(nextFriends.filter(f => f.isOnline).map(f => f.userId));
        friendsInitRef.current = true;
        setFriends(nextFriends);
        setIncomingRequests(data.incoming || []);
        setOutgoingRequests(data.outgoing || []);
        setGameInvites(data.invites || []);
      }
    } catch {
      // Sessizce geç
    }
  }, [token]);

  useEffect(() => {
    if (!token || token.startsWith("guest_token_")) {
      setFriends([]);
      setIncomingRequests([]);
      setOutgoingRequests([]);
      setGameInvites([]);
      return;
    }
    fetchFriends();
    const interval = setInterval(fetchFriends, 5000);
    return () => clearInterval(interval);
  }, [token, fetchFriends]);

  const sendGameInvite = async (friendUserId: number, roomCode: string) => {
    if (!token || token.startsWith("guest_token_")) return;
    try {
      await fetch(`${API_BASE}/friends/invite`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + token
        },
        body: JSON.stringify({ friendUserId, roomCode })
      });
    } catch (e) {
      console.error(e);
    }
  };

  const dismissGameInvite = async (inviteId: number) => {
    setGameInvites(prev => prev.filter(inv => inv.id !== inviteId));
    if (!token || token.startsWith("guest_token_")) return;
    try {
      await fetch(`${API_BASE}/friends/invite`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Authorization": "Bearer " + token
        },
        body: JSON.stringify({ dismissId: inviteId })
      });
    } catch (e) {
      console.error(e);
    }
  };

  const handleAuthSuccess = (username: string, jwtToken: string) => {
    localStorage.setItem("yatzy_jwt_token", jwtToken); 
    localStorage.setItem("yatzy_username", username);
    setProfileLoaded(false);
    setToken(jwtToken);
    setProfile(prev => ({
      ...prev,
      name: username,
      seed: username.substring(0, 2).toUpperCase()
    }));
    setScreen("home"); 
  };

  const clearSession = useCallback(() => {
    localStorage.removeItem("yatzy_jwt_token");
    localStorage.removeItem("yatzy_username");
    setToken(null);
    setProfileLoaded(false);
    setScreen("auth");
  }, []);

  const closeSocket = useCallback((socket: WebSocket | null) => {
    if (!socket) return;
    closedByAppRef.current.add(socket);
    socket.close();
  }, []);

  const handleLogout = () => {
    closeSocket(ws);
    clearSession();
  };

  // OYUNA GİRİŞ YAPILDIĞINDA: Veritabanından güncel profil ve zar koleksiyonunu çek
  useEffect(() => {
    if (!token) return;
    // Eski sürümlerin yerel misafir kimliği sunucuda geçersizdir
    if (token.startsWith("guest_token_")) {
      clearSession();
      return;
    }
    fetch(`${API_BASE}/profile`, {
      headers: { "Authorization": "Bearer " + token }
    })
    .then(res => {
      if (res.status === 401 || res.status === 404) {
        emitToast("Oturumunun süresi doldu. Lütfen tekrar giriş yap.");
        clearSession();
        return null;
      }
      if (!res.ok) throw new Error("Profil çekilemedi");
      return res.json();
    })
    .then(data => {
      if (!data) return;
      const uname = data.username || localStorage.getItem("yatzy_username") || "Oyuncu";
      localStorage.setItem("yatzy_username", uname);
      setProfile({
        name: uname,
        seed: uname.substring(0, 2).toUpperCase(),
        avatar: data.avatar || "",
        country: data.country || "TR",
        xp: data.xp,
        level: computeLevel(data.xp),
        tokens: data.tokens,
        cases: data.cases,
        unlockedDice: (data.unlockedDice && data.unlockedDice.length > 0 ? data.unlockedDice : ["ivory"]) as DieMat[],
        loginStreak: data.loginStreak || 0,
        lastDailyReward: data.lastDailyReward || "",
        upperBonusesWon: data.upperBonusesWon || 0,
        totalYachts: data.totalYachts || 0,
        maxYachtsInMatch: data.maxYachtsInMatch || 0,
        claimedQuests: data.claimedQuests || [],
        stats: {
          wins: data.wins,
          gamesPlayed: data.gamesPlayed,
          highScore: data.highScore
        }
      });
      if (data.selectedDice) {
        setDieMat(data.selectedDice as DieMat);
      }
      setProfileLoaded(true);
      if (data.activeMatch) reconnectRef.current();
    })
    .catch(err => {
      console.error("Veritabanı okuma hatası:", err);
      setProfileLoaded(true);
    });
  }, [token, clearSession, profileRefresh]);

  // Bağlantı reddedildiyse oturumun geçerliliğini kontrol eder (WebSocket HTTP durumunu göstermez)
  const handleSocketRejected = async () => {
    try {
      const res = await fetch(`${API_BASE}/profile`, {
        headers: { "Authorization": "Bearer " + token }
      });
      if (res.status === 401 || res.status === 404) {
        emitToast("Oturumunun süresi doldu. Lütfen tekrar giriş yap.");
        handleLogout();
        return;
      }
    } catch {
      // Aşağıdaki genel bağlantı uyarısı gösterilir
    }
    emitToast("Sunucuya bağlanılamadı. Lütfen tekrar dene.");
  };

  const handleMatchResumed = useCallback((data: ResumedMatch) => {
    resumeInFlightRef.current = false;
    setMyPlayerIndex(data.playerIndex);
    setOpponentName(data.opponentName || "Rakip");
    setActiveBet(data.betAmount || 0);
    setResumeState(data.state);
    setReconnecting(false);
    setIsSearching(false);
    setCreatedRoomCode(null);
    setScreen("game");
  }, []);

  const handleResumeFailed = useCallback(() => {
    resumeInFlightRef.current = false;
    setReconnecting(false);
    setResumeState(null);
    if (screenRef.current === "game") {
      emitToast("Maça geri dönülemedi, maç sona erdi.");
      setScreen("home");
    }
    setProfileRefresh(value => value + 1);
  }, []);

  const handleSessionReplaced = useCallback(() => {
    closeSocket(ws);
    setReconnecting(false);
    emitToast("Bu maç başka bir cihazda açıldı.");
    setScreen("home");
  }, [closeSocket, ws]);

  const closeCurrentSocket = useCallback(() => closeSocket(ws), [closeSocket, ws]);

  const reconnectToMatch = (attempt = 0) => {
    const savedToken = localStorage.getItem("yatzy_jwt_token");
    if (!savedToken || (attempt === 0 && resumeInFlightRef.current)) return;
    if (attempt >= 20) {
      handleResumeFailed();
      return;
    }
    resumeInFlightRef.current = true;
    setReconnecting(screenRef.current === "game");

    const socket = new WebSocket(`${WS_BASE}/play?token=${encodeURIComponent(savedToken)}`);
    let opened = false;
    socket.onopen = () => {
      opened = true;
      socket.send(JSON.stringify({ action: "RESUME" }));
    };
    socket.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.type === "MATCH_FOUND" && data.resumed) handleMatchResumed(data);
      else if (data.type === "RESUME_FAILED") handleResumeFailed();
    };
    socket.onclose = () => {
      if (closedByAppRef.current.has(socket) || screenRef.current !== "game") {
        resumeInFlightRef.current = false;
        return;
      }
      setTimeout(() => reconnectRef.current(opened ? 1 : attempt + 1), 3000);
    };
    setWs(socket);
  };

  useEffect(() => { reconnectRef.current = reconnectToMatch; });

  const connectToServer = (action: "QUICK_MATCH" | "BOT_MATCH" | "CREATE_ROOM" | "JOIN_ROOM", roomCode?: string, autoInviteFriendId?: number, betOverride?: BetOption) => {
    if (isSearching || !token) return;
    const requestedBet = betOverride ?? betAmount;
    const effectiveBet: BetOption = profile.tokens >= requestedBet ? requestedBet : 0;
    closeSocket(ws);
    setIsSearching(true);
    
    const socket = new WebSocket(`${WS_BASE}/play?token=${encodeURIComponent(token)}`);
    let opened = false;
    
    socket.onopen = () => {
      opened = true;
      socket.send(JSON.stringify({ action: action, roomCode: roomCode, betAmount: effectiveBet }));
    };

    socket.onmessage = (event) => {
      const data = JSON.parse(event.data);
      
      if (data.type === "ROOM_CREATED") {
        setCreatedRoomCode(data.roomCode);
        setIsSearching(false);
        if (autoInviteFriendId) {
          sendGameInvite(autoInviteFriendId, data.roomCode);
        }
      }
      else if (data.type === "MATCH_FOUND" && data.resumed) {
        handleMatchResumed(data);
      }
      else if (data.type === "MATCH_FOUND") {
        sfx.pop();
        setResumeState(null);
        const matchBet = typeof data.betAmount === "number" ? data.betAmount : effectiveBet;
        setActiveBet(matchBet);
        if (matchBet > 0) {
          setProfile(prev => ({
            ...prev,
            tokens: Math.max(0, prev.tokens - matchBet)
          }));
        }
        setMyPlayerIndex(data.playerIndex);
        setOpponentName(data.opponentName || "Rakip");
        setIsSearching(false);
        setCreatedRoomCode(null);
        setScreen("game");
      }
      else if (data.type === "ERROR") {
        emitToast(data.message || "Bağlantı hatası");
        setIsSearching(false);
        setCreatedRoomCode(null);
        socket.close();
      }
    };

    socket.onclose = () => {
      setIsSearching(false);
      setCreatedRoomCode(null);
      if (closedByAppRef.current.has(socket)) return;
      if (screenRef.current === "game") {
        reconnectRef.current();
      } else if (!opened) {
        handleSocketRejected();
      }
    };

    setWs(socket);
  };

  const cancelMatchmaking = () => {
    closeSocket(ws);
    setIsSearching(false);
    setCreatedRoomCode(null);
    setWs(null);
  };

  const badgeCount = incomingRequests.length + gameInvites.length;
  const claimableQuests = getClaimableQuestCount(profile);

  return (
    <div style={{ minHeight:"100dvh", background:dark?"#111318":"#dedad3", display:"flex", alignItems:"center", justifyContent:"center" }}>
      <div style={{
        width:"min(420px,100vw)", height:"min(860px,100dvh)", boxSizing:"border-box",
        paddingTop:"env(safe-area-inset-top)",
        background:c.bg, borderRadius:"min(44px,0px)",
        overflow:"hidden", position:"relative", display:"flex", flexDirection:"column",
        boxShadow:"0 24px 64px rgba(0,0,0,0.35), 0 0 0 1px rgba(0,0,0,0.12)",
      }}>

        {/* Uygulama İçi Modern Bildirim (Toast) */}
        <AnimatePresence>
          {appToast && (
            <motion.div
              initial={{ opacity: 0, y: -30 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -30 }}
              style={{
                position: "absolute", top: 14, left: 16, right: 16,
                background: c.panel, color: c.text,
                border: `1.5px solid ${c.danger}`, borderRadius: 14,
                padding: "11px 14px", fontSize: 12, fontWeight: 700,
                zIndex: 250, boxShadow: "0 12px 30px rgba(0,0,0,0.45)",
                display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10
              }}
            >
              <span>⚠️ {appToast}</span>
              <button onClick={() => setAppToast(null)} style={{ background: "none", border: "none", color: c.textSub, cursor: "pointer", display: "flex" }}>
                <X size={15} />
              </button>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Arkadaş Çevrimiçi Oldu Bildirimi */}
        <AnimatePresence>
          {screen !== "auth" && screen !== "game" && onlineToast && gameInvites.length === 0 && (
            <motion.div
              initial={{ opacity: 0, y: -35 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -35 }}
              style={{
                background: c.panel,
                borderBottom: `2px solid #2ecc71`,
                padding: "10px 16px",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 10,
                zIndex: 79,
                boxShadow: "0 8px 24px rgba(0,0,0,0.35)"
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <Av seed={onlineToast.username} size={32} dark={dark} avatar={onlineToast.avatar} country={onlineToast.country} active={true} />
                <div>
                  <p style={{ fontSize: 12, fontWeight: 800, color: c.text, margin: 0 }}>
                    🟢 {onlineToast.username} çevrimiçi oldu!
                  </p>
                  <p style={{ fontSize: 10, color: c.textSub, margin: 0 }}>Hemen maça davet edebilirsin</p>
                </div>
              </div>
              <button
                onClick={() => {
                  const targetId = onlineToast.userId;
                  setOnlineToast(null);
                  setScreen("home");
                  if (createdRoomCode) {
                    sendGameInvite(targetId, createdRoomCode);
                  } else {
                    connectToServer("CREATE_ROOM", undefined, targetId);
                  }
                }}
                style={{
                  background: c.accent, color: dark ? c.bg : "#fff",
                  border: "none", borderRadius: 10, padding: "7px 12px",
                  fontSize: 11, fontWeight: 800, cursor: "pointer"
                }}
              >
                🎲 Davet Et
              </button>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Gelen Oyun Daveti Bildirim Banner'ı */}
        <AnimatePresence>
          {screen !== "auth" && screen !== "game" && gameInvites.length > 0 && (
            <motion.div
              initial={{ opacity: 0, y: -40 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -40 }}
              style={{
                background: c.panel,
                borderBottom: `2px solid ${c.accent}`,
                padding: "12px 16px",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 10,
                zIndex: 80,
                boxShadow: "0 8px 24px rgba(0,0,0,0.35)"
              }}
            >
              <div>
                <p style={{ fontSize: 12, fontWeight: 800, color: c.text, margin: 0 }}>
                  🎲 {gameInvites[0].fromName} seni maça davet ediyor!
                </p>
                <p style={{ fontSize: 11, color: c.accent, fontWeight: 600, margin: "2px 0 0" }}>
                  Oda Kodu: {gameInvites[0].roomCode}
                </p>
              </div>
              <div style={{ display: "flex", gap: 6 }}>
                <button
                  onClick={() => {
                    const inv = gameInvites[0];
                    dismissGameInvite(inv.id);
                    connectToServer("JOIN_ROOM", inv.roomCode);
                  }}
                  style={{
                    background: c.accent, color: dark ? c.bg : "#fff",
                    border: "none", borderRadius: 10, padding: "8px 12px",
                    fontSize: 12, fontWeight: 800, cursor: "pointer"
                  }}
                >
                  Katıl
                </button>
                <button
                  onClick={() => dismissGameInvite(gameInvites[0].id)}
                  style={{
                    background: c.panelAlt, color: c.textSub,
                    border: `1px solid ${c.border}`, borderRadius: 10, padding: "8px 10px",
                    fontSize: 12, fontWeight: 600, cursor: "pointer"
                  }}
                >
                  Reddet
                </button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
        
        {/* İçerik Ekranları */}
        <div ref={contentRef} style={{ flex:1, overflowY:"auto", position:"relative", paddingBottom: hasBottomNav ? 0 : "env(safe-area-inset-bottom)" }}>
          <AnimatePresence mode="wait" onExitComplete={() => contentRef.current?.scrollTo(0, 0)}>
            <motion.div key={screen} initial={{ opacity:0, y:8 }} animate={{ opacity:1, y:0 }} exit={{ opacity:0, y:-8 }} transition={{ duration:0.16 }} style={{ height:"100%" }}>
              {screen==="auth" && <AuthScreen onLogin={handleAuthSuccess} dark={dark} />}
              {screen==="home" && (
                <HomeScreen
                  go={setScreen} dark={dark} toggle={()=>setDark(d=>!d)}
                  profile={profile} setProfile={setProfile}
                  isSearching={isSearching} connectToServer={connectToServer}
                  createdRoomCode={createdRoomCode} cancelMatchmaking={cancelMatchmaking}
                  onLogout={handleLogout} token={token} profileLoaded={profileLoaded}
                  friends={friends} onSendInvite={sendGameInvite}
                  betAmount={betAmount} setBetAmount={setBetAmount}
                />
              )}
              {screen==="modes" && (
                <ModeScreen
                  dark={dark}
                  betAmount={betAmount}
                  setBetAmount={setBetAmount}
                  tokens={profile.tokens}
                  onSelectMode={(mode, bet) => {
                    setScreen("home");
                    connectToServer(mode, undefined, undefined, bet);
                  }}
                />
              )}
              {screen==="quests" && (
                <QuestsScreen
                  dark={dark}
                  profile={profile}
                  setProfile={setProfile}
                  token={token}
                />
              )}
              {screen==="friends" && (
                <FriendsScreen
                  dark={dark} token={token}
                  friends={friends} incoming={incomingRequests} outgoing={outgoingRequests}
                  onRefresh={fetchFriends}
                  onInviteToGame={(friendUserId) => {
                    setScreen("home");
                    if (createdRoomCode) {
                      sendGameInvite(friendUserId, createdRoomCode);
                    } else {
                      connectToServer("CREATE_ROOM", undefined, friendUserId);
                    }
                  }}
                />
              )}
              {screen==="shop" && (
                <ShopScreen
                  dark={dark} sel={dieMat} setSel={setDieMat}
                  profile={profile} setProfile={setProfile} token={token}
                />
              )}
              {screen==="game" && (
                <GameScreen
                  go={setScreen} dark={dark} diceMat={dieMat}
                  profile={profile} setProfile={setProfile}
                  ws={ws} myPlayerIndex={myPlayerIndex}
                  opponentName={opponentName} onMatchEnd={setMatchSummary}
                  activeBet={activeBet}
                  reconnecting={reconnecting} resumeState={resumeState}
                  onCloseSocket={closeCurrentSocket} onMatchResumed={handleMatchResumed}
                  onResumeFailed={handleResumeFailed} onSessionReplaced={handleSessionReplaced}
                />
              )}
              {screen==="result" && (
                <ResultScreen
                  go={setScreen} dark={dark} profile={profile}
                  summary={matchSummary}
                  onPlayAgain={() => {
                    setScreen("home");
                    connectToServer("QUICK_MATCH");
                  }}
                />
              )}
            </motion.div>
          </AnimatePresence>
        </div>

        {/* Alt Navigasyon */}
        {hasBottomNav && (
          <div style={{ background:c.panel, borderTop:`1px solid ${c.border}`, padding:"8px 0 calc(16px + env(safe-area-inset-bottom))", display:"flex", justifyContent:"space-evenly", alignItems:"center", zIndex:50 }}>
            {TABS.map(({ id, Icon, label })=>{
              const isCenter = id === "modes";
              const active = screen===id;
              
              return (  
                <button key={id} onClick={()=>setScreen(id as Screen)} style={{ background:"none", border:"none", cursor:"pointer", display:"flex", flexDirection:"column", alignItems:"center", gap:4, width:"80px", position:"relative" }}>
                  <div style={{ 
                    width: isCenter ? 54 : 32, 
                    height: isCenter ? 44 : 32, 
                    borderRadius: isCenter ? 14 : 9, 
                    background: isCenter ? (active ? c.accent : c.panelAlt) : (active ? c.accentBg : "transparent"), 
                    display:"flex", alignItems:"center", justifyContent:"center", transition:"0.2s", position:"relative"
                  }}>
                    <Icon size={isCenter ? 22 : 18} color={isCenter ? (active ? (dark?c.bg:"#fff") : c.accent) : (active ? c.accent : c.textSub)} strokeWidth={active || isCenter ? 2.5 : 1.8}/>
                    {id === "friends" && badgeCount > 0 && (
                      <span style={{
                        position: "absolute", top: -4, right: -6,
                        background: c.danger, color: "#fff", fontSize: 9, fontWeight: 800,
                        width: 16, height: 16, borderRadius: "50%",
                        display: "flex", alignItems: "center", justifyContent: "center"
                      }}>
                        {badgeCount}
                      </span>
                    )}
                    {id === "quests" && claimableQuests > 0 && (
                      <span style={{
                        position: "absolute", top: -4, right: -6,
                        background: c.gold, color: "#1a1405", fontSize: 9, fontWeight: 900,
                        width: 16, height: 16, borderRadius: "50%",
                        display: "flex", alignItems: "center", justifyContent: "center"
                      }}>
                        {claimableQuests}
                      </span>
                    )}
                  </div>
                  <span style={{ fontSize:9, fontWeight:active?700:500, color:active?c.accent:c.textSub, letterSpacing:"0.04em", textTransform:"uppercase" }}>{label}</span>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

