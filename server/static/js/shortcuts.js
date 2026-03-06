// Keyboard shortcuts
var Shortcuts = {
    init() {
        document.addEventListener('keydown', function(e) {
            // Don't trigger shortcuts when typing in inputs
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;

            if (e.ctrlKey && e.key === 'z') {
                e.preventDefault();
                HistoryManager.undo();
            }
            if (e.ctrlKey && e.key === 'y') {
                e.preventDefault();
                HistoryManager.redo();
            }
            if (e.key === 'Delete' || e.key === 'Backspace') {
                e.preventDefault();
                Toolbar.deleteSelected();
            }
            if (e.ctrlKey && e.key === 'd') {
                e.preventDefault();
                Toolbar.duplicateSelected();
            }
            if (e.ctrlKey && e.key === 's') {
                e.preventDefault();
                var saveBtn = document.getElementById('save-btn');
                if (saveBtn) saveBtn.click();
            }
        });
    }
};
