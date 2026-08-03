(function () {
  'use strict';

  // --- xterm setup ---
  var term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: { background: '#1e1e1e' }
  });
  var fitAddon = new FitAddon.FitAddon();
  term.loadAddon(fitAddon);
  term.open(document.getElementById('terminal'));
  fitAddon.fit();

  // --- helper ---
  function xhr(method, url, body) {
    var r = new XMLHttpRequest();
    r.open(method, url);
    r.send(body);
  }

  // --- output polling ---
  var offset = 0;

  function poll() {
    var r = new XMLHttpRequest();
    r.open('GET', '/output?offset=' + offset);
    r.timeout = 30000; // server uses 25s, give 30s margin
    r.responseType = 'text';

    r.onload = function () {
      if (r.status === 200) {
        var newOffset = r.getResponseHeader('X-Offset');
        if (newOffset !== null) {
          offset = parseInt(newOffset, 10);
        }
        if (r.responseText && r.responseText.length > 0) {
          term.write(r.responseText);
        }
        poll();
      } else {
        setTimeout(poll, 1000);
      }
    };

    r.ontimeout = function () {
      // Server already returned after its 25s window; retry.
      poll();
    };

    r.onerror = function () {
      setTimeout(poll, 1000);
    };

    r.send();
  }

  poll();

  // --- key input ---
  term.onData(function (data) {
    // Ctrl sticky: intercept single letters to send control characters.
    if (ctrlActive && data.length === 1 && /[a-zA-Z]/.test(data)) {
      var code = data.toUpperCase().charCodeAt(0);
      xhr('POST', '/input', String.fromCharCode(code & 0x1f));
      ctrlActive = false;
      updateCtrlStyle();
      return;
    }
    // Any non-letter terminal input deactivates Ctrl sticky.
    if (ctrlActive) {
      ctrlActive = false;
      updateCtrlStyle();
    }
    xhr('POST', '/input', data);
  });

  // --- resize ---
  function reportSize() {
    var dims = fitAddon.proposeDimensions();
    if (dims && dims.rows && dims.cols) {
      xhr('POST', '/resize', JSON.stringify({ rows: dims.rows, cols: dims.cols }));
    }
  }

  window.addEventListener('resize', function () {
    fitAddon.fit();
    reportSize();
  });

  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', function () {
      fitAddon.fit();
      reportSize();
    });
  }

  // --- mobile toolbar ---
  var toolbar = document.getElementById('toolbar');

  function showToolbar() {
    toolbar.classList.add('visible');
    document.getElementById('terminal').style.paddingBottom = toolbar.offsetHeight + 'px';
  }

  function hideToolbar() {
    toolbar.classList.remove('visible');
    document.getElementById('terminal').style.paddingBottom = '0';
  }

  // Detect touch device.
  if ('ontouchstart' in window) {
    showToolbar();
  }

  // Ctrl sticky modifier.
  var ctrlActive = false;
  var btnCtrl = document.getElementById('btn-ctrl');

  function updateCtrlStyle() {
    if (ctrlActive) {
      btnCtrl.classList.add('ctrl-active');
    } else {
      btnCtrl.classList.remove('ctrl-active');
    }
  }

  // Button byte sequences.
  var keyBytes = {
    esc: '\x1b',
    tab: '\x09',
    up: '\x1b[A',
    down: '\x1b[B',
    left: '\x1b[D',
    right: '\x1b[C'
  };

  toolbar.addEventListener('click', function (e) {
    var btn = e.target.closest('button');
    if (!btn) return;
    var key = btn.dataset.key;
    if (!key) return;

    e.preventDefault();

    if (key === 'ctrl') {
      ctrlActive = !ctrlActive;
      updateCtrlStyle();
      return;
    }

    if (ctrlActive && key.length === 1 && /[a-zA-Z]/.test(key)) {
      // Ctrl+letter: send control character.
      var code = key.toUpperCase().charCodeAt(0);
      xhr('POST', '/input', String.fromCharCode(code & 0x1f));
      ctrlActive = false;
      updateCtrlStyle();
      return;
    }

    // Any non-letter button deactivates Ctrl sticky.
    if (ctrlActive) {
      ctrlActive = false;
      updateCtrlStyle();
    }

    var bytes = keyBytes[key];
    if (bytes) {
      xhr('POST', '/input', bytes);
    }
  });

  // --- service worker ---
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js');
  }
})();
