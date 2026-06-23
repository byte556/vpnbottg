// Генератор карточек-экранов VEXA VPN (тёмный миниме).
// Один шаблон → по html на экран. Меняется только заголовок + центральная иконка.
const fs = require('fs');
const path = require('path');

const OUT = path.join(__dirname, 'cards');
fs.mkdirSync(OUT, { recursive: true });

const G = '<defs><linearGradient id="g" x1="0" y1="0" x2="48" y2="48"><stop stop-color="#ff7a6b"/><stop offset="1" stop-color="#ff4757"/></linearGradient></defs>';
const BG = '#0a0b0e';

const shield     = G + '<path d="M24 3 6 10v12c0 11 7.6 18.6 18 23 10.4-4.4 18-12 18-23V10L24 3Z" fill="url(#g)"/>';
const shieldChk  = shield + '<path d="M16.5 24.5l5 5 10-11" stroke="'+BG+'" stroke-width="3.4" fill="none" stroke-linecap="round" stroke-linejoin="round"/>';
const rocket     = G +
  '<path d="M24 4c6 4 9 11 9 19l-4 6H19l-4-6c0-8 3-15 9-19Z" fill="url(#g)"/>' +
  '<circle cx="24" cy="18" r="3.2" fill="'+BG+'"/>' +
  '<path d="M15 29l-5 4 1 6 6-5z" fill="url(#g)"/><path d="M33 29l5 4-1 6-6-5z" fill="url(#g)"/>' +
  '<path d="M24 40c-2.2 0-4 2-4 4.5 2 .6 6 .6 8 0 0-2.5-1.8-4.5-4-4.5Z" fill="url(#g)"/>';
const gift       = G +
  '<rect x="9" y="20" width="30" height="20" rx="2" fill="url(#g)"/>' +
  '<rect x="7" y="13" width="34" height="8" rx="2" fill="url(#g)"/>' +
  '<rect x="22" y="13" width="4" height="27" fill="'+BG+'"/>' +
  '<path d="M24 13c-3-6-11-4-8 0 1.6 2.6 8 0 8 0Zm0 0c3-6 11-4 8 0-1.6 2.6-8 0-8 0Z" fill="url(#g)"/>';
const devices    = G +
  '<rect x="5" y="10" width="27" height="18" rx="2.5" fill="url(#g)"/>' +
  '<rect x="9" y="14" width="19" height="10" rx="1" fill="'+BG+'"/>' +
  '<rect x="14" y="29" width="9" height="2.5" fill="url(#g)"/><rect x="11" y="32" width="15" height="2.5" rx="1" fill="url(#g)"/>' +
  '<rect x="31" y="19" width="12" height="21" rx="3" fill="url(#g)"/>' +
  '<rect x="33.5" y="22" width="7" height="12" rx="1" fill="'+BG+'"/><circle cx="37" cy="37" r="1" fill="'+BG+'"/>';
const sliders    = G + '<g fill="url(#g)">' +
  '<rect x="8" y="12" width="32" height="3.4" rx="1.7"/>' +
  '<rect x="8" y="22.3" width="32" height="3.4" rx="1.7"/>' +
  '<rect x="8" y="32.6" width="32" height="3.4" rx="1.7"/>' +
  '<circle cx="18" cy="13.7" r="4.6"/><circle cx="31" cy="24" r="4.6"/><circle cx="22" cy="34.3" r="4.6"/></g>';
const ticket     = G +
  '<path d="M8 14h32a2 2 0 0 1 2 2v5.5a3 3 0 0 0 0 5.8V35a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2v-5.7a3 3 0 0 0 0-5.8V16a2 2 0 0 1 2-2Z" fill="url(#g)"/>' +
  '<g fill="'+BG+'"><rect x="27" y="16" width="2" height="3.2"/><rect x="27" y="21.5" width="2" height="3.2"/><rect x="27" y="27" width="2" height="3.2"/><rect x="27" y="32.5" width="2" height="3.2"/></g>';
const card       = G +
  '<rect x="6" y="12" width="36" height="24" rx="4" fill="url(#g)"/>' +
  '<rect x="6" y="18" width="36" height="5" fill="'+BG+'"/>' +
  '<rect x="11" y="28" width="10" height="4" rx="1" fill="'+BG+'"/>';
const success    = G +
  '<circle cx="24" cy="24" r="20" fill="url(#g)"/>' +
  '<path d="M15 24.5l6 6 12-13" stroke="'+BG+'" stroke-width="3.6" fill="none" stroke-linecap="round" stroke-linejoin="round"/>';
const help       = G +
  '<circle cx="24" cy="24" r="20" fill="url(#g)"/>' +
  '<path d="M19 19.5c0-3 2.4-5.2 5.4-5.2s5.4 2.2 5.4 5c0 3-3 3.6-4.4 5.1-.9 1-1 2-1 3.3" stroke="'+BG+'" stroke-width="3" fill="none" stroke-linecap="round"/>' +
  '<circle cx="24.3" cy="35" r="2.1" fill="'+BG+'"/>';
const invite     = G +
  '<circle cx="33" cy="19" r="5" fill="url(#g)" opacity=".75"/>' +
  '<path d="M28 39c0-6 4-10 9-10 3.6 0 6.6 2.4 8 6" fill="url(#g)" opacity=".75"/>' +
  '<circle cx="19" cy="17.5" r="6.2" fill="url(#g)"/>' +
  '<path d="M7 40c0-7 5.4-11.5 12-11.5S31 33 31 40Z" fill="url(#g)"/>';
const expired    =
  '<path d="M24 3 6 10v12c0 11 7.6 18.6 18 23 10.4-4.4 18-12 18-23V10L24 3Z" fill="#3a3d46"/>' +
  '<path d="M18.5 18.5l11 11M29.5 18.5l-11 11" stroke="'+BG+'" stroke-width="3.4" fill="none" stroke-linecap="round"/>';
const activating = G + '<path d="M27 3 9 27h11l-2 18 19-26H25l3-16Z" fill="url(#g)"/>';

const screens = [
  { name: 'menu_main',  title: 'Главное меню',      icon: shield },
  { name: 'buy',        title: 'Купить VPN',        icon: rocket },
  { name: 'trial',      title: 'Пробный период',    icon: gift },
  { name: 'subscriber', title: 'Твой VPN',          icon: shieldChk },
  { name: 'devices',    title: 'Мои устройства',    icon: devices },
  { name: 'settings',   title: 'Мой тариф',         icon: sliders },
  { name: 'promo',      title: 'Промокод',          icon: ticket },
  { name: 'payment',    title: 'Оплата',            icon: card },
  { name: 'activating', title: 'Активируем VPN',    icon: activating },
  { name: 'success',    title: 'VPN активирован',   icon: success },
  { name: 'help',       title: 'Помощь',            icon: help },
  { name: 'invite',     title: 'Пригласить друга',  icon: invite },
  { name: 'expired',    title: 'Подписка истекла',  icon: expired },
];

const tmpl = (title, icon) => `<!DOCTYPE html><html lang="ru"><head><meta charset="utf-8"><style>
*{box-sizing:border-box;margin:0;padding:0}html,body{width:1080px;height:1080px}
:root{--bg:#0a0b0e;--txt:#f5f6f8;--muted:#878d99;--accent:#ff4757}
body{width:1080px;height:1080px;overflow:hidden;position:relative;font-family:"Segoe UI","Arial",sans-serif;color:var(--txt);
  background:radial-gradient(700px 480px at 50% -6%, rgba(255,71,87,.16), transparent 62%),radial-gradient(760px 540px at 50% 108%, rgba(255,71,87,.06), transparent 60%),var(--bg)}
.wrap{position:absolute;inset:0;padding:128px 90px;display:flex;flex-direction:column;align-items:center;text-align:center}
h1{font-size:100px;line-height:.94;font-weight:800;letter-spacing:-.02em;max-width:900px}
.art{margin:auto 0;display:flex;align-items:center;justify-content:center}
.ring{width:420px;height:420px;border-radius:50%;display:flex;align-items:center;justify-content:center;position:relative;
  background:radial-gradient(circle at 50% 40%, rgba(255,71,87,.18), rgba(255,71,87,.02) 60%, transparent 70%)}
.ring::before{content:"";position:absolute;inset:36px;border-radius:50%;border:1px solid rgba(255,71,87,.18)}
.ring::after{content:"";position:absolute;inset:78px;border-radius:50%;border:1px solid rgba(255,71,87,.10)}
.art svg{width:228px;height:228px;filter:drop-shadow(0 26px 50px rgba(255,71,87,.45));position:relative}
.handle{font-size:30px;font-weight:700;color:var(--muted)}.handle .at{color:var(--accent)}
</style></head><body><div class="wrap">
<h1>${title}</h1>
<div class="art"><div class="ring"><svg viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">${icon}</svg></div></div>
<div class="handle"><span class="at">@</span>vexa_vpn_bot</div>
</div></body></html>`;

for (const s of screens) {
  fs.writeFileSync(path.join(OUT, s.name + '.html'), tmpl(s.title, s.icon));
}

// ── Логотип бренда: щит с «V» (VEXA) негативом. Для аватара бота и обложки ТГ ──
const MARK =
  '<defs><linearGradient id="vg" x1="0" y1="0" x2="48" y2="48"><stop stop-color="#ff7a6b"/><stop offset="1" stop-color="#ff4757"/></linearGradient></defs>' +
  '<path fill-rule="evenodd" clip-rule="evenodd" fill="url(#vg)" ' +
  'd="M24 3 6 10v12c0 11 7.6 18.6 18 23 10.4-4.4 18-12 18-23V10L24 3Z M14 14.5 20 14.5 24 29 28 14.5 34 14.5 26.5 35 21.5 35Z"/>';

const logoPage = (withText) => `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
*{margin:0;box-sizing:border-box}html,body{width:1024px;height:1024px}
body{width:1024px;height:1024px;overflow:hidden;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:56px;
  font-family:"Segoe UI",Arial,sans-serif;
  background:radial-gradient(560px 560px at 50% ${withText ? 44 : 50}%, rgba(255,71,87,.22), rgba(255,71,87,.03) 58%, transparent 72%),#0a0b0e}
.mark{width:${withText ? 430 : 640}px;height:${withText ? 430 : 640}px;filter:drop-shadow(0 34px 80px rgba(255,71,87,.5))}
.wm{font-size:96px;font-weight:800;letter-spacing:.02em;color:#f5f6f8}.wm .a{color:#ff4757}
</style></head><body>
<svg class="mark" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">${MARK}</svg>
${withText ? '<div class="wm">VEXA<span class="a"> VPN</span></div>' : ''}
</body></html>`;

fs.writeFileSync(path.join(__dirname, 'logo.html'), logoPage(true));
fs.writeFileSync(path.join(__dirname, 'avatar.html'), logoPage(false));

console.log(screens.map(s => s.name).join('\n'));
