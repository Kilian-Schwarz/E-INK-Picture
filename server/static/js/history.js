// Undo/Redo history management
// Saves canvas states as JSON, max 50 entries
const HistoryManager = {
    states: [],
    index: -1,
    maxStates: 50,
    isRestoring: false,

    saveState() {
        if (this.isRestoring) return;
        const canvas = CanvasManager.getCanvas();
        if (!canvas) return;

        const json = JSON.stringify(canvas.toJSON([
            'elementId', 'elementType', 'elementData',
            'lockMovementX', 'lockMovementY',
            'lockScalingX', 'lockScalingY',
            'lockRotation', 'hasControls', 'selectable'
        ]));

        // Remove any states after current index (discard redo history)
        if (this.index < this.states.length - 1) {
            this.states = this.states.slice(0, this.index + 1);
        }

        this.states.push(json);

        // Limit history size
        if (this.states.length > this.maxStates) {
            this.states.shift();
        } else {
            this.index++;
        }

        this.updateButtons();
    },

    undo() {
        if (!this.canUndo()) return;
        this.index--;
        this.restoreState();
    },

    redo() {
        if (!this.canRedo()) return;
        this.index++;
        this.restoreState();
    },

    restoreState() {
        const canvas = CanvasManager.getCanvas();
        if (!canvas || !this.states[this.index]) return;

        this.isRestoring = true;
        canvas.loadFromJSON(this.states[this.index], () => {
            canvas.renderAll();
            this.isRestoring = false;
            this.updateButtons();
            LayersPanel.refresh();
            PropertiesPanel.hide();
        });
    },

    canUndo() {
        return this.index > 0;
    },

    canRedo() {
        return this.index < this.states.length - 1;
    },

    updateButtons() {
        const undoBtn = document.getElementById('undo-btn');
        const redoBtn = document.getElementById('redo-btn');
        if (undoBtn) undoBtn.disabled = !this.canUndo();
        if (redoBtn) redoBtn.disabled = !this.canRedo();
    },

    clear() {
        this.states = [];
        this.index = -1;
        this.updateButtons();
    }
};
