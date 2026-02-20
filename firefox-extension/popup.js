const API_BASE = "http://localhost:8080/api";
let apiToken = "";

function showStatus(msg, isError = false) {
    const status = document.getElementById("status");
    if (!status) return;
    status.textContent = msg;
    status.className = isError ? "status error" : "status";
}

function apiHeaders() {
    return {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${apiToken}`
    };
}

// ---- Tag Input ----
class TagInput {
    constructor(container) {
        this.container = container;
        this.tags = [];
        this.suggestions = [];
        this.chipsContainer = container.querySelector(".tag-chips");
        this.input = container.querySelector(".tag-entry");
        this.hiddenInput = container.querySelector("input[type=\"hidden\"]");
        this.suggestionsContainer = container.querySelector(".tag-suggestions");

        this.input.addEventListener("keydown", this.handleKeyDown.bind(this));
        this.input.addEventListener("input", this.handleInput.bind(this));
        document.addEventListener("click", (e) => {
            if (!this.container.contains(e.target))
                this.suggestionsContainer.style.display = "none";
        });
    }
    setSuggestions(s) { this.suggestions = s; }
    addTag(tag) {
        tag = tag.trim().toLowerCase();
        if (!tag || this.tags.includes(tag)) return;
        this.tags.push(tag);
        this.renderChips(); this.updateHiddenInput();
        this.input.value = "";
        this.suggestionsContainer.style.display = "none";
        this.input.focus();
    }
    removeTag(tag) {
        this.tags = this.tags.filter(t => t !== tag);
        this.renderChips(); this.updateHiddenInput();
    }
    updateHiddenInput() { this.hiddenInput.value = this.tags.join(","); }
    renderChips() {
        this.chipsContainer.innerHTML = "";
        this.tags.forEach(tag => {
            const chip = document.createElement("span");
            chip.className = "tag-chip";
            const text = document.createElement("span");
            text.textContent = tag;
            chip.appendChild(text);
            const close = document.createElement("button");
            close.className = "delete-tag";
            close.innerHTML = "&times;";
            close.onclick = () => this.removeTag(tag);
            chip.appendChild(close);
            this.chipsContainer.appendChild(chip);
        });
    }
    handleKeyDown(e) {
        if (e.key === "Enter") { e.preventDefault(); this.addTag(this.input.value); }
        else if (e.key === "Backspace" && this.input.value === "" && this.tags.length > 0)
            this.removeTag(this.tags[this.tags.length - 1]);
    }
    handleInput() {
        const val = this.input.value.toLowerCase();
        if (val.length < 1) { this.suggestionsContainer.style.display = "none"; return; }
        const matches = this.suggestions.filter(t => t.toLowerCase().includes(val) && !this.tags.includes(t));
        if (matches.length === 0) { this.suggestionsContainer.style.display = "none"; return; }
        this.suggestionsContainer.innerHTML = "";
        matches.forEach(tag => {
            const div = document.createElement("div");
            div.className = "suggestion-item";
            div.textContent = tag;
            div.onclick = () => this.addTag(tag);
            this.suggestionsContainer.appendChild(div);
        });
        this.suggestionsContainer.style.display = "block";
    }
}

let tagInputs = {};

function switchTab(tabId) {
    document.querySelectorAll(".tab-btn").forEach(t => t.classList.remove("active"));
    document.querySelectorAll(".tab-content").forEach(c => c.classList.remove("active"));
    const btn = document.querySelector(`[data-tab="${tabId}"]`);
    if (btn) btn.classList.add("active");
    const content = document.getElementById(tabId);
    if (content) content.classList.add("active");
}

function populateLists(lists) {
    const select = document.getElementById("list-select");
    while (select.options.length > 1) select.remove(1);
    lists.forEach(list => {
        const option = document.createElement("option");
        option.value = list.id;
        option.textContent = list.title;
        select.appendChild(option);
    });
}

function fetchTagSuggestions() {
    fetch(`${API_BASE}/tags`, { headers: apiHeaders() })
        .then(r => r.ok ? r.json() : [])
        .then(tags => Object.values(tagInputs).forEach(ti => ti.setSuggestions(tags || [])))
        .catch(() => { });
}

function initExtension() {
    fetch(`${API_BASE}/rated-lists`, { headers: apiHeaders() })
        .then(response => {
            if (response.status === 401) throw new Error("invalid_token");
            if (!response.ok) throw new Error("connect_failed");
            return response.json();
        })
        .then(lists => {
            populateLists(lists);
            fetchTagSuggestions();
        })
        .catch(err => {
            if (err.message === "invalid_token") {
                showStatus("Invalid token. Go to ⚙ Settings to update it.", true);
                switchTab("settings");
            }
            // Silently ignore network errors — user can still save items
        });

    if (typeof browser !== "undefined" && browser.tabs) {
        browser.tabs.query({ active: true, currentWindow: true }).then(tabs => {
            const activeTab = tabs[0];
            if (document.getElementById("bookmark-title"))
                document.getElementById("bookmark-title").value = activeTab.title || "";
            if (document.getElementById("bookmark-url"))
                document.getElementById("bookmark-url").value = activeTab.url || "";
        });
    }
}

function saveToken() {
    const input = document.getElementById("api-token-input").value.trim();
    if (!input) {
        showStatus("Please enter a token.", true);
        return;
    }
    apiToken = input;
    browser.storage.local.set({ apiToken: input }).then(() => {
        showStatus("Token saved! Connecting...");
        switchTab("bookmark");
        initExtension();
    });
}

function submitData(endpoint, data) {
    if (!apiToken) {
        showStatus("Please set your API token in ⚙ Settings.", true);
        switchTab("settings");
        return;
    }
    const btn = document.querySelector(".tab-content.active .submit-btn");
    const originalText = btn ? btn.textContent : "";
    if (btn) { btn.textContent = "Saving..."; btn.disabled = true; }

    fetch(`${API_BASE}${endpoint}`, {
        method: "POST",
        headers: apiHeaders(),
        body: JSON.stringify(data)
    })
        .then(response => {
            if (response.status === 401) throw new Error("Please set your API token in ⚙ Settings.");
            if (!response.ok) throw new Error("Server error");
            return response.json();
        })
        .then(() => {
            showStatus("Saved successfully!");
            document.querySelector(".tab-content.active form")?.reset();
            const chips = document.querySelector(".tab-content.active .tag-chips");
            if (chips) chips.innerHTML = "";
            setTimeout(() => window.close(), 1500);
        })
        .catch(err => {
            showStatus(err.message || "Error saving item.", true);
        })
        .finally(() => {
            if (btn) { btn.textContent = originalText; btn.disabled = false; }
        });
}

document.addEventListener("DOMContentLoaded", () => {
    // Init Tag Inputs
    tagInputs.bookmark = new TagInput(document.getElementById("bookmark-tags-container"));
    tagInputs.recipe = new TagInput(document.getElementById("recipe-tags-container"));
    tagInputs.note = new TagInput(document.getElementById("note-tags-container"));

    // Tabs
    document.querySelectorAll(".tab-btn").forEach(tab => {
        tab.addEventListener("click", () => switchTab(tab.dataset.tab));
    });

    // Forms
    document.getElementById("bookmark-form").addEventListener("submit", (e) => {
        e.preventDefault();
        submitData("/bookmarks", {
            title: document.getElementById("bookmark-title").value,
            url: document.getElementById("bookmark-url").value,
            description: document.getElementById("bookmark-desc").value,
            tags: document.getElementById("bookmark-tags").value
        });
    });

    document.getElementById("recipe-form").addEventListener("submit", (e) => {
        e.preventDefault();
        if (typeof browser !== "undefined" && browser.tabs) {
            browser.tabs.query({ active: true, currentWindow: true }).then(tabs => {
                submitData("/recipes/clipper", {
                    url: tabs[0].url,
                    tags: document.getElementById("recipe-tags").value
                });
            });
        }
    });

    document.getElementById("note-form").addEventListener("submit", (e) => {
        e.preventDefault();
        submitData("/notes", {
            title: document.getElementById("note-title").value,
            content: document.getElementById("note-content").value,
            tags: document.getElementById("note-tags").value
        });
    });

    document.getElementById("list-form").addEventListener("submit", (e) => {
        e.preventDefault();
        const listId = document.getElementById("list-select").value;
        if (!listId) { showStatus("Please select a list", true); return; }
        submitData(`/rated-lists/${listId}/items`, {
            title: document.getElementById("list-item-title").value,
            score: parseInt(document.getElementById("list-item-score").value),
            note: document.getElementById("list-item-note").value
        });
    });

    // Wire up save token button
    const saveTokenBtn = document.getElementById("save-token-btn");
    if (saveTokenBtn) {
        saveTokenBtn.addEventListener("click", saveToken);
    }

    // Load stored token, then init or prompt
    browser.storage.local.get("apiToken").then(result => {
        apiToken = result.apiToken || "";
        if (apiToken) {
            document.getElementById("api-token-input").value = apiToken;
            initExtension();
        } else {
            showStatus("Please set your API token in ⚙ Settings.", true);
            switchTab("settings");
        }
    }).catch(err => {
        showStatus("Extension storage error: " + err.message, true);
        switchTab("settings");
    });
});
