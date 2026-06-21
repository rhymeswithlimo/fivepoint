/* ============================================================
   Fivepoint — wallet icon library
   Loads and parses assets/wallet_icons.txt once, shared by the setup icon
   picker and the home wallet tiles. Exposed as FP.icons.
   ============================================================ */

window.FP = window.FP || {};

FP.icons = (function () {
    const ICONS_PATH = 'assets/wallet_icons.txt';

    // Fallback used when a wallet references an unknown icon name.
    const FALLBACK_SVG =
        '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" fill="#0e0e0e" viewBox="0 0 256 256"><path d="M240,102c0,70-103.79,126.66-108.21,129a8,8,0,0,1-7.58,0C119.79,228.66,16,172,16,102A62.07,62.07,0,0,1,78,40c20.65,0,38.73,8.88,50,23.89C139.27,48.88,157.35,40,178,40A62.07,62.07,0,0,1,240,102Z"></path></svg>';

    let cache = null; // sorted [{name, svg}]

    function parse(text) {
        return text
            .split(/\n\s*\n/)
            .map((block) => block.trim())
            .filter(Boolean)
            .map((block) => {
                const eq = block.indexOf('=');
                if (eq === -1) return null;
                const name = block.slice(0, eq).trim();
                const svg = block.slice(eq + 1).trim();
                return name && svg ? { name, svg } : null;
            })
            .filter(Boolean)
            .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
    }

    // all() returns the sorted icon list, fetching once and caching.
    async function all() {
        if (cache) return cache;
        const res = await fetch(ICONS_PATH);
        if (!res.ok) throw new Error(`wallet_icons.txt: ${res.status}`);
        cache = parse(await res.text());
        return cache;
    }

    // svgFor(name) returns the icon markup for a name, or the fallback.
    async function svgFor(name) {
        const list = await all();
        const key = (name || '').toLowerCase();
        const hit = list.find((i) => i.name.toLowerCase() === key);
        return hit ? hit.svg : FALLBACK_SVG;
    }

    return { all, svgFor, FALLBACK_SVG };
})();
