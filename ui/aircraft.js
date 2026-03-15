/* ============================================================
   SkyTracker Display UI — aircraft.js
   Aircraft list panel and detail panel logic
   ============================================================ */

var SkyTrackerAircraft = (function () {
  'use strict';

  // --- DOM refs ---
  var listBody = null;
  var detailEmpty = null;
  var detailContent = null;
  var detailCallsign = null;
  var detailType = null;
  var detailAirline = null;
  var detailAltitude = null;
  var detailSpeed = null;
  var detailHeading = null;
  var detailDistance = null;
  var detailRarityBadge = null;
  var detailRarityValue = null;
  var networkStatusEl = null;
  var networkDotEl = null;
  var networkLabelEl = null;

  // --- State ---
  var selectedIcao = null;
  var aircraftMap = {};  // icao -> aircraft object for quick lookup

  function init() {
    // Cache DOM elements
    listBody = document.getElementById('list-body');
    detailEmpty = document.getElementById('detail-empty');
    detailContent = document.getElementById('detail-content');
    detailCallsign = document.getElementById('detail-callsign');
    detailType = document.getElementById('detail-type');
    detailAirline = document.getElementById('detail-airline');
    detailAltitude = document.getElementById('detail-altitude');
    detailSpeed = document.getElementById('detail-speed');
    detailHeading = document.getElementById('detail-heading');
    detailDistance = document.getElementById('detail-distance');
    detailRarityBadge = document.getElementById('detail-rarity-badge');
    detailRarityValue = document.getElementById('detail-rarity-value');
    networkStatusEl = document.getElementById('network-status');
    networkDotEl = networkStatusEl ? networkStatusEl.querySelector('.network-dot') : null;
    networkLabelEl = networkStatusEl ? networkStatusEl.querySelector('.network-label') : null;

    // Listen for aircraft data updates
    SkyTrackerWS.on('aircraft', handleAircraftUpdate);
    SkyTrackerWS.on('status', handleNetworkStatus);

    // Listen for radar selection
    SkyTrackerRadar.onSelect(function (icao) {
      selectAircraft(icao);
    });
  }

  // --- Aircraft data update ---
  function handleAircraftUpdate(data) {
    var aircraft = data.aircraft || [];

    // Build lookup map
    aircraftMap = {};
    for (var i = 0; i < aircraft.length; i++) {
      if (aircraft[i].icao) {
        aircraftMap[aircraft[i].icao] = aircraft[i];
      }
    }

    // Sort by distance (nearest first)
    aircraft.sort(function (a, b) {
      var da = a.distance != null ? a.distance : Infinity;
      var db = b.distance != null ? b.distance : Infinity;
      return da - db;
    });

    renderList(aircraft);

    // Always show the closest aircraft in the detail panel
    if (aircraft.length > 0) {
      renderDetail(aircraft[0]);
      SkyTrackerRadar.setSelected(aircraft[0].icao);
    } else {
      clearSelection();
    }
  }

  // --- List rendering ---
  function renderList(aircraft) {
    // Use DocumentFragment for performance
    var fragment = document.createDocumentFragment();

    for (var i = 0; i < aircraft.length; i++) {
      var ac = aircraft[i];
      var row = createRow(ac);
      fragment.appendChild(row);
    }

    // Clear list safely (no innerHTML)
    while (listBody.firstChild) {
      listBody.removeChild(listBody.firstChild);
    }
    listBody.appendChild(fragment);
  }

  function createRow(ac) {
    var row = document.createElement('div');
    row.className = 'aircraft-row';
    if (ac.icao === selectedIcao) {
      row.className += ' selected';
    }

    row.setAttribute('data-icao', ac.icao || '');
    row.addEventListener('click', function () {
      selectAircraft(ac.icao);
    });

    // Callsign column (with optional rarity badge)
    var colCallsign = document.createElement('span');
    colCallsign.className = 'col-callsign';

    if (ac.rarity != null && ac.rarity >= 7) {
      var badge = document.createElement('span');
      badge.className = 'list-rarity';
      if (ac.rarity >= 9) {
        badge.className += ' legendary';
      }
      badge.textContent = ac.rarity;
      colCallsign.appendChild(badge);
    }

    var callsignText = document.createTextNode(ac.callsign || ac.registration || ac.icao || '---');
    colCallsign.appendChild(callsignText);
    row.appendChild(colCallsign);

    // Type column
    var colType = document.createElement('span');
    colType.className = 'col-type';
    colType.textContent = ac.type_code || '---';
    row.appendChild(colType);

    // Altitude column
    var colAlt = document.createElement('span');
    colAlt.className = 'col-alt';
    var altClass = SkyTrackerRadar.getAltitudeClass(ac.altitude);
    if (altClass) colAlt.className += ' ' + altClass;
    colAlt.textContent = formatAltitude(ac.altitude);
    row.appendChild(colAlt);

    // Speed column
    var colSpd = document.createElement('span');
    colSpd.className = 'col-spd';
    colSpd.textContent = ac.speed != null ? Math.round(ac.speed) : '---';
    row.appendChild(colSpd);

    // Distance column
    var colDist = document.createElement('span');
    colDist.className = 'col-dist';
    colDist.textContent = ac.distance != null ? ac.distance.toFixed(1) : '---';
    row.appendChild(colDist);

    return row;
  }

  // --- Detail panel ---
  function renderDetail(ac) {
    detailEmpty.classList.add('hidden');
    detailContent.classList.remove('hidden');

    detailCallsign.textContent = ac.callsign || ac.registration || ac.icao || '---';
    detailCallsign.style.color = SkyTrackerRadar.getAltitudeColor(ac.altitude);

    var typeText = ac.type_name || ac.type_code || '';
    if (ac.registration) typeText += ' · ' + ac.registration;
    detailType.textContent = typeText || 'Unknown Type';

    var subLine = ac.operator || ac.airline || '';
    if (ac.origin && ac.destination) {
      subLine = ac.origin + ' → ' + ac.destination + (subLine ? '  ·  ' + subLine : '');
    }
    detailAirline.textContent = subLine;

    detailAltitude.textContent = ac.altitude != null ? ac.altitude.toLocaleString() + ' ft' : '---';
    detailAltitude.style.color = SkyTrackerRadar.getAltitudeColor(ac.altitude);

    detailSpeed.textContent = ac.speed != null ? Math.round(ac.speed) + ' kts' : '---';
    detailHeading.textContent = ac.heading != null ? Math.round(ac.heading) + '\u00B0' : '---';
    detailDistance.textContent = ac.distance != null ? ac.distance.toFixed(1) + ' nm' : '---';

    // Rarity badge
    if (ac.rarity != null && ac.rarity >= 7) {
      detailRarityBadge.classList.remove('hidden');
      detailRarityValue.textContent = ac.rarity + '/10';

      if (ac.rarity >= 9) {
        detailRarityBadge.classList.add('legendary');
      } else {
        detailRarityBadge.classList.remove('legendary');
      }
    } else {
      detailRarityBadge.classList.add('hidden');
    }
  }

  function clearSelection() {
    selectedIcao = null;
    detailContent.classList.add('hidden');
    detailEmpty.classList.remove('hidden');
    SkyTrackerRadar.setSelected(null);

    // Remove selected class from list rows
    var rows = listBody.querySelectorAll('.aircraft-row.selected');
    for (var i = 0; i < rows.length; i++) {
      rows[i].classList.remove('selected');
    }
  }

  function selectAircraft(icao) {
    if (icao === null) {
      clearSelection();
      return;
    }

    selectedIcao = icao;
    SkyTrackerRadar.setSelected(icao);

    // Update list selection
    var rows = listBody.querySelectorAll('.aircraft-row');
    for (var i = 0; i < rows.length; i++) {
      var rowIcao = rows[i].getAttribute('data-icao');
      if (rowIcao === icao) {
        rows[i].classList.add('selected');
        // Scroll into view if needed
        rows[i].scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      } else {
        rows[i].classList.remove('selected');
      }
    }

    // Update detail panel
    var ac = aircraftMap[icao];
    if (ac) {
      renderDetail(ac);
    }
  }

  // --- Network status ---
  function handleNetworkStatus(network) {
    if (!networkStatusEl) return;

    var status = network.status || 'offline';

    // Remove all state classes
    networkStatusEl.classList.remove('sharing', 'ai-online', 'wifi-only', 'hidden');

    switch (status) {
      case 'sharing':
        networkStatusEl.classList.add('sharing');
        networkLabelEl.textContent = 'Sharing to skytracker.ai';
        break;
      case 'ai_online':
        networkStatusEl.classList.add('ai-online');
        networkLabelEl.textContent = 'SkyTracker AI Online';
        break;
      case 'wifi_only':
        networkStatusEl.classList.add('wifi-only');
        networkLabelEl.textContent = 'WiFi connected';
        break;
      case 'offline':
      default:
        networkStatusEl.classList.add('hidden');
        break;
    }
  }

  // --- Formatting helpers ---
  function formatAltitude(alt) {
    if (alt == null) return '---';
    if (alt >= 1000) {
      return Math.round(alt / 1000) + 'k';
    }
    return Math.round(alt).toString();
  }

  // --- Public API ---
  return {
    init: init,
    selectAircraft: selectAircraft,
    clearSelection: clearSelection
  };
})();

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', function () {
  SkyTrackerAircraft.init();
});
