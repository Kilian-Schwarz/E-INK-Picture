// Condition editor UI
var ConditionsPanel = {
    render(container, conditions) {
        if (!container) return;
        conditions = conditions || [];
        container.innerHTML = '';

        if (conditions.length === 0) {
            container.innerHTML = '<p class="text-muted">No conditions set.</p>';
            return;
        }

        conditions.forEach(function(cond, idx) {
            var item = document.createElement('div');
            item.className = 'condition-item';
            item.innerHTML = '' +
                '<span>' + (cond.field || '') + ' ' + (cond.operator || '') + ' ' + (cond.value || '') + '</span>' +
                '<button class="condition-remove" data-idx="' + idx + '">X</button>';
            container.appendChild(item);
        });
    }
};
