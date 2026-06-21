/* ============================================================
   Fivepoint — home page (index.html)
   Loads the real portfolio: total balance + per-wallet USD values, with the
   figures concealed by default behind the encrypt (eye) toggle.
   ============================================================ */

(function () {
    const COLORS = new Set(['magenta', 'lime', 'orange', 'silver']);

    const SYMBOLS = '&%^#@!*~|+=<>?kxzqw';
    const DIGITS = '0123456789';
    const pick = FP.scramble.weightedPicker(SYMBOLS, DIGITS, 0.7);
    let isEncrypted = true;

    function usd(n) {
        return '$' + Number(n || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    }

    function renderTile(w) {
        const a = document.createElement('a');
        a.href = `wallet.html?id=${encodeURIComponent(w.id)}`;
        a.className = 'wallet-tile c-' + (COLORS.has(w.color) ? w.color : 'magenta');

        const icon = document.createElement('div');
        icon.className = 'wallet-tile-icon';

        const info = document.createElement('div');
        info.className = 'wallet-tile-info';
        const name = document.createElement('p');
        name.className = 'wallet-tile-name';
        name.textContent = w.name;
        const bal = document.createElement('p');
        bal.className = 'monospace wallet-tile-balance encrypt';
        bal.textContent = usd(w.usd);
        info.append(name, bal);
        a.append(icon, info);

        FP.icons.svgFor(w.icon).then((svg) => { icon.innerHTML = svg; }).catch(() => {});
        return a;
    }

    async function loadOverview() {
        const grid = document.getElementById('walletGrid');
        const empty = document.querySelector('.wallets-empty');
        const total = document.getElementById('total-balance');

        let ov;
        try {
            ov = await FP.api.overview();
        } catch (_) {
            return; // locked/error
        }

        total.textContent = usd(ov.totalUsd);
        total.classList.add('encrypt');

        grid.innerHTML = '';
        if (!ov.wallets || ov.wallets.length === 0) {
            empty.classList.remove('hidden');
        } else {
            empty.classList.add('hidden');
            ov.wallets.forEach((w) => grid.appendChild(renderTile(w)));
        }

        // Conceal freshly rendered figures if currently in the hidden state.
        if (isEncrypted) {
            document.querySelectorAll('.encrypt').forEach((el) => FP.scramble.scrambleNow(el, pick));
        }
    }

    function wireEncryptToggle() {
        const toggle = document.getElementById('toggle-encrypt');
        if (!toggle) return;
        const [eyeSlashIcon, eyeIcon] = toggle.querySelectorAll('svg');

        function syncIcons() {
            eyeSlashIcon.style.display = isEncrypted ? '' : 'none';
            eyeIcon.style.display = isEncrypted ? 'none' : '';
        }
        syncIcons();

        toggle.addEventListener('click', () => {
            isEncrypted = !isEncrypted;
            document.querySelectorAll('.encrypt').forEach((el) =>
                FP.scramble.animate(el, { pick, encrypting: isEncrypted, duration: 500 })
            );
            syncIcons();
        });
    }

    function wireNav() {
        const add = document.getElementById('addBtn');
        if (add) add.addEventListener('click', () => (window.location.href = 'setup.html'));
    }

    document.addEventListener('DOMContentLoaded', () => {
        wireEncryptToggle();
        wireNav();
        loadOverview();
    });
})();
