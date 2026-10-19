(() => {
  "use strict";

  const REPO = "UCKETX/NEMCBridge";
  const FALLBACK_TAG = "v0.1.8";
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const $ = (s, root = document) => root.querySelector(s);
  const $$ = (s, root = document) => Array.from(root.querySelectorAll(s));

  /* ---------- Pixel art ---------- */
  const PALETTE = {
    g: "#5fd35f", G: "#3a9a3a", d: "#8b5a2b", D: "#5e3a1a",
    p: "#c084fc", P: "#7c3aed", o: "#2d1f4a",
    w: "#ffffff", k: "#1e1b2e",
    c: "#35d3dc", C: "#0e7490",
    y: "#fde047", Y: "#f59e0b",
    s: "#9ca3af", S: "#4b5563",
    r: "#f87171", R: "#b91c1c",
    h: "#4a2f1b", f: "#e0a982", F: "#c8875e", B: "#3b5bdb",
  };

  const ICONS = {
    grass: ["gggggggg", "gGgggGgg", "GdGgdGgG", "ddDddGdd", "dDdddddD", "ddddDddd", "dDdddddd", "ddddddDd"],
    room: ["oooooooo", "oPPpPPPo", "oPpPPpPo", "opPPpPPo", "oPPpPPpo", "oPpPPpPo", "oPPPpPPo", "oooooooo"],
    "player-a": ["kkkkkkkk", "kkkkkkkk", "kffffffk", "ffffffff", "fwkffkwf", "ffffffff", "ffFFFFff", "RrrrrrrR"],
    "player-b": ["hhhhhhhh", "hhhhhhhh", "hffffffh", "fFffffFf", "fwBffBwf", "fffhhfff", "ffhffhff", "ffhhhhff"],
    lantern: ["...SS...", "..S..S..", ".SSSSSS.", ".SyyyyS.", ".SywyyS.", ".SyyyYS.", ".SSSSSS.", "........"],
    friends: ["........", ".rr..rr.", "rwrrrrrr", "rrrrrrrr", "rrrrrrrR", ".rrrrrR.", "..rrRR..", "...RR..."],
    monitor: ["cccccccc", "cCCCCCCc", "cCgCCCCc", "cCCgggCc", "cCCCCCCc", "cccccccc", "...ss...", ".ssssss."],
    terminal: ["SSSSSSSS", "SkkkkkkS", "SkckkkkS", "SkkckkkS", "SkckwwkS", "SkkkkkkS", "SSSSSSSS", "........"],
    diamond: ["...cc...", "..cwcc..", ".cwcccC.", "cccccCCC", ".cCcCCC.", "..cCCC..", "...CC...", "........"],
    lock: ["..ssss..", ".s....s.", ".s....s.", "yyyyyyyy", "yYYYYYYy", "yYYkkYYy", "yYYYkYYy", "yyyyyyyy"],
  };

  function pixelSVG(rows, palette = PALETTE) {
    const h = rows.length, w = rows[0].length;
    let rects = "";
    rows.forEach((row, y) => {
      // Merge horizontal runs of the same colour to keep the DOM small.
      for (let x = 0; x < w;) {
        const ch = row[x];
        let run = 1;
        while (x + run < w && row[x + run] === ch) run++;
        if (ch !== "." && palette[ch]) rects += `<rect x="${x}" y="${y}" width="${run}" height="1" fill="${palette[ch]}"/>`;
        x += run;
      }
    });
    return `<svg viewBox="0 0 ${w} ${h}" shape-rendering="crispEdges" aria-hidden="true">${rects}</svg>`;
  }

  $$("[data-icon]").forEach((el) => {
    const rows = ICONS[el.dataset.icon];
    if (rows) el.innerHTML = pixelSVG(rows);
  });

  /* ---------- Terminal banner (same 5×5 font and gradient as the real CLI) ---------- */
  const FONT = {
    N: ["10001", "11001", "10101", "10011", "10001"],
    E: ["11111", "10000", "11110", "10000", "11111"],
    M: ["10001", "11011", "10101", "10001", "10001"],
    C: ["01111", "10000", "10000", "10000", "01111"],
    B: ["11110", "10001", "11110", "10001", "11110"],
    R: ["11110", "10001", "11110", "10100", "10010"],
    I: ["11111", "00100", "00100", "00100", "11111"],
    D: ["11110", "10001", "10001", "10001", "11110"],
    G: ["01111", "10000", "10111", "10001", "01111"],
  };
  const TITLE = "NEMCBRIDGE";
  const letterColor = (i) => `rgb(${104 - i * 6},${155 + i * 8},214)`;
  const esc = (s) => s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

  function renderTerminal(version) {
    const term = $("[data-term]");
    if (!term) return;
    const width = TITLE.length * 6 - 1 + 4;
    const lines = [`<span class="t-b">╔${"═".repeat(width)}╗</span>`];
    for (let row = 0; row < 5; row++) {
      let line = `<span class="t-b">║  </span>`;
      [...TITLE].forEach((letter, i) => {
        const px = FONT[letter][row].replace(/1/g, "█").replace(/0/g, " ");
        line += `<span style="color:${letterColor(i)}">${px}</span>${i < TITLE.length - 1 ? " " : ""}`;
      });
      lines.push(line + `<span class="t-b">  ║</span>`);
    }
    lines.push(`<span class="t-b">╚${"═".repeat(width)}╝</span>`);
    lines.push(`  <span class="t-name">NEMCBridge</span> ${esc(version)}  <span class="t-dim">·  windows/amd64</span>`);
    lines.push(`  网易版 ↔︎ 国际版 Bedrock · 在同一个世界相遇`, "");
    lines.push(`<span class="t-dim">NEMCBridge 控制台监听：0.0.0.0:18321</span>`);
    lines.push(`本机访问：http://127.0.0.1:18321/`);
    lines.push(`局域网访问：http://192.168.1.20:18321/`);
    lines.push(`管理员账号：admin`);
    term.innerHTML = lines.join("\n");
  }

  renderTerminal(FALLBACK_TAG);

  /* ---------- Nav ---------- */
  const nav = $(".nav");
  const onScroll = () => nav.classList.toggle("is-scrolled", window.scrollY > 12);
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });


  /* ---------- Docs table of contents ---------- */
  const tocLinks = $$(".docs-nav nav a");
  if (tocLinks.length && "IntersectionObserver" in window) {
    const byId = new Map(tocLinks.map((a) => [a.hash.slice(1), a]));
    const setActive = (id) => {
      tocLinks.forEach((a) => a.classList.toggle("is-active", a.hash === `#${id}`));
      // Keep the active chip visible in the horizontal mobile bar.
      const a = byId.get(id);
      const bar = a?.closest(".docs-nav");
      if (a && bar && bar.scrollWidth > bar.clientWidth) bar.scrollTo({ left: a.offsetLeft - 16, behavior: reduceMotion ? "auto" : "smooth" });
    };
    const spy = new IntersectionObserver((entries) => {
      entries.forEach((e) => { if (e.isIntersecting) setActive(e.target.id); });
    }, { rootMargin: "-30% 0px -65% 0px" });
    byId.forEach((_, id) => { const s = document.getElementById(id); if (s) spy.observe(s); });
    setActive(location.hash.slice(1) || tocLinks[0].hash.slice(1));
  }

  /* ---------- Reveal on scroll ---------- */
  const revealEls = $$("[data-reveal]");
  // Stagger siblings that enter together.
  revealEls.forEach((el) => {
    const sibs = Array.from(el.parentElement.children).filter((c) => c.hasAttribute("data-reveal"));
    const idx = sibs.indexOf(el);
    if (idx > 0) el.style.setProperty("--d", `${Math.min(idx, 6) * 0.07}s`);
  });
  const onReveal = (el) => el.classList.add("is-in");
  if ("IntersectionObserver" in window && !reduceMotion) {
    const io = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) { onReveal(entry.target); io.unobserve(entry.target); }
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
    revealEls.forEach((el) => io.observe(el));
  } else {
    revealEls.forEach(onReveal);
  }

  /* ---------- Tabs ---------- */
  const tabs = $$('[role="tab"]');
  const selectTab = (tab, focus) => {
    tabs.forEach((t) => {
      const on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
    });
    if (focus) tab.focus();
  };
  tabs.forEach((tab, i) => {
    tab.tabIndex = tab.getAttribute("aria-selected") === "true" ? 0 : -1;
    tab.addEventListener("click", () => selectTab(tab));
    tab.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
      const next = tabs[(i + (e.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
      selectTab(next, true);
    });
  });

  /* ---------- Copy buttons ---------- */
  $$("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const text = btn.parentElement.querySelector("code").innerText.trim();
      try {
        await navigator.clipboard.writeText(text);
      } catch {
        const ta = Object.assign(document.createElement("textarea"), { value: text });
        ta.style.cssText = "position:fixed;opacity:0";
        document.body.appendChild(ta); ta.select();
        try { document.execCommand("copy"); } catch { /* ignore */ }
        ta.remove();
      }
      btn.textContent = "已复制";
      btn.classList.add("is-done");
      setTimeout(() => { btn.textContent = "复制"; btn.classList.remove("is-done"); }, 1600);
    });
  });

  /* ---------- Platform detection & downloads ---------- */
  const OS_NAME = { windows: "Windows", linux: "Linux", macos: "macOS" };
  const platform = detectPlatform();

  function detectPlatform() {
    const ua = navigator.userAgent;
    const p = (navigator.userAgentData?.platform || navigator.platform || "").toLowerCase();
    if (/android|iphone|ipad|ipod/i.test(ua)) return null;
    if (p.includes("win") || /windows/i.test(ua)) return { os: "windows", arch: /arm64|aarch64/i.test(ua) ? "arm64" : "amd64" };
    // Browsers report Intel for Apple Silicon too; most current Macs are arm64.
    if (p.includes("mac") || /mac os x/i.test(ua)) return { os: "macos", arch: "arm64" };
    if (p.includes("linux") || /linux/i.test(ua)) return { os: "linux", arch: /aarch64|arm64/i.test(ua + p) ? "arm64" : "amd64" };
    return null;
  }

  if (platform && navigator.userAgentData?.getHighEntropyValues) {
    navigator.userAgentData.getHighEntropyValues(["architecture", "bitness"]).then((v) => {
      if (v.architecture === "arm") platform.arch = "arm64";
      else if (v.architecture === "x86" && platform.os !== "macos") platform.arch = "amd64";
      applyRelease(current);
    }).catch(() => {});
  }

  if (platform) {
    const tabId = { windows: "t-win", linux: "t-linux", macos: "t-mac" }[platform.os];
    const tab = document.getElementById(tabId);
    if (tab) selectTab(tab);
  }

  const assetName = (tag, os, arch) => `NEMCBridge_${tag}_${os}_${arch}${os === "windows" ? ".zip" : ".tar.gz"}`;
  let current = { tag: FALLBACK_TAG, assets: null };

  function applyRelease(rel) {
    const { tag } = rel;
    const urlFor = (name) => rel.assets?.find((a) => a.name === name)?.browser_download_url
      || `https://github.com/${REPO}/releases/download/${tag}/${name}`;

    $$("[data-version]").forEach((el) => { el.textContent = tag; });
    $$(".dl").forEach((card) => {
      const name = assetName(tag, card.dataset.os, card.dataset.arch);
      card.href = urlFor(name);
      card.querySelector(".dl-file").textContent = name;
      card.classList.toggle("is-recommended", !!platform && card.dataset.os === platform.os && card.dataset.arch === platform.arch);
    });

    const heroBtn = $("[data-os-download]");
    if (heroBtn) {
      if (platform) {
        heroBtn.href = urlFor(assetName(tag, platform.os, platform.arch));
        $("[data-os-label]").textContent = `下载 ${OS_NAME[platform.os]} 版`;
        $("[data-version-label]").textContent = `${tag} · ${platform.arch} · 免安装单文件`;
      } else {
        $("[data-version-label]").textContent = `${tag} · 免安装单文件`;
      }
    }
    renderTerminal(tag);
  }
  applyRelease(current);

  const cacheGet = (k) => { try { const v = JSON.parse(sessionStorage.getItem(k)); return v && Date.now() - v.t < 6e5 ? v.d : null; } catch { return null; } };
  const cacheSet = (k, d) => { try { sessionStorage.setItem(k, JSON.stringify({ t: Date.now(), d })); } catch { /* ignore */ } };
  async function gh(path) {
    const hit = cacheGet(path);
    if (hit) return hit;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 6000);
    try {
      const res = await fetch(`https://api.github.com/repos/${REPO}${path}`, { signal: ctrl.signal, headers: { Accept: "application/vnd.github+json" } });
      if (!res.ok) throw new Error(res.status);
      const data = await res.json();
      cacheSet(path, data);
      return data;
    } finally {
      clearTimeout(timer);
    }
  }

  /* ---------- Release notes ---------- */
  // Release titles carry a summary: "NEMCBridge v0.1.8 · 控制台一键更新".
  const titleSummary = (r) => (r.name || "").replace(/^NEMCBridge\s+v\S+\s*/, "").replace(/^[·:：|—-]\s*/, "").trim();
  // The "更新内容" section of a release body, the part that describes the changes.
  const changesOf = (body) => {
    const text = (body || "").replace(/\r\n/g, "\n");
    const start = text.indexOf("## 更新内容");
    if (start < 0) return text.replace(/^# .*\n/, "").split(/\n## /)[0].trim();
    const rest = text.slice(start + "## 更新内容".length);
    const end = rest.search(/\n## /);
    return (end < 0 ? rest : rest.slice(0, end)).trim();
  };
  // Minimal inline Markdown (bold, code, links, bare URLs) built as DOM nodes.
  const inline = (parent, text) => {
    const re = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)|(https?:\/\/[^\s，。、）)]+)/g;
    let last = 0, m;
    while ((m = re.exec(text))) {
      parent.append(text.slice(last, m.index));
      let node;
      if (m[1] !== undefined) { node = document.createElement("strong"); inline(node, m[1]); }
      else if (m[2] !== undefined) { node = document.createElement("code"); node.textContent = m[2]; }
      else {
        node = document.createElement("a");
        node.href = m[4] || m[5];
        node.textContent = m[3] || m[5];
        node.target = "_blank";
        node.rel = "noopener";
      }
      parent.append(node);
      last = re.lastIndex;
    }
    parent.append(text.slice(last));
  };
  const renderChanges = (target, markdown) => {
    let list = null;
    for (const raw of markdown.split("\n")) {
      const line = raw.trim();
      if (!line) { list = null; continue; }
      if (/^[-*] /.test(line)) {
        if (!list) target.append(list = document.createElement("ul"));
        const li = document.createElement("li");
        inline(li, line.slice(2));
        list.append(li);
      } else {
        list = null;
        const p = document.createElement("p");
        inline(p, line.replace(/^#+\s*/, ""));
        target.append(p);
      }
    }
  };
  const releaseDate = (iso) => new Date(iso).toLocaleDateString("zh-CN", { year: "numeric", month: "long", day: "numeric" });
  // Releases up to v0.1.8 were re-published when this repository replaced the
  // original one; keep their original publication times.
  const ORIGINAL_PUBLISHED = {
    "v0.1.0": "2026-10-15T07:21:07Z", "v0.1.1": "2026-10-15T07:44:01Z", "v0.1.2": "2026-10-15T08:00:29Z",
    "v0.1.3": "2026-10-15T09:37:50Z", "v0.1.4": "2026-10-15T11:34:08Z", "v0.1.5": "2026-10-15T15:52:36Z",
    "v0.1.6": "2026-10-15T17:04:46Z", "v0.1.7": "2026-10-16T16:16:08Z", "v0.1.8": "2026-10-18T11:59:36Z",
  };
  const publishedAt = (r) => ORIGINAL_PUBLISHED[r.tag_name] || r.published_at;

  const timeline = $(".timeline");
  if (timeline) {
    gh("/releases?per_page=100").then((list) => {
      const releases = (Array.isArray(list) ? list : []).filter((r) => r.tag_name && !r.draft && !r.prerelease);
      if (!releases.length) return;
      timeline.replaceChildren(...releases.map((r, i) => {
        const item = document.createElement("li");
        const head = document.createElement("div");
        head.className = "tl-head";
        const ver = document.createElement("a");
        ver.className = "tl-ver";
        ver.href = r.html_url;
        ver.target = "_blank";
        ver.rel = "noopener";
        ver.textContent = r.tag_name;
        head.append(ver);
        const summary = titleSummary(r);
        if (summary) {
          const title = document.createElement("span");
          title.className = "tl-title";
          title.textContent = summary;
          head.append(title);
        }
        if (i === 0) {
          const badge = document.createElement("span");
          badge.className = "tl-badge";
          badge.textContent = "最新";
          head.append(badge);
        }
        if (publishedAt(r)) {
          const date = document.createElement("time");
          date.className = "tl-date";
          date.dateTime = publishedAt(r);
          date.textContent = releaseDate(publishedAt(r));
          head.append(date);
        }
        item.append(head);
        renderChanges(item, changesOf(r.body));
        return item;
      }));
    }).catch(() => {});
  }

  gh("/releases/latest").then((r) => {
    if (!r?.tag_name) return;
    current = { tag: r.tag_name, assets: r.assets || [] };
    applyRelease(current);
    const first = (r.body || "").split("\n").map((l) => l.trim()).find((l) => /^[-*] /.test(l));
    const summary = titleSummary(r) || (first ? first.replace(/^[-*]\s+/, "").replace(/[`*_]/g, "").replace(/[。.]$/, "") : "查看更新内容");
    const label = $("[data-release-label]");
    if (label) label.textContent = `${r.tag_name} 已发布 · ${summary}`;
    const dateEl = $("[data-release-date]");
    if (dateEl && publishedAt(r)) dateEl.textContent = `${releaseDate(publishedAt(r))}发布，选择你的系统与架构。`;
  }).catch(() => {});

  gh("").then((repo) => {
    const n = repo?.stargazers_count;
    if (!n) return;
    const el = $("[data-stars]");
    el.textContent = n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
    el.hidden = false;
  }).catch(() => {});

  /* ---------- Mock web console: tick uptime and stream log lines ---------- */
  const mock = $(".mock-console");
  if (mock && !reduceMotion) {
    let secs = 3 * 3600 + 12 * 60 + 45;
    let players = 4;
    const uptime = $("[data-uptime]", mock);
    const count = $("[data-players]", mock);
    const log = $("[data-log]", mock);
    const feed = [
      ["Alex 加入了游戏", 1], ["<竹林小筑> 门口集合！", 0], ["<Steve> on my way", 0],
      ["山海 加入了游戏", 1], ["<山海> 我带了钻石镐 ⛏️", 0], ["[DEBUG] 区块批次 214 KiB", 0],
      ["Alex 被苦力怕炸死了", 0], ["<Alex> creeper... 😭", 0], ["山海 离开了游戏", -1],
      ["好友申请已自动同意：夜航船", 0], ["夜航船 加入了游戏", 1], ["<夜航船> 你好呀", 0],
      ["RakNet 连接超时，正在重试", 0], ["Alex 离开了游戏", -1],
    ];
    let i = 0;
    const pad = (n) => String(n).padStart(2, "0");
    const clock = () => { const d = new Date(); return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`; };
    let visible = true;
    if ("IntersectionObserver" in window) new IntersectionObserver(([e]) => { visible = e.isIntersecting; }).observe(mock);
    setInterval(() => {
      secs++;
      if (!visible) return;
      // Same format as the real console.
      uptime.textContent = `${Math.floor(secs / 3600)}时 ${Math.floor(secs % 3600 / 60)}分`;
    }, 1000);
    setInterval(() => {
      if (!visible || document.hidden) return;
      const [text, delta] = feed[i++ % feed.length];
      players = Math.max(1, Math.min(9, players + delta));
      count.textContent = players;
      const p = document.createElement("p");
      if (text.startsWith("[DEBUG]")) p.className = "is-debug";
      else if (/失败|错误|断开|超时/.test(text)) p.className = "is-warn";
      p.innerHTML = `<time>${clock()}</time>${esc(text)}`;
      log.appendChild(p);
      while (log.children.length > 3) log.firstElementChild.remove();
    }, 2600);
  }

  /* ---------- Pointer effects ---------- */
  const finePointer = window.matchMedia("(pointer: fine)").matches;
  if (finePointer && !reduceMotion) {
    $$(".card").forEach((card) => {
      card.addEventListener("pointermove", (e) => {
        const r = card.getBoundingClientRect();
        card.style.setProperty("--mx", `${e.clientX - r.left}px`);
        card.style.setProperty("--my", `${e.clientY - r.top}px`);
      });
    });
    const show = $("[data-tilt]");
    if (show) {
      show.addEventListener("pointermove", (e) => {
        const r = show.getBoundingClientRect();
        const x = (e.clientX - r.left) / r.width - 0.5;
        const y = (e.clientY - r.top) / r.height - 0.5;
        show.style.setProperty("--ry", `${(x * 5).toFixed(2)}deg`);
        show.style.setProperty("--rx", `${(-y * 4).toFixed(2)}deg`);
      });
      show.addEventListener("pointerleave", () => {
        show.style.setProperty("--ry", "0deg");
        show.style.setProperty("--rx", "0deg");
      });
    }
  }

  /* ---------- Portal embers ---------- */
  const canvas = $(".embers");
  if (canvas && !reduceMotion && canvas.getContext) {
    const ctx = canvas.getContext("2d");
    const hero = $(".hero");
    const colors = ["#c084fc", "#a855f7", "#e9d5ff", "#f0abfc", "#67e8f9"];
    let w = 0, h = 0, dpr = 1, parts = [], running = true, raf = 0;

    const resize = () => {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = canvas.clientWidth; h = canvas.clientHeight;
      canvas.width = w * dpr; canvas.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      const target = Math.round(Math.min(70, w / 18));
      while (parts.length < target) parts.push(spawn(true));
      parts.length = target;
    };
    const spawn = (anywhere) => ({
      x: w / 2 + (Math.random() - 0.5) * w * 0.9,
      y: anywhere ? Math.random() * h : h + 10,
      s: [2, 3, 3, 4, 5][Math.floor(Math.random() * 5)],
      vy: 0.2 + Math.random() * 0.55,
      vx: (Math.random() - 0.5) * 0.15,
      a: 0.25 + Math.random() * 0.6,
      c: colors[Math.floor(Math.random() * colors.length)],
      t: Math.random() * Math.PI * 2,
    });
    const tick = () => {
      ctx.clearRect(0, 0, w, h);
      for (let i = 0; i < parts.length; i++) {
        const p = parts[i];
        p.y -= p.vy; p.t += 0.02; p.x += p.vx + Math.sin(p.t) * 0.15;
        if (p.y < -10) parts[i] = spawn(false);
        const fade = Math.min(1, p.y / (h * 0.25));
        ctx.globalAlpha = p.a * Math.max(0, fade);
        ctx.fillStyle = p.c;
        ctx.fillRect(Math.round(p.x), Math.round(p.y), p.s, p.s);
      }
      ctx.globalAlpha = 1;
      if (running) raf = requestAnimationFrame(tick);
    };
    resize();
    window.addEventListener("resize", resize);
    if ("IntersectionObserver" in window) {
      new IntersectionObserver(([e]) => {
        running = e.isIntersecting;
        cancelAnimationFrame(raf);
        if (running) raf = requestAnimationFrame(tick);
      }).observe(hero);
    } else {
      raf = requestAnimationFrame(tick);
    }
  }
})();
