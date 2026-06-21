/* ============================================================
   Fivepoint — backend bridge
   Exposes FP.api with the same async methods whether running inside the Wails
   desktop runtime (real Go backend at window.go.app.App) or in a plain browser
   during development (an in-memory localStorage mock so the UI still works).
   Secrets are NEVER stored by the mock in a real build path — it exists only so
   the static pages render and flows can be clicked through in a browser.
   ============================================================ */

window.FP = window.FP || {};

FP.api = (function () {
    const wails = () => window.go && window.go.app && window.go.app.App;

    // ---- Real backend: proxy straight to the bound Go methods ----
    function realApi() {
        const A = wails();
        return {
            isReal: true,
            status: () => A.Status(),
            createVault: (pass) => A.CreateVault(pass),
            unlock: (pass) => A.Unlock(pass),
            unlockPIN: (pin) => A.UnlockPIN(pin),
            lock: () => A.Lock(),
            setPIN: (pin) => A.SetPIN(pin),
            createWallet: (name, icon, color) => A.CreateWallet(name, icon, color),
            importMnemonic: (name, icon, color, mnemonic, passphrase) =>
                A.ImportMnemonic(name, icon, color, mnemonic, passphrase || ''),
            importPrivateKey: (name, icon, color, key) => A.ImportPrivateKey(name, icon, color, key),
            listWallets: () => A.ListWallets(),
            revealSeed: (id, passphrase) => A.RevealSeed(id, passphrase),
            overview: () => A.Overview(),
            walletPortfolio: (id) => A.WalletPortfolio(id),
        };
    }

    // ---- Dev mock: enough to click through flows in a browser ----
    function mockApi() {
        const KEY = 'fp.mock.vault';
        const read = () => JSON.parse(localStorage.getItem(KEY) || 'null');
        const write = (s) => localStorage.setItem(KEY, JSON.stringify(s));
        // Unlock state is per-session so it survives navigation, like the real
        // Go backend which holds the vault unlocked across page loads.
        const UNLOCK_KEY = 'fp.mock.unlocked';
        const isUnlocked = () => sessionStorage.getItem(UNLOCK_KEY) === '1';
        const setUnlocked = (v) => v
            ? sessionStorage.setItem(UNLOCK_KEY, '1')
            : sessionStorage.removeItem(UNLOCK_KEY);

        const rand = (n) =>
            Array.from(crypto.getRandomValues(new Uint8Array(n)))
                .map((b) => b.toString(16).padStart(2, '0'))
                .join('');

        const ok = (v) => Promise.resolve(v);
        const fail = (m) => Promise.reject(new Error(m));

        return {
            isReal: false,
            status: () => {
                const s = read();
                return ok({ hasVault: !!s, unlocked: isUnlocked(), hasPIN: !!(s && s.pin) });
            },
            createVault: (pass) => {
                if (read()) return fail('a vault already exists');
                write({ pass, pin: null, wallets: [] });
                setUnlocked(true);
                return ok();
            },
            unlock: (pass) => {
                const s = read();
                if (!s) return fail('no vault');
                if (s.pass !== pass) return fail('wrong passphrase');
                setUnlocked(true);
                return ok();
            },
            unlockPIN: (pin) => {
                const s = read();
                if (!s || !s.pin) return fail('PIN not set');
                if (s.pin !== pin) return fail('wrong PIN');
                setUnlocked(true);
                return ok();
            },
            lock: () => { setUnlocked(false); return ok(); },
            setPIN: (pin) => {
                const s = read();
                if (!s) return fail('locked');
                s.pin = pin; write(s); return ok();
            },
            createWallet: (name, icon, color) => {
                const s = read();
                if (!s || !isUnlocked()) return fail("locked");
                const info = { id: rand(16), name, icon, color, createdAt: new Date().toISOString(), kind: 'mnemonic' };
                s.wallets.push(info); write(s);
                return ok({ wallet: info, mnemonic: 'demo only — real mnemonic comes from the Go backend' });
            },
            importMnemonic: (name, icon, color) => {
                const s = read();
                if (!s || !isUnlocked()) return fail("locked");
                const info = { id: rand(16), name, icon, color, createdAt: new Date().toISOString(), kind: 'mnemonic' };
                s.wallets.push(info); write(s);
                return ok(info);
            },
            importPrivateKey: (name, icon, color) => {
                const s = read();
                if (!s || !isUnlocked()) return fail("locked");
                const info = { id: rand(16), name, icon, color, createdAt: new Date().toISOString(), kind: 'privatekey' };
                s.wallets.push(info); write(s);
                return ok(info);
            },
            listWallets: () => {
                const s = read();
                if (!s || !isUnlocked()) return fail("locked");
                return ok(s.wallets);
            },
            revealSeed: () => ok('demo only — real backend re-authenticates and returns the seed'),
            overview: () => {
                const s = read();
                if (!s || !isUnlocked()) return fail("locked");
                // Deterministic fake values so the dev UI shows numbers.
                const wallets = s.wallets.map((w, i) => ({ ...w, usd: 1000 * (i + 1) + 137.21 }));
                const total = wallets.reduce((t, w) => t + w.usd, 0);
                return ok({ totalUsd: total, change1d: 1.83, changeUsd1d: total * 0.0183, wallets });
            },
            walletPortfolio: () => ok({
                totalUsd: 9933.87, changeUsd1d: 121.12, change1d: 1.83, change7d: 4.2, change30d: -2.1, change1y: 38.5,
                assets: [
                    { symbol: 'BTC', name: 'Bitcoin', amount: '0.12', priceUsd: 64158, valueUsd: 7696.17, change1d: 1.72, change7d: 3, change30d: -1, change1y: 40 },
                    { symbol: 'SOL', name: 'Solana', amount: '23.21', priceUsd: 73.02, valueUsd: 1579.18, change1d: 1.32, change7d: 5, change30d: 2, change1y: 60 },
                    { symbol: 'ETH', name: 'Ethereum', amount: '0.2', priceUsd: 1731, valueUsd: 335.30, change1d: 0.81, change7d: 2, change30d: -3, change1y: 20 },
                    { symbol: 'XRP', name: 'XRP', amount: '87.20', priceUsd: 1.14, valueUsd: 100.04, change1d: -1.79, change7d: 1, change30d: 4, change1y: 12 },
                ],
            }),
        };
    }

    const impl = wails() ? realApi() : mockApi();

    // Normalize: every method returns a Promise.
    return new Proxy(impl, {
        get(target, prop) {
            const v = target[prop];
            if (typeof v !== 'function') return v;
            return (...args) => {
                try {
                    return Promise.resolve(v.apply(target, args));
                } catch (e) {
                    return Promise.reject(e);
                }
            };
        },
    });
})();
