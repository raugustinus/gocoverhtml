let activeFile = null;
let activeFilter = 'all';

// Command Palette State
let paletteOpen = false;
let paletteSelectedIndex = 0;
let paletteResults = [];
let allFiles = [];

// Build file index from DOM
function buildFileIndex() {
    const fileList = document.getElementById('file-list');
    const items = fileList.querySelectorAll('.file-item');

    allFiles = Array.from(items).map(item => {
        const name = item.querySelector('.file-name')?.textContent || '';
        const dir = item.closest('.file-group')?.dataset.dir || '';
        const coverage = item.dataset.coverage;
        const percent = parseFloat(item.dataset.percent);
        const fileId = item.dataset.file;

        return {
            id: fileId,
            name: name,
            dir: dir,
            coverage: coverage,
            percent: percent,
            fullPath: dir ? `${dir}/${name}` : name
        };
    });
}

function showFile(fileId) {
    // Hide welcome message
    document.getElementById('welcome').style.display = 'none';

    // Hide current file if any
    if (activeFile) {
        document.getElementById('file-' + activeFile).style.display = 'none';
        document.querySelector('[data-file="' + activeFile + '"]')?.classList.remove('active');
    }

    // Show new file
    const fileView = document.getElementById('file-' + fileId);
    if (fileView) {
        fileView.style.display = 'flex';
        document.querySelector('[data-file="' + fileId + '"]')?.classList.add('active');
        activeFile = fileId;
    }
}

// Command Palette Functions
function openCommandPalette() {
    const overlay = document.getElementById('command-palette-overlay');
    const palette = document.getElementById('command-palette');
    const input = document.getElementById('command-palette-input');

    paletteOpen = true;
    overlay.classList.add('open');
    palette.classList.add('open');

    // Reset state
    input.value = '';
    paletteSelectedIndex = 0;

    // Show all files initially
    filterPaletteResults('');

    // Focus input
    setTimeout(() => input.focus(), 50);
}

function closeCommandPalette() {
    const overlay = document.getElementById('command-palette-overlay');
    const palette = document.getElementById('command-palette');

    paletteOpen = false;
    overlay.classList.remove('open');
    palette.classList.remove('open');
}

function filterPaletteResults(query) {
    const resultsContainer = document.getElementById('command-palette-results');

    // Filter files based on query
    if (!query) {
        paletteResults = [...allFiles];
    } else {
        let regex;
        try {
            regex = new RegExp(query, 'i');
        } catch (e) {
            regex = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i');
        }

        paletteResults = allFiles.filter(file =>
            regex.test(file.name) || regex.test(file.dir) || regex.test(file.fullPath)
        );
    }

    // Reset selection
    paletteSelectedIndex = 0;

    // Render results
    renderPaletteResults(resultsContainer);
}

function renderPaletteResults(container) {
    if (paletteResults.length === 0) {
        container.innerHTML = '<div class="palette-empty">No files found</div>';
        return;
    }

    // Group files by directory
    const grouped = {};
    paletteResults.forEach(file => {
        const dir = file.dir || 'Root';
        if (!grouped[dir]) {
            grouped[dir] = [];
        }
        grouped[dir].push(file);
    });

    let html = '';
    let globalIndex = 0;

    Object.keys(grouped).sort().forEach(dir => {
        html += `<div class="palette-group">`;
        html += `<div class="palette-group-header">${dir}</div>`;

        grouped[dir].forEach(file => {
            const isSelected = globalIndex === paletteSelectedIndex;
            html += `
                <div class="palette-item ${isSelected ? 'selected' : ''}"
                     data-index="${globalIndex}"
                     data-file-id="${file.id}"
                     onclick="selectPaletteItem('${file.id}')">
                    <svg class="palette-item-icon" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
                    </svg>
                    <div class="palette-item-content">
                        <div class="palette-item-name">${file.name}</div>
                        <div class="palette-item-path">${file.fullPath}</div>
                    </div>
                    <span class="palette-item-badge ${file.coverage}">${Math.round(file.percent)}%</span>
                </div>
            `;
            globalIndex++;
        });

        html += `</div>`;
    });

    container.innerHTML = html;

    // Scroll selected item into view
    const selectedItem = container.querySelector('.palette-item.selected');
    if (selectedItem) {
        selectedItem.scrollIntoView({ block: 'nearest' });
    }
}

function selectPaletteItem(fileId) {
    closeCommandPalette();
    showFile(fileId);
}

function navigatePalette(direction) {
    const maxIndex = paletteResults.length - 1;

    if (direction === 'up') {
        paletteSelectedIndex = paletteSelectedIndex > 0 ? paletteSelectedIndex - 1 : maxIndex;
    } else {
        paletteSelectedIndex = paletteSelectedIndex < maxIndex ? paletteSelectedIndex + 1 : 0;
    }

    const container = document.getElementById('command-palette-results');
    renderPaletteResults(container);
}

function confirmPaletteSelection() {
    if (paletteResults.length > 0 && paletteResults[paletteSelectedIndex]) {
        selectPaletteItem(paletteResults[paletteSelectedIndex].id);
    }
}

// Filter functionality
function filterByCoverage(filter) {
    activeFilter = filter;
    const fileList = document.getElementById('file-list');
    const items = fileList.querySelectorAll('.file-item');
    const groups = fileList.querySelectorAll('.file-group');

    // Update active button
    document.querySelectorAll('.filter-btn').forEach(btn => {
        btn.classList.remove('active');
        if (btn.dataset.filter === filter) {
            btn.classList.add('active');
        }
    });

    // Filter items
    items.forEach(item => {
        const coverage = item.dataset.coverage;
        if (filter === 'all' || coverage === filter) {
            item.classList.remove('filter-hidden');
        } else {
            item.classList.add('filter-hidden');
        }
    });

    // Hide groups with no visible files
    groups.forEach(group => {
        const visibleFiles = group.querySelectorAll('.file-item:not(.filter-hidden)');
        if (visibleFiles.length === 0) {
            group.classList.add('filter-hidden');
        } else {
            group.classList.remove('filter-hidden');
        }
    });
}

// Sort functionality
function sortFiles(sortBy) {
    const fileList = document.getElementById('file-list');
    const groups = Array.from(fileList.querySelectorAll('.file-group'));

    groups.forEach(group => {
        const items = Array.from(group.querySelectorAll('.file-item'));

        items.sort((a, b) => {
            if (sortBy === 'name') {
                const nameA = a.querySelector('.file-name').textContent.toLowerCase();
                const nameB = b.querySelector('.file-name').textContent.toLowerCase();
                return nameA.localeCompare(nameB);
            } else if (sortBy === 'coverage-asc') {
                return parseFloat(a.dataset.percent) - parseFloat(b.dataset.percent);
            } else if (sortBy === 'coverage-desc') {
                return parseFloat(b.dataset.percent) - parseFloat(a.dataset.percent);
            }
            return 0;
        });

        // Re-append items in sorted order
        items.forEach(item => group.appendChild(item));
    });

    // Sort groups by their first visible item's coverage if sorting by coverage
    if (sortBy !== 'name') {
        const sortedGroups = groups.sort((a, b) => {
            const itemsA = a.querySelectorAll('.file-item');
            const itemsB = b.querySelectorAll('.file-item');
            if (itemsA.length === 0 || itemsB.length === 0) return 0;

            const avgA = Array.from(itemsA).reduce((sum, i) => sum + parseFloat(i.dataset.percent), 0) / itemsA.length;
            const avgB = Array.from(itemsB).reduce((sum, i) => sum + parseFloat(i.dataset.percent), 0) / itemsB.length;

            return sortBy === 'coverage-asc' ? avgA - avgB : avgB - avgA;
        });

        sortedGroups.forEach(group => fileList.appendChild(group));
    } else {
        // Sort groups alphabetically by directory name
        const sortedGroups = groups.sort((a, b) => {
            return a.dataset.dir.localeCompare(b.dataset.dir);
        });
        sortedGroups.forEach(group => fileList.appendChild(group));
    }
}

// Setup filter button listeners
document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        filterByCoverage(btn.dataset.filter);
    });
});

// Command palette input listener
const paletteInput = document.getElementById('command-palette-input');
if (paletteInput) {
    paletteInput.addEventListener('input', (e) => {
        filterPaletteResults(e.target.value);
    });

    paletteInput.addEventListener('keydown', (e) => {
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            navigatePalette('down');
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            navigatePalette('up');
        } else if (e.key === 'Enter') {
            e.preventDefault();
            confirmPaletteSelection();
        } else if (e.key === 'Escape') {
            e.preventDefault();
            closeCommandPalette();
        }
    });
}

// Keyboard navigation
document.addEventListener('keydown', (e) => {
    // Open command palette on Ctrl/Cmd + K
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        if (paletteOpen) {
            closeCommandPalette();
        } else {
            openCommandPalette();
        }
        return;
    }

    // Close palette on Escape when open
    if (e.key === 'Escape' && paletteOpen) {
        e.preventDefault();
        closeCommandPalette();
        return;
    }

    // Close file view on Escape when not in palette
    if (e.key === 'Escape' && !paletteOpen) {
        if (activeFile) {
            document.getElementById('file-' + activeFile).style.display = 'none';
            document.querySelector('[data-file="' + activeFile + '"]')?.classList.remove('active');
            document.getElementById('welcome').style.display = 'flex';
            activeFile = null;
        }
    }
});

// Prevent clicks inside palette from closing it
document.getElementById('command-palette')?.addEventListener('click', (e) => {
    e.stopPropagation();
});

// Initialize file index on load
document.addEventListener('DOMContentLoaded', buildFileIndex);
if (document.readyState !== 'loading') {
    buildFileIndex();
}
