/* ============================================================
   Fivepoint — shared text-scramble engine
   Used by the home page (balance/amount encryption toggle) and the
   wallet page (balance reveal-on-load). Exposed as FP.scramble.
   ============================================================ */

window.FP = window.FP || {};

FP.scramble = (function () {
    // Characters that are never scrambled, so the shape of a figure stays readable.
    const STATIC_CHARS = new Set(['$', ',', '.', ' ']);

    /** Build a random-character picker from a pool string. */
    function poolPicker(pool) {
        return () => pool[Math.floor(Math.random() * pool.length)];
    }

    /** Picker weighted toward symbols (used by the home page). */
    function weightedPicker(symbols, digits, symbolChance) {
        return () =>
            Math.random() < symbolChance
                ? symbols[Math.floor(Math.random() * symbols.length)]
                : digits[Math.floor(Math.random() * digits.length)];
    }

    /** Replace every non-static character with a random one. */
    function scrambleString(text, pick) {
        let out = '';
        for (const ch of text) out += STATIC_CHARS.has(ch) ? ch : pick();
        return out;
    }

    /** Remember an element's real text once, so animations can restore it. */
    function rememberOriginal(el) {
        if (el.dataset.original === undefined) el.dataset.original = el.textContent;
        return el.dataset.original;
    }

    /** Instantly scramble an element with no animation (initial concealed state). */
    function scrambleNow(el, pick) {
        const original = rememberOriginal(el);
        el.textContent = scrambleString(original, pick);
    }

    /**
     * Animate an element between its real text and a scrambled state.
     * encrypting=false reveals the real text; encrypting=true conceals it.
     */
    function animate(el, { pick, encrypting = false, duration = 1000, steps = 20 } = {}) {
        const finalText = rememberOriginal(el);
        const interval = duration / steps;
        let frame = 0;

        const tick = setInterval(() => {
            frame++;
            const revealedUpTo = Math.floor((frame / steps) * finalText.length);
            let display = '';

            for (let i = 0; i < finalText.length; i++) {
                const ch = finalText[i];
                if (STATIC_CHARS.has(ch)) {
                    display += ch;
                } else if (encrypting) {
                    display += i < revealedUpTo ? pick() : ch;
                } else {
                    display += i < revealedUpTo ? ch : pick();
                }
            }

            el.textContent = display;

            if (frame >= steps) {
                clearInterval(tick);
                el.textContent = encrypting ? scrambleString(finalText, pick) : finalText;
            }
        }, interval);
    }

    return { STATIC_CHARS, poolPicker, weightedPicker, scrambleString, scrambleNow, animate };
})();
