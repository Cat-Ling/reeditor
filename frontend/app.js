let currentSaveData = null;

const elFileInput = document.getElementById('fileInput');
const elBtnExport = document.getElementById('btnExport');
const elLoading = document.getElementById('loading');
const elErrorBox = document.getElementById('errorBox');
const elEditorArea = document.getElementById('editorArea');
const elTreeContainer = document.getElementById('treeContainer');
const elMetaInfo = document.getElementById('metaInfo');

function showError(msg) {
    elErrorBox.textContent = msg;
    elErrorBox.classList.remove('hidden');
    elLoading.classList.add('hidden');
}

function hideError() {
    elErrorBox.classList.add('hidden');
}

elFileInput.addEventListener('change', async (e) => {
    const file = e.target.files[0];
    if (!file) return;

    hideError();
    elEditorArea.classList.add('hidden');
    elLoading.classList.remove('hidden');
    elBtnExport.disabled = true;

    const formData = new FormData();
    formData.append('savefile', file);

    try {
        const res = await fetch('/api/load', {
            method: 'POST',
            body: formData
        });

        const data = await res.json();

        if (data.error) {
            showError(`Error loading save:\n${data.error}\n\n${data.traceback || ''}`);
            return;
        }

        currentSaveData = data;
        renderMeta(data);
        renderTree(data.roots, elTreeContainer);

        elLoading.classList.add('hidden');
        elEditorArea.classList.remove('hidden');
        elBtnExport.disabled = false;

    } catch (err) {
        showError(`Network error: ${err.message}`);
    }
});

elBtnExport.addEventListener('click', async () => {
    if (!currentSaveData) return;

    hideError();
    elLoading.classList.remove('hidden');
    elEditorArea.classList.add('hidden');

    const file = elFileInput.files[0];
    const formData = new FormData();
    formData.append('savefile', file);
    formData.append('payload', JSON.stringify({
        roots: currentSaveData.roots,
        log: currentSaveData.log,
        json_meta: currentSaveData.json_meta,
        extra_info: currentSaveData.extra_info
    }));

    try {
        const res = await fetch('/api/save', {
            method: 'POST',
            body: formData
        });

        if (!res.ok) {
            const errText = await res.text();
            showError(`Save failed: ${errText}`);
            return;
        }

        const contentType = res.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
            const data = await res.json();
            showError(`Error saving:\n${data.error}\n\n${data.traceback || ''}`);
            return;
        }

        const blob = await res.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        let origName = file.name;
        if(origName.endsWith('.save')) origName = origName.slice(0, -5);
        a.download = `${origName}_edited.save`;
        a.click();
        window.URL.revokeObjectURL(url);

        elLoading.classList.add('hidden');
        elEditorArea.classList.remove('hidden');

    } catch (err) {
        showError(`Network error: ${err.message}`);
    }
});

function renderMeta(data) {
    let html = '<div class="meta-grid">';
    html += `<div class="meta-key">Extra Info:</div><div>${escapeHtml(data.extra_info)}</div>`;

    if (data.json_meta) {
        for (const [k, v] of Object.entries(data.json_meta)) {
            html += `<div class="meta-key">${escapeHtml(k)}:</div><div>${escapeHtml(JSON.stringify(v))}</div>`;
        }
    }
    html += '</div>';
    elMetaInfo.innerHTML = html;
}

function escapeHtml(unsafe) {
    if (unsafe == null) return 'null';
    return String(unsafe)
         .replace(/&/g, "&amp;")
         .replace(/</g, "&lt;")
         .replace(/>/g, "&gt;")
         .replace(/"/g, "&quot;")
         .replace(/'/g, "&#039;");
}

function renderTree(data, container) {
    container.innerHTML = '';
    const rootNode = createNode(data, 'roots', data);
    rootNode.classList.add('tree-node-root');
    container.appendChild(rootNode);
}

function createNode(val, key, parentObj, parentKey) {
    const wrapper = document.createElement('div');

    const row = document.createElement('div');
    row.className = 'tree-item';

    const toggle = document.createElement('span');
    toggle.className = 'tree-toggle';
    row.appendChild(toggle);

    if (key !== null) {
        const keySpan = document.createElement('span');
        keySpan.className = 'tree-key';
        keySpan.textContent = String(key) + ':';
        row.appendChild(keySpan);
    }

    let isExpandable = false;
    let typeSpan = document.createElement('span');
    typeSpan.className = 'tree-type';

    let valSpan = document.createElement('span');
    valSpan.className = 'tree-value';

    let childrenData = null;

    if (val === null) {
        valSpan.textContent = 'None';
        valSpan.classList.add('null');
        setupPrimitiveEdit(valSpan, parentObj, parentKey, 'null');
    } else if (typeof val === 'boolean') {
        valSpan.textContent = val ? 'True' : 'False';
        valSpan.classList.add('boolean');
        setupPrimitiveEdit(valSpan, parentObj, parentKey, 'boolean');
    } else if (typeof val === 'number') {
        valSpan.textContent = val;
        valSpan.classList.add('number');
        setupPrimitiveEdit(valSpan, parentObj, parentKey, 'number');
    } else if (typeof val === 'string') {
        valSpan.textContent = `"${val}"`;
        valSpan.classList.add('string');
        setupPrimitiveEdit(valSpan, parentObj, parentKey, 'string');
    } else if (Array.isArray(val)) {
        isExpandable = true;
        typeSpan.textContent = `list [${val.length}]`;
        childrenData = val.map((v, i) => ({k: i, v, parent: val, pK: i}));
    } else if (typeof val === 'object') {
        isExpandable = true;

        if (val.__dict__) {
            typeSpan.textContent = `dict {${val.__dict__.length}}`;
            childrenData = val.__dict__.map((pair, idx) => ({
                k: formatComplexKey(pair[0]),
                v: pair[1],
                parent: pair,
                pK: 1
            }));
        } else if (val.__tuple__) {
            typeSpan.textContent = `tuple (${val.__tuple__.length})`;
            childrenData = val.__tuple__.map((v, i) => ({k: i, v, parent: val.__tuple__, pK: i}));
        } else if (val.__set__) {
            typeSpan.textContent = `set {${val.__set__.length}}`;
            childrenData = val.__set__.map((v, i) => ({k: i, v, parent: val.__set__, pK: i}));
        } else if (val.__class__) {
            typeSpan.textContent = `<${val.__class__}>`;
            typeSpan.title = `ID: ${val.__id__}`;
            childrenData = [];

            if (val.__state__ !== null) {
                childrenData.push({k: '__state__', v: val.__state__, parent: val, pK: '__state__'});
            }
            if (val.__base__ !== null) {
                childrenData.push({k: '__base__', v: val.__base__, parent: val, pK: '__base__'});
            }
        } else {
            typeSpan.textContent = `object`;
            childrenData = Object.keys(val).map(k => ({k, v: val[k], parent: val, pK: k}));
        }
    }

    if (typeSpan.textContent) row.appendChild(typeSpan);
    if (!isExpandable) row.appendChild(valSpan);

    wrapper.appendChild(row);

    if (isExpandable) {
        toggle.textContent = '▶';
        const childrenWrapper = document.createElement('div');
        childrenWrapper.className = 'tree-node hidden';
        wrapper.appendChild(childrenWrapper);

        let loaded = false;

        toggle.addEventListener('click', () => {
            const isHidden = childrenWrapper.classList.contains('hidden');
            if (isHidden) {
                childrenWrapper.classList.remove('hidden');
                toggle.textContent = '▼';
                if (!loaded) {
                    childrenData.forEach(child => {
                        childrenWrapper.appendChild(createNode(child.v, child.k, child.parent, child.pK));
                    });
                    loaded = true;
                }
            } else {
                childrenWrapper.classList.add('hidden');
                toggle.textContent = '▶';
            }
        });

        if (key === 'roots') toggle.click();
    } else {
        toggle.innerHTML = '&nbsp;&nbsp;';
        toggle.style.cursor = 'default';
    }

    return wrapper;
}

function formatComplexKey(k) {
    if (typeof k === 'string') return k;
    return JSON.stringify(k);
}

function setupPrimitiveEdit(el, parentObj, parentKey, type) {
    if (!parentObj) return;

    el.addEventListener('click', () => {
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'tree-value-input';

        let currentVal = parentObj[parentKey];
        if (type === 'string') input.value = currentVal;
        else if (type === 'boolean') input.value = currentVal ? 'True' : 'False';
        else if (type === 'null') input.value = 'None';
        else input.value = String(currentVal);

        const save = () => {
            let newVal = input.value;
            let finalVal = currentVal;

            try {
                if (type === 'number') {
                    if (newVal.includes('.')) finalVal = parseFloat(newVal);
                    else finalVal = parseInt(newVal, 10);
                    if(isNaN(finalVal)) throw 'NaN';
                } else if (type === 'boolean') {
                    const lc = newVal.toLowerCase();
                    if (lc === 'true' || lc === '1') finalVal = true;
                    else if (lc === 'false' || lc === '0') finalVal = false;
                    else throw 'Invalid boolean';
                } else if (type === 'null') {
                    if (newVal.toLowerCase() === 'none' || newVal === 'null') finalVal = null;
                    else throw 'Invalid None';
                } else {
                    finalVal = newVal;
                }

                parentObj[parentKey] = finalVal;

                if (finalVal === null) el.textContent = 'None';
                else if (type === 'string') el.textContent = `"${finalVal}"`;
                else if (type === 'boolean') el.textContent = finalVal ? 'True' : 'False';
                else el.textContent = finalVal;

            } catch (e) {
                // Keep old
            }

            if(input.parentNode === el.parentNode) {
                el.parentNode.replaceChild(el, input);
            }
        };

        input.addEventListener('blur', save);
        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') save();
            if (e.key === 'Escape') {
                if(input.parentNode === el.parentNode) {
                    el.parentNode.replaceChild(el, input);
                }
            }
        });

        el.parentNode.replaceChild(input, el);
        input.focus();
    });
}

document.getElementById('btnCollapseAll').addEventListener('click', () => {
    const toggles = elTreeContainer.querySelectorAll('.tree-toggle');
    for(let i = toggles.length - 1; i >= 0; i--) {
        if(toggles[i].textContent === '▼' && !toggles[i].closest('.tree-node-root')) {
            toggles[i].click();
        }
    }
});

document.getElementById('searchInput').addEventListener('input', (e) => {
    const query = e.target.value.toLowerCase();
    const items = elTreeContainer.querySelectorAll('.tree-item');

    if (!query) {
        items.forEach(item => item.style.display = 'flex');
        return;
    }

    items.forEach(item => {
        const text = item.textContent.toLowerCase();
        if (text.includes(query)) {
            item.style.display = 'flex';
            let parent = item.closest('.tree-node');
            while (parent) {
                const prev = parent.previousElementSibling;
                if (prev && prev.classList.contains('tree-item')) {
                    prev.style.display = 'flex';
                    const toggle = prev.querySelector('.tree-toggle');
                    if (toggle && toggle.textContent === '▶') toggle.click();
                }
                parent = parent.parentElement.closest('.tree-node');
            }
        } else {
            item.style.display = 'none';
        }
    });
});
