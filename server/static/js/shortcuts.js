// Keyboard shortcuts
var Shortcuts = {
    // True while the user is typing into a field, where the browser's own
    // editing shortcuts (native undo/redo) must win over ours.
    isEditingTarget(el) {
        if (!el) return false;
        var tag = el.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
        return el.isContentEditable === true;
    },

    init() {
        var self = this;
        document.addEventListener('keydown', function(e) {
            // Don't trigger shortcuts when typing in inputs
            if (self.isEditingTarget(e.target)) return;

            // Ctrl on Windows/Linux, Cmd on macOS
            var mod = e.ctrlKey || e.metaKey;
            // e.key is uppercase while Shift is held
            var key = typeof e.key === 'string' ? e.key.toLowerCase() : '';

            if (mod && key === 'z') {
                e.preventDefault();
                // Shift+Z is the macOS redo, but accept it everywhere
                if (e.shiftKey) {
                    HistoryManager.redo();
                } else {
                    HistoryManager.undo();
                }
            }
            if (mod && key === 'y') {
                e.preventDefault();
                HistoryManager.redo();
            }
            if (e.key === 'Delete' || e.key === 'Backspace') {
                e.preventDefault();
                Toolbar.deleteSelected();
            }
            if (mod && key === 'd') {
                e.preventDefault();
                Toolbar.duplicateSelected();
            }
            if (mod && key === 's') {
                e.preventDefault();
                var saveBtn = document.getElementById('save-btn');
                if (saveBtn) saveBtn.click();
            }
        });
    }
};
