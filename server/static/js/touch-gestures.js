// Touch gestures for the designer canvas (E3.2)
// - Pinch-zoom + two-finger pan as ONE gesture: the finger midpoint pans via
//   #canvas-area scrollLeft/scrollTop (this frontend never translates the
//   viewportTransform), the finger distance zooms around the gesture midpoint
//   (CanvasManager.setZoom clamps to 0.25-4 and updates #zoom-level live).
// - Long-press (500 ms, 8 px tolerance) on a canvas object opens
//   #canvas-context-menu; actions map 1:1 onto existing Toolbar functions.
// - Single-finger input stays 100% fabric (E3.1 behavior). Swallow policy:
//   no event is touched unless a gesture or long-press menu state is active.
//
// Listeners sit on #canvas-area in the CAPTURE phase: fabric binds pointerdown
// on the upper-canvas and swaps pointermove/pointerup onto document in the
// BUBBLE phase (fabric.js 5.3.1:13268-13320), so stopPropagation() here starves
// fabric of all touch events during a gesture. Swallowed pointerups are
// harmless: addEventListener dedupes identical handler references, the next
// pointerdown re-binds idempotently. No fabric patching, no gesture addon,
// pointer events only (E3.1 conventions).
const TouchGestures = {
    area: null,
    menu: null,
    pointers: new Map(), // pointerId -> {x, y}; touch pointers only
    // Swallow policy: active from the 2nd finger down (or long-press fire)
    // until ALL touch pointers are up; every touch pointer event gets
    // stopPropagation() while active.
    swallow: false,
    pinch: null, // { idA, idB, startDist, startZoom, anchorX, anchorY, lastZoom, lastMidX, lastMidY }
    rafId: null,
    longPressTimer: null,
    longPress: null, // { pointerId, startX, startY, target }
    menuOpenedAt: -Infinity, // performance.now() stamp for the 400 ms click-through guard

    LONG_PRESS_MS: 500,
    LONG_PRESS_TOLERANCE_PX: 8,
    MENU_GUARD_MS: 400,

    init() {
        this.area = document.getElementById('canvas-area');
        this.menu = document.getElementById('canvas-context-menu');
        if (!this.area || !this.menu) return;

        // touch-action via JS, not CSS (existing CSS is regression-protected):
        // fabric sets touch-action:none only on the canvas elements; fingers
        // landing on the wrapper padding would otherwise start native
        // scrolling / browser pinch and fire pointercancel into the gesture.
        // Rendering-neutral inline property.
        this.area.style.touchAction = 'none';

        // .canvas-area is display:flex with justify-content:center
        // (designer.css); once the wrapper grows wider than the area, flex
        // centering pushes the left overhang OUTSIDE the scrollable range
        // (unreachable content, and the pinch anchor drifts by half the
        // overflow). Neutralize via JS (CSS files are regression-protected).
        // Rendering-neutral while the wrapper fits the area, because
        // centerCanvas() sets the wrapper padding to exactly fill it; in the
        // overflow state the whole canvas becomes scroll-reachable (also
        // fixes a pre-existing desktop defect at high zoom). The vertical
        // axis uses align-items:flex-start and never had the problem.
        this.area.style.justifyContent = 'flex-start';

        this.buildMenu();

        const opts = { capture: true };
        this.area.addEventListener('pointerdown', (e) => this.onPointerDown(e), opts);
        this.area.addEventListener('pointermove', (e) => this.onPointerMove(e), opts);
        this.area.addEventListener('pointerup', (e) => this.onPointerUp(e), opts);
        this.area.addEventListener('pointercancel', (e) => this.onPointerUp(e), opts);

        // Suppress the native context menu ONLY while touch pointers are
        // active or right after a long-press menu opened (Android/Chromium
        // synthesizes contextmenu on touch long-press). Desktop right-click
        // keeps the browser default menu.
        this.area.addEventListener('contextmenu', (e) => {
            if (this.pointers.size > 0 ||
                performance.now() - this.menuOpenedAt <= this.MENU_GUARD_MS) {
                e.preventDefault();
            }
        }, opts);

        // Outside tap/click closes the menu. Ignore events within 400 ms of
        // opening: the browser synthesizes a click from the long-press
        // pointerup, which must not close the fresh menu.
        document.addEventListener('click', (e) => {
            if (!this.isMenuOpen()) return;
            if (performance.now() - this.menuOpenedAt < this.MENU_GUARD_MS) return;
            if (e.target.closest && e.target.closest('#canvas-context-menu')) return;
            this.hideMenu();
        });

        // Runs after ResponsiveLayout's Escape handler (script order):
        // menu + open sheet closing on one Escape is an accepted edge case.
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isMenuOpen()) this.hideMenu();
        });
    },

    // ---- pointer handling ------------------------------------------------

    onPointerDown(e) {
        if (e.pointerType !== 'touch') return; // mouse/pen go straight to fabric
        this.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });

        if (this.pointers.size === 1 && !this.swallow) {
            this.armLongPress(e);
        } else if (this.pointers.size === 2) {
            // Second finger down: the two-finger gesture takes over. Abort a
            // running fabric transform BEFORE the first gesture frame.
            this.cancelLongPress();
            this.hideMenu(); // a new gesture closes the menu
            this.abortFabricInteraction();
            this.swallow = true;
            this.startPinch();
        }
        if (this.swallow) e.stopPropagation();
    },

    onPointerMove(e) {
        if (e.pointerType !== 'touch') return;
        const p = this.pointers.get(e.pointerId);
        if (!p) return;
        p.x = e.clientX;
        p.y = e.clientY;

        if (this.longPress && e.pointerId === this.longPress.pointerId) {
            const dx = e.clientX - this.longPress.startX;
            const dy = e.clientY - this.longPress.startY;
            if (Math.hypot(dx, dy) > this.LONG_PRESS_TOLERANCE_PX) {
                this.cancelLongPress(); // tolerance exceeded -> normal fabric drag
            }
        }
        if (this.pinch) this.scheduleFrame();
        if (this.swallow) e.stopPropagation();
    },

    // Also bound to pointercancel (treated exactly like pointerup).
    onPointerUp(e) {
        if (e.pointerType !== 'touch') return;
        if (!this.pointers.has(e.pointerId)) return;
        // The final up still belongs to the gesture and is swallowed too.
        if (this.swallow) e.stopPropagation();

        if (this.pinch && (e.pointerId === this.pinch.idA || e.pointerId === this.pinch.idB)) {
            // Apply the final coalesced frame before the finger's position is
            // dropped (a pending rAF would no-op once the pinch is cleared).
            this.applyPinchFrame();
            this.pinch = null;
        }
        this.pointers.delete(e.pointerId);
        if (this.longPress && e.pointerId === this.longPress.pointerId) {
            this.cancelLongPress();
        }
        if (!this.pinch && this.pointers.size >= 2) {
            this.startPinch(); // e.g. 3 fingers -> 2: re-baseline
        }

        if (this.pointers.size === 0 && this.swallow) {
            this.swallow = false;
            if (this.rafId !== null) {
                cancelAnimationFrame(this.rafId);
                this.rafId = null;
            }
            // Belt and braces (as in E3.3): re-sync fabric's cached element
            // offsets after the gesture changed scroll/zoom.
            const canvas = CanvasManager.getCanvas();
            if (canvas) canvas.calcOffset();
        }
    },

    // ---- long-press ------------------------------------------------------

    armLongPress(e) {
        const canvas = CanvasManager.getCanvas();
        if (!canvas) return;
        // Grid lines are evented:false and never become a target; empty
        // canvas area never arms the long-press.
        let target = null;
        try {
            target = canvas.findTarget(e, true);
        } catch (err) {
            return; // defensive: findTarget on a degenerate event
        }
        if (!target) return;
        this.longPress = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startY: e.clientY,
            target: target,
        };
        this.longPressTimer = setTimeout(() => this.fireLongPress(), this.LONG_PRESS_MS);
    },

    cancelLongPress() {
        if (this.longPressTimer !== null) {
            clearTimeout(this.longPressTimer);
            this.longPressTimer = null;
        }
        this.longPress = null;
    },

    fireLongPress() {
        this.longPressTimer = null;
        const lp = this.longPress;
        this.longPress = null;
        if (!lp || !this.pointers.has(lp.pointerId)) return;
        const p = this.pointers.get(lp.pointerId);

        // fabric already set up a drag transform on the pointerdown — abort
        // and restore, then select the pressed object and show the menu.
        this.abortFabricInteraction();
        const canvas = CanvasManager.getCanvas();
        canvas.setActiveObject(lp.target);
        canvas.requestRenderAll();
        this.swallow = true; // the following pointerup never reaches fabric
        this.showMenu(p.x, p.y);
    },

    // ---- pinch-zoom + two-finger pan (one gesture) -------------------------

    startPinch() {
        const canvas = CanvasManager.getCanvas();
        if (!canvas) return;
        const ids = Array.from(this.pointers.keys());
        const idA = ids[0];
        const idB = ids[1];
        const a = this.pointers.get(idA);
        const b = this.pointers.get(idB);

        // Hold both move streams even if a finger leaves #canvas-area (E3.1
        // convention: setPointerCapture in try/catch — synthetic events throw).
        try { this.area.setPointerCapture(idA); } catch (err) { /* synthetic event */ }
        try { this.area.setPointerCapture(idB); } catch (err) { /* synthetic event */ }

        const midX = (a.x + b.x) / 2;
        const midY = (a.y + b.y) / 2;
        // Anchor = design point under the gesture midpoint. wrapperEl is
        // fabric's .canvas-container around the canvas elements (fabric.js
        // 5.3.1, public field); the viewportTransform translation is always
        // 0 in this frontend, so client offset / zoom = design coords.
        const box = canvas.wrapperEl.getBoundingClientRect();
        const z0 = CanvasManager.zoom;
        this.pinch = {
            idA: idA,
            idB: idB,
            startDist: Math.max(Math.hypot(b.x - a.x, b.y - a.y), 1),
            startZoom: z0,
            anchorX: (midX - box.left) / z0,
            anchorY: (midY - box.top) / z0,
            lastZoom: z0,
            lastMidX: midX,
            lastMidY: midY,
        };
    },

    // rAF coalescing: pointermove only updates the pointer map; at most one
    // setZoom + scroll application per animation frame (setDimensions per
    // move event is too expensive on low-end phones).
    scheduleFrame() {
        if (this.rafId !== null) return;
        this.rafId = requestAnimationFrame(() => {
            this.rafId = null;
            this.applyPinchFrame();
        });
    },

    applyPinchFrame() {
        const pinch = this.pinch;
        if (!pinch) return;
        const a = this.pointers.get(pinch.idA);
        const b = this.pointers.get(pinch.idB);
        if (!a || !b) return;

        const dist = Math.max(Math.hypot(b.x - a.x, b.y - a.y), 1);
        const midX = (a.x + b.x) / 2;
        const midY = (a.y + b.y) / 2;
        // Compare against the CLAMPED zoom so panning keeps working while the
        // zoom sits at a clamp boundary (0.25 / 4).
        const nextZoom = Math.min(Math.max(pinch.startZoom * (dist / pinch.startDist), 0.25), 4);

        // Threshold skip: < 0.5 % zoom delta and < 1 px midpoint delta.
        if (Math.abs(nextZoom - pinch.lastZoom) < pinch.lastZoom * 0.005 &&
            Math.abs(midX - pinch.lastMidX) < 1 &&
            Math.abs(midY - pinch.lastMidY) < 1) {
            return;
        }

        CanvasManager.setZoom(nextZoom); // clamps, updates #zoom-level, centerCanvas()
        const z = CanvasManager.zoom;

        // Keep the design anchor under the current finger midpoint: re-read
        // the anchor's live screen position from a FRESH wrapperEl BCR (after
        // setZoom/centerCanvas re-layout; captures padding, flex layout and
        // any future centering mechanics) and scroll by the residual —
        // delta-based and self-correcting per frame. The browser clamps
        // scroll values to the valid range automatically; if the canvas is
        // smaller than the viewport (scroll range 0) the anchor is not
        // exactly holdable and panning is a no-op — accepted, nothing is
        // covered.
        const canvas = CanvasManager.getCanvas();
        const box = canvas.wrapperEl.getBoundingClientRect();
        this.area.scrollLeft += box.left + pinch.anchorX * z - midX;
        this.area.scrollTop += box.top + pinch.anchorY * z - midY;

        pinch.lastZoom = z;
        pinch.lastMidX = midX;
        pinch.lastMidY = midY;
    },

    // ---- fabric abort helper ----------------------------------------------

    // The ONLY place touching private fabric fields. Abort a running fabric
    // transformation and restore the pre-transform state:
    // _setupCurrentTransform stores a saveObjectTransform snapshot in
    // transform.original (fabric.js 5.3.1:12301; snapshot fields 1530-1542;
    // originX/originY 12309-12310). object:modified does NOT fire (that would
    // require _finalizeCurrentTransform in __onMouseUp, which the swallow
    // policy starves) — history stays clean. _groupSelector (13634-13641)
    // aborts a drag-to-select rectangle. No discardActiveObject(): the
    // selection survives, only the transformation is discarded.
    abortFabricInteraction() {
        const canvas = CanvasManager.getCanvas();
        if (!canvas) return;
        const t = canvas._currentTransform;
        if (t && t.target) {
            t.target.set(t.original);   // saveObjectTransform snapshot, fabric.js:12301/1530
            t.target.setCoords();
        }
        canvas._currentTransform = null;
        canvas._groupSelector = null;   // abort a drag-to-select rectangle
        canvas.requestRenderAll();
        if (typeof PropertiesPanel !== 'undefined') PropertiesPanel.updateFromCanvas();
    },

    // ---- context menu -------------------------------------------------------

    buildMenu() {
        // Static content, built once; reuses the existing .context-menu look
        // (media-dashboard.css). Actions map 1:1 onto existing Toolbar
        // functions — history and layers refresh included.
        this.menu.innerHTML =
            '<button class="context-menu-item" data-action="duplicate">Duplicate</button>' +
            '<button class="context-menu-item" data-action="front">Bring to Front</button>' +
            '<button class="context-menu-item" data-action="back">Send to Back</button>' +
            '<div class="context-menu-divider"></div>' +
            '<button class="context-menu-item danger" data-action="delete">Delete</button>';

        this.menu.addEventListener('click', (e) => {
            const item = e.target.closest('.context-menu-item');
            if (!item) return;
            this.runAction(item.dataset.action);
            this.hideMenu();
        });
    },

    runAction(action) {
        switch (action) {
            case 'duplicate': Toolbar.duplicateSelected(); break;
            case 'front': Toolbar.moveLayer('front'); break;
            case 'back': Toolbar.moveLayer('back'); break;
            case 'delete': Toolbar.deleteSelected(); break;
        }
    },

    isMenuOpen() {
        return this.menu.classList.contains('visible');
    },

    showMenu(x, y) {
        // Measure after making it visible, then clamp fully into the viewport.
        this.menu.classList.add('visible');
        const w = this.menu.offsetWidth;
        const h = this.menu.offsetHeight;
        this.menu.style.left = Math.min(x, window.innerWidth - w - 8) + 'px';
        this.menu.style.top = Math.min(y, window.innerHeight - h - 8) + 'px';
        this.menuOpenedAt = performance.now();
    },

    hideMenu() {
        this.menu.classList.remove('visible');
    },
};

document.addEventListener('DOMContentLoaded', () => TouchGestures.init());
