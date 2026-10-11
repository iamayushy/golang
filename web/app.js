(() => {
  "use strict";

  const API = ((window.SORTLY_CONFIG && window.SORTLY_CONFIG.apiUrl) || "").replace(/\/+$/, "");
  const STORE_LIVE = "sortly.live";
  const STORE_DEAD = "sortly.graves";
  const POLL_MS = 15000;
  const STATS_MS = 10000;
  const HURRY_SECONDS = 60;
  const MAX_GRAVES = 5;

  const HINTS = {
    10: "About as long as a coffee break.",
    15: "Roughly one pizza delivery.",
    20: "Enough for a power nap.",
    25: "One sitcom episode, ads included.",
    30: "A whole lunch break. Luxury.",
  };
  const PLACEHOLDERS = [
    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    "https://maps.google.com/?q=that+taco+place",
    "https://docs.google.com/document/d/1a2b3c-very-long-id/edit",
    "https://github.com/golang/go/issues/12345#issuecomment-987654",
    "https://store.example.com/cart?item=socks&size=L&color=chaos",
  ];
  const BURST_COLORS = ["--yellow", "--pink", "--lime", "--cyan", "--lilac", "--orange"];

  const $ = (sel) => document.querySelector(sel);
  const form = $("#maker");
  const input = $("#url");
  const goBtn = $("#go");
  const errorEl = $("#error");
  const hintEl = $("#hint");
  const listEl = $("#links");
  const emptyEl = $("#empty");
  const countEl = $("#live-count");
  const gravesEl = $("#graves");
  const graveyardEl = $("#graveyard");
  const tpl = $("#link-tpl");
  const statsEl = $("#stats");
  const statTotal = $("#stat-total");
  const statTotalLabel = $("#stat-total-label");
  const statLive = $("#stat-live");
  const numberFmt = new Intl.NumberFormat();
  const shown = { total: null, live: null };

  let live = load(STORE_LIVE);
  let graves = load(STORE_DEAD);
  const cards = new Map();

  function load(key) {
    try {
      const data = JSON.parse(localStorage.getItem(key) || "[]");
      return Array.isArray(data) ? data : [];
    } catch {
      return [];
    }
  }

  function save() {
    try {
      localStorage.setItem(STORE_LIVE, JSON.stringify(live));
      localStorage.setItem(STORE_DEAD, JSON.stringify(graves));
    } catch {
      /* private mode or storage full: links just won't survive a reload */
    }
  }

  async function api(path, options = {}) {
    const res = await fetch(API + path, options);
    const body = res.status === 204 ? null : await res.json().catch(() => null);
    if (!res.ok) {
      const err = new Error((body && body.error) || `Request failed (${res.status})`);
      err.status = res.status;
      throw err;
    }
    return body;
  }

  // ---------- making links ----------

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    let url = input.value.trim();
    if (url && !/^[a-z][a-z0-9+.-]*:/i.test(url)) url = "https://" + url;
    if (!isHttpUrl(url)) {
      fail("That doesn't look like a link. Try something starting with https://");
      return;
    }

    const ttl = Number(form.elements.ttl.value);
    goBtn.disabled = true;
    goBtn.textContent = "Lighting…";
    errorEl.textContent = "";

    try {
      const link = await api("/api/links", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url, ttl_minutes: ttl }),
      });
      const item = {
        code: link.code,
        url: link.url,
        shortUrl: link.short_url,
        token: link.delete_token,
        clicks: link.clicks,
        ttl: ttl * 60,
        // Server tells us seconds left so a wrong device clock doesn't matter.
        expiresAt: Date.now() + link.expires_in_seconds * 1000,
      };
      live.unshift(item);
      save();
      addCard(item, true);
      input.value = "";
      if (shown.total !== null) showStats(shown.total + 1, shown.live + 1);
      burst(goBtn);
      goBtn.classList.remove("lit");
      void goBtn.offsetWidth;
      goBtn.classList.add("lit");
    } catch (err) {
      if (err.status === 429) fail("Whoa, easy. You've made a lot of links. Give it a minute.");
      else if (err.status === 400) fail(capitalise(err.message) + ".");
      else fail("Couldn't reach the server. Is it awake?");
    } finally {
      goBtn.disabled = false;
      goBtn.textContent = "Light the fuse";
    }
  });

  function isHttpUrl(value) {
    try {
      const u = new URL(value);
      return (u.protocol === "http:" || u.protocol === "https:") && u.hostname.includes(".");
    } catch {
      return false;
    }
  }

  function fail(message) {
    errorEl.textContent = message;
    form.classList.remove("shake");
    void form.offsetWidth;
    form.classList.add("shake");
    input.focus();
  }

  function capitalise(s) {
    return s.charAt(0).toUpperCase() + s.slice(1);
  }

  form.addEventListener("change", (event) => {
    if (event.target.name === "ttl") hintEl.textContent = HINTS[event.target.value];
  });

  // ---------- cards ----------

  function addCard(item, fresh) {
    const node = tpl.content.firstElementChild.cloneNode(true);
    const short = node.querySelector(".short");
    short.href = item.shortUrl;
    short.textContent = item.shortUrl.replace(/^https?:\/\//, "");
    const dest = node.querySelector(".dest");
    dest.textContent = "→ " + item.url;
    dest.title = item.url;

    node.querySelector('[data-act="copy"]').addEventListener("click", (e) => copy(item, e.currentTarget));
    node.querySelector('[data-act="kill"]').addEventListener("click", () => detonate(item));

    if (fresh) listEl.prepend(node);
    else listEl.append(node);
    cards.set(item.code, node);
    paint(item);
    refreshCounts();
  }

  function paint(item) {
    const node = cards.get(item.code);
    if (!node) return;
    const left = Math.max(0, Math.round((item.expiresAt - Date.now()) / 1000));
    node.querySelector(".timer").textContent = clock(left);
    node.querySelector(".burn").style.width = (100 * left) / item.ttl + "%";
    node.querySelector(".clicks").textContent = item.clicks === 1 ? "1 click" : `${item.clicks} clicks`;
    node.classList.toggle("hurry", left > 0 && left <= HURRY_SECONDS);
    if (left === 0) explode(item, "fuse ran out");
  }

  function clock(seconds) {
    const m = Math.floor(seconds / 60);
    const s = String(seconds % 60).padStart(2, "0");
    return `${m}:${s}`;
  }

  async function copy(item, button) {
    try {
      await navigator.clipboard.writeText(item.shortUrl);
    } catch {
      window.prompt("Copy this link:", item.shortUrl);
      return;
    }
    button.textContent = "Copied!";
    setTimeout(() => (button.textContent = "Copy"), 1400);
  }

  async function detonate(item) {
    try {
      await api(`/api/links/${encodeURIComponent(item.code)}`, {
        method: "DELETE",
        headers: { "X-Delete-Token": item.token || "" },
      });
    } catch (err) {
      if (err.status !== 404) {
        errorEl.textContent = "Couldn't detonate that one. It'll still go off on its own.";
        return;
      }
    }
    explode(item, "detonated early");
  }

  function explode(item, cause) {
    const node = cards.get(item.code);
    if (!node || node.classList.contains("dead")) return;
    cards.delete(item.code);
    live = live.filter((l) => l.code !== item.code);
    graves.unshift({ code: item.code, shortUrl: item.shortUrl, clicks: item.clicks, cause, at: Date.now() });
    graves = graves.slice(0, MAX_GRAVES);
    save();

    node.classList.remove("hurry");
    node.classList.add("dead");
    node.querySelectorAll("button").forEach((b) => (b.disabled = true));
    node.querySelector(".short").removeAttribute("href");
    refreshCounts();
    setTimeout(() => {
      node.classList.add("gone");
      node.addEventListener("animationend", () => {
        node.remove();
        renderGraves();
        refreshCounts();
      }, { once: true });
    }, 1600);
  }

  function renderGraves() {
    gravesEl.replaceChildren(
      ...graves.map((g) => {
        const li = document.createElement("li");
        const code = document.createElement("code");
        code.textContent = "RIP /" + g.code;
        const note = document.createElement("span");
        note.textContent = `${g.clicks} ${g.clicks === 1 ? "click" : "clicks"} · ${g.cause}`;
        li.append(code, note);
        return li;
      }),
    );
    graveyardEl.hidden = graves.length === 0;
  }

  function refreshCounts() {
    countEl.textContent = String(cards.size);
    emptyEl.hidden = listEl.children.length > 0;
  }

  // ---------- clocks ----------

  function tick() {
    live.forEach(paint);
    const next = live.reduce((min, l) => Math.min(min, l.expiresAt), Infinity);
    document.title = Number.isFinite(next)
      ? `${clock(Math.max(0, Math.round((next - Date.now()) / 1000)))} · sortly`
      : "sortly · links with a short life";
  }

  async function pollClicks() {
    if (document.hidden) return;
    await Promise.all(
      live.map(async (item) => {
        try {
          const link = await api(`/api/links/${encodeURIComponent(item.code)}`);
          item.clicks = link.clicks;
          item.expiresAt = Date.now() + link.expires_in_seconds * 1000;
        } catch (err) {
          if (err.status === 404) item.expiresAt = Date.now();
        }
      }),
    );
    save();
    tick();
  }

  // ---------- site-wide counters ----------

  async function loadStats() {
    if (document.hidden) return;
    try {
      const st = await api("/api/stats");
      showStats(st.total_created, st.live);
    } catch {
      /* stats are a garnish; keep whatever we showed last */
    }
  }

  function showStats(total, liveNow) {
    setCount(statTotal, "total", total);
    setCount(statLive, "live", liveNow);
    statTotalLabel.textContent = total === 1 ? "link lit so far" : "links lit so far";
    statsEl.hidden = false;
  }

  // Rolls the number up to its new value and gives it a little bump.
  function setCount(el, key, value) {
    const from = shown[key];
    shown[key] = value;
    if (from === null || from === value || matchMedia("(prefers-reduced-motion: reduce)").matches) {
      el.textContent = numberFmt.format(value);
      return;
    }
    const start = performance.now();
    const step = (t) => {
      const p = Math.min(1, (t - start) / 500);
      el.textContent = numberFmt.format(Math.round(from + (value - from) * (1 - (1 - p) ** 3)));
      if (p < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
    if (value > from) {
      el.classList.remove("bump");
      void el.offsetWidth;
      el.classList.add("bump");
    }
  }

  // ---------- garnish ----------

  function burst(origin) {
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const box = origin.getBoundingClientRect();
    const styles = getComputedStyle(document.documentElement);
    for (let i = 0; i < 14; i++) {
      const bit = document.createElement("i");
      bit.className = "bit";
      const angle = (Math.PI * 2 * i) / 14 + Math.random() * 0.4;
      const dist = 70 + Math.random() * 90;
      bit.style.left = box.left + box.width / 2 + "px";
      bit.style.top = box.top + box.height / 2 + "px";
      bit.style.background = styles.getPropertyValue(BURST_COLORS[i % BURST_COLORS.length]);
      bit.style.borderRadius = i % 3 === 0 ? "50%" : "2px";
      bit.style.setProperty("--dx", Math.cos(angle) * dist + "px");
      bit.style.setProperty("--dy", Math.sin(angle) * dist + "px");
      bit.style.setProperty("--r", Math.round(Math.random() * 540 - 270) + "deg");
      bit.addEventListener("animationend", () => bit.remove(), { once: true });
      document.body.append(bit);
    }
  }

  function rotatePlaceholder() {
    let i = 0;
    input.placeholder = PLACEHOLDERS[0];
    setInterval(() => {
      if (document.activeElement === input || input.value) return;
      i = (i + 1) % PLACEHOLDERS.length;
      input.placeholder = PLACEHOLDERS[i];
    }, 3000);
  }

  function setupPaste() {
    const paste = $("#paste");
    if (!navigator.clipboard || !navigator.clipboard.readText) return;
    paste.hidden = false;
    paste.addEventListener("click", async () => {
      try {
        input.value = (await navigator.clipboard.readText()).trim();
        input.focus();
      } catch {
        paste.hidden = true;
      }
    });
  }

  function showGoneBanner() {
    const params = new URLSearchParams(location.search);
    const code = params.get("gone");
    if (!code) return;
    const banner = $("#gone");
    banner.append("Too late! ");
    const c = document.createElement("code");
    c.textContent = "/" + code;
    banner.append(c, " already went poof. Make your own below.");
    banner.hidden = false;
    params.delete("gone");
    const qs = params.toString();
    history.replaceState(null, "", location.pathname + (qs ? "?" + qs : "") + location.hash);
  }

  // ---------- boot ----------

  live = live.filter((l) => l && l.code && l.expiresAt > Date.now());
  live.forEach((item) => addCard(item, false));
  save();
  renderGraves();
  refreshCounts();
  showGoneBanner();
  rotatePlaceholder();
  setupPaste();
  tick();
  setInterval(tick, 1000);
  setInterval(pollClicks, POLL_MS);
  loadStats();
  setInterval(loadStats, STATS_MS);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) return;
    pollClicks();
    loadStats();
  });
})();
