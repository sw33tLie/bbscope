(function () {
  'use strict';

  // --- State ---
  var state = {
    search: '',
    sortBy: 'handle',
    sortOrder: 'asc',
    platform: '',      // comma-separated or empty
    programType: '',   // '', 'bbp', 'vdp'
    page: 1,
    perPage: 50,
  };

  var allPrograms = [];       // currently active dataset (AI or raw)
  var cachedAI = null;        // cached AI-enhanced programs
  var cachedRaw = null;       // cached raw programs
  var aiMode = false;          // true = AI enhanced, false = raw
  var container = document.getElementById('scope-table-container');
  var debounceTimer = null;

  // --- Init ---
  function init() {
    parseURLState();
    attachOuterListeners();
    attachContainerListener();
    syncOuterControls();
    render(); // show spinner immediately
    fetchPrograms(false);
  }

  function fetchPrograms(showSpinner) {
    if (showSpinner) {
      allPrograms = [];
      render(); // shows spinner
    }

    var url = aiMode ? '/api/v1/programs' : '/api/v1/programs?raw=true';
    var cache = aiMode ? cachedAI : cachedRaw;

    if (cache) {
      allPrograms = cache;
      render();
      return;
    }

    fetch(url)
      .then(function (res) { return res.json(); })
      .then(function (data) {
        var programs = data.programs || [];
        if (aiMode) {
          cachedAI = programs;
        } else {
          cachedRaw = programs;
        }
        allPrograms = programs;
        render();
      })
      .catch(function (err) {
        console.error('Failed to load programs:', err);
        container.innerHTML = '<div class="text-red-400 text-center py-8">Failed to load scope data. Please try refreshing the page.</div>';
      });
  }

  function syncAIToggle() {
    var rawBtn = document.getElementById('scope-toggle-raw');
    var aiBtn = document.getElementById('scope-toggle-ai');
    if (!rawBtn || !aiBtn) return;
    var active = 'px-3 py-1.5 text-sm font-medium cursor-pointer transition-all duration-200 bg-cyan-500 text-white';
    var inactive = 'px-3 py-1.5 text-sm font-medium cursor-pointer transition-all duration-200 bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200';
    if (aiMode) {
      rawBtn.className = inactive;
      aiBtn.className = active + ' flex items-center gap-1.5';
    } else {
      rawBtn.className = active;
      aiBtn.className = inactive + ' flex items-center gap-1.5';
    }
  }

  function parseURLState() {
    var params = new URLSearchParams(window.location.search);
    state.search = params.get('search') || '';
    state.platform = params.get('platform') || '';
    state.programType = params.get('programType') || '';
    state.page = parseInt(params.get('page'), 10) || 1;
    state.perPage = parseInt(params.get('perPage'), 10) || 50;

    // Validate
    var validPerPages = [25, 50, 100, 250, 500];
    if (validPerPages.indexOf(state.perPage) === -1) state.perPage = 50;
    if (state.programType !== 'bbp' && state.programType !== 'vdp') state.programType = '';
    if (state.page < 1) state.page = 1;
  }

  function pushState() {
    var params = new URLSearchParams();
    if (state.search) params.set('search', state.search);
    if (state.platform) params.set('platform', state.platform);
    if (state.programType) params.set('programType', state.programType);
    if (state.page > 1) params.set('page', String(state.page));
    if (state.perPage !== 50) params.set('perPage', String(state.perPage));

    var qs = params.toString();
    var url = '/scope' + (qs ? '?' + qs : '');
    history.replaceState(null, '', url);
  }

  // --- Filter pipeline ---
  function getFilteredPrograms() {
    var list = allPrograms;

    // Platform filter
    if (state.platform) {
      var platforms = state.platform.toLowerCase().split(',');
      var platSet = {};
      for (var i = 0; i < platforms.length; i++) {
        var p = platforms[i].trim();
        if (p) platSet[p] = true;
      }
      list = list.filter(function (prog) {
        return platSet[prog.platform.toLowerCase()];
      });
    }

    // Program type filter
    if (state.programType === 'bbp') {
      list = list.filter(function (prog) { return prog.is_bbp; });
    } else if (state.programType === 'vdp') {
      list = list.filter(function (prog) { return !prog.is_bbp; });
    }

    // Search by handle and target names
    if (state.search) {
      var q = state.search.toLowerCase();
      list = list.filter(function (prog) {
        if (prog.handle.toLowerCase().indexOf(q) !== -1) return true;
        var targets = prog.targets;
        if (targets) {
          for (var j = 0; j < targets.length; j++) {
            if (targets[j].toLowerCase().indexOf(q) !== -1) return true;
          }
        }
        return false;
      });
    }

    return list;
  }

  function sortPrograms(list) {
    var key = state.sortBy;
    var asc = state.sortOrder === 'asc';

    list.sort(function (a, b) {
      var va, vb;
      if (key === 'handle') {
        va = a.handle.toLowerCase();
        vb = b.handle.toLowerCase();
      } else if (key === 'platform') {
        va = a.platform.toLowerCase();
        vb = b.platform.toLowerCase();
      } else if (key === 'in_scope_count') {
        va = a.in_scope_count;
        vb = b.in_scope_count;
      } else if (key === 'out_of_scope_count') {
        va = a.out_of_scope_count;
        vb = b.out_of_scope_count;
      }

      if (va < vb) return asc ? -1 : 1;
      if (va > vb) return asc ? 1 : -1;

      // Secondary sort by handle
      if (key !== 'handle') {
        var ha = a.handle.toLowerCase();
        var hb = b.handle.toLowerCase();
        if (ha < hb) return -1;
        if (ha > hb) return 1;
      }
      return 0;
    });

    return list;
  }

  // --- Platform helpers ---
  var platformNames = {
    h1: 'HackerOne',
    bc: 'Bugcrowd',
    it: 'Intigriti',
    ywh: 'YesWeHack',
  };
  var platformColors = {
    h1: 'bg-blue-900/50 text-blue-300 border border-blue-800',
    bc: 'bg-orange-900/50 text-orange-300 border border-orange-800',
    it: 'bg-purple-900/50 text-purple-300 border border-purple-800',
    ywh: 'bg-yellow-900/50 text-yellow-300 border border-yellow-800',
  };

  function platformBadgeHTML(plat) {
    var p = plat.toLowerCase();
    var name = platformNames[p] || plat;
    var colors = platformColors[p] || 'bg-zinc-700 text-zinc-300';
    return '<span class="inline-flex items-center px-2.5 py-0.5 text-[11px] font-semibold rounded-md ' + colors + '">' + escapeHTML(name) + '</span>';
  }

  function programTypeBadgeHTML(isBBP) {
    if (isBBP) {
      return '<span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded bg-emerald-900/50 text-emerald-300 border border-emerald-800">BBP</span>';
    }
    return '<span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded bg-amber-900/50 text-amber-300 border border-amber-800">VDP</span>';
  }

  function escapeHTML(str) {
    var div = document.createElement('div');
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
  }

  // --- Render ---
  // Only replaces innerHTML of #scope-table-container. Event handling is done
  // via a single delegated listener attached once in attachContainerListener().
  function render() {
    if (allPrograms.length === 0 && container) {
      container.innerHTML =
        '<div class="flex flex-col items-center justify-center py-20 gap-4">' +
        '<div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>' +
        '<span class="text-zinc-400 text-sm">Loading scope data...</span>' +
        '</div>';
      return;
    }

    var filtered = getFilteredPrograms();
    filtered = sortPrograms(filtered);

    var totalCount = filtered.length;
    var totalPages = Math.max(1, Math.ceil(totalCount / state.perPage));
    if (state.page > totalPages) state.page = totalPages;
    if (state.page < 1) state.page = 1;

    var startIdx = (state.page - 1) * state.perPage;
    var endIdx = Math.min(startIdx + state.perPage, totalCount);
    var pagePrograms = filtered.slice(startIdx, endIdx);

    pushState();

    var html = '';

    // --- Controls row ---
    var resultsText = '';
    if (totalCount > 0) {
      resultsText = 'Showing ' + (startIdx + 1) + ' to ' + endIdx + ' of ' + totalCount + ' programs.';
    } else if (state.search) {
      resultsText = "No programs found for '" + escapeHTML(state.search) + "'.";
    } else {
      resultsText = 'No programs to display.';
    }

    html += '<div class="flex flex-col sm:flex-row justify-between items-center mb-3 gap-4">';
    html += '<div class="text-sm text-zinc-400">' + resultsText + '</div>';
    html += '<div class="flex flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:gap-3 w-full sm:w-auto">';
    html += renderPerPageSelector();
    html += '</div>';
    html += '</div>';

    // --- Table ---
    html += '<div class="overflow-x-auto rounded-none sm:rounded-xl border-y sm:border border-zinc-700/50 sm:shadow-xl sm:shadow-black/10">';
    html += '<table class="min-w-full divide-y divide-zinc-700">';
    html += '<thead class="bg-zinc-800/80">';
    html += '<tr>';
    html += renderSortHeader('Program', 'handle', 'w-2/5');
    html += renderSortHeader('Platform', 'platform', 'w-1/5');
    html += renderSortHeader('In Scope', 'in_scope_count', 'w-1/5 text-center');
    html += renderSortHeader('Out of Scope', 'out_of_scope_count', 'w-1/5 text-center');
    html += '</tr>';
    html += '</thead>';
    html += '<tbody class="bg-zinc-900/50 divide-y divide-zinc-800">';

    if (pagePrograms.length === 0) {
      html += '<tr><td colspan="4" class="text-center py-16 text-zinc-500">';
      html += '<div class="flex flex-col items-center gap-3">';
      html += '<svg class="w-12 h-12 text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>';
      html += '<span>' + (state.search ? "No programs found for '" + escapeHTML(state.search) + "'." : 'No programs to display.') + '</span>';
      html += '</div>';
      html += '</td></tr>';
    } else {
      for (var i = 0; i < pagePrograms.length; i++) {
        var p = pagePrograms[i];
        var rowBg = i % 2 === 1 ? ' bg-zinc-800/20' : '';
        var programURL = '/program/' + encodeURIComponent(p.platform.toLowerCase()) + '/' + encodeURIComponent(p.handle);
        var externalURL = escapeHTML(p.url.replace('api.yeswehack.com', 'yeswehack.com'));

        html += '<tr class="border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150 cursor-pointer' + rowBg + '" data-href="' + escapeHTML(programURL) + '">';

        // Program cell
        html += '<td class="px-4 py-3 text-sm text-zinc-200 w-2/5">';
        html += '<div class="flex items-center gap-2">';
        html += '<a href="' + externalURL + '" target="_blank" rel="noopener noreferrer" class="text-zinc-500 hover:text-cyan-400 transition-colors flex-shrink-0" onclick="event.stopPropagation()" title="Open program page">';
        html += '<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>';
        html += '</a>';
        var displayHandle = p.handle.replace(/^\/engagements\//, '');
        html += '<span class="font-medium text-zinc-100">' + escapeHTML(displayHandle) + '</span>';
        html += programTypeBadgeHTML(p.is_bbp);
        html += '</div>';
        html += '</td>';

        // Platform cell
        html += '<td class="px-4 py-3 text-sm w-1/5">' + platformBadgeHTML(p.platform) + '</td>';

        // In scope count
        html += '<td class="px-4 py-3 text-sm text-zinc-200 w-1/5 text-center"><span class="text-emerald-400 font-medium">' + p.in_scope_count + '</span></td>';

        // Out of scope count
        html += '<td class="px-4 py-3 text-sm text-zinc-200 w-1/5 text-center">' + p.out_of_scope_count + '</td>';

        html += '</tr>';
      }
    }

    html += '</tbody></table></div>';

    // --- Pagination ---
    if (totalPages > 1) {
      html += '<div class="mt-6 flex justify-center">';
      html += renderPagination(state.page, totalPages);
      html += '</div>';
    }

    container.innerHTML = html;
  }

  function renderSortHeader(label, key, extraClasses) {
    var indicator = '';
    if (state.sortBy === key) {
      indicator = state.sortOrder === 'asc' ? ' \u25B2' : ' \u25BC';
    }
    return '<th class="px-4 py-3 text-xs font-semibold text-zinc-500 uppercase tracking-wider ' + extraClasses + '">' +
      '<a href="#" class="hover:text-zinc-200 transition-colors scope-sort" data-sort="' + key + '">' + label + indicator + '</a>' +
      '</th>';
  }

  function renderPerPageSelector() {
    var options = [25, 50, 100, 250, 500];
    var html = '<div class="w-full sm:w-auto flex items-center justify-center sm:justify-start gap-1 sm:gap-2 text-sm">';
    html += '<label for="perPageSelect" class="text-zinc-400 whitespace-nowrap">Items per page:</label>';
    html += '<select id="perPageSelect" class="px-2.5 py-1.5 border border-zinc-700 rounded-lg focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500 text-sm bg-zinc-800/50 text-zinc-200 transition-colors duration-200 scope-perpage">';
    for (var i = 0; i < options.length; i++) {
      var sel = options[i] === state.perPage ? ' selected' : '';
      html += '<option value="' + options[i] + '"' + sel + '>' + options[i] + ' items</option>';
    }
    html += '</select>';
    html += '</div>';
    return html;
  }

  function renderPagination(currentPage, totalPages) {
    var html = '<nav class="inline-flex items-center gap-1 bg-zinc-800/30 rounded-full px-1 py-1">';

    // Previous
    if (currentPage <= 1) {
      html += '<span class="px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-600 cursor-not-allowed"><span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span></span>';
    } else {
      html += '<a href="#" class="px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200 scope-page" data-page="' + (currentPage - 1) + '"><span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span></a>';
    }

    // Page numbers
    var start = Math.max(1, currentPage - 2);
    var end = Math.min(totalPages, currentPage + 2);

    if (start > 1) {
      html += '<a href="#" class="px-3 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200 scope-page" data-page="1">1</a>';
      if (start > 2) {
        html += '<span class="px-1 sm:px-2 py-1.5 text-sm text-zinc-600">...</span>';
      }
    }

    for (var i = start; i <= end; i++) {
      var hideOnMobile = '';
      if (i !== currentPage && (i < currentPage - 1 || i > currentPage + 1)) {
        hideOnMobile = ' hidden sm:inline-flex';
      }
      if (i === currentPage) {
        html += '<span class="px-3 py-1.5 text-sm font-medium rounded-full bg-cyan-600 text-white shadow-md shadow-cyan-500/20' + hideOnMobile + '">' + i + '</span>';
      } else {
        html += '<a href="#" class="px-3 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200 scope-page' + hideOnMobile + '" data-page="' + i + '">' + i + '</a>';
      }
    }

    if (end < totalPages) {
      if (end < totalPages - 1) {
        html += '<span class="px-1 sm:px-2 py-1.5 text-sm text-zinc-600">...</span>';
      }
      html += '<a href="#" class="px-3 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200 scope-page" data-page="' + totalPages + '">' + totalPages + '</a>';
    }

    // Next
    if (currentPage >= totalPages) {
      html += '<span class="px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-600 cursor-not-allowed"><span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span></span>';
    } else {
      html += '<a href="#" class="px-2 py-1.5 text-sm font-medium rounded-full bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 transition-all duration-200 scope-page" data-page="' + (currentPage + 1) + '"><span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span></a>';
    }

    html += '</nav>';
    return html;
  }

  // --- Event listeners ---
  // Single delegated listener on the container — handles all clicks on elements
  // that get replaced by render() (sort headers, pagination, rows, per-page select).
  // Attached once, never stacks.
  function attachContainerListener() {
    container.addEventListener('click', function (e) {
      // Sort headers
      var sortEl = e.target.closest('.scope-sort');
      if (sortEl) {
        e.preventDefault();
        var key = sortEl.getAttribute('data-sort');
        if (state.sortBy === key) {
          state.sortOrder = state.sortOrder === 'asc' ? 'desc' : 'asc';
        } else {
          state.sortBy = key;
          state.sortOrder = 'asc';
        }
        state.page = 1;
        render();
        return;
      }

      // Pagination
      var pageEl = e.target.closest('.scope-page');
      if (pageEl) {
        e.preventDefault();
        state.page = parseInt(pageEl.getAttribute('data-page'), 10);
        render();
        window.scrollTo({ top: container.offsetTop - 80, behavior: 'smooth' });
        return;
      }

      // Row navigation
      var row = e.target.closest('tr[data-href]');
      if (row && !e.target.closest('a')) {
        window.location.href = row.getAttribute('data-href');
        return;
      }
    });

    // Per-page select (uses change event, also delegated)
    container.addEventListener('change', function (e) {
      if (e.target.classList.contains('scope-perpage')) {
        state.perPage = parseInt(e.target.value, 10);
        state.page = 1;
        render();
      }
    });
  }

  // One-time listeners on elements outside the container that never get replaced.
  function attachOuterListeners() {
    // Search input
    var searchInput = document.getElementById('scope-search-input');
    if (searchInput) {
      searchInput.addEventListener('input', function () {
        clearTimeout(debounceTimer);
        var val = this.value;
        debounceTimer = setTimeout(function () {
          state.search = val.trim();
          state.page = 1;
          render();
        }, 200);
      });
      searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') {
          e.preventDefault();
          clearTimeout(debounceTimer);
          state.search = this.value.trim();
          state.page = 1;
          render();
        }
      });
    }

    // Clear search
    var clearBtn = document.getElementById('scope-search-clear');
    if (clearBtn) {
      clearBtn.addEventListener('click', function () {
        state.search = '';
        state.page = 1;
        var input = document.getElementById('scope-search-input');
        if (input) input.value = '';
        render();
      });
    }

    // Platform filter apply
    var platformApplyBtn = document.getElementById('platform-apply-btn');
    if (platformApplyBtn) {
      platformApplyBtn.addEventListener('click', function () {
        applyPlatformFilter();
      });
    }

    // Platform "All" button
    var platformAllBtn = document.getElementById('platform-all-btn');
    if (platformAllBtn) {
      platformAllBtn.addEventListener('click', function () {
        var checkboxes = document.querySelectorAll('#platform-dropdown-menu input[type=checkbox]');
        for (var i = 0; i < checkboxes.length; i++) {
          checkboxes[i].checked = false;
        }
        applyPlatformFilter();
      });
    }

    // Program type pills
    var typePills = document.querySelectorAll('.scope-type-pill');
    for (var i = 0; i < typePills.length; i++) {
      typePills[i].addEventListener('click', function (e) {
        e.preventDefault();
        state.programType = this.getAttribute('data-type');
        state.page = 1;
        syncOuterControls();
        render();
      });
    }

    // Data source toggle (Raw / AI Enhanced)
    var scopeToggleRaw = document.getElementById('scope-toggle-raw');
    var scopeToggleAI = document.getElementById('scope-toggle-ai');
    if (scopeToggleRaw) {
      scopeToggleRaw.addEventListener('click', function (e) {
        e.preventDefault();
        if (!aiMode) return;
        aiMode = false;
        syncAIToggle();
        fetchPrograms(true);
      });
    }
    if (scopeToggleAI) {
      scopeToggleAI.addEventListener('click', function (e) {
        e.preventDefault();
        if (aiMode) return;
        aiMode = true;
        syncAIToggle();
        fetchPrograms(true);
      });
    }

    // Platform dropdown toggle
    var dropdownBtn = document.getElementById('platform-dropdown-btn');
    var dropdownMenu = document.getElementById('platform-dropdown-menu');
    if (dropdownBtn && dropdownMenu) {
      dropdownBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        dropdownMenu.classList.toggle('hidden');
      });
    }

    // Close platform dropdown when clicking outside
    document.addEventListener('click', function (e) {
      var filter = document.getElementById('platform-filter');
      var menu = document.getElementById('platform-dropdown-menu');
      if (filter && menu && !filter.contains(e.target)) {
        menu.classList.add('hidden');
      }
    });
  }

  function applyPlatformFilter() {
    var checked = [];
    var checkboxes = document.querySelectorAll('#platform-dropdown-menu input[type=checkbox]:checked');
    for (var i = 0; i < checkboxes.length; i++) {
      checked.push(checkboxes[i].value);
    }
    state.platform = checked.join(',');
    state.page = 1;

    var menu = document.getElementById('platform-dropdown-menu');
    if (menu) menu.classList.add('hidden');

    updatePlatformButtonLabel();
    render();
  }

  function updatePlatformButtonLabel() {
    var btn = document.getElementById('platform-dropdown-btn');
    if (!btn) return;

    if (!state.platform) {
      btn.firstChild.textContent = 'All Platforms';
      return;
    }

    var platforms = state.platform.split(',');
    var names = [];
    for (var i = 0; i < platforms.length; i++) {
      names.push(platformNames[platforms[i]] || platforms[i]);
    }
    btn.firstChild.textContent = names.length <= 2 ? names.join(', ') : names.length + ' platforms';
  }

  // Sync outer controls (pills, search, platform checkboxes) to match current state.
  // Called once on init and when state changes from outer control interactions.
  function syncOuterControls() {
    // Program type pills
    var pills = document.querySelectorAll('.scope-type-pill');
    for (var i = 0; i < pills.length; i++) {
      var pillType = pills[i].getAttribute('data-type');
      if (pillType === state.programType) {
        pills[i].className = 'scope-type-pill px-4 py-1.5 text-sm font-medium rounded-lg transition-all duration-200 bg-cyan-500 text-white shadow-md shadow-cyan-500/20 cursor-pointer';
      } else {
        pills[i].className = 'scope-type-pill px-4 py-1.5 text-sm font-medium rounded-lg transition-all duration-200 bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 border border-zinc-700/50 cursor-pointer';
      }
    }

    // Search input
    var searchInput = document.getElementById('scope-search-input');
    if (searchInput) searchInput.value = state.search;

    // Platform checkboxes
    if (state.platform) {
      var selected = {};
      var parts = state.platform.split(',');
      for (var j = 0; j < parts.length; j++) {
        selected[parts[j].trim()] = true;
      }
      var checkboxes = document.querySelectorAll('#platform-dropdown-menu input[type=checkbox]');
      for (var k = 0; k < checkboxes.length; k++) {
        checkboxes[k].checked = !!selected[checkboxes[k].value];
      }
    }
    updatePlatformButtonLabel();
  }

  // --- Kick off ---
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
