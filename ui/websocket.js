/* ============================================================
   SkyTracker Display UI — websocket.js
   WebSocket client with auto-reconnect and exponential backoff
   ============================================================ */

var SkyTrackerWS = (function () {
  'use strict';

  // --- Configuration ---
  var DEFAULT_PORT = 8080;
  var INITIAL_RETRY_MS = 1000;
  var MAX_RETRY_MS = 30000;
  var BACKOFF_MULTIPLIER = 2;
  var JITTER_FACTOR = 0.3;

  // --- State ---
  var ws = null;
  var retryMs = INITIAL_RETRY_MS;
  var retryTimer = null;
  var intentionalClose = false;
  var listeners = {
    aircraft: [],
    status: [],
    config: []
  };

  // --- Connection status ---
  var Status = {
    CONNECTING: 'connecting',
    CONNECTED: 'connected',
    DISCONNECTED: 'disconnected'
  };

  var currentStatus = Status.DISCONNECTED;

  // --- DOM refs ---
  var connectionLostEl = null;

  function init() {
    // Create connection-lost banner
    connectionLostEl = document.createElement('div');
    connectionLostEl.className = 'connection-lost';
    connectionLostEl.textContent = 'Connection to SkyTracker Agent lost — reconnecting...';
    document.body.appendChild(connectionLostEl);

    connect();
  }

  function getWsUrl() {
    // In production, connect to the agent on the same host
    var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    var host = location.hostname || 'localhost';
    var port = location.port || DEFAULT_PORT;
    return protocol + '//' + host + ':' + port + '/ws';
  }

  function connect() {
    if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
      return;
    }

    intentionalClose = false;
    setStatus(Status.CONNECTING);

    var url = getWsUrl();

    try {
      ws = new WebSocket(url);
    } catch (e) {
      console.error('[SkyTrackerWS] Failed to create WebSocket:', e);
      scheduleReconnect();
      return;
    }

    ws.onopen = function () {
      console.log('[SkyTrackerWS] Connected to', url);
      retryMs = INITIAL_RETRY_MS;
      setStatus(Status.CONNECTED);
      showConnectionLost(false);
    };

    ws.onmessage = function (event) {
      var data;
      try {
        data = JSON.parse(event.data);
      } catch (e) {
        console.warn('[SkyTrackerWS] Invalid JSON:', e);
        return;
      }

      handleMessage(data);
    };

    ws.onclose = function (event) {
      console.log('[SkyTrackerWS] Closed:', event.code, event.reason);
      ws = null;

      if (!intentionalClose) {
        setStatus(Status.DISCONNECTED);
        showConnectionLost(true);
        scheduleReconnect();
      }
    };

    ws.onerror = function (event) {
      console.error('[SkyTrackerWS] Error:', event);
      // onclose will fire after onerror, so reconnect is handled there
    };
  }

  function handleMessage(data) {
    // Expected message format from the Agent:
    // {
    //   type: "aircraft_update",
    //   aircraft: [ ... ],
    //   station: { lat, lon, name, ... },
    //   network: { status: "sharing" | "ai_online" | "wifi_only" | "offline" },
    //   config: { night_mode: bool, max_range_nm: 250, ... },
    //   timestamp: "2026-03-11T..."
    // }

    if (data.aircraft !== undefined) {
      // Map server field names to UI field names
      var mapped = [];
      for (var i = 0; i < data.aircraft.length; i++) {
        var a = data.aircraft[i];
        mapped.push({
          icao: a.icao_hex || a.icao || '',
          callsign: (a.callsign || '').trim(),
          registration: a.registration || '',
          type_code: a.type || '',
          type_name: a.type_name || '',
          airline: a.airline || '',
          operator: a.operator || '',
          origin: a.origin || '',
          destination: a.destination || '',
          altitude: a.altitude,
          speed: a.speed,
          heading: a.heading,
          lat: a.lat,
          lon: a.lon,
          distance: a.distance_nm != null ? a.distance_nm : a.distance,
          bearing: a.bearing,
          rarity: a.rarity_score != null ? a.rarity_score : a.rarity
        });
      }
      data.aircraft = mapped;
      emit('aircraft', data);
    }

    if (data.network !== undefined) {
      emit('status', data.network);
    }

    if (data.config !== undefined) {
      emit('config', data.config);
    }
  }

  function scheduleReconnect() {
    if (retryTimer) {
      clearTimeout(retryTimer);
    }

    // Add jitter to avoid thundering herd
    var jitter = retryMs * JITTER_FACTOR * (Math.random() * 2 - 1);
    var delay = Math.min(retryMs + jitter, MAX_RETRY_MS);

    console.log('[SkyTrackerWS] Reconnecting in', Math.round(delay), 'ms');

    retryTimer = setTimeout(function () {
      retryTimer = null;
      connect();
    }, delay);

    retryMs = Math.min(retryMs * BACKOFF_MULTIPLIER, MAX_RETRY_MS);
  }

  function setStatus(status) {
    currentStatus = status;
  }

  function showConnectionLost(show) {
    if (connectionLostEl) {
      connectionLostEl.classList.toggle('visible', show);
    }
  }

  // --- Event system ---
  function on(event, callback) {
    if (listeners[event]) {
      listeners[event].push(callback);
    }
  }

  function off(event, callback) {
    if (listeners[event]) {
      listeners[event] = listeners[event].filter(function (cb) {
        return cb !== callback;
      });
    }
  }

  function emit(event, data) {
    if (listeners[event]) {
      for (var i = 0; i < listeners[event].length; i++) {
        try {
          listeners[event][i](data);
        } catch (e) {
          console.error('[SkyTrackerWS] Listener error:', e);
        }
      }
    }
  }

  function disconnect() {
    intentionalClose = true;
    if (retryTimer) {
      clearTimeout(retryTimer);
      retryTimer = null;
    }
    if (ws) {
      ws.close();
      ws = null;
    }
    setStatus(Status.DISCONNECTED);
  }

  function getStatus() {
    return currentStatus;
  }

  // --- Public API ---
  return {
    init: init,
    on: on,
    off: off,
    connect: connect,
    disconnect: disconnect,
    getStatus: getStatus,
    Status: Status
  };
})();

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', function () {
  SkyTrackerWS.init();
});
