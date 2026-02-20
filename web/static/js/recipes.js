// Recipe management functions

// Global variables
let recipeTags = [];

function initRecipeTagInput() {
    const input = document.getElementById('recipe-tags-input');
    if (!input) return; // Not on a page with recipe modal

    const chipsContainer = document.getElementById('recipe-tags-chips');
    const hidden = document.getElementById('recipe-tags-hidden');

    // Remove existing listeners to avoid duplicates if re-initialized?
    // Actually simpler to just add if not present, but init is called on DOMContentLoaded.
    // We'll rely on idempotency or page isolation.

    input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ',') {
            e.preventDefault();
            const tag = this.value.trim().replace(/,/g, '');
            if (tag && !recipeTags.includes(tag)) {
                recipeTags.push(tag);
                renderRecipeTagChips();
            }
            this.value = '';
        }
    });

    // Load suggestions
    fetch('/tags/suggestions')
        .then(r => r.text())
        .then(html => {
            const suggestions = document.getElementById('recipe-tag-suggestions');
            if (suggestions) suggestions.innerHTML = html;
        });
}

function renderRecipeTagChips() {
    const container = document.getElementById('recipe-tags-chips');
    const hidden = document.getElementById('recipe-tags-hidden');
    if (!container || !hidden) return;

    container.innerHTML = '';
    recipeTags.forEach(tag => {
        const chip = document.createElement('span');
        chip.className = 'tag is-info is-medium mr-1 mb-1';
        chip.innerHTML = tag + ' <button class="button is-small is-delete" type="button" onclick="removeRecipeTag(\'' + tag + '\')"></button>';
        // Note: button type=button to avoid submitting form
        // Also 'delete' class on button usually works but explicit button is safer for onclick
        // Bulma uses <button class="delete"></button>
        chip.innerHTML = tag + ' <button class="delete is-small" onclick="removeRecipeTag(\'' + tag + '\')"></button>';
        container.appendChild(chip);
    });
    hidden.value = recipeTags.join(',');
}

function removeRecipeTag(tag) {
    recipeTags = recipeTags.filter(t => t !== tag);
    renderRecipeTagChips();
}

// Modal functions
function openRecipeModal() {
    document.getElementById('recipe-modal').classList.add('is-active');
    document.getElementById('recipe-modal-title').textContent = 'Add New Recipe';
    document.getElementById('recipe-form').reset();
    document.getElementById('recipe-edit-id').value = '';
    document.getElementById('recipe-thumbnail').value = '';
    document.getElementById('thumbnail-preview-container').style.display = 'none';
    document.getElementById('image-preview-grid').innerHTML = '';
    document.getElementById('recipe-images-name').textContent = 'No images selected';
    recipeTags = [];
    renderRecipeTagChips();
    // Re-init input just in case of dynamic loading? No, DOM is static usually.
}

function closeRecipeModal() {
    document.getElementById('recipe-modal').classList.remove('is-active');
}

function openImportRecipeModal() {
    const modal = document.getElementById('import-recipe-modal');
    if (modal) {
        modal.classList.add('is-active');
        document.getElementById('import-url').value = '';
        document.getElementById('import-status').style.display = 'none';
    }
}

function closeImportRecipeModal() {
    const modal = document.getElementById('import-recipe-modal');
    if (modal) modal.classList.remove('is-active');
}

// Import from URL
function importRecipeFromURL() {
    const url = document.getElementById('import-url').value.trim();
    if (!url) return;

    const status = document.getElementById('import-status');
    status.style.display = 'block';
    status.className = 'notification is-info is-light';
    status.innerHTML = '<i class="fas fa-spinner fa-pulse mr-2"></i> Importing recipe...';

    fetch('/recipes/import?url=' + encodeURIComponent(url))
        .then(r => {
            if (!r.ok) throw new Error('Failed to import recipe');
            return r.json();
        })
        .then(data => {
            closeImportRecipeModal();
            openRecipeModal();

            document.getElementById('recipe-title').value = data.title || '';
            document.getElementById('recipe-ingredients').value = data.ingredients || '';
            document.getElementById('recipe-instructions').value = data.instructions || '';
            document.getElementById('recipe-source-url').value = data.source_url || '';

            if (data.thumbnail) {
                document.getElementById('recipe-thumbnail').value = data.thumbnail;
                document.getElementById('thumbnail-preview').src = data.thumbnail;
                document.getElementById('thumbnail-preview-container').style.display = 'block';
            }
        })
        .catch(err => {
            status.className = 'notification is-danger is-light';
            status.innerHTML = '<i class="fas fa-exclamation-triangle mr-2"></i> ' + err.message;
        });
}

// Image preview
function previewRecipeImages(input) {
    const grid = document.getElementById('image-preview-grid');
    grid.innerHTML = '';
    const nameSpan = document.getElementById('recipe-images-name');

    if (input.files.length === 0) {
        nameSpan.textContent = 'No images selected';
        return;
    }

    nameSpan.textContent = input.files.length + ' image(s) selected';

    Array.from(input.files).forEach(file => {
        const reader = new FileReader();
        reader.onload = function (e) {
            const col = document.createElement('div');
            col.className = 'column is-3';
            col.innerHTML = '<figure class="image is-4by3"><img src="' + e.target.result + '" style="object-fit: cover; border-radius: 4px;"></figure>';
            grid.appendChild(col);
        };
        reader.readAsDataURL(file);
    });
}

// Submit recipe
function submitRecipe() {
    const form = document.getElementById('recipe-form');
    const formData = new FormData(form);
    const editId = document.getElementById('recipe-edit-id').value;

    const url = editId ? '/recipes/' + editId : '/recipes';

    fetch(url, {
        method: 'POST',
        body: formData,
        headers: { 'HX-Request': 'true' }
    })
        .then(r => r.text())
        .then(html => {
            // If we are on the recipes list page, update the grid.
            const target = document.getElementById('main-search-target');
            if (target) {
                target.innerHTML = html;
            } else {
                // If we are on detail page, reload to show changes
                window.location.reload();
            }
            closeRecipeModal();
        })
        .catch(err => {
            console.error('Error saving recipe:', err);
        });
}

// Edit recipe
function editRecipe(id) {
    // UPDATED: Explicitly request JSON
    fetch('/recipes/' + id, {
        headers: {
            'Accept': 'application/json'
        }
    })
        .then(r => {
            if (!r.ok) throw new Error('Network response was not ok');
            return r.json();
        })
        .then(recipe => {
            openRecipeModal();
            document.getElementById('recipe-modal-title').textContent = 'Edit Recipe';
            document.getElementById('recipe-edit-id').value = id;
            document.getElementById('recipe-title').value = recipe.title;
            document.getElementById('recipe-ingredients').value = recipe.ingredients;
            document.getElementById('recipe-instructions').value = recipe.instructions;
            document.getElementById('recipe-source-url').value = recipe.source_url || '';
            document.getElementById('recipe-notes').value = recipe.notes || '';
            document.getElementById('recipe-thumbnail').value = recipe.thumbnail || '';

            if (recipe.thumbnail) {
                document.getElementById('thumbnail-preview').src = recipe.thumbnail;
                document.getElementById('thumbnail-preview-container').style.display = 'block';
            } else {
                document.getElementById('thumbnail-preview-container').style.display = 'none';
            }

            recipeTags = recipe.tags || [];
            renderRecipeTagChips();
        })
        .catch(error => console.error('Error fetching recipe for edit:', error));
}


// Init
document.addEventListener('DOMContentLoaded', initRecipeTagInput);
