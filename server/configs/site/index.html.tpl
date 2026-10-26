<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{SITE_NAME}}</title>
<link rel="icon" href="/favicon.ico">
<style>
:root {
  color-scheme: dark;
  --bg: #0b0d12;
  --panel: rgba(18, 21, 29, 0.86);
  --line: #2a303d;
  --fg: #eceef3;
  --muted: #8e97a9;
  --danger: #f08a7e;
  --accent: {{ACCENT}};
  --accent-fg: {{ACCENT_FG}};
  --sans: system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif;
}
* { box-sizing: border-box; }
[hidden] { display: none !important; }
body { margin: 0; background: var(--bg); color: var(--fg); font-family: var(--sans); font-size: 14px; line-height: 1.6; }

.hero { position: relative; overflow: hidden; }
.backdrop { position: absolute; inset: -40px; display: grid; grid-template-columns: repeat(7, 1fr); gap: 8px; filter: blur(14px) brightness(0.5) saturate(1.2); transform: rotate(-4deg) scale(1.1); }
.backdrop img { width: 100%; aspect-ratio: 2 / 3; object-fit: cover; }
.hero::after { content: ""; position: absolute; inset: 0; background: linear-gradient(to bottom, rgba(11, 13, 18, 0.1), var(--bg)); }
.hero-inner {
  position: relative; z-index: 1; max-width: 1080px; margin: 0 auto;
  padding-block: 72px 56px;
  padding-left: max(16px, env(safe-area-inset-left, 0px)); padding-right: max(16px, env(safe-area-inset-right, 0px));
  display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 48px; align-items: center;
}
@media (max-width: 820px) { .hero-inner { grid-template-columns: minmax(0, 1fr); padding-block: 40px; } }
.brand { display: flex; align-items: center; gap: 12px; }
.logo { width: 44px; height: 44px; border-radius: 12px; background: linear-gradient(135deg, var(--accent), #8e6bd8); display: grid; place-items: center; }
.logo::before { content: ""; border-left: 14px solid #fff; border-top: 9px solid transparent; border-bottom: 9px solid transparent; margin-left: 4px; }
.brand h1 { font-size: 26px; margin: 0; letter-spacing: 1px; }
.lead { color: #c3c9d6; font-size: 16px; margin: 18px 0 8px; max-width: 34ch; text-wrap: balance; }
.stats { color: var(--muted); font-size: 13px; }

.login { background: var(--panel); border: 1px solid var(--line); border-radius: 12px; padding: 28px 26px; backdrop-filter: blur(8px); }
.login h2 { font-size: 18px; margin: 0 0 18px; }
.field { display: grid; gap: 6px; margin-bottom: 14px; }
.field label { font-size: 12.5px; color: var(--muted); }
.field input[type="text"], .field input[type="password"] { background: #0e1118; border: 1px solid var(--line); border-radius: 6px; padding: 9px 12px; color: var(--fg); font: inherit; }
.field input:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; border-color: transparent; }
.row { display: flex; justify-content: space-between; align-items: center; font-size: 12.5px; color: var(--muted); margin-bottom: 16px; gap: 8px; flex-wrap: wrap; }
.row label { display: flex; gap: 6px; align-items: center; }
.submit { width: 100%; border: 0; border-radius: 6px; padding: 10px; font: inherit; font-weight: 700; background: var(--accent); color: var(--accent-fg); cursor: pointer; }
.submit:disabled { opacity: 0.6; cursor: progress; }
.submit:focus-visible { outline: 2px solid #fff; outline-offset: 2px; }
.err { color: var(--danger); font-size: 13px; margin: -4px 0 12px; }
.hint { color: var(--muted); font-size: 12px; margin: 14px 0 0; }

.wrap { max-width: 1080px; margin: 0 auto; padding-block: 8px 48px; padding-left: max(16px, env(safe-area-inset-left, 0px)); padding-right: max(16px, env(safe-area-inset-right, 0px)); }
.sec-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-bottom: 14px; }
.sec-head h2 { font-size: 17px; margin: 0; }
.sec-head span { color: var(--muted); font-size: 12.5px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 18px 14px; }
.card { min-width: 0; cursor: pointer; border: 0; padding: 0; background: none; color: inherit; font: inherit; text-align: left; }
.card:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: 6px; }
.card img { width: 100%; max-width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 6px; display: block; box-shadow: 0 6px 18px rgba(0, 0, 0, 0.5); background: #161a22; }
.card .name { margin-top: 8px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card .sub { color: var(--muted); font-size: 12px; font-variant-numeric: tabular-nums; }
@media (prefers-reduced-motion: no-preference) { .card img { transition: translate 0.2s; } .card:hover img { translate: 0 -3px; } }

.sheet { position: fixed; inset: 0; z-index: 20; background: rgba(5, 6, 10, 0.8); display: grid; place-items: center; padding: 16px; }
.sheet-box { width: min(880px, 100%); max-height: 100%; overflow: auto; background: #12151d; border: 1px solid var(--line); border-radius: 12px; }
.player { position: relative; aspect-ratio: 16 / 9; background: #000; overflow: hidden; border-radius: 12px 12px 0 0; }
.player img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; filter: blur(3px) brightness(0.5); transform: scale(1.05); }
.play { position: absolute; left: 50%; top: 50%; translate: -50% -50%; width: 64px; height: 64px; border-radius: 50%; border: 0; background: rgba(255, 255, 255, 0.16); cursor: pointer; display: grid; place-items: center; }
.play::before { content: ""; border-left: 19px solid #fff; border-top: 11px solid transparent; border-bottom: 11px solid transparent; margin-left: 5px; }
.bar { position: absolute; left: 0; right: 0; bottom: 0; padding: 10px 14px; display: flex; gap: 12px; align-items: center; font-size: 12px; color: #cfd3dc; background: linear-gradient(transparent, rgba(0, 0, 0, 0.75)); font-variant-numeric: tabular-nums; }
.track { flex: 1; height: 3px; background: rgba(255, 255, 255, 0.25); border-radius: 2px; }
.gate { position: absolute; inset: 0; display: grid; place-items: center; text-align: center; background: rgba(5, 6, 10, 0.75); padding: 16px; }
.gate p { margin: 6px 0 14px; color: var(--muted); }
.info { padding: 18px 22px 22px; display: grid; gap: 6px; }
.info h3 { margin: 0; font-size: 20px; }
.info .sub { color: var(--muted); font-size: 12.5px; }
.info p { margin: 4px 0 0; color: #c6cbd6; max-width: 70ch; }
.info .lic { font-size: 12px; color: var(--muted); }
.btn { border: 1px solid var(--line); background: transparent; color: var(--fg); border-radius: 6px; padding: 7px 16px; font: inherit; cursor: pointer; }
.btn.pri { background: var(--accent); border-color: var(--accent); color: var(--accent-fg); font-weight: 700; }
.btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.close { justify-self: end; margin-top: 6px; }
footer { color: var(--muted); font-size: 12px; border-top: 1px solid var(--line); padding-top: 16px; margin-top: 36px; display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
footer a { color: inherit; }
</style>
</head>
<body>
<div class="hero">
  <div class="backdrop" id="backdrop" aria-hidden="true"></div>
  <div class="hero-inner">
    <div>
      <div class="brand"><div class="logo"></div><h1>{{SITE_NAME}}</h1></div>
      <p class="lead">个人影音收藏。电影、剧集与纪录片，随时随地续播。</p>
      <div class="stats">仅限受邀成员</div>
    </div>
    <form class="login" id="login" novalidate>
      <h2>登录</h2>
      <div class="field"><label for="u">用户名</label><input id="u" name="u" type="text" autocomplete="username"></div>
      <div class="field"><label for="p">密码</label><input id="p" name="p" type="password" autocomplete="current-password"></div>
      <div class="err" id="err" role="alert" hidden></div>
      <div class="row"><label><input type="checkbox" id="remember"> 记住我</label><span>忘记密码请联系管理员</span></div>
      <button class="submit" id="submit" type="submit">登录</button>
      <p class="hint">本服务器不开放注册，新成员需管理员邀请。</p>
    </form>
  </div>
</div>

<div class="wrap">
  <div class="sec-head"><h2>公开放映</h2><span>以下为开放授权影片，无需登录即可浏览简介</span></div>
  <div class="grid" id="grid"></div>
  <footer>
    <span>公开放映的影片版权归各自权利人所有，依 <a href="https://creativecommons.org/licenses/by/4.0/" target="_blank" rel="noopener noreferrer">CC BY</a> 许可展示。</span>
    <span>{{SITE_NAME}} · {{YEAR}}</span>
  </footer>
</div>

<div class="sheet" id="sheet" hidden>
  <div class="sheet-box" role="dialog" aria-modal="true" aria-labelledby="dt">
    <div class="player">
      <img id="dimg" alt="">
      <button class="play" id="play" type="button" aria-label="播放"></button>
      <div class="bar"><span>00:00</span><div class="track"></div><span id="dlen"></span><span>1080P</span></div>
      <div class="gate" id="gate" hidden>
        <div><strong>请先登录</strong><p>登录后即可观看全片</p><button class="btn pri" type="button" id="toLogin">去登录</button></div>
      </div>
    </div>
    <div class="info">
      <h3 id="dt"></h3>
      <div class="sub" id="dsub"></div>
      <p id="ddesc"></p>
      <div class="lic" id="dlic"></div>
      <button class="btn close" type="button" id="close">关闭</button>
    </div>
  </div>
</div>

<script>
const FILMS = [
{{FILMS_JS}}
];
const $ = id => document.getElementById(id);
const posterUrl = f => '/posters/' + encodeURIComponent(f.slug) + '.jpg';

// 背景海报墙：重复使用片单里的海报
const backdrop = $('backdrop');
for (let i = 0; i < 14; i++) {
  const img = new Image();
  img.alt = '';
  img.src = posterUrl(FILMS[i % FILMS.length]);
  backdrop.appendChild(img);
}

// 公开放映网格
const grid = $('grid');
FILMS.forEach((f, i) => {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = 'card';
  b.dataset.i = i;
  const img = new Image();
  img.alt = f.en + ' 海报';
  img.src = posterUrl(f);
  const name = document.createElement('div');
  name.className = 'name';
  name.textContent = f.t;
  const sub = document.createElement('div');
  sub.className = 'sub';
  sub.textContent = f.y + ' · ' + f.g;
  b.append(img, name, sub);
  grid.appendChild(b);
});

// 详情弹层：播放器界面完整，点播放提示需要登录
const sheet = $('sheet'), gate = $('gate');
grid.addEventListener('click', e => {
  const c = e.target.closest('.card');
  if (!c) return;
  const f = FILMS[+c.dataset.i];
  $('dimg').src = posterUrl(f);
  $('dt').textContent = f.t + '（' + f.en + '）';
  $('dsub').textContent = f.y + ' · ' + f.g + ' · 片长 ' + f.len;
  $('dlen').textContent = f.len;
  $('ddesc').textContent = f.d;
  $('dlic').textContent = f.lic;
  gate.hidden = true;
  sheet.hidden = false;
  $('close').focus();
});
$('play').addEventListener('click', () => { gate.hidden = false; });
$('close').addEventListener('click', () => { sheet.hidden = true; });
$('toLogin').addEventListener('click', () => { sheet.hidden = true; $('u').focus(); });
sheet.addEventListener('click', e => { if (e.target === sheet) sheet.hidden = true; });
document.addEventListener('keydown', e => { if (e.key === 'Escape') sheet.hidden = true; });

// 登录：真实向 /api/auth/login 发请求，由 nginx 返回 401；界面稍作停顿，和真实服务的体验接近
$('login').addEventListener('submit', async e => {
  e.preventDefault();
  const btn = $('submit'), err = $('err');
  err.hidden = true;
  const user = $('u').value, pass = $('p').value;
  if (!user || !pass) {
    err.textContent = '请输入用户名和密码';
    err.hidden = false;
    return;
  }
  btn.disabled = true;
  btn.textContent = '正在登录…';
  const pause = new Promise(r => setTimeout(r, 700 + Math.random() * 500));
  try {
    await Promise.all([
      fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: user, password: pass, remember: $('remember').checked }),
      }),
      pause,
    ]);
  } catch (_) {
    await pause;
  }
  btn.disabled = false;
  btn.textContent = '登录';
  err.textContent = '用户名或密码错误';
  err.hidden = false;
});
</script>
</body>
</html>
