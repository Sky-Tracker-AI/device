/**
 * SkyTracker Setup Screen
 * Shows QR code + claim code when device is unclaimed.
 * Polls /api/status and auto-transitions to radar when claimed.
 */
(function () {
  'use strict';

  var POLL_INTERVAL = 5000; // 5 seconds
  var pollTimer = null;
  var overlay = null;

  function init() {
    overlay = document.getElementById('setup-overlay');
    if (!overlay) return;
    poll();
  }

  /** Clear all children from an element. */
  function clearChildren(el) {
    while (el.firstChild) {
      el.removeChild(el.firstChild);
    }
  }

  /** Create an element with className and optional textContent. */
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text) node.textContent = text;
    return node;
  }

  function poll() {
    fetch('/api/status')
      .then(function (res) { return res.json(); })
      .then(function (status) {
        if (status.claimed) {
          hideSetup();
          return;
        }

        if (!status.registered) {
          showConnecting();
        } else {
          showClaimScreen(status.claim_code, status.claim_url);
        }

        pollTimer = setTimeout(poll, POLL_INTERVAL);
      })
      .catch(function () {
        showConnecting();
        pollTimer = setTimeout(poll, POLL_INTERVAL);
      });
  }

  function showConnecting() {
    overlay.classList.remove('hidden');
    var content = overlay.querySelector('.setup-content');
    clearChildren(content);

    var iconDiv = el('div', 'setup-icon');
    iconDiv.appendChild(el('div', 'setup-spinner'));
    content.appendChild(iconDiv);
    content.appendChild(el('div', 'setup-title', 'Connecting...'));
    content.appendChild(el('div', 'setup-subtitle', 'Registering with skytracker.ai'));
  }

  function showClaimScreen(claimCode, claimURL) {
    overlay.classList.remove('hidden');
    var content = overlay.querySelector('.setup-content');
    clearChildren(content);

    var displayCode = claimCode || '----';

    content.appendChild(el('div', 'setup-title', 'Claim Your SkyTracker'));
    content.appendChild(el('div', 'setup-subtitle', 'Scan the QR code or enter the code at skytracker.ai/claim'));

    var qrContainer = el('div', 'setup-qr');
    qrContainer.id = 'setup-qr';
    content.appendChild(qrContainer);

    content.appendChild(el('div', 'setup-code', displayCode));
    content.appendChild(el('div', 'setup-hint', 'This screen will disappear once claimed'));

    // Generate QR code.
    if (claimURL && typeof qrcode !== 'undefined') {
      var qr = qrcode(0, 'M');
      qr.addData(claimURL);
      qr.make();

      var moduleCount = qr.getModuleCount();
      var cellSize = Math.floor(200 / moduleCount);
      var size = cellSize * moduleCount;

      var canvas = document.createElement('canvas');
      canvas.width = size;
      canvas.height = size;
      var ctx = canvas.getContext('2d');

      // Dark background matching theme.
      ctx.fillStyle = '#0a0a0f';
      ctx.fillRect(0, 0, size, size);

      // Cyan QR modules.
      ctx.fillStyle = '#60efff';
      for (var row = 0; row < moduleCount; row++) {
        for (var col = 0; col < moduleCount; col++) {
          if (qr.isDark(row, col)) {
            ctx.fillRect(col * cellSize, row * cellSize, cellSize, cellSize);
          }
        }
      }

      qrContainer.appendChild(canvas);
    }
  }

  function hideSetup() {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    if (overlay) {
      overlay.classList.add('hidden');
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
