/* ============================================================
   Fivepoint — route guard
   Sends the user to the unlock screen unless the vault is currently unlocked.
   Included on every authenticated page (home, setup, wallet). Hides the page
   until the check resolves so locked content never flashes.
   ============================================================ */

(function () {
    // Hide content immediately; restore once confirmed unlocked.
    const root = document.documentElement;
    root.style.visibility = 'hidden';

    function reveal() { root.style.visibility = ''; }
    function redirect() { window.location.replace('unlock.html'); }

    document.addEventListener('DOMContentLoaded', async () => {
        try {
            const status = await FP.api.status();
            if (status && status.unlocked) {
                reveal();
            } else {
                redirect();
            }
        } catch (_) {
            redirect();
        }
    });
})();
