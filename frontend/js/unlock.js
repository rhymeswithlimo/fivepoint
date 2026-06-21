/* ============================================================
   Fivepoint — unlock / first-run controller (unlock.html)
   Decides between "create a master passphrase" (first run) and "enter your
   passphrase" (returning) based on backend status, then routes onward.
   ============================================================ */

(function () {
    const MIN_PASSPHRASE = 8;

    let creating = false; // first-run vs unlock
    let pinMode = false;

    document.addEventListener('DOMContentLoaded', async () => {
        const form = document.getElementById('unlockForm');
        const prompt = document.getElementById('prompt');
        const pass = document.getElementById('pass');
        const confirm = document.getElementById('passConfirm');
        const error = document.getElementById('error');
        const submitBtn = document.getElementById('submitBtn');
        const pinBtn = document.getElementById('pinBtn');

        function showError(msg) {
            error.textContent = msg;
            error.classList.toggle('hidden', !msg);
        }

        let status = { hasVault: false, hasPIN: false };
        try {
            status = await FP.api.status();
        } catch (_) { /* treat as first run */ }

        creating = !status.hasVault;

        if (creating) {
            prompt.textContent = 'create a master passphrase';
            confirm.classList.remove('hidden');
            submitBtn.textContent = '[create]';
            pass.setAttribute('placeholder', 'new master passphrase');
        } else {
            prompt.textContent = 'enter your passphrase';
            confirm.classList.add('hidden');
            submitBtn.textContent = '[unlock]';
            pinBtn.classList.toggle('hidden', !status.hasPIN);
        }

        // Toggle to numeric PIN entry when available.
        pinBtn.addEventListener('click', () => {
            pinMode = !pinMode;
            showError('');
            if (pinMode) {
                prompt.textContent = 'enter your PIN';
                pass.value = '';
                pass.setAttribute('inputmode', 'numeric');
                pass.setAttribute('placeholder', '6-digit PIN');
                submitBtn.textContent = '[unlock]';
                pinBtn.textContent = '[use passphrase]';
            } else {
                prompt.textContent = 'enter your passphrase';
                pass.removeAttribute('inputmode');
                pass.setAttribute('placeholder', 'master passphrase');
                pinBtn.textContent = '[use PIN]';
            }
            pass.focus();
        });

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            showError('');
            const value = pass.value;

            try {
                if (creating) {
                    if (value.length < MIN_PASSPHRASE) {
                        showError(`use at least ${MIN_PASSPHRASE} characters`);
                        return;
                    }
                    if (value !== confirm.value) {
                        showError('passphrases do not match');
                        return;
                    }
                    await FP.api.createVault(value);
                    // First run continues to first-wallet setup.
                    window.location.href = 'setup.html';
                    return;
                }

                if (pinMode) {
                    await FP.api.unlockPIN(value);
                } else {
                    await FP.api.unlock(value);
                }
                window.location.href = 'index.html';
            } catch (err) {
                showError((err && err.message) ? err.message.toLowerCase() : 'unlock failed');
                pass.select();
            }
        });
    });
})();
