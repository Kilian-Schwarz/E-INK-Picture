        let selectedElement = null;
        let currentDesign = null;
        let currentDisplay = null;
        let isDraggingElement = false;
        let dragOffsetX, dragOffsetY;
        let isResizing = false;
        let currentHandle = null;
        let mediaModalOpen = false;
        let imageSelectCallback = null;
        let selectedMedia = null;

        // Keep track of loaded fonts to avoid duplicating @font-face
        let loadedFonts = new Set();

        // Crop variables
        let cropModal = document.getElementById('crop-modal');
        let cropClose = document.getElementById('crop-close');
        let cropApply = document.getElementById('crop-apply');
        let cropCancel = document.getElementById('crop-cancel');
        let cropReset = document.getElementById('crop-reset');
        let cropImage = document.getElementById('crop-image');
        let cropRect = document.getElementById('crop-rect');
        let cropTargetElement = null;
        let cropStartX, cropStartY;
        let isCropDragging = false;
        let isCropResizing = false;
        let cropCurrentHandle = null;

        let cropRectX=0,cropRectY=0,cropRectW=50,cropRectH=50;
        let originalImageWidth=0, originalImageHeight=0;

        function openCropModal(el){
            cropTargetElement = el;
            if(el.dataset.image){
                cropImage.src='/image/'+el.dataset.image;
                cropImage.onload = ()=>{
                    originalImageWidth = cropImage.naturalWidth;
                    originalImageHeight = cropImage.naturalHeight;
                    cropRectW= parseInt(el.dataset.crop_w)||50;
                    cropRectH= parseInt(el.dataset.crop_h)||50;
                    cropRectX= parseInt(el.dataset.crop_x)||0;
                    cropRectY= parseInt(el.dataset.crop_y)||0;

                    let displayedW = cropImage.clientWidth;
                    let displayedH = cropImage.clientHeight;

                    let scaleX = displayedW / originalImageWidth;
                    let scaleY = displayedH / originalImageHeight;

                    cropRect.style.width=cropRectW*scaleX+'px';
                    cropRect.style.height=cropRectH*scaleY+'px';
                    cropRect.style.left=cropRectX*scaleX+'px';
                    cropRect.style.top=cropRectY*scaleY+'px';

                    cropModal.style.display='flex';
                };
            }
        }

        cropClose.addEventListener('click',()=>{
            cropModal.style.display='none';
        });
        cropCancel.addEventListener('click',()=>{
            cropModal.style.display='none';
        });

        cropRect.addEventListener('mousedown',(e)=>{
            if(e.target.classList.contains('crop-resize-handle')){
                isCropResizing=true;
                cropCurrentHandle=e.target;
                startWidth=cropRect.clientWidth;
                startHeight=cropRect.clientHeight;
                resizeStartMouseX=e.clientX;
                resizeStartMouseY=e.clientY;
            } else {
                isCropDragging=true;
                cropStartX=e.clientX - cropRect.offsetLeft;
                cropStartY=e.clientY - cropRect.offsetTop;
            }
            e.preventDefault();
        });

        document.addEventListener('mousemove',(e)=>{
            if(isCropDragging){
                let nx = e.clientX - cropStartX;
                let ny = e.clientY - cropStartY;

                if(nx<0) nx=0;
                if(ny<0) ny=0;
                if(nx+cropRect.clientWidth>cropImage.clientWidth) nx=cropImage.clientWidth - cropRect.clientWidth;
                if(ny+cropRect.clientHeight>cropImage.clientHeight) ny=cropImage.clientHeight - cropRect.clientHeight;
                cropRect.style.left=nx+'px';
                cropRect.style.top=ny+'px';
            } else if(isCropResizing){
                const dx = e.clientX - resizeStartMouseX;
                const dy = e.clientY - resizeStartMouseY;
                let newW=startWidth;
                let newH=startHeight;
                let newX=cropRect.offsetLeft;
                let newY=cropRect.offsetTop;

                if(cropCurrentHandle.classList.contains('bottom-right')){
                    newW = startWidth+dx;
                    newH = startHeight+dy;
                } else if(cropCurrentHandle.classList.contains('bottom-left')){
                    newW = startWidth - dx;
                    newH = startHeight + dy;
                    newX = cropRect.offsetLeft + dx;
                } else if(cropCurrentHandle.classList.contains('top-right')){
                    newW = startWidth + dx;
                    newH = startHeight - dy;
                    newY = cropRect.offsetTop + dy;
                } else if(cropCurrentHandle.classList.contains('top-left')){
                    newW = startWidth - dx;
                    newH = startHeight - dy;
                    newX = cropRect.offsetLeft + dx;
                    newY = cropRect.offsetTop + dy;
                }

                if(newW<5) newW=5;
                if(newH<5) newH=5;
                if(newX<0) { newW += newX; newX=0; }
                if(newY<0) { newH += newY; newY=0; }
                if(newX+newW>cropImage.clientWidth) newW=cropImage.clientWidth - newX;
                if(newY+newH>cropImage.clientHeight) newH=cropImage.clientHeight - newY;

                cropRect.style.width=newW+'px';
                cropRect.style.height=newH+'px';
                cropRect.style.left=newX+'px';
                cropRect.style.top=newY+'px';
            }
        });
        document.addEventListener('mouseup',()=>{
            isCropDragging=false;
            isCropResizing=false;
            cropCurrentHandle=null;
        });

        document.getElementById('crop-apply').addEventListener('click',()=>{
            let displayedW = cropImage.clientWidth;
            let displayedH = cropImage.clientHeight;

            let scaleX = originalImageWidth / displayedW;
            let scaleY = originalImageHeight / displayedH;

            let rx = Math.round(parseInt(cropRect.style.left)*scaleX);
            let ry = Math.round(parseInt(cropRect.style.top)*scaleY);
            let rw = Math.round(parseInt(cropRect.style.width)*scaleX);
            let rh = Math.round(parseInt(cropRect.style.height)*scaleY);

            cropTargetElement.dataset.crop_x = rx;
            cropTargetElement.dataset.crop_y = ry;
            cropTargetElement.dataset.crop_w = rw;
            cropTargetElement.dataset.crop_h = rh;

            const imgEl = cropTargetElement.querySelector('img');
            if(imgEl){
                imgEl.style.objectFit='none';
                imgEl.style.objectPosition=(-rx)+'px '+(-ry)+'px';
                imgEl.style.width = rw+'px';
                imgEl.style.height = rh+'px';
            }

            cropModal.style.display='none';
        });

        document.getElementById('crop-reset').addEventListener('click', ()=>{
            delete cropTargetElement.dataset.crop_x;
            delete cropTargetElement.dataset.crop_y;
            delete cropTargetElement.dataset.crop_w;
            delete cropTargetElement.dataset.crop_h;
            const imgEl = cropTargetElement.querySelector('img');
            if(imgEl){
                imgEl.style.objectFit='cover';
                imgEl.style.objectPosition='center';
                imgEl.style.width='';
                imgEl.style.height='';
            }
            cropModal.style.display='none';
        });

        const displayArea = document.getElementById('display-area');
        const modules = document.querySelectorAll('.module');
        const propertiesPanel = document.getElementById('properties-panel');
        const currentDesignName = document.getElementById('current-design-name');
        const designSelectContainer = document.getElementById('design-list');

        function showNotification(message) {
            const container = document.getElementById('notification-container');
            const notif = document.createElement('div');
            notif.className = 'notification';
            notif.textContent = message;
            container.appendChild(notif);

            setTimeout(()=>{
                notif.style.opacity = '0';
                setTimeout(()=> container.removeChild(notif), 300);
            },3000);
        }

        function loadDesignList(){
            fetch('/designs')
            .then(r=>r.json())
            .then(designs=>{
                designSelectContainer.innerHTML = '';
                const sel = document.createElement('select');
                sel.id = 'design-select';
                designs.forEach(d=>{
                    const opt = document.createElement('option');
                    opt.value = d.name;
                    opt.textContent = d.name + (d.active ? " (active)" : "");
                    sel.appendChild(opt);
                });
                designSelectContainer.appendChild(sel);
            });
        }

        document.getElementById('load-design').addEventListener('click', ()=>{
            const sel = document.getElementById('design-select');
            if(!sel) return;
            const val = sel.value;
            fetch('/get_design_by_name?name='+encodeURIComponent(val))
            .then(r=>r.json())
            .then(d=>{
                if(d.message){
                    showNotification(d.message);
                    return;
                }
                currentDesign = d;
                currentDesignName.textContent = d.name;
                document.getElementById('design-name').value = d.name;
                document.getElementById('keep-alive').checked = !!d.keep_alive;
                loadDesignIntoEditor(d);
            });
        });

        document.getElementById('clone-design').addEventListener('click',()=>{
            const sel = document.getElementById('design-select');
            if(!sel) return;
            const val = sel.value;
            fetch('/clone_design',{
                method:'POST',
                headers:{'Content-Type':'application/json'},
                body: JSON.stringify({name:val})
            })
            .then(r=>r.json())
            .then(msg=>{
                showNotification(msg.message);
                loadDesignList();
            });
        });

        document.getElementById('delete-design').addEventListener('click',()=>{
            const sel = document.getElementById('design-select');
            if(!sel) return;
            const val = sel.value;
            fetch('/delete_design',{
                method:'POST',
                headers:{'Content-Type':'application/json'},
                body: JSON.stringify({name:val})
            })
            .then(r=>r.json())
            .then(msg=>{
                showNotification(msg.message);
                loadDesignList();
            });
        });

        document.getElementById('set-active').addEventListener('click',()=>{
            const sel = document.getElementById('design-select');
            if(!sel) return;
            const val = sel.value;
            fetch('/set_active_design',{
                method:'POST',
                headers:{'Content-Type':'application/json'},
                body: JSON.stringify({name:val})
            })
            .then(r=>r.json())
            .then(msg=>{
                showNotification(msg.message);
                loadDesignList();
            });
        });

        function loadActiveDesign(){
            fetch('/design')
            .then(r=>r.json())
            .then(d=>{
                if(d.message){
                    showNotification(d.message);
                    loadDesignList();
                    return;
                }
                currentDesign = d;
                currentDesignName.textContent = d.name;
                document.getElementById('design-name').value = d.name;
                document.getElementById('keep-alive').checked = !!d.keep_alive;
                loadDesignIntoEditor(d);
                loadDesignList();
            });
        }

        async function loadDisplaySettings() {
            try {
                const res = await fetch('/settings');
                const settings = await res.json();
                currentDisplay = settings.display;
                updateDisplayInfo();
            } catch(e) {
                console.error('Failed to load display settings:', e);
            }
        }

        async function loadDisplayProfiles() {
            try {
                const res = await fetch('/display_profiles');
                const profiles = await res.json();
                const select = document.getElementById('display-select');
                select.innerHTML = '';
                profiles.forEach(p => {
                    const opt = document.createElement('option');
                    opt.value = p.type;
                    opt.textContent = p.name;
                    if (currentDisplay && p.type === currentDisplay.type) opt.selected = true;
                    select.appendChild(opt);
                });
            } catch(e) {
                console.error('Failed to load display profiles:', e);
            }
        }

        function updateDisplayInfo() {
            const info = document.getElementById('display-info');
            if (!currentDisplay) { info.innerHTML = ''; return; }
            let dotsHtml = currentDisplay.colors.map((c, i) =>
                `<div class="palette-dot" style="background:${c};" title="${currentDisplay.color_names[i]}"></div>`
            ).join('');
            info.innerHTML = `
                <div>${currentDisplay.width}x${currentDisplay.height}px</div>
                <div>Driver: ${currentDisplay.driver}</div>
                <div>Refresh: ~${currentDisplay.refresh_sec}s</div>
                <div class="palette-preview">${dotsHtml}</div>
            `;
        }

        document.getElementById('display-select').addEventListener('change', onDisplayChange);

        async function onDisplayChange() {
            const select = document.getElementById('display-select');
            const displayType = select.value;
            try {
                const res = await fetch('/update_settings', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({display_type: displayType})
                });
                const data = await res.json();
                if (!res.ok) {
                    showNotification('Error: ' + (data.message || 'Unknown error'));
                    return;
                }
                currentDisplay = data.display;
                updateDisplayInfo();
                if (selectedElement) showProperties(selectedElement);
                showNotification('Display changed to ' + currentDisplay.name);
            } catch(e) {
                showNotification('Failed to save display settings: ' + e.message);
            }
        }

        function renderColorSwatches(containerId, selectedColor, onSelect) {
            const container = document.getElementById(containerId);
            if (!container || !currentDisplay) return;
            container.innerHTML = '';
            currentDisplay.colors.forEach((color, i) => {
                const swatch = document.createElement('div');
                swatch.className = 'color-swatch' + (color.toLowerCase() === (selectedColor||'').toLowerCase() ? ' selected' : '');
                swatch.style.backgroundColor = color;
                swatch.title = currentDisplay.color_names[i];
                swatch.onclick = () => {
                    container.querySelectorAll('.color-swatch').forEach(s => s.classList.remove('selected'));
                    swatch.classList.add('selected');
                    onSelect(color);
                };
                if (color.toLowerCase() === '#ffffff') {
                    swatch.style.border = '2px solid #ccc';
                }
                container.appendChild(swatch);
            });
        }

        // Initialize: load display settings first, then design
        loadDisplaySettings().then(() => {
            loadDisplayProfiles();
            loadActiveDesign();
        });

        function applyTextStyles(el){
            const fontSize = el.dataset.fontSize || '18';
            const bold = (el.dataset.fontBold==='true');
            const italic = (el.dataset.fontItalic==='true');
            const strike = (el.dataset.fontStrike==='true');
            const align = el.dataset.textAlign || 'left';
            const textColor = el.dataset.textColor || '#000000';
            el.style.fontSize = fontSize+'px';
            el.style.fontWeight = bold?'bold':'normal';
            el.style.fontStyle = italic?'italic':'normal';
            el.style.textDecoration = strike?'line-through':'none';
            el.style.textAlign = align;
            el.style.color = textColor;

            let fontName = el.dataset.font;
            if(fontName){
                loadAndApplyFont(fontName, el);
            } else {
                el.style.fontFamily = "sans-serif";
            }
        }

        function loadAndApplyFont(fontName, el){
            if(!fontName) {
                el.style.fontFamily = "sans-serif";
                return;
            }

            if(!loadedFonts.has(fontName)){
                let fontUrl = '/font/'+encodeURIComponent(fontName);
                let fontFaceName = 'font_'+fontName.replace(/[^a-zA-Z0-9]/g,'_');
                let styleEl = document.createElement('style');
                styleEl.type = 'text/css';
                styleEl.innerHTML = `
                @font-face {
                    font-family: '${fontFaceName}';
                    src: url('${fontUrl}') format('truetype');
                }`;
                document.head.appendChild(styleEl);
                loadedFonts.add(fontName);
                el.style.fontFamily = `'${fontFaceName}', sans-serif`;
            } else {
                let fontFaceName = 'font_'+fontName.replace(/[^a-zA-Z0-9]/g,'_');
                el.style.fontFamily = `'${fontFaceName}', sans-serif`;
            }
        }

        function loadDesignIntoEditor(design){
            displayArea.innerHTML = '';
            const einkFrame = document.createElement('div');
            einkFrame.id = 'eink-frame';
            displayArea.appendChild(einkFrame);

            design.modules.forEach(m=>{
                const el = createModuleElement(m.type, m.position.x, m.position.y, m.size.width, m.size.height, m.content, m.styleData);
                displayArea.appendChild(el);
            });
        }

        modules.forEach(module => {
            module.addEventListener('dragstart', (e) => {
                if(mediaModalOpen) return;
                e.dataTransfer.setData('text/plain', module.dataset.type);
            });
        });

        displayArea.addEventListener('dragover', (e) => {
            e.preventDefault();
        });

        displayArea.addEventListener('drop', (e) => {
            e.preventDefault();
            const type = e.dataTransfer.getData('text/plain');
            if (type) {
                const el = createModuleElement(type, e.offsetX, e.offsetY);
                displayArea.appendChild(el);
            }
        });

        function insideEinkArea(x,y,w,h){
            const left = 200;
            const top = 160;
            const right = left+800;
            const bottom = top+480;
            if(x+w < left || x > right || y+h<top || y>bottom) return false;
            return true;
        }

        function updateElementOpacity(el){
            const x = parseInt(el.style.left);
            const y = parseInt(el.style.top);
            const w = parseInt(el.style.width);
            const h = parseInt(el.style.height);
            if(insideEinkArea(x,y,w,h)){
                el.classList.add('inside');
            } else {
                el.classList.remove('inside');
            }
        }

        function createModuleElement(type, x, y, w=100, h=50, content=null, styleData={}){
            const element = document.createElement('div');
            element.className = 'draggable';
            element.dataset.type = type;
            for(let [k,v] of Object.entries(styleData)){
                element.dataset[k] = v;
            }

            element.style.left = x + 'px';
            element.style.top = y + 'px';
            element.style.width = w + 'px';
            element.style.height = h + 'px';

            if(content===null){
                switch(type) {
                    case 'text':
                        content = element.dataset.textContent || 'Edit Text';
                        break;
                    case 'image':
                        content = 'No Image Selected';
                        break;
                    case 'weather':
                        content = 'No data';
                        break;
                    case 'datetime':
                        content = 'YYYY-MM-DD HH:mm';
                        break;
                    case 'timer':
                        content = 'D days, HH:MM:SS';
                        break;
                    case 'news':
                        content = 'News Headline';
                        break;
                    case 'line':
                        content = '';
                        element.style.background = element.dataset.textColor || '#000000';
                        break;
                    case 'calendar':
                        content = 'No Events';
                        break;
                }
            }

            if(type==='image' && element.dataset.image){
                const img = document.createElement('img');
                img.src = '/image/' + element.dataset.image;
                img.onload=()=>{
                    const ratio = img.naturalWidth/img.naturalHeight;
                    element.dataset.aspectRatio=ratio;
                    if(element.dataset.crop_x){
                        img.style.objectFit='none';
                        img.style.objectPosition=(-parseInt(element.dataset.crop_x))+'px '+(-parseInt(element.dataset.crop_y))+'px';
                        img.style.width = element.dataset.crop_w+'px';
                        img.style.height = element.dataset.crop_h+'px';
                    } else {
                        img.style.objectFit='cover';
                        img.style.objectPosition='center';
                    }
                };
                element.appendChild(img);
            } else if (type!=='image') {
                if(type === 'calendar'){
                    // Spezielle Darstellung für Kalender-Module
                    const eventList = document.createElement('ul');
                    eventList.style.listStyle = 'none';
                    eventList.style.padding = '0';
                    eventList.style.margin = '0';
                    const events = content.split('\n');
                    events.forEach(event => {
                        if(event.trim() !== "No events"){
                            const li = document.createElement('li');
                            li.textContent = event;
                            eventList.appendChild(li);
                        }
                    });
                    element.appendChild(eventList);
                } else {
                    element.textContent = content;
                }
            }

            applyTextStyles(element);
            addResizeHandles(element);

            element.addEventListener('mousedown', (e) => {
                if(mediaModalOpen) return;
                selectElement(e);
                if(!e.target.classList.contains('resize-handle')){
                    isDraggingElement = true;
                    dragOffsetX = e.clientX - element.getBoundingClientRect().left;
                    dragOffsetY = e.clientY - element.getBoundingClientRect().top;
                }
            });

            updateElementOpacity(element);
            return element;
        }

        function selectElement(e) {
            e.stopPropagation();
            clearSelection();
            selectedElement = e.currentTarget;
            selectedElement.classList.add('selected');
            showProperties(selectedElement);
        }

        function clearSelection() {
            if (selectedElement) selectedElement.classList.remove('selected');
            selectedElement = null;
        }

        displayArea.addEventListener('mousedown', (e) => {
            if (e.target === displayArea) {
                clearSelection();
                propertiesPanel.innerHTML = '<p>Select an element to view/edit its properties.</p>';
            }
        });

        document.addEventListener('mousemove', (e) => {
            if (isDraggingElement && selectedElement && !isResizing) {
                let newX = e.clientX - displayArea.getBoundingClientRect().left - dragOffsetX;
                let newY = e.clientY - displayArea.getBoundingClientRect().top - dragOffsetY;
                selectedElement.style.left = newX + 'px';
                selectedElement.style.top = newY + 'px';
                updateElementOpacity(selectedElement);
            } else if (isResizing && selectedElement) {
                const dx = e.clientX - resizeStartMouseX;
                const dy = e.clientY - resizeStartMouseY;
                let width = startWidth;
                let height = startHeight;
                let left = startLeft;
                let top = startTop;

                if(selectedElement.dataset.type==='image' && selectedElement.dataset.aspectRatio){
                    const ratio = parseFloat(selectedElement.dataset.aspectRatio);
                    if(currentHandle.classList.contains('bottom-right')){
                        width = startWidth + dx;
                        height = Math.round(width/ratio);
                    } else if(currentHandle.classList.contains('bottom-left')){
                        width = startWidth - dx;
                        height = Math.round(width/ratio);
                        left = startLeft + dx;
                    } else if(currentHandle.classList.contains('top-right')){
                        width = startWidth + dx;
                        height = Math.round(width/ratio);
                        top = startTop + (startHeight - height);
                    } else if(currentHandle.classList.contains('top-left')){
                        width = startWidth - dx;
                        height = Math.round(width/ratio);
                        left = startLeft + dx;
                        top = startTop + (startHeight - height);
                    }
                    if(width<10) width=10;
                    if(height<10) height=10;
                } else {
                    if (currentHandle.classList.contains('bottom-right')) {
                        width = startWidth + dx;
                        height = startHeight + dy;
                    } else if (currentHandle.classList.contains('bottom-left')) {
                        width = startWidth - dx;
                        height = startHeight + dy;
                        left = startLeft + dx;
                    } else if (currentHandle.classList.contains('top-right')) {
                        width = startWidth + dx;
                        height = startHeight - dy;
                        top = startTop + dy;
                    } else if (currentHandle.classList.contains('top-left')) {
                        width = startWidth - dx;
                        height = startHeight - dy;
                        left = startLeft + dx;
                        top = startTop + dy;
                    }
                    if(width<10) width=10;
                    if(height<10) height=10;
                }

                selectedElement.style.width = width + 'px';
                selectedElement.style.height = height + 'px';
                selectedElement.style.left = left + 'px';
                selectedElement.style.top = top + 'px';
                updateElementOpacity(selectedElement);
            }
        });

        document.addEventListener('mouseup', () => {
            isDraggingElement = false;
            isResizing = false;
            currentHandle = null;
        });

        function addResizeHandles(el) {
            ['top-left','top-right','bottom-left','bottom-right'].forEach(pos => {
                const handle = document.createElement('div');
                handle.className = 'resize-handle ' + pos;
                handle.addEventListener('mousedown', startResize);
                el.appendChild(handle);
            });
        }

        function startResize(e) {
            e.stopPropagation();
            isResizing = true;
            currentHandle = e.target;
            startWidth = parseInt(selectedElement.style.width,10);
            startHeight = parseInt(selectedElement.style.height,10);
            startLeft = parseInt(selectedElement.style.left,10);
            startTop = parseInt(selectedElement.style.top,10);
            resizeStartMouseX = e.clientX;
            resizeStartMouseY = e.clientY;
        }

        async function loadFontsToSelect(){
            const resp = await fetch('/fonts_all');
            const fonts = await resp.json();
            const fontSelect = document.getElementById('fontSelect');
            fontSelect.innerHTML = '<option value="">Default</option>';
            fonts.forEach(f=>{
                const opt = document.createElement('option');
                opt.value = f;
                opt.textContent = f;
                fontSelect.appendChild(opt);
            });
        }

        async function loadWeatherStylesSelect(){
            const resp = await fetch('/weather_styles');
            const styles = await resp.json();
            const weatherStyleSelect = document.getElementById('weatherStyleSelect');
            weatherStyleSelect.innerHTML = '';
            styles.forEach(s=>{
                const opt = document.createElement('option');
                opt.value = s;
                opt.textContent = s;
                weatherStyleSelect.appendChild(opt);
            });
        }

        function showProperties(el) {
            const type = el.dataset.type;
            propertiesPanel.innerHTML = '';

            const positionField = document.createElement('div');
            positionField.className = 'property-field';
            positionField.innerHTML = '<label>Position X</label><input type="number" id="posX" /><label>Position Y</label><input type="number" id="posY" />';

            const sizeField = document.createElement('div');
            sizeField.className = 'property-field';
            sizeField.innerHTML = '<label>Width</label><input type="number" id="widthVal" /><label>Height</label><input type="number" id="heightVal" />';

            const deleteBtn = document.createElement('button');
            deleteBtn.textContent = 'Delete Module';
            deleteBtn.style.padding = '5px 10px';
            deleteBtn.style.fontSize = '0.9em';
            deleteBtn.style.marginTop = '10px';
            deleteBtn.addEventListener('click', () => {
                el.remove();
                clearSelection();
                propertiesPanel.innerHTML = '<p>Select an element to view/edit its properties.</p>';
            });

            propertiesPanel.appendChild(positionField);
            propertiesPanel.appendChild(sizeField);

            document.getElementById('posX').value = parseInt(el.style.left,10);
            document.getElementById('posY').value = parseInt(el.style.top,10);
            document.getElementById('widthVal').value = parseInt(el.style.width,10);
            document.getElementById('heightVal').value = parseInt(el.style.height,10);

            document.getElementById('posX').addEventListener('input', (ev) => {
                let val = parseInt(ev.target.value);
                if(val<0) val=0;
                el.style.left = val + 'px';
                updateElementOpacity(el);
            });
            document.getElementById('posY').addEventListener('input', (ev) => {
                let val = parseInt(ev.target.value);
                if(val<0) val=0;
                el.style.top = val + 'px';
                updateElementOpacity(el);
            });
            document.getElementById('widthVal').addEventListener('input', (ev) => {
                let val = parseInt(ev.target.value);
                if(val<10) val=10;
                el.style.width = val + 'px';
                updateElementOpacity(el);
            });
            document.getElementById('heightVal').addEventListener('input', (ev) => {
                let val = parseInt(ev.target.value);
                if(val<10) val=10;
                el.style.height = val + 'px';
                updateElementOpacity(el);
            });

            if (['text','news','weather','datetime','timer','calendar'].includes(type)) {
                const colorField = document.createElement('div');
                colorField.className = 'property-field';
                colorField.innerHTML = '<label>Text Color</label><div id="textColorSwatches"></div>';
                propertiesPanel.appendChild(colorField);
                renderColorSwatches('textColorSwatches', el.dataset.textColor || '#000000', (color) => {
                    el.dataset.textColor = color;
                    el.style.color = color;
                });

                const fontField = document.createElement('div');
                fontField.className = 'property-field';
                fontField.innerHTML = '<label>Font</label><select id="fontSelect"></select>';
                propertiesPanel.appendChild(fontField);

                const sizeFieldFont = document.createElement('div');
                sizeFieldFont.className = 'property-field';
                sizeFieldFont.innerHTML = '<label>Font Size</label><input type="number" id="fontSizeVal" value="18"/>';
                propertiesPanel.appendChild(sizeFieldFont);

                const styleField = document.createElement('div');
                styleField.className = 'property-field';
                styleField.innerHTML = `
                <label><input type="checkbox" id="fontBold"/> Bold</label>
                <label><input type="checkbox" id="fontItalic"/> Italic</label>
                <label><input type="checkbox" id="fontStrike"/> Strike</label>
                <label>Text Align</label>
                <select id="textAlign">
                    <option value="left">Left</option>
                    <option value="center">Center</option>
                    <option value="right">Right</option>
                </select>
                `;
                propertiesPanel.appendChild(styleField);

                // Offline Client Sync für datetime, weather, timer, calendar
                if (type==='datetime' || type==='weather' || type==='timer' || type==='calendar'){
                    const offlineField = document.createElement('div');
                    offlineField.className = 'property-field';
                    offlineField.innerHTML = '<label><input type="checkbox" id="offlineClientSync"/> Offline Client Sync</label>';
                    propertiesPanel.appendChild(offlineField);
                    document.getElementById('offlineClientSync').checked = (el.dataset.offlineClientSync==='true');
                    document.getElementById('offlineClientSync').addEventListener('change',(ev)=>{
                        el.dataset.offlineClientSync = ev.target.checked.toString();
                    });
                }

                loadFontsToSelect().then(()=>{
                    if(el.dataset.font) document.getElementById('fontSelect').value = el.dataset.font;
                });

                if(el.dataset.fontSize) document.getElementById('fontSizeVal').value = el.dataset.fontSize;
                if(el.dataset.fontBold==='true') document.getElementById('fontBold').checked = true;
                if(el.dataset.fontItalic==='true') document.getElementById('fontItalic').checked = true;
                if(el.dataset.fontStrike==='true') document.getElementById('fontStrike').checked = true;
                if(el.dataset.textAlign) document.getElementById('textAlign').value=el.dataset.textAlign;

                document.getElementById('fontSelect').addEventListener('change',(ev)=>{
                    el.dataset.font = ev.target.value;
                    applyTextStyles(el);
                });
                document.getElementById('fontSizeVal').addEventListener('input',(ev)=>{
                    el.dataset.fontSize = ev.target.value;
                    applyTextStyles(el);
                });
                document.getElementById('fontBold').addEventListener('change',(ev)=>{
                    el.dataset.fontBold = ev.target.checked.toString();
                    applyTextStyles(el);
                });
                document.getElementById('fontItalic').addEventListener('change',(ev)=>{
                    el.dataset.fontItalic = ev.target.checked.toString();
                    applyTextStyles(el);
                });
                document.getElementById('fontStrike').addEventListener('change',(ev)=>{
                    el.dataset.fontStrike = ev.target.checked.toString();
                    applyTextStyles(el);
                });
                document.getElementById('textAlign').addEventListener('change',(ev)=>{
                    el.dataset.textAlign = ev.target.value;
                    applyTextStyles(el);
                });
            }

            if (type === 'text') {
                const textField = document.createElement('div');
                textField.className = 'property-field';
                textField.innerHTML = '<label>Text Content</label><textarea id="textContent"></textarea>';
                propertiesPanel.appendChild(textField);
                document.getElementById('textContent').value = el.dataset.textContent || el.textContent;
                document.getElementById('textContent').addEventListener('input', (ev) => {
                    el.textContent = ev.target.value;
                    el.dataset.textContent = ev.target.value;
                    applyTextStyles(el);
                });

            } else if (type === 'image') {
                const imageField = document.createElement('div');
                imageField.className = 'property-field';
                imageField.innerHTML = `
                <label>Select Image</label><button id="imageSelectBtn">Open Media Library</button>
                <button id="cropImageBtn">Crop Image</button>
                `;
                propertiesPanel.appendChild(imageField);

                document.getElementById('imageSelectBtn').addEventListener('click', ()=>{
                    openMediaLibrary((selectedImage)=>{
                        el.innerHTML = '';
                        if (selectedImage) {
                            const imgTag = document.createElement('img');
                            imgTag.src = '/image/' + selectedImage;
                            imgTag.onload=()=>{
                                const ratio = imgTag.naturalWidth/imgTag.naturalHeight;
                                el.dataset.aspectRatio=ratio;
                            }
                            el.appendChild(imgTag);
                            el.dataset.image = selectedImage;
                            delete el.dataset.crop_x;
                            delete el.dataset.crop_y;
                            delete el.dataset.crop_w;
                            delete el.dataset.crop_h;
                        } else {
                            el.textContent = 'No Image Selected';
                            delete el.dataset.image;
                        }
                    });
                });

                document.getElementById('cropImageBtn').addEventListener('click', ()=>{
                    if(!el.dataset.image){
                        showNotification("No image selected!");
                        return;
                    }
                    openCropModal(el);
                });

            } else if (type === 'weather') {
                const weatherField = document.createElement('div');
                weatherField.className = 'property-field';
                weatherField.innerHTML = `
                    <label>Location Search</label>
                    <input type="text" id="weatherLocationSearch" placeholder="Enter city or zip..."/>
                    <div id="weatherLocationResults" style="border:1px solid #ccc;max-height:150px;overflow:auto;display:none;"></div>
                    <label>Weather Style</label>
                    <select id="weatherStyleSelect"></select>
                    <button id="updateWeatherBtn" style="margin-top:5px;padding:5px;">Update Weather</button>
                `;
                propertiesPanel.appendChild(weatherField);

                loadWeatherStylesSelect().then(()=>{
                    if(el.dataset.weatherStyle){
                        document.getElementById('weatherStyleSelect').value=el.dataset.weatherStyle;
                    }
                });

                const locationSearch = document.getElementById('weatherLocationSearch');
                const locationResults = document.getElementById('weatherLocationResults');
                locationSearch.value = el.dataset.locationName || '';

                locationSearch.addEventListener('input', (ev)=>{
                    const query = ev.target.value;
                    if(query.length<2) {
                        locationResults.style.display='none';
                        locationResults.innerHTML='';
                        return;
                    }
                    fetch('/location_search?q='+encodeURIComponent(query))
                    .then(r=>r.json())
                    .then(results=>{
                        locationResults.innerHTML='';
                        if(results.length>0) {
                            locationResults.style.display='block';
                            results.forEach(res=>{
                                const div = document.createElement('div');
                                div.textContent = res.display_name;
                                div.style.padding='5px';
                                div.style.cursor='pointer';
                                div.addEventListener('click',()=>{
                                    el.dataset.latitude = res.lat;
                                    el.dataset.longitude = res.lon;
                                    el.dataset.locationName = res.display_name;
                                    locationSearch.value=res.display_name;
                                    locationResults.style.display='none';
                                    locationResults.innerHTML='';
                                });
                                locationResults.appendChild(div);
                            });
                        } else {
                            locationResults.style.display='none';
                        }
                    });
                });

                document.getElementById('updateWeatherBtn').addEventListener('click', ()=>{
                    showNotification("Weather will be updated on next design fetch.");
                });

                document.getElementById('weatherStyleSelect').addEventListener('change',(ev)=>{
                    el.dataset.weatherStyle = ev.target.value;
                });

            } else if (type === 'datetime') {
                const dtField = document.createElement('div');
                dtField.className = 'property-field';
                dtField.innerHTML = '<label>Format</label><input type="text" id="datetimeFormat" value="YYYY-MM-DD HH:mm"/>';
                propertiesPanel.appendChild(dtField);

                document.getElementById('datetimeFormat').value = el.dataset.datetimeFormat || 'YYYY-MM-DD HH:mm';
                document.getElementById('datetimeFormat').addEventListener('input',(ev)=>{
                    el.dataset.datetimeFormat = ev.target.value;
                    el.textContent = ev.target.value;
                    applyTextStyles(el);
                });

            } else if (type === 'news') {
                const newsField = document.createElement('div');
                newsField.className = 'property-field';
                newsField.innerHTML = '<label>Headline</label><input type="text" id="newsHeadline"/>';
                propertiesPanel.appendChild(newsField);

                document.getElementById('newsHeadline').value = el.dataset.newsHeadline || el.textContent;
                document.getElementById('newsHeadline').addEventListener('input', (ev) => {
                    el.textContent = ev.target.value;
                    el.dataset.newsHeadline = ev.target.value;
                    applyTextStyles(el);
                });
            } else if (type==='timer'){
                const timerField = document.createElement('div');
                timerField.className = 'property-field';
                timerField.innerHTML = '<label>Timer Target (YYYY-MM-DD HH:mm:ss)</label><input type="text" id="timerTarget" value="2025-01-01 00:00:00"/>';
                propertiesPanel.appendChild(timerField);

                const timerFormatField = document.createElement('div');
                timerFormatField.className = 'property-field';
                timerFormatField.innerHTML = '<label>Timer Format (e.g. D days, HH:MM:SS)</label><input type="text" id="timerFormat" value="D days, HH:MM:SS"/>';
                propertiesPanel.appendChild(timerFormatField);

                document.getElementById('timerTarget').value = el.dataset.timerTarget || '2025-01-01 00:00:00';
                document.getElementById('timerFormat').value = el.dataset.timerFormat || 'D days, HH:MM:SS';

                document.getElementById('timerTarget').addEventListener('input',(ev)=>{
                    el.dataset.timerTarget = ev.target.value;
                });
                document.getElementById('timerFormat').addEventListener('input',(ev)=>{
                    el.dataset.timerFormat = ev.target.value;
                });
            } else if (type === 'calendar') { <!-- Neue Kalender-Modul-Properties -->
                const calendarField = document.createElement('div');
                calendarField.className = 'property-field';
                calendarField.innerHTML = `
                    <label>Calendar URL (iCal/Webcal)</label>
                    <input type="text" id="calendarURL" placeholder="https://example.com/calendar.ics" value="${el.dataset.calendarURL || ''}"/>
                `;
                propertiesPanel.appendChild(calendarField);

                const maxEventsField = document.createElement('div');
                maxEventsField.className = 'property-field';
                maxEventsField.innerHTML = '<label>Max Events to Display</label><input type="number" id="maxEvents" value="' + (el.dataset.maxEvents || 5) + '"/>';
                propertiesPanel.appendChild(maxEventsField);

                document.getElementById('calendarURL').addEventListener('input',(ev)=>{
                    el.dataset.calendarURL = ev.target.value;
                });
                document.getElementById('maxEvents').addEventListener('input',(ev)=>{
                    el.dataset.maxEvents = ev.target.value;
                });
            } else if (type === 'line') {
                const lineColorField = document.createElement('div');
                lineColorField.className = 'property-field';
                lineColorField.innerHTML = '<label>Color</label><div id="lineColorSwatches"></div>';
                propertiesPanel.appendChild(lineColorField);
                renderColorSwatches('lineColorSwatches', el.dataset.textColor || '#000000', (color) => {
                    el.dataset.textColor = color;
                    el.style.background = color;
                });
            }

            propertiesPanel.appendChild(deleteBtn);
        }

        document.getElementById('save-design').addEventListener('click', () => {
            saveDesign(false);
        });

        document.getElementById('save-as-new').addEventListener('click', () => {
            saveDesign(true);
        });

        function saveDesign(asNew){
            const modulesData = [];
            document.querySelectorAll('.draggable').forEach(element => {
                const type = element.dataset.type;
                let content = element.dataset.textContent || element.textContent;
                let styleData = {};

                for(let [k,v] of Object.entries(element.dataset)){
                    if(k!=='type') styleData[k]=v;
                }

                if(type==='image'){
                    if(element.dataset.image){
                        styleData.image = element.dataset.image;
                    } else {
                        content = '';
                    }
                }

                modulesData.push({
                    type: type,
                    content: content,
                    position: { x: parseInt(element.style.left), y: parseInt(element.style.top) },
                    size: { width: parseInt(element.style.width), height: parseInt(element.style.height) },
                    styleData: styleData
                });
            });
            const designName = document.getElementById('design-name').value || 'Unnamed Design';
            const keepAlive = document.getElementById('keep-alive').checked;

            fetch('/update_design', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    modules: modulesData,
                    resolution: [800,480],
                    name: designName,
                    save_as_new: asNew,
                    keep_alive: keepAlive
                })
            })
            .then(response => response.json())
            .then(data => {
                showNotification(data.message);
                loadActiveDesign();
            });
        }

        document.getElementById('preview-btn').addEventListener('click', () => {
            const sel = document.getElementById('design-select');
            let url='/preview';
            if(sel && sel.value) url = '/preview?name='+encodeURIComponent(sel.value);
            fetch(url)
            .then(r=>{
                if(!r.ok) throw new Error('No preview available');
                return r.blob();
            })
            .then(blob=>{
                const url = URL.createObjectURL(blob);
                document.getElementById('preview-image').src = url;
                document.getElementById('preview-modal').style.display='flex';
            })
            .catch(e=>{
                showNotification("Failed to load preview: "+e.message);
            });
        });

        document.getElementById('preview-close').addEventListener('click',()=>{
            document.getElementById('preview-modal').style.display='none';
        });

        document.getElementById('refresh-display-btn').addEventListener('click', () => {
            const btn = document.getElementById('refresh-display-btn');
            btn.disabled = true;
            btn.textContent = 'Refreshing...';
            fetch('/refresh-display', { method: 'POST' })
            .then(r => r.json())
            .then(data => {
                showNotification(data.message);
            })
            .catch(e => {
                showNotification("Failed to refresh display: " + e.message);
            })
            .finally(() => {
                btn.disabled = false;
                btn.textContent = 'Refresh Display';
            });
        });

        const mediaModal = document.getElementById('media-modal');
        const mediaClose = document.getElementById('media-close');
        const mediaOkBtn = document.getElementById('media-ok-btn');
        const mediaCancelBtn = document.getElementById('media-cancel-btn');
        const mediaUploadBtn = document.getElementById('media-upload-btn');
        const mediaUploadFile = document.getElementById('media-upload-file');
        const mediaLibrary = document.getElementById('media-library');
        const fontsLibrary = document.getElementById('fonts-library');
        const uploadProgressContainer = document.getElementById('upload-progress-container');
        const uploadProgressBar = document.getElementById('upload-progress-bar');

        function openMediaLibrary(cb){
            imageSelectCallback = cb;
            mediaModalOpen = true;
            mediaModal.style.display='flex';
            loadMediaLibrary();
        }

        function loadMediaLibrary(){
            fetch('/images_all')
            .then(r=>r.json())
            .then(images=>{
                mediaLibrary.innerHTML='';
                selectedMedia=null;
                images.forEach(img=>{
                    const div = document.createElement('div');
                    div.className='media-item';

                    const imageTag = document.createElement('img');
                    imageTag.src='/image/'+img;
                    div.appendChild(imageTag);

                    const delBtn = document.createElement('div');
                    delBtn.className='media-delete-btn';
                    delBtn.textContent='X';
                    delBtn.addEventListener('click',(ev)=>{
                        ev.stopPropagation();
                        deleteImage(img);
                    });
                    div.appendChild(delBtn);

                    div.addEventListener('click',()=>{
                        selectedMedia=img;
                        document.querySelectorAll('.media-item').forEach(i=>i.style.borderColor='#ccc');
                        div.style.borderColor='#00f';
                    });
                    mediaLibrary.appendChild(div);
                });
            });

            fetch('/fonts_all')
            .then(r=>r.json())
            .then(fonts=>{
                fontsLibrary.innerHTML='';
                fonts.forEach(f=>{
                    const div = document.createElement('div');
                    div.className='media-item';
                    div.textContent=f;
                    div.addEventListener('click',()=>{
                        selectedMedia=null;
                        document.querySelectorAll('#fonts-library .media-item').forEach(i=>i.style.borderColor='#ccc');
                        div.style.borderColor='#00f';
                    });
                    fontsLibrary.appendChild(div);
                });
            });
        }

        function deleteImage(filename){
            fetch('/delete_image',{
                method:'POST',
                headers:{'Content-Type':'application/json'},
                body:JSON.stringify({filename})
            })
            .then(r=>r.json())
            .then(msg=>{
                showNotification(msg.message);
                loadMediaLibrary();
            });
        }

        mediaClose.addEventListener('click', ()=>{
            mediaModal.style.display='none';
            mediaModalOpen=false;
        });

        mediaCancelBtn.addEventListener('click', ()=>{
            mediaModal.style.display='none';
            mediaModalOpen=false;
        });

        mediaOkBtn.addEventListener('click', ()=>{
            mediaModal.style.display='none';
            mediaModalOpen=false;
            if(imageSelectCallback) imageSelectCallback(selectedMedia);
        });

        mediaUploadBtn.addEventListener('click', () => {
            if(!mediaUploadFile.files[0]) {
                showNotification("Please select a file first");
                return;
            }

            uploadProgressContainer.style.display='block';
            uploadProgressBar.style.width='0%';

            const formData = new FormData();
            formData.append('file', mediaUploadFile.files[0]);

            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/upload_image', true);

            xhr.upload.onprogress = (e)=>{
                if(e.lengthComputable) {
                    const percent = (e.loaded/e.total)*100;
                    uploadProgressBar.style.width=percent+'%';
                }
            };

            xhr.onload = ()=>{
                if(xhr.status===200){
                    showNotification('Upload completed!');
                    loadMediaLibrary();
                    uploadProgressContainer.style.display='none';
                } else {
                    showNotification('Upload failed!');
                    uploadProgressContainer.style.display='none';
                }
            };

            xhr.onerror = () => {
                showNotification('Upload failed due to a network error.');
                uploadProgressContainer.style.display='none';
            };

            xhr.send(formData);
        });
