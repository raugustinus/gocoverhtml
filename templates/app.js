let activeFile = null;

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

function toggleDir(header) {
    const dir = header.parentElement;
    if (!dir.classList.contains('expandable')) return;

    const isExpanded = dir.classList.contains('expanded');
    dir.classList.toggle('expanded');

    const icon = header.querySelector('.tree-icon');
    icon.innerHTML = isExpanded ? '&#128193;' : '&#128194;';
}

// Search functionality
const searchInput = document.getElementById('search-input');
const searchContainer = searchInput?.parentElement;

function performSearch(query) {
    const tree = document.getElementById('tree');
    const files = tree.querySelectorAll('.tree-file');
    const dirs = tree.querySelectorAll('.tree-dir');

    // Reset all
    files.forEach(f => {
        f.classList.remove('search-hidden', 'search-match');
    });
    dirs.forEach(d => {
        d.classList.remove('search-hidden');
    });

    if (!query) {
        searchContainer?.classList.remove('has-value');
        return;
    }

    searchContainer?.classList.add('has-value');

    // Try to compile as regex, fall back to substring match
    let regex;
    try {
        regex = new RegExp(query, 'i');
    } catch (e) {
        regex = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i');
    }

    // Find matching files
    const matchingFiles = new Set();
    files.forEach(file => {
        const name = file.querySelector('.tree-name')?.textContent || '';
        const fileId = file.getAttribute('data-file') || '';
        // Match against filename or full path (fileId contains path with dashes)
        const path = fileId.replace(/-/g, '/');
        if (regex.test(name) || regex.test(path)) {
            matchingFiles.add(file);
            file.classList.add('search-match');
        } else {
            file.classList.add('search-hidden');
        }
    });

    // Show parent directories of matching files, hide others
    dirs.forEach(dir => {
        const hasVisibleContent = hasVisibleDescendant(dir, matchingFiles);
        if (!hasVisibleContent) {
            dir.classList.add('search-hidden');
        } else {
            // Expand directories with matches
            if (dir.classList.contains('expandable') && !dir.classList.contains('expanded')) {
                dir.classList.add('expanded');
                const icon = dir.querySelector('.tree-dir-header .tree-icon');
                if (icon) icon.innerHTML = '&#128194;';
            }
        }
    });
}

function hasVisibleDescendant(dir, matchingFiles) {
    const children = dir.querySelector('.tree-children');
    if (!children) return false;

    // Check direct file children
    const files = children.querySelectorAll(':scope > .tree-file');
    for (const f of files) {
        if (matchingFiles.has(f)) return true;
    }

    // Check directory children recursively
    const subDirs = children.querySelectorAll(':scope > .tree-dir');
    for (const d of subDirs) {
        if (hasVisibleDescendant(d, matchingFiles)) return true;
    }

    return false;
}

function clearSearch() {
    if (searchInput) {
        searchInput.value = '';
        performSearch('');
        searchInput.focus();
    }
}

// Setup search event listeners
if (searchInput) {
    searchInput.addEventListener('input', (e) => {
        performSearch(e.target.value);
    });

    searchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            if (searchInput.value) {
                clearSearch();
                e.stopPropagation();
            }
        }
    });
}

// Keyboard navigation
document.addEventListener('keydown', (e) => {
    // Focus search on Ctrl/Cmd + F
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
        e.preventDefault();
        searchInput?.focus();
        searchInput?.select();
        return;
    }

    if (e.key === 'Escape') {
        if (activeFile) {
            document.getElementById('file-' + activeFile).style.display = 'none';
            document.querySelector('[data-file="' + activeFile + '"]')?.classList.remove('active');
            document.getElementById('welcome').style.display = 'flex';
            activeFile = null;
        }
    }
});
