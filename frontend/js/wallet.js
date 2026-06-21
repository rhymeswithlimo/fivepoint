/* ============================================================
   Fivepoint — wallet detail page (wallet.html)
   Segmented switcher, responsive transfer button, balance reveal,
   wallet-card tilt/shine, and back navigation.
   ============================================================ */

/* ----------  Segmented switcher (range selectors)  ---------- */
(function () {
    const PADDING = 4;

    function positionSlider(switcher, animate) {
        const slider = switcher.querySelector('.switcher-slider');
        const active = switcher.querySelector('.switcher-btn.active');
        if (!slider || !active) return;

        slider.style.transition = 'none';
        slider.style.width = active.offsetWidth + 'px';

        if (animate) {
            requestAnimationFrame(() => {
                slider.style.transition = '';
                slider.style.transform = 'translateX(' + (active.offsetLeft - PADDING) + 'px)';
            });
        } else {
            slider.style.transform = 'translateX(' + (active.offsetLeft - PADDING) + 'px)';
        }
    }

    function snapAll() {
        document.querySelectorAll('.switcher').forEach((sw) => positionSlider(sw, false));
    }

    document.addEventListener('DOMContentLoaded', () => {
        document.querySelectorAll('.switcher').forEach((switcher) => {
            switcher.querySelectorAll('.switcher-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    switcher.querySelectorAll('.switcher-btn').forEach((b) => b.classList.remove('active'));
                    btn.classList.add('active');
                    positionSlider(switcher, true);
                });
            });
        });

        snapAll();
    });

    window.addEventListener('resize', snapAll);
    window.addEventListener('load', snapAll);
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(snapAll);
})();

/* ----------  Responsive transfer button  ---------- */
(function () {
    const ICON = `
        <svg xmlns="http://www.w3.org/2000/svg"
            width="32"
            height="32"
            fill="#0e0e0e"
            viewBox="0 0 256 256"
            aria-label="Transfer Funds">
            <path d="M213.66,181.66l-32,32a8,8,0,0,1-11.32-11.32L188.69,184H48a8,8,0,0,1,0-16H188.69l-18.35-18.34a8,8,0,0,1,11.32-11.32l32,32A8,8,0,0,1,213.66,181.66Zm-139.32-64a8,8,0,0,0,11.32-11.32L67.31,88H208a8,8,0,0,0,0-16H67.31L85.66,53.66A8,8,0,0,0,74.34,42.34l-32,32a8,8,0,0,0,0,11.32Z"></path>
        </svg>
    `;
    const LABEL = 'Transfer Funds';

    document.addEventListener('DOMContentLoaded', () => {
        const btn = document.getElementById('transferBtn');
        if (!btn) return;

        function update() {
            if (window.innerWidth < 360) {
                btn.innerHTML = ICON;
            } else {
                btn.textContent = LABEL;
            }
        }

        update();
        window.addEventListener('resize', update);
    });
})();

/* ----------  Wallet data: balance, card, holdings  ---------- */
(function () {
    const pick = FP.scramble.poolPicker('0123456789&%^#@!kxzqw*');
    const COLORS = new Set(['magenta', 'lime', 'orange', 'silver']);
    const PERIOD_KEY = { '1d': 'change1d', '1w': 'change7d', '1m': 'change30d', '1y': 'change1y' };

    let summary = null;

    const usd = (n) => '$' + Number(n || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    const pct = (n) => (n >= 0 ? '+' : '') + Number(n || 0).toFixed(2) + '%';
    const changeClass = (n) => (n > 0 ? 'value-up' : n < 0 ? 'value-down' : 'value-flat');
    const truncate = (a) => (a && a.length > 12 ? a.slice(0, 6) + '••••' + a.slice(-4) : a || '');

    function currentPeriod() {
        const active = document.querySelector('.switcher-btn.active');
        return (active && active.dataset.period) || '1d';
    }

    function renderPeriod(period) {
        if (!summary) return;
        const key = PERIOD_KEY[period] || 'change1d';
        const p = summary[key] || 0;
        const changeUsd = (summary.totalUsd || 0) * p / 100;

        const head = document.getElementById('holdingsChange');
        head.className = 'monospace unselectable ' + changeClass(p);
        const arrow = p >= 0 ? '↑' : '↓';
        const sign = changeUsd < 0 ? '-' : '';
        head.innerHTML = `${arrow} ${pct(p)} <span class="text-muted">(${sign}$${Math.abs(changeUsd).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })})</span>`;

        const container = document.getElementById('holdingsAssets');
        container.innerHTML = '';
        (summary.assets || []).forEach((a) => {
            const c = a[key] || 0;
            const el = document.createElement('div');
            el.className = 'asset unselectable';
            el.innerHTML =
                `<div class="asset-info"><p>${a.symbol}</p><p>${usd(a.valueUsd)}</p></div>` +
                `<div class="asset-info"><p class="${changeClass(c)}">${pct(c)}</p>` +
                `<p class="text-muted">${a.amount} ${a.symbol}</p></div>`;
            container.appendChild(el);
        });
    }

    async function load() {
        const id = new URLSearchParams(window.location.search).get('id');

        // Wallet metadata (name / icon / color).
        let meta = null;
        try {
            const list = await FP.api.listWallets();
            meta = list.find((w) => w.id === id) || list[0] || null;
        } catch (_) { /* ignore */ }

        if (meta) {
            document.title = meta.name + ' - Fivepoint';
            document.getElementById('walletCardName').textContent = meta.name;
            const color = COLORS.has(meta.color) ? meta.color : 'magenta';
            document.querySelector('.wallet-card').classList.add('c-' + color);
            FP.icons.svgFor(meta.icon).then((svg) => { document.getElementById('walletCardIcon').innerHTML = svg; }).catch(() => {});
        }

        // Valued portfolio.
        try {
            summary = await FP.api.walletPortfolio(id || (meta && meta.id) || '');
        } catch (_) {
            return;
        }

        document.getElementById('walletCardValue').textContent = usd(summary.totalUsd);
        const balanceEl = document.getElementById('balance-text');
        balanceEl.textContent = usd(summary.totalUsd);
        FP.scramble.animate(balanceEl, { pick, encrypting: false, duration: 1000 });

        // Copy-address affordance uses the EVM address (most universal).
        const eth = (summary.assets || []).find((a) => a.symbol === 'ETH' && a.address);
        if (eth) {
            const btn = document.getElementById('walletCardAddress');
            document.getElementById('walletCardAddressText').textContent = truncate(eth.address);
            btn.classList.remove('hidden');
            btn.addEventListener('click', async () => {
                try { await navigator.clipboard.writeText(eth.address); } catch (_) {}
            });
        }

        renderPeriod(currentPeriod());
    }

    document.addEventListener('DOMContentLoaded', () => {
        document.querySelectorAll('.switcher-btn').forEach((btn) => {
            btn.addEventListener('click', () => renderPeriod(btn.dataset.period));
        });
        load();
    });
})();

/* ----------  Wallet card tilt / shine  ---------- */
(function () {
    const card = document.querySelector('.wallet-card');
    if (!card) return;

    const REFLECTION_OPACITY = 2;
    const GLINT_OPACITY = 0.12;
    const GLINT_INTERVAL_MS = 31000;
    const GLINT_DURATION_MS = 1400;
    const IDLE_TRANSITION_MS = 3600; // ease-in duration for idle rotation
    const MAX_TILT = 2;
    const TRANSITION_ON = 'transform 0.1s ease-out';
    const TRANSITION_OFF = 'transform 0.6s ease-out';
    const IDLE_TIMEOUT_MS = 120000;

    let isHovered = false;
    let idleTimer = null;
    let idleRafId = null;
    let glintRafId = null;

    const reflection = document.createElement('div');
    Object.assign(reflection.style, {
        position: 'absolute', inset: '0', borderRadius: 'inherit',
        pointerEvents: 'none', opacity: '0', transition: 'opacity 0.3s ease',
        zIndex: '10', mixBlendMode: 'overlay',
    });

    const glint = document.createElement('div');
    Object.assign(glint.style, {
        position: 'absolute', inset: '0', borderRadius: 'inherit',
        pointerEvents: 'none', opacity: '0', zIndex: '11',
    });

    card.style.position = 'relative';
    card.style.overflow = 'hidden';
    card.appendChild(reflection);
    card.appendChild(glint);

    function updateReflection(nx, ny, strength) {
        const angle = Math.atan2(ny - 0.5, nx - 0.5) * 180 / Math.PI;
        const intensity = Math.min(strength, 1) * REFLECTION_OPACITY;
        reflection.style.background = `
            radial-gradient(ellipse at ${nx * 100}% ${ny * 100}%,
                rgba(255,255,255,${0.22 * intensity}) 0%,
                rgba(255,255,255,${0.06 * intensity}) 40%,
                transparent 70%),
            linear-gradient(${angle + 90}deg,
                rgba(255,255,255,${0.12 * intensity}) 0%,
                transparent 60%)
        `;
        reflection.style.opacity = String(Math.max(0.1, intensity));
    }

    function runGlint() {
        if (glintRafId) return;
        const t0 = performance.now();
        function tick(now) {
            const progress = Math.min((now - t0) / GLINT_DURATION_MS, 1);
            const eased = progress < 0.5
                ? 2 * progress * progress
                : 1 - Math.pow(-2 * progress + 2, 2) / 2;
            const pos = -30 + 160 * eased;
            const bell = Math.sin(progress * Math.PI) * GLINT_OPACITY;
            glint.style.background = `linear-gradient(
                135deg,
                transparent                      ${pos - 40}%,
                rgba(255,255,255,${bell * 0.3})  ${pos - 20}%,
                rgba(255,255,255,${bell})        ${pos}%,
                rgba(255,255,255,${bell * 0.3})  ${pos + 20}%,
                transparent                      ${pos + 40}%
            )`;
            glint.style.opacity = '1';
            if (progress < 1) {
                glintRafId = requestAnimationFrame(tick);
            } else {
                glintRafId = null;
                glint.style.opacity = '0';
            }
        }
        glintRafId = requestAnimationFrame(tick);
    }

    setInterval(runGlint, GLINT_INTERVAL_MS);

    function startIdleAnimation() {
        if (isHovered) return;
        const startTime = performance.now();

        function tick(now) {
            if (isHovered) { stopIdleAnimation(); return; }
            const elapsed = now - startTime;
            const t = elapsed / 1000;

            // Blend factor: 0 at start, 1 after IDLE_TRANSITION_MS, eased with a cubic curve.
            const blend = Math.min(elapsed / IDLE_TRANSITION_MS, 1);
            const eased = blend * blend * (3 - 2 * blend);

            const rotY = Math.sin(t * 0.6) * 8 * eased;
            const rotX = Math.sin(t * 0.4 + 1.0) * 5 * eased;

            card.style.transition = 'transform 0.1s ease-out';
            card.style.transform = `perspective(900px) rotateX(${rotX}deg) rotateY(${rotY}deg)`;

            const nx = (rotY / 8 + 1) / 2;
            const ny = (-rotX / 5 + 1) / 2;
            updateReflection(nx, ny, Math.sqrt(rotY * rotY + rotX * rotX) / 9.4);

            idleRafId = requestAnimationFrame(tick);
        }

        idleRafId = requestAnimationFrame(tick);
    }

    function stopIdleAnimation() {
        if (idleRafId) { cancelAnimationFrame(idleRafId); idleRafId = null; }
    }

    function resetIdleTimer() {
        clearTimeout(idleTimer);
        stopIdleAnimation();
        idleTimer = setTimeout(startIdleAnimation, IDLE_TIMEOUT_MS);
    }

    card.addEventListener('mouseenter', () => { isHovered = true; stopIdleAnimation(); resetIdleTimer(); });

    card.addEventListener('mousemove', (e) => {
        const rect = card.getBoundingClientRect();
        const nx = (e.clientX - rect.left) / rect.width;
        const ny = (e.clientY - rect.top) / rect.height;
        const cx = nx - 0.5;
        const cy = ny - 0.5;
        card.style.transition = TRANSITION_ON;
        card.style.transform = `perspective(900px) rotateX(${-cy * 2 * MAX_TILT}deg) rotateY(${cx * 2 * MAX_TILT}deg)`;
        updateReflection(nx, ny, Math.sqrt(cx * cx + cy * cy) * 1.414);
    });

    card.addEventListener('mouseleave', () => {
        isHovered = false;
        card.style.transition = TRANSITION_OFF;
        card.style.transform = 'perspective(900px) rotateX(0deg) rotateY(0deg)';
        reflection.style.opacity = '0';
        resetIdleTimer();
    });

    card.querySelectorAll('button').forEach((btn) => btn.addEventListener('click', resetIdleTimer));

    resetIdleTimer();
})();

/* ----------  Back navigation  ---------- */
(function () {
    document.addEventListener('DOMContentLoaded', () => {
        document.querySelectorAll('.back-button, .close-button').forEach((button) => {
            button.addEventListener('click', () => {
                if (window.history.length > 1) {
                    window.history.back();
                } else {
                    window.location.href = 'index.html';
                }
            });
        });
    });
})();
