(function () {
  'use strict';

  // --- State ---
  var state = {
    search: '',
    platform: '',
    since: '',
    until: '',
    page: 1,
    perPage: 25,
  };

  var container = document.getElementById('updates-table-container');
  var debounceTimer = null;

  // --- Init ---
  function init() {
    parseURLState();
    syncOuterControls();
    fetchAndRender();
  }

  function parseURLState() {
    var params = new URLSearchParams(window.location.search);
    state.search = params.get('search') || '';
    state.platform = params.get('platform') || '';
    state.since = params.get('since') || '';
    state.until = params.get('until') || '';
    state.page = parseInt(params.get('page'), 10) || 1;
    state.perPage = parseInt(params.get('perPage'), 10) || 25;
    if ([25, 50, 100, 250].indexOf(state.perPage) === -1) state.perPage = 25;
  }

  function pushURL() {
    var params = new URLSearchParams();
    if (state.page > 1) params.set('page', state.page);
    if (state.perPage !== 25) params.set('perPage', state.perPage);
    if (state.platform) params.set('platform', state.platform);
    if (state.search) params.set('search', state.search);
    if (state.since) params.set('since', state.since);
    if (state.until) params.set('until', state.until);
    var qs = params.toString();
    var url = '/updates' + (qs ? '?' + qs : '');
    history.replaceState(null, '', url);
  }

  // --- Sync outer controls (platform tabs, date dropdown, search) ---
  function syncOuterControls() {
    // Platform tabs
    var tabs = document.querySelectorAll('.updates-platform-tab');
    tabs.forEach(function (tab) {
      var val = tab.getAttribute('data-platform');
      if (val === state.platform) {
        tab.className = 'updates-platform-tab px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200 bg-cyan-500 text-white shadow-md shadow-cyan-500/20 cursor-pointer';
      } else {
        tab.className = 'updates-platform-tab px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200 bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200 border border-zinc-700/50 cursor-pointer';
      }
    });

    // Date dropdown
    var dateSel = document.getElementById('updates-date-select');
    if (dateSel) {
      var presets = ['', 'today', 'yesterday', '7d', '30d', '90d', '1y'];
      if (presets.indexOf(state.since) !== -1 && !state.until) {
        dateSel.value = state.since;
        document.getElementById('updates-date-range').classList.add('hidden');
      } else if (state.since) {
        dateSel.value = 'custom';
        document.getElementById('updates-date-range').classList.remove('hidden');
        document.getElementById('updates-date-from').value = state.since;
        document.getElementById('updates-date-to').value = state.until || '';
      }
    }

    // Search input
    var searchInput = document.getElementById('updates-search-input');
    if (searchInput) searchInput.value = state.search;

    // Per-page select
    var ppSel = document.getElementById('updates-perpage-select');
    if (ppSel) ppSel.value = state.perPage;
  }

  // --- Fetch & Render ---
  function fetchAndRender() {
    // Show spinner
    container.innerHTML = '<div class="flex flex-col items-center justify-center py-20 gap-4">' +
      '<div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>' +
      '<span class="text-zinc-400 text-sm">Loading updates...</span></div>';

    var params = new URLSearchParams();
    params.set('page', state.page);
    params.set('per_page', state.perPage);
    if (state.platform) params.set('platform', state.platform);
    if (state.search) params.set('search', state.search);
    if (state.since) params.set('since', state.since);
    if (state.until) params.set('until', state.until);

    fetch('/api/v1/updates?' + params.toString())
      .then(function (res) { return res.json(); })
      .then(function (data) {
        renderTable(data);
      })
      .catch(function () {
        container.innerHTML = '<div class="text-center py-16 text-red-400">Failed to load updates. Please try again.</div>';
      });
  }

  // --- Render ---
  function renderTable(data) {
    var updates = data.updates || [];
    var totalPages = data.total_pages || 0;
    var html = '';

    // Table
    html += '<div class="overflow-x-auto rounded-none sm:rounded-xl border-y sm:border border-zinc-700/50 sm:shadow-xl sm:shadow-black/10">';
    html += '<table class="min-w-full divide-y divide-zinc-700">';
    html += '<thead class="bg-zinc-800/80"><tr>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider">Change</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider">Asset</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28">Category</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28">Scope</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider">Program</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-28">Platform</th>';
    html += '<th class="px-4 py-3 text-left text-xs font-semibold text-zinc-500 uppercase tracking-wider w-36">Time</th>';
    html += '</tr></thead>';
    html += '<tbody class="bg-zinc-900/50 divide-y divide-zinc-800">';

    if (updates.length === 0) {
      var msg = state.search ? 'No updates found for search \'' + escapeHTML(state.search) + '\'.' : 'No updates to display.';
      html += '<tr><td colspan="7" class="text-center py-16 text-zinc-500">';
      html += '<div class="flex flex-col items-center gap-3">';
      html += '<svg class="w-12 h-12 text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>';
      html += '<span>' + msg + '</span></div></td></tr>';
    } else {
      for (var i = 0; i < updates.length; i++) {
        var u = updates[i];
        var rowClass = 'border-b border-zinc-800/50 hover:bg-zinc-800/50 transition-colors duration-150';
        if (i % 2 === 1) rowClass += ' bg-zinc-800/20';
        var isProgramLevel = u.type === 'program_added' || u.type === 'program_removed';

        html += '<tr class="' + rowClass + '">';
        html += '<td class="px-4 py-3 text-sm">' + changeTypeBadge(u.type) + '</td>';

        if (isProgramLevel) {
          html += '<td class="px-4 py-3 text-sm text-zinc-500">&mdash;</td>';
          html += '<td class="px-4 py-3 text-sm text-zinc-500">&mdash;</td>';
          html += '<td class="px-4 py-3 text-sm text-zinc-500">&mdash;</td>';
        } else {
          html += '<td class="px-4 py-3 text-sm text-zinc-200 break-all">' + escapeHTML(u.target || '') + '</td>';
          html += '<td class="px-4 py-3 text-sm">' + categoryBadge(u.category || 'OTHER') + '</td>';
          html += '<td class="px-4 py-3 text-sm">' + scopeBadge(u.scope_type) + '</td>';
        }

        html += '<td class="px-4 py-3 text-sm">' + programCell(u) + '</td>';
        html += '<td class="px-4 py-3 text-sm">' + platformBadge(u.platform) + '</td>';
        html += '<td class="px-4 py-3 text-sm text-zinc-400 whitespace-nowrap">' + formatTime(u.timestamp) + '</td>';
        html += '</tr>';
      }
    }

    html += '</tbody></table></div>';

    // Pagination
    if (totalPages > 1) {
      html += '<div class="mt-6 flex justify-center">' + renderPagination(state.page, totalPages) + '</div>';
    }

    container.innerHTML = html;

    // Attach pagination click handlers
    container.querySelectorAll('[data-page]').forEach(function (el) {
      el.addEventListener('click', function (e) {
        e.preventDefault();
        state.page = parseInt(el.getAttribute('data-page'), 10);
        pushURL();
        fetchAndRender();
        // Scroll to top of container
        window.scrollTo({ top: 0, behavior: 'smooth' });
      });
    });
  }

  // --- Badge helpers ---
  function changeTypeBadge(type) {
    var label, colors;
    switch (type) {
      case 'program_added': label = 'Program Added'; colors = 'bg-emerald-900/50 text-emerald-300 border border-emerald-800'; break;
      case 'program_removed': label = 'Program Removed'; colors = 'bg-red-900/50 text-red-300 border border-red-800'; break;
      case 'asset_added': label = 'Added'; colors = 'bg-emerald-900/50 text-emerald-300 border border-emerald-800'; break;
      case 'asset_removed': label = 'Removed'; colors = 'bg-red-900/50 text-red-300 border border-red-800'; break;
      default: label = type; colors = 'bg-zinc-700 text-zinc-300'; break;
    }
    return '<span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md ' + colors + '">' + escapeHTML(label) + '</span>';
  }

  function categoryBadge(cat) {
    var colors = 'bg-zinc-700 text-zinc-300';
    switch (cat.toLowerCase()) {
      case 'url': colors = 'bg-blue-900/50 text-blue-300 border border-blue-800'; break;
      case 'wildcard': colors = 'bg-purple-900/50 text-purple-300 border border-purple-800'; break;
      case 'cidr': colors = 'bg-amber-900/50 text-amber-300 border border-amber-800'; break;
      case 'android': colors = 'bg-green-900/50 text-green-300 border border-green-800'; break;
      case 'ios': colors = 'bg-gray-900/50 text-gray-300 border border-gray-600'; break;
      case 'api': colors = 'bg-cyan-900/50 text-cyan-300 border border-cyan-800'; break;
      case 'other': colors = 'bg-zinc-800 text-zinc-400 border border-zinc-700'; break;
      case 'hardware': colors = 'bg-orange-900/50 text-orange-300 border border-orange-800'; break;
      case 'executable': colors = 'bg-red-900/50 text-red-300 border border-red-800'; break;
    }
    return '<span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md ' + colors + '">' + escapeHTML(cat) + '</span>';
  }

  function scopeBadge(scopeType) {
    if (!scopeType) return '<span class="text-zinc-500 text-xs">&mdash;</span>';
    var label, colors;
    if (scopeType === 'in') {
      label = 'In Scope';
      colors = 'bg-emerald-900/30 text-emerald-400 border border-emerald-800';
    } else {
      label = 'Out of Scope';
      colors = 'bg-zinc-800 text-zinc-400 border border-zinc-700';
    }
    return '<span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-md whitespace-nowrap ' + colors + '">' + label + '</span>';
  }

  function platformBadge(platform) {
    var colors = 'bg-zinc-700 text-zinc-300';
    switch (platform.toLowerCase()) {
      case 'h1': case 'hackerone': colors = 'bg-blue-900/50 text-blue-300 border border-blue-800'; break;
      case 'bc': case 'bugcrowd': colors = 'bg-orange-900/50 text-orange-300 border border-orange-800'; break;
      case 'it': case 'intigriti': colors = 'bg-purple-900/50 text-purple-300 border border-purple-800'; break;
      case 'ywh': case 'yeswehack': colors = 'bg-yellow-900/50 text-yellow-300 border border-yellow-800'; break;
    }
    return '<span class="inline-flex items-center px-2.5 py-0.5 text-[11px] font-semibold rounded-md ' + colors + '">' + escapeHTML(capitalizedPlatform(platform)) + '</span>';
  }

  function capitalizedPlatform(p) {
    switch (p.toLowerCase()) {
      case 'h1': case 'hackerone': return 'HackerOne';
      case 'bc': case 'bugcrowd': return 'Bugcrowd';
      case 'it': case 'intigriti': return 'Intigriti';
      case 'ywh': case 'yeswehack': return 'YesWeHack';
      default: return p;
    }
  }

  function displayHandle(platform, handle) {
    if (platform.toLowerCase() === 'bc' || platform.toLowerCase() === 'bugcrowd') {
      return handle.replace(/^\/engagements\//, '');
    }
    return handle;
  }

  function programCell(u) {
    var externalIcon = '<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>';
    if (u.handle) {
      var internalURL = '/program/' + encodeURIComponent(u.platform.toLowerCase()) + '/' + encodeURIComponent(u.handle);
      return '<div class="flex items-center gap-2">' +
        '<a href="' + escapeAttr(u.program_url) + '" target="_blank" rel="noopener noreferrer" class="text-zinc-500 hover:text-cyan-400 transition-colors flex-shrink-0" title="Open on ' + escapeAttr(capitalizedPlatform(u.platform)) + '">' + externalIcon + '</a>' +
        '<a href="' + escapeAttr(internalURL) + '" class="text-cyan-400 hover:text-cyan-300 hover:underline transition-colors">' + escapeHTML(displayHandle(u.platform, u.handle)) + '</a>' +
        '</div>';
    }
    return '<a href="' + escapeAttr(u.program_url) + '" target="_blank" rel="noopener noreferrer" class="text-cyan-400 hover:text-cyan-300 hover:underline transition-colors" title="' + escapeAttr(u.program_url) + '">' + escapeHTML(truncateMiddle(u.program_url, 40)) + '</a>';
  }

  function truncateMiddle(s, max) {
    if (s.length <= max) return s;
    var half = Math.floor((max - 3) / 2);
    return s.substring(0, half) + '...' + s.substring(s.length - half);
  }

  function formatTime(ts) {
    if (!ts) return '';
    var d = new Date(ts);
    var pad = function (n) { return n < 10 ? '0' + n : '' + n; };
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // --- Pagination ---
  function renderPagination(current, total) {
    var items = '';
    var btnBase = 'px-3 py-1.5 text-sm font-medium rounded-full transition-all duration-200 cursor-pointer';
    var btnActive = btnBase + ' bg-cyan-600 text-white shadow-md shadow-cyan-500/20';
    var btnNormal = btnBase + ' bg-zinc-800/50 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200';
    var btnDisabled = btnBase + ' bg-zinc-800/50 text-zinc-600 cursor-not-allowed';

    // Previous
    if (current <= 1) {
      items += '<span class="' + btnDisabled + '"><span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span></span>';
    } else {
      items += '<a data-page="' + (current - 1) + '" class="' + btnNormal + '"><span class="hidden sm:inline">Previous</span><span class="sm:hidden">&larr;</span></a>';
    }

    var start = Math.max(1, current - 2);
    var end = Math.min(total, current + 2);

    if (start > 1) {
      items += '<a data-page="1" class="' + btnNormal + '">1</a>';
      if (start > 2) items += '<span class="px-1 sm:px-2 py-1.5 text-sm text-zinc-600">...</span>';
    }

    for (var i = start; i <= end; i++) {
      var hideClass = '';
      if (i !== current && (i < current - 1 || i > current + 1)) hideClass = ' hidden sm:inline-flex';
      if (i === current) {
        items += '<span class="' + btnActive + hideClass + '">' + i + '</span>';
      } else {
        items += '<a data-page="' + i + '" class="' + btnNormal + hideClass + '">' + i + '</a>';
      }
    }

    if (end < total) {
      if (end < total - 1) items += '<span class="px-1 sm:px-2 py-1.5 text-sm text-zinc-600">...</span>';
      items += '<a data-page="' + total + '" class="' + btnNormal + '">' + total + '</a>';
    }

    // Next
    if (current >= total) {
      items += '<span class="' + btnDisabled + '"><span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span></span>';
    } else {
      items += '<a data-page="' + (current + 1) + '" class="' + btnNormal + '"><span class="hidden sm:inline">Next</span><span class="sm:hidden">&rarr;</span></a>';
    }

    return '<nav class="inline-flex items-center gap-1 bg-zinc-800/30 rounded-full px-1 py-1">' + items + '</nav>';
  }

  // --- Utility ---
  function escapeHTML(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function escapeAttr(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // --- Event listeners for outer controls ---
  function attachOuterListeners() {
    // Platform tabs
    document.querySelectorAll('.updates-platform-tab').forEach(function (tab) {
      tab.addEventListener('click', function () {
        state.platform = tab.getAttribute('data-platform');
        state.page = 1;
        pushURL();
        syncOuterControls();
        fetchAndRender();
      });
    });

    // Date dropdown
    var dateSel = document.getElementById('updates-date-select');
    if (dateSel) {
      dateSel.addEventListener('change', function () {
        var v = dateSel.value;
        if (v === 'custom') {
          document.getElementById('updates-date-range').classList.remove('hidden');
        } else {
          document.getElementById('updates-date-range').classList.add('hidden');
          state.since = v;
          state.until = '';
          state.page = 1;
          pushURL();
          fetchAndRender();
        }
      });
    }

    // Date range apply button
    var dateApplyBtn = document.getElementById('updates-date-apply');
    if (dateApplyBtn) {
      dateApplyBtn.addEventListener('click', function () {
        var from = document.getElementById('updates-date-from').value;
        var to = document.getElementById('updates-date-to').value;
        if (from) {
          state.since = from;
          state.until = to || '';
          state.page = 1;
          pushURL();
          fetchAndRender();
        }
      });
    }

    // Search: button click and Enter key
    var searchContainer = document.getElementById('updates-search-form');
    if (searchContainer) {
      var searchBtn = searchContainer.querySelector('button[type="submit"]');
      var searchInput = document.getElementById('updates-search-input');
      function doSearch() {
        state.search = searchInput.value.trim();
        state.page = 1;
        pushURL();
        fetchAndRender();
      }
      if (searchBtn) searchBtn.addEventListener('click', doSearch);
      if (searchInput) searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); doSearch(); }
      });
    }

    // Per-page select
    var ppSel = document.getElementById('updates-perpage-select');
    if (ppSel) {
      ppSel.addEventListener('change', function () {
        state.perPage = parseInt(ppSel.value, 10);
        state.page = 1;
        pushURL();
        fetchAndRender();
      });
    }
  }

  // --- Start ---
  attachOuterListeners();
  init();
})();
