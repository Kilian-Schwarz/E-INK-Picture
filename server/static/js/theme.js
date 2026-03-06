// Theme management - Dark/Light mode toggle
// Saves preference to localStorage
const ThemeManager = {
    init() {
        const saved = localStorage.getItem('eink-theme') || 'dark';
        document.documentElement.setAttribute('data-theme', saved);
    },

    toggle() {
        const current = document.documentElement.getAttribute('data-theme');
        const next = current === 'dark' ? 'light' : 'dark';
        document.documentElement.setAttribute('data-theme', next);
        localStorage.setItem('eink-theme', next);
    }
};
