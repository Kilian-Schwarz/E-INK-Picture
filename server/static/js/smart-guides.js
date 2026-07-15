// Smart alignment guides for the designer canvas (E3.4)
// - While MOVING an object, thin guide lines appear when the edges or centers
//   of its axis-parallel bounding box come within ~6 screen px (6 / zoom in
//   design px) of another object's edges/centers or the canvas edges/center,
//   and the object snaps exactly onto the line. While RESIZING, the same
//   lines are shown as visual feedback but nothing snaps (deliberate
//   non-goal, spec E3.4 direction 6).
// - Grid coexistence (canvas-manager.js setupSnap stays untouched): fabric
//   fires handlers in registration order, and this script loads after
//   designer.js has run CanvasManager.init(), so this object:moving handler
//   runs AFTER the grid snap. The conflict rule evaluates the RAW pointer
//   position (pointer - transform.offset, fabric's own dragHandler formula):
//   within the threshold the guide wins and overwrites the grid result,
//   outside it whatever the grid handler left stands.
// - Rendering goes to fabric's contextTop overlay (upper canvas). No fabric
//   objects are created, so there is no object:added churn into the layers
//   panel or the live-preview debounce. Setting canvas.contextTopDirty =
//   true after drawing makes fabric's next renderAll clear the overlay
//   automatically (self-cleaning, fabric.js 5.3.1 renderAll).
// - Clear hooks: fabric mouse:up + object:modified for the normal drop, plus
//   document-level CAPTURE listeners for pointerup/pointercancel (the E3.2
//   swallow policy starves fabric of mouse:up when a pinch aborts a drag —
//   capture runs root-down, so these fire before any stopPropagation on
//   #canvas-area) and for a non-primary touch pointerdown (second finger =
//   pinch starts, lines vanish immediately). Observers only: no
//   preventDefault, no stopPropagation.
// - Session-only toggle (#guides-toggle-btn), default ON. Deliberately not
//   persisted (spec E3.4 direction 7).
const SmartGuides = {
    canvas: null,
    enabled: true,
    // Design coordinate of the winner line per axis (test observable).
    activeGuides: { x: null, y: null },
    // { x: [{pos,isCenter,isCanvas}], y: [...] } — built lazily once per
    // transformation, cleared by the clear hooks. No other object moves
    // while one is being dragged, so the cache never goes stale mid-drag.
    candidates: null,
    // { target, offX, offY, w, h } — AABB offset vs. left/top plus AABB
    // size; constant while dragging (angle/scale do not change on a move).
    moveGeom: null,
    SNAP_SCREEN_PX: 6,
    // Deliberately distinct from the selection blue #89b4fa so pixel probes
    // are unambiguous.
    GUIDE_COLOR: '#f38ba8',

    init() {
        if (typeof CanvasManager === 'undefined' || !CanvasManager.canvas) return;
        this.canvas = CanvasManager.canvas;

        this.canvas.on('object:moving', (e) => this.onMoving(e));
        this.canvas.on('object:scaling', (e) => this.onScaling(e));
        this.canvas.on('after:render', () => this.drawGuides());

        // Normal drop path (desktop and single-finger touch through fabric).
        this.canvas.on('mouse:up', () => this.clear());
        this.canvas.on('object:modified', () => this.clear());

        // E3.2 abort path: fabric's mouse:up never fires when the swallow
        // policy eats the gesture — observe below fabric on the document.
        document.addEventListener('pointerup', () => this.clear(), true);
        document.addEventListener('pointercancel', () => this.clear(), true);
        document.addEventListener('pointerdown', (e) => {
            if (e.pointerType === 'touch' && !e.isPrimary) this.clear();
        }, true);

        const btn = document.getElementById('guides-toggle-btn');
        if (btn) {
            btn.classList.toggle('active', this.enabled);
            btn.addEventListener('click', () => {
                this.enabled = !this.enabled;
                btn.classList.toggle('active', this.enabled);
                if (!this.enabled) this.clear();
            });
        }
    },

    // ---- snap + conflict rule (object:moving) -----------------------------

    onMoving(e) {
        if (!this.enabled) return;
        const target = e.target;
        if (!target || !e.transform || !e.pointer) return;

        this.ensureCandidates(target);
        if (!this.moveGeom || this.moveGeom.target !== target) {
            const bb = target.getBoundingRect(true, true);
            this.moveGeom = {
                target: target,
                offX: bb.left - target.left,
                offY: bb.top - target.top,
                w: bb.width,
                h: bb.height,
            };
        }

        // Raw (un-snapped) position from the pointer — fabric's dragHandler
        // formula. The grid handler ran before this one and may have moved
        // left/top; the conflict rule is evaluated on the RAW position.
        const rawLeft = e.pointer.x - e.transform.offsetX;
        const rawTop = e.pointer.y - e.transform.offsetY;
        const g = this.moveGeom;
        const threshold = this.SNAP_SCREEN_PX / CanvasManager.zoom;

        const bestX = this.matchAxis([
            { v: rawLeft + g.offX, isCenter: false },
            { v: rawLeft + g.offX + g.w / 2, isCenter: true },
            { v: rawLeft + g.offX + g.w, isCenter: false },
        ], this.candidates.x, threshold);
        const bestY = this.matchAxis([
            { v: rawTop + g.offY, isCenter: false },
            { v: rawTop + g.offY + g.h / 2, isCenter: true },
            { v: rawTop + g.offY + g.h, isCenter: false },
        ], this.candidates.y, threshold);

        if (bestX) {
            target.set('left', rawLeft + (bestX.pos - bestX.value));
            this.activeGuides.x = bestX.pos;
        } else {
            this.activeGuides.x = null; // grid result (or raw) stands
        }
        if (bestY) {
            target.set('top', rawTop + (bestY.pos - bestY.value));
            this.activeGuides.y = bestY.pos;
        } else {
            this.activeGuides.y = null;
        }
    },

    // ---- lines on resize, no snap (object:scaling) -------------------------

    onScaling(e) {
        if (!this.enabled) return;
        const target = e.target;
        if (!target) return;
        this.ensureCandidates(target);
        // The AABB changes size while scaling — read it live every event.
        // Display only: neither scale nor position is ever mutated here.
        const bb = target.getBoundingRect(true, true);
        const threshold = this.SNAP_SCREEN_PX / CanvasManager.zoom;
        const bestX = this.matchAxis([
            { v: bb.left, isCenter: false },
            { v: bb.left + bb.width / 2, isCenter: true },
            { v: bb.left + bb.width, isCenter: false },
        ], this.candidates.x, threshold);
        const bestY = this.matchAxis([
            { v: bb.top, isCenter: false },
            { v: bb.top + bb.height / 2, isCenter: true },
            { v: bb.top + bb.height, isCenter: false },
        ], this.candidates.y, threshold);
        this.activeGuides.x = bestX ? bestX.pos : null;
        this.activeGuides.y = bestY ? bestY.pos : null;
    },

    // ---- candidates ---------------------------------------------------------

    ensureCandidates(target) {
        if (this.candidates) return;
        const exclude = new Set();
        exclude.add(target);
        // An ActiveSelection moves as one box; its members stay in the
        // canvas object list and must not align against themselves.
        if (target.type === 'activeSelection' && typeof target.getObjects === 'function') {
            target.getObjects().forEach((o) => exclude.add(o));
        }
        const xs = [];
        const ys = [];
        this.canvas.getObjects().forEach((obj) => {
            if (exclude.has(obj) || obj.isGridLine || obj.visible === false) return;
            const bb = obj.getBoundingRect(true, true);
            xs.push({ pos: bb.left, isCenter: false, isCanvas: false });
            xs.push({ pos: bb.left + bb.width / 2, isCenter: true, isCanvas: false });
            xs.push({ pos: bb.left + bb.width, isCenter: false, isCanvas: false });
            ys.push({ pos: bb.top, isCenter: false, isCanvas: false });
            ys.push({ pos: bb.top + bb.height / 2, isCenter: true, isCanvas: false });
            ys.push({ pos: bb.top + bb.height, isCenter: false, isCanvas: false });
        });
        const W = CanvasManager.displayConfig.width;
        const H = CanvasManager.displayConfig.height;
        xs.push({ pos: 0, isCenter: false, isCanvas: true });
        xs.push({ pos: W / 2, isCenter: true, isCanvas: true });
        xs.push({ pos: W, isCenter: false, isCanvas: true });
        ys.push({ pos: 0, isCenter: false, isCanvas: true });
        ys.push({ pos: H / 2, isCenter: true, isCanvas: true });
        ys.push({ pos: H, isCenter: false, isCanvas: true });
        this.candidates = { x: xs, y: ys };
    },

    // Winner pair (moving value x candidate) with minimal distance within the
    // threshold, or null. Tie-break at exactly equal distance: center before
    // edge, canvas before object — exactly one winner line per axis.
    matchAxis(movingValues, candidates, threshold) {
        let best = null;
        for (const mv of movingValues) {
            for (const c of candidates) {
                const dist = Math.abs(mv.v - c.pos);
                if (dist > threshold) continue;
                const rank = this.tieRank(mv, c);
                if (best === null || dist < best.dist ||
                    (dist === best.dist && rank < best.rank)) {
                    best = { dist: dist, rank: rank, pos: c.pos, value: mv.v };
                }
            }
        }
        return best;
    },

    // Lower rank wins on distance ties: a center pairing beats an edge
    // pairing; among equals, canvas candidates beat object candidates.
    tieRank(mv, c) {
        return ((mv.isCenter || c.isCenter) ? 0 : 2) + (c.isCanvas ? 0 : 1);
    },

    // ---- rendering (contextTop overlay) -------------------------------------

    // after:render — the lower canvas just finished; draw the winner lines
    // on fabric's contextTop in screen px (design x zoom, the viewport
    // translation is always 0 in this frontend). contextTopDirty = true
    // makes the next renderAll clear the overlay: no ghost lines, no own
    // before:render cleanup.
    drawGuides() {
        if (this.activeGuides.x === null && this.activeGuides.y === null) return;
        const ctx = this.canvas.contextTop;
        if (!ctx) return;
        const zoom = CanvasManager.zoom;
        const w = this.canvas.width;
        const h = this.canvas.height;
        ctx.save();
        ctx.strokeStyle = this.GUIDE_COLOR;
        ctx.lineWidth = 1; // screen px, does not scale with zoom
        if (this.activeGuides.x !== null) {
            // Clamp into the canvas so edge guides (x=0 / x=W) stay visible;
            // +0.5 centers the 1px stroke on one crisp pixel column.
            const sx = Math.min(Math.max(Math.round(this.activeGuides.x * zoom), 0), w - 1) + 0.5;
            ctx.beginPath();
            ctx.moveTo(sx, 0);
            ctx.lineTo(sx, h);
            ctx.stroke();
        }
        if (this.activeGuides.y !== null) {
            const sy = Math.min(Math.max(Math.round(this.activeGuides.y * zoom), 0), h - 1) + 0.5;
            ctx.beginPath();
            ctx.moveTo(0, sy);
            ctx.lineTo(w, sy);
            ctx.stroke();
        }
        ctx.restore();
        this.canvas.contextTopDirty = true;
    },

    // ---- clear ---------------------------------------------------------------

    // Drop/abort: reset guides + per-drag caches and render once so fabric
    // wipes contextTop (the last draw left contextTopDirty = true). Cheap
    // no-op when idle — the document-level hooks fire for every pointerup
    // anywhere on the page.
    clear() {
        if (!this.candidates && !this.moveGeom &&
            this.activeGuides.x === null && this.activeGuides.y === null) return;
        this.activeGuides.x = null;
        this.activeGuides.y = null;
        this.candidates = null;
        this.moveGeom = null;
        this.canvas.requestRenderAll();
    },
};

document.addEventListener('DOMContentLoaded', () => SmartGuides.init());
