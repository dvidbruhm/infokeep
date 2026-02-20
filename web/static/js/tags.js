class TagInput {
    constructor(container) {
        this.container = container;
        this.container._tagInput = this; // Store instance on DOM element
        this.tags = [];
        this.suggestions = [];

        // Elements
        this.chipsContainer = container.querySelector('.tag-chips');
        this.input = container.querySelector('.tag-entry');
        this.hiddenInput = container.querySelector('input[type="hidden"]');
        this.suggestionsContainer = container.querySelector('.tag-suggestions');

        // Initial Load
        const initialTags = container.dataset.existingTags || "";
        this.parseInitialTags(initialTags);

        // Event Listeners
        this.input.addEventListener('keydown', this.handleKeyDown.bind(this));
        this.input.addEventListener('input', this.handleInput.bind(this));

        // Close suggestions on click outside
        document.addEventListener('click', (e) => {
            if (!this.container.contains(e.target)) {
                this.suggestionsContainer.style.display = 'none';
            }
        });

        // Fetch suggestions
        this.fetchSuggestions();
    }

    setTags(tags) {
        this.tags = [];
        if (Array.isArray(tags)) {
            tags.forEach(t => this.addTag(t, false)); // false = don't focus
        } else if (typeof tags === 'string') {
            this.parseInitialTags(tags);
        }
    }

    parseInitialTags(tagString) {
        // Remove brackets if they exist (go standard formatting)
        let clean = tagString.replace(/^\[|\]$/g, '');
        if (!clean) return;

        clean.split(' ').forEach(tag => {
            if (tag.trim()) this.addTag(tag.trim());
        });
    }

    fetchSuggestions() {
        fetch('/api/tags')
            .then(res => res.json())
            .then(data => {
                this.suggestions = data || [];
            })
            .catch(err => console.error("Failed to load tags", err));
    }

    addTag(tag, focus = true) {
        tag = tag.trim().toLowerCase();
        if (!tag || this.tags.includes(tag)) return;

        this.tags.push(tag);
        this.renderChips();
        this.updateHiddenInput();
        this.input.value = '';
        this.suggestionsContainer.style.display = 'none';
        if (focus) this.input.focus();
    }

    removeTag(tag) {
        this.tags = this.tags.filter(t => t !== tag);
        this.renderChips();
        this.updateHiddenInput();
    }

    updateHiddenInput() {
        this.hiddenInput.value = this.tags.join(',');
    }

    renderChips() {
        this.chipsContainer.innerHTML = '';
        this.tags.forEach(tag => {
            const chip = document.createElement('span');
            chip.className = 'tag is-info is-light is-medium';
            chip.style.marginRight = '4px';
            chip.style.marginBottom = '4px';

            const text = document.createElement('span');
            text.textContent = tag;
            chip.appendChild(text);

            const close = document.createElement('button');
            close.className = 'delete is-small';
            close.onclick = () => this.removeTag(tag);
            chip.appendChild(close);

            this.chipsContainer.appendChild(chip);
        });
    }

    handleKeyDown(e) {
        if (e.key === 'Enter') {
            e.preventDefault(); // Prevent form submission
            this.addTag(this.input.value);
        } else if (e.key === 'Backspace' && this.input.value === '' && this.tags.length > 0) {
            this.removeTag(this.tags[this.tags.length - 1]);
        }
    }

    handleInput(e) {
        const val = this.input.value.toLowerCase();
        if (val.length < 1) {
            this.suggestionsContainer.style.display = 'none';
            return;
        }

        const matches = this.suggestions.filter(t => t.toLowerCase().includes(val) && !this.tags.includes(t));
        this.renderSuggestions(matches);
    }

    renderSuggestions(matches) {
        if (matches.length === 0) {
            this.suggestionsContainer.style.display = 'none';
            return;
        }

        this.suggestionsContainer.innerHTML = '';
        matches.forEach(tag => {
            const div = document.createElement('div');
            div.className = 'suggestion-item';
            div.textContent = tag;
            div.onclick = () => this.addTag(tag);
            this.suggestionsContainer.appendChild(div);
        });
        this.suggestionsContainer.style.display = 'block';
    }
}

// Initialize on load and after HTMX swaps
function initTagInputs() {
    document.querySelectorAll('.tag-input-container').forEach(container => {
        if (!container.dataset.initialized) {
            new TagInput(container);
            container.dataset.initialized = "true";
        }
    });
}

document.addEventListener('DOMContentLoaded', initTagInputs);
document.addEventListener('htmx:afterSwap', initTagInputs);
