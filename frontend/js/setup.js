/* ============================================================
   Fivepoint — setup controller (setup.html)
   Drives create-wallet (with one-time seed backup) and import-wallet flows,
   talking to the backend via FP.api.
   ============================================================ */

(function () {
    document.addEventListener('DOMContentLoaded', () => {
        const sections = {
            landing: document.querySelector('.landing'),
            create: document.getElementById('createForm'),
            import: document.getElementById('importForm'),
            backup: document.getElementById('seedBackup'),
        };

        function show(name) {
            Object.entries(sections).forEach(([key, el]) =>
                el.classList.toggle('hidden', key !== name)
            );
        }

        /* ----------  Icon picker (create flow)  ---------- */
        let selectedIcon = 'alien';
        (async function initIconPicker() {
            const selector = sections.create.querySelector('.icon-selector');
            const svgSlot = selector.querySelector('.icon-selector-svg');
            const nameLabel = selector.querySelector('.icon-name');
            const [prevBtn, nextBtn] = selector.querySelectorAll('button');

            let icons;
            try {
                icons = await FP.icons.all();
            } catch (e) {
                console.error('icon load failed', e);
                return; // keep the inline default icon/name
            }
            if (!icons.length) return;

            let index = 0;
            function render() {
                const icon = icons[index];
                selectedIcon = icon.name;
                svgSlot.innerHTML = icon.svg;
                nameLabel.innerHTML = `<i>${icon.name}</i>`;
            }
            function step(delta) {
                index = (index + delta + icons.length) % icons.length;
                render();
            }

            function holdScroll(btn, delta) {
                let timeout, interval, speed = 360;
                function tick() {
                    step(delta);
                    speed = Math.max(60, speed * 0.84);
                    interval = setTimeout(tick, speed);
                }
                function start() { step(delta); timeout = setTimeout(tick, 400); }
                function stop() { clearTimeout(timeout); clearTimeout(interval); speed = 360; }
                btn.addEventListener('mousedown', start);
                btn.addEventListener('touchstart', (e) => { e.preventDefault(); start(); }, { passive: false });
                btn.addEventListener('mouseup', stop);
                btn.addEventListener('mouseleave', stop);
                btn.addEventListener('touchend', stop);
                btn.addEventListener('touchcancel', stop);
            }
            nameLabel.setAttribute('aria-live', 'polite');
            holdScroll(prevBtn, -1);
            holdScroll(nextBtn, 1);
            render();
        })();

        /* ----------  Create wizard  ---------- */
        const createForm = sections.create;
        const questions = Array.from(createForm.querySelectorAll('.form-question'));
        const cancelBtn = document.getElementById('cancelBtn');
        const backBtn = document.getElementById('backBtn');
        const continueBtn = document.getElementById('continueBtn');
        const createError = document.getElementById('createError');
        const wname = document.getElementById('wname');
        let current = 0;

        function setError(el, msg) {
            el.textContent = msg || '';
            el.classList.toggle('hidden', !msg);
        }

        function canProceed(index) {
            switch (index) {
                case 0: return wname.value.trim() !== '';
                case 1: return true;
                case 2: return createForm.querySelector('input[name="color"]:checked') !== null;
                default: return false;
            }
        }

        function shake() {
            const q = questions[current];
            q.classList.remove('shake');
            void q.offsetWidth;
            q.classList.add('shake');
            q.addEventListener('animationend', () => q.classList.remove('shake'), { once: true });
        }

        function goTo(index) {
            questions[current].classList.remove('active');
            current = index;
            questions[current].classList.add('active');
            cancelBtn.classList.toggle('hidden', current !== 0);
            backBtn.classList.toggle('hidden', current === 0);
        }

        async function submitCreate() {
            const name = wname.value.trim();
            const color = (createForm.querySelector('input[name="color"]:checked') || {}).value || 'magenta';
            setError(createError, '');
            continueBtn.disabled = true;
            try {
                const res = await FP.api.createWallet(name, selectedIcon, color);
                showBackup(res.mnemonic);
            } catch (err) {
                setError(createError, (err && err.message) || 'could not create wallet');
            } finally {
                continueBtn.disabled = false;
            }
        }

        document.getElementById('createWalletBtn').addEventListener('click', () => { show('create'); goTo(0); });
        cancelBtn.addEventListener('click', () => show('landing'));
        backBtn.addEventListener('click', () => { if (current > 0) goTo(current - 1); });
        continueBtn.addEventListener('click', () => {
            if (!canProceed(current)) { shake(); return; }
            if (current < questions.length - 1) goTo(current + 1);
            else submitCreate();
        });
        wname.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') { e.preventDefault(); continueBtn.click(); }
        });

        /* ----------  Import flow  ---------- */
        const importForm = sections.import;
        const iname = document.getElementById('iname');
        const isecret = document.getElementById('isecret');
        const secretLabel = document.getElementById('secretLabel');
        const importError = document.getElementById('importError');
        const typeSeed = document.getElementById('typeSeed');
        const typeKey = document.getElementById('typeKey');
        let importType = 'seed';

        function setImportType(type) {
            importType = type;
            const seed = type === 'seed';
            secretLabel.textContent = seed ? 'Recovery Seed Phrase' : 'Private Key';
            isecret.setAttribute('placeholder', seed ? 'word1 word2 word3 ...' : 'paste your private key');
            typeSeed.style.opacity = seed ? '1' : '';
            typeKey.style.opacity = seed ? '' : '1';
        }

        document.getElementById('importWalletBtn').addEventListener('click', () => { show('import'); setImportType('seed'); });
        document.getElementById('importCancelBtn').addEventListener('click', () => show('landing'));
        typeSeed.addEventListener('click', () => setImportType('seed'));
        typeKey.addEventListener('click', () => setImportType('key'));

        document.getElementById('importSubmitBtn').addEventListener('click', async () => {
            setError(importError, '');
            const name = iname.value.trim();
            const secret = isecret.value.trim();
            const color = (importForm.querySelector('input[name="color"]:checked') || {}).value || 'magenta';
            if (!name) { setError(importError, 'name your wallet'); return; }
            if (!secret) { setError(importError, importType === 'seed' ? 'enter your seed phrase' : 'enter your private key'); return; }
            try {
                if (importType === 'seed') {
                    await FP.api.importMnemonic(name, '', color, secret, '');
                } else {
                    await FP.api.importPrivateKey(name, '', color, secret);
                }
                window.location.href = 'index.html';
            } catch (err) {
                setError(importError, (err && err.message) || 'could not import wallet');
            }
        });

        /* ----------  Seed backup  ---------- */
        let backupMnemonic = '';
        function showBackup(mnemonic) {
            backupMnemonic = mnemonic;
            const grid = document.getElementById('seedGrid');
            grid.innerHTML = '';
            mnemonic.split(/\s+/).forEach((word, i) => {
                const cell = document.createElement('div');
                cell.className = 'seed-word';
                cell.innerHTML = `<span class="seed-word-index">${i + 1}</span><span>${word}</span>`;
                grid.appendChild(cell);
            });
            show('backup');
        }

        document.getElementById('copySeedBtn').addEventListener('click', async () => {
            const btn = document.getElementById('copySeedBtn');
            try {
                await navigator.clipboard.writeText(backupMnemonic);
                btn.textContent = '[copied]';
                setTimeout(() => (btn.textContent = '[copy]'), 1500);
            } catch (_) { /* clipboard may be unavailable */ }
        });
        document.getElementById('seedDoneBtn').addEventListener('click', () => {
            window.location.href = 'index.html';
        });
    });
})();
