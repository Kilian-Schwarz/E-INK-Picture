// Responsive designer layout (E3.3)
// Modes: mobile (<768px) = bottom sheets + tab bar, tablet (768-1023.98px) =
// collapsible icon rails with flyout overlays, desktop (>=1024px) = untouched.
// All new interactions are plain click handlers (tap-compatible, E3.1
// pointer-events discipline: no mouse* handlers). Sheet swipe/drag is E3.2.
const ResponsiveLayout = {
    // Topbar controls moved into the burger menu, in DOM order per parent
    // (restore iterates in reverse so nextSibling chains resolve correctly).
    MENU_SELECTORS_MOBILE: [
        '#new-design-btn', '#clone-design-btn', '#delete-design-btn',
        '#undo-btn', '#redo-btn', '.zoom-controls',
        '#refresh-now-btn', '#preview-btn', '#history-btn',
        '#logout-btn', '#activate-btn', '#settings-btn', '#theme-toggle',
    ],
    // Tablet keeps undo/redo/zoom in the topbar
    MENU_SELECTORS_TABLET: [
        '#new-design-btn', '#clone-design-btn', '#delete-design-btn',
        '#refresh-now-btn', '#preview-btn', '#history-btn',
        '#logout-btn', '#activate-btn', '#settings-btn', '#theme-toggle',
    ],

    mqMobile: null,
    mqTablet: null,
    mode: null,
    homes: [],
    resizeTimer: null,

    init() {
        this.mqMobile = window.matchMedia('(max-width: 767.98px)');
        this.mqTablet = window.matchMedia('(min-width: 768px) and (max-width: 1023.98px)');

        this.captureHomes();
        this.bindTabbar();
        this.bindSheetHandles();
        this.bindPanelToggles();
        this.bindTopbarMenu();
        this.bindKeyboard();
        this.bindSelectionIndicator();

        // Mode switches are driven by matchMedia (fires during resize too)
        this.mqMobile.addEventListener('change', () => this.applyMode());
        this.mqTablet.addEventListener('change', () => this.applyMode());
        // Re-fit within a mode (URL bar shows/hides, window resize, rotation)
        window.addEventListener('resize', () => this.scheduleFit());
        window.addEventListener('orientationchange', () => this.scheduleFit());

        this.applyMode();
    },

    currentMode() {
        if (this.mqMobile.matches) return 'mobile';
        if (this.mqTablet.matches) return 'tablet';
        return 'desktop';
    },

    // Capture each element's original topbar position exactly once,
    // before any reparenting ever happens.
    captureHomes() {
        this.homes = [];
        this.MENU_SELECTORS_MOBILE.forEach((sel) => {
            const el = document.querySelector(sel);
            if (el) {
                this.homes.push({ el, parent: el.parentElement, next: el.nextElementSibling });
            }
        });
    },

    // Restore in reverse capture order: each element's original next sibling
    // is guaranteed to be back in place before it is needed as an anchor.
    restoreHomes() {
        for (let i = this.homes.length - 1; i >= 0; i--) {
            const h = this.homes[i];
            if (h.el.parentElement === h.parent && h.el.nextElementSibling === h.next) continue;
            const anchor = (h.next && h.next.parentElement === h.parent) ? h.next : null;
            h.parent.insertBefore(h.el, anchor);
        }
    },

    moveToMenu(selectors) {
        const menu = document.getElementById('topbar-menu');
        if (!menu) return;
        // Reset membership first so tablet<->mobile switches stay consistent
        this.restoreHomes();
        selectors.forEach((sel) => {
            const el = document.querySelector(sel);
            if (el) menu.appendChild(el);
        });
    },

    applyMode() {
        const mode = this.currentMode();
        // Reset all responsive UI state on every mode (re)application
        document.body.dataset.sheet = '';
        document.body.dataset.panelLeft = '';
        document.body.dataset.panelRight = '';
        document.body.classList.remove('topbar-menu-open');

        if (mode === 'mobile') {
            this.moveToMenu(this.MENU_SELECTORS_MOBILE);
        } else if (mode === 'tablet') {
            this.moveToMenu(this.MENU_SELECTORS_TABLET);
        } else {
            this.restoreHomes();
        }
        this.mode = mode;

        if (mode === 'desktop') {
            // Never touch the zoom on desktop entry — only restore geometry.
            CanvasManager.centerCanvas();
            const canvas = CanvasManager.getCanvas();
            if (canvas) canvas.calcOffset();
        } else {
            CanvasManager.fitToViewport();
        }
    },

    scheduleFit() {
        clearTimeout(this.resizeTimer);
        this.resizeTimer = setTimeout(() => {
            CanvasManager.fitToViewport();
        }, 150);
    },

    toggleSheet(name) {
        document.body.dataset.sheet = document.body.dataset.sheet === name ? '' : name;
    },

    closeSheet() {
        document.body.dataset.sheet = '';
    },

    bindTabbar() {
        const bar = document.getElementById('mobile-tabbar');
        if (!bar) return;
        bar.addEventListener('click', (e) => {
            const btn = e.target.closest('button[data-sheet]');
            if (btn) this.toggleSheet(btn.dataset.sheet);
        });
    },

    bindSheetHandles() {
        document.querySelectorAll('.sheet-handle').forEach((handle) => {
            handle.addEventListener('click', () => this.closeSheet());
        });
    },

    bindPanelToggles() {
        document.querySelectorAll('.panel-toggle').forEach((toggle) => {
            const key = toggle.closest('.panel-left') ? 'panelLeft' : 'panelRight';
            toggle.addEventListener('click', () => {
                document.body.dataset[key] = document.body.dataset[key] === 'open' ? '' : 'open';
            });
        });
    },

    bindTopbarMenu() {
        const btn = document.getElementById('topbar-menu-btn');
        const menu = document.getElementById('topbar-menu');
        if (!btn || !menu) return;
        btn.addEventListener('click', () => {
            document.body.classList.toggle('topbar-menu-open');
        });
        // Tapping a menu entry closes the menu. Buttons nested in
        // .zoom-controls (+/-/reset) act in place and keep it open.
        menu.addEventListener('click', (e) => {
            const item = e.target.closest('button');
            if (item && item.parentElement === menu) {
                document.body.classList.remove('topbar-menu-open');
            }
        });
    },

    bindKeyboard() {
        document.addEventListener('keydown', (e) => {
            if (e.key !== 'Escape') return;
            const t = e.target;
            if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT')) return;
            if (document.body.classList.contains('topbar-menu-open')) {
                document.body.classList.remove('topbar-menu-open');
            } else if (document.body.dataset.sheet) {
                this.closeSheet();
            }
        });
    },

    // Selection does NOT auto-open the properties sheet (the canvas would be
    // half covered on every tap) — the tab shows a badge instead.
    bindSelectionIndicator() {
        const canvas = CanvasManager.getCanvas();
        const tab = document.querySelector('#mobile-tabbar button[data-sheet="properties"]');
        if (!canvas || !tab) return;
        const mark = () => tab.classList.add('has-selection');
        const clear = () => tab.classList.remove('has-selection');
        canvas.on('selection:created', mark);
        canvas.on('selection:updated', mark);
        canvas.on('selection:cleared', clear);
    },
};

// Registered after designer.js: its DOMContentLoaded handler runs first and
// initializes CanvasManager synchronously before its first await.
document.addEventListener('DOMContentLoaded', () => {
    ResponsiveLayout.init();
});
