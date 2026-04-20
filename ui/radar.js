/* ============================================================
   SkyTracker Display UI — radar.js
   Canvas-based radar display with rotating sweep line,
   range rings, aircraft dots, and smooth animation
   ============================================================ */

var SkyTrackerRadar = (function () {
  'use strict';

  // --- Constants ---
  var TWO_PI = Math.PI * 2;
  var DEG_TO_RAD = Math.PI / 180;
  var SWEEP_SPEED = TWO_PI / 6;           // Full rotation every 6 seconds
  var SWEEP_ARC_LENGTH = 0.35;            // Radians of sweep arc trail
  var RANGE_RINGS = 5;                    // Number of range rings
  var AIRCRAFT_DOT_RADIUS = 3.5;          // Base dot radius in px
  var SELECTED_DOT_RADIUS = 6;            // Selected dot radius
  var TRAIL_LENGTH = 8;                   // Number of trail positions
  var TRAIL_DOT_RADIUS = 1.2;            // Trail dot radius
  var CLICK_TOLERANCE = 15;               // Click detection radius in px

  // --- Altitude colors ---
  var ALT_COLORS = {
    cyan:   { hex: '#60efff', threshold: 30000 },
    purple: { hex: '#a78bfa', threshold: 15000 },
    green:  { hex: '#34d399', threshold: 5000 },
    amber:  { hex: '#fbbf24', threshold: 0 }
  };

  // --- State ---
  var canvas = null;
  var ctx = null;
  var width = 0;
  var height = 0;
  var dpr = 1;
  var centerX = 0;
  var centerY = 0;
  var radarRadius = 0;
  var sweepAngle = -Math.PI / 2;  // Start at north (top)
  var maxRangeNm = 54;
  var aircraftData = [];
  var stationLat = 0;
  var stationLon = 0;
  var selectedIcao = null;
  var onSelectCallback = null;
  var animFrameId = null;
  var lastTimestamp = 0;
  var nightMode = false;

  // Aircraft trails: icao -> array of {bearing, distance} positions
  var trails = {};

  function init() {
    canvas = document.getElementById('radar-canvas');
    if (!canvas) return;

    ctx = canvas.getContext('2d');
    dpr = window.devicePixelRatio || 1;

    resize();
    window.addEventListener('resize', resize);

    canvas.addEventListener('click', handleClick);
    canvas.addEventListener('mousemove', handleMouseMove);
    canvas.addEventListener('touchstart', handleTouch, { passive: false });

    // Listen for aircraft data
    SkyTrackerWS.on('aircraft', handleAircraftUpdate);
    SkyTrackerWS.on('config', handleConfig);

    // Start render loop
    lastTimestamp = performance.now();
    animFrameId = requestAnimationFrame(tick);
  }

  function resize() {
    var container = canvas.parentElement;
    width = container.clientWidth;
    height = container.clientHeight;

    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    centerX = width / 2;
    centerY = height / 2;
    radarRadius = Math.min(centerX, centerY) * 0.88;
  }

  // --- Main render loop ---
  function tick(timestamp) {
    var dt = (timestamp - lastTimestamp) / 1000;
    lastTimestamp = timestamp;

    // Advance sweep
    sweepAngle += SWEEP_SPEED * dt;
    if (sweepAngle > TWO_PI) sweepAngle -= TWO_PI;

    render(dt);
    animFrameId = requestAnimationFrame(tick);
  }

  function render() {
    ctx.clearRect(0, 0, width, height);

    drawBackground();
    drawRangeRings();
    drawCardinalLabels();
    drawSweepLine();
    drawAircraft();
    drawCenterDot();
  }

  // --- Background ---
  function drawBackground() {
    // Subtle radial gradient from center
    var grad = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, radarRadius * 1.2);
    grad.addColorStop(0, nightMode ? '#060609' : '#0c0c14');
    grad.addColorStop(1, nightMode ? '#030305' : '#080810');
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, width, height);
  }

  // --- Range rings ---
  function drawRangeRings() {
    ctx.strokeStyle = getCSSVar('--radar-grid');
    ctx.lineWidth = 0.5;

    for (var i = 1; i <= RANGE_RINGS; i++) {
      var r = (radarRadius / RANGE_RINGS) * i;

      // Make every other ring slightly brighter
      if (i % 2 === 0) {
        ctx.strokeStyle = getCSSVar('--radar-grid-major');
        ctx.lineWidth = 0.7;
      } else {
        ctx.strokeStyle = getCSSVar('--radar-grid');
        ctx.lineWidth = 0.5;
      }

      ctx.beginPath();
      ctx.arc(centerX, centerY, r, 0, TWO_PI);
      ctx.stroke();

      // Range label
      var rangeLabel = Math.round((maxRangeNm / RANGE_RINGS) * i * 1.852) + ' km';
      ctx.fillStyle = nightMode ? 'rgba(96,239,255,0.12)' : 'rgba(96,239,255,0.2)';
      ctx.font = '0.55rem monospace';
      ctx.textAlign = 'left';
      ctx.fillText(rangeLabel, centerX + r * Math.cos(-Math.PI / 4) + 3, centerY + r * Math.sin(-Math.PI / 4) - 3);
    }

    // Cross-hair lines (subtle)
    ctx.strokeStyle = getCSSVar('--radar-grid');
    ctx.lineWidth = 0.3;

    // Vertical line
    ctx.beginPath();
    ctx.moveTo(centerX, centerY - radarRadius);
    ctx.lineTo(centerX, centerY + radarRadius);
    ctx.stroke();

    // Horizontal line
    ctx.beginPath();
    ctx.moveTo(centerX - radarRadius, centerY);
    ctx.lineTo(centerX + radarRadius, centerY);
    ctx.stroke();
  }

  // --- Cardinal labels ---
  function drawCardinalLabels() {
    ctx.fillStyle = nightMode ? 'rgba(96,239,255,0.2)' : 'rgba(96,239,255,0.35)';
    ctx.font = '600 0.65rem monospace';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';

    var offset = radarRadius + 14;

    // N
    ctx.fillText('N', centerX, centerY - offset);
    // S
    ctx.fillText('S', centerX, centerY + offset);
    // E
    ctx.fillText('E', centerX + offset, centerY);
    // W
    ctx.fillText('W', centerX - offset, centerY);
  }

  // --- Sweep line ---
  function drawSweepLine() {
    // Main sweep line
    var endX = centerX + radarRadius * Math.sin(sweepAngle);
    var endY = centerY - radarRadius * Math.cos(sweepAngle);

    ctx.strokeStyle = nightMode ? 'rgba(96,239,255,0.25)' : 'rgba(96,239,255,0.5)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(centerX, centerY);
    ctx.lineTo(endX, endY);
    ctx.stroke();

    // Trailing arc (fading fan behind the sweep line)
    var steps = 40;
    for (var i = 0; i < steps; i++) {
      var t = i / steps;
      var angle = sweepAngle - SWEEP_ARC_LENGTH * t;
      var alpha = (1 - t) * (nightMode ? 0.04 : 0.07);

      var x1 = centerX + radarRadius * Math.sin(angle);
      var y1 = centerY - radarRadius * Math.cos(angle);

      ctx.strokeStyle = 'rgba(96,239,255,' + alpha + ')';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(centerX, centerY);
      ctx.lineTo(x1, y1);
      ctx.stroke();
    }

    // Glowing tip
    var glowGrad = ctx.createRadialGradient(endX, endY, 0, endX, endY, 8);
    glowGrad.addColorStop(0, nightMode ? 'rgba(96,239,255,0.2)' : 'rgba(96,239,255,0.4)');
    glowGrad.addColorStop(1, 'rgba(96,239,255,0)');
    ctx.fillStyle = glowGrad;
    ctx.beginPath();
    ctx.arc(endX, endY, 8, 0, TWO_PI);
    ctx.fill();
  }

  // --- Center dot (station) ---
  function drawCenterDot() {
    // Outer glow
    var glowGrad = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, 12);
    glowGrad.addColorStop(0, nightMode ? 'rgba(96,239,255,0.2)' : 'rgba(96,239,255,0.35)');
    glowGrad.addColorStop(1, 'rgba(96,239,255,0)');
    ctx.fillStyle = glowGrad;
    ctx.beginPath();
    ctx.arc(centerX, centerY, 12, 0, TWO_PI);
    ctx.fill();

    // Inner dot
    ctx.fillStyle = nightMode ? 'rgba(96,239,255,0.4)' : 'rgba(96,239,255,0.7)';
    ctx.beginPath();
    ctx.arc(centerX, centerY, 2.5, 0, TWO_PI);
    ctx.fill();
  }

  // --- Aircraft rendering ---
  function drawAircraft() {
    for (var i = 0; i < aircraftData.length; i++) {
      var ac = aircraftData[i];
      if (ac.distance == null || ac.bearing == null) continue;
      if (ac.distance > maxRangeNm) continue;

      var pos = polarToCanvas(ac.bearing, ac.distance);
      if (!pos) continue;

      var isUat = ac.source === 'uat';
      var color = isUat ? '#ff9f43' : getAltitudeColor(ac.altitude);
      var isSelected = ac.icao === selectedIcao;
      var isRare = ac.rarity != null && ac.rarity >= 7;

      // Draw trail
      drawTrail(ac.icao, color);

      // Draw aircraft dot
      var dotRadius = isSelected ? SELECTED_DOT_RADIUS : AIRCRAFT_DOT_RADIUS;

      // Glow effect
      var glowSize = isSelected ? 18 : (isRare ? 14 : 10);
      var glowAlpha = isSelected ? 0.3 : (isRare ? 0.2 : 0.12);
      var glowGrad = ctx.createRadialGradient(pos.x, pos.y, 0, pos.x, pos.y, glowSize);
      glowGrad.addColorStop(0, colorWithAlpha(color, nightMode ? glowAlpha * 0.5 : glowAlpha));
      glowGrad.addColorStop(1, colorWithAlpha(color, 0));
      ctx.fillStyle = glowGrad;
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, glowSize, 0, TWO_PI);
      ctx.fill();

      // Heading indicator (small line in direction of travel)
      if (ac.heading != null) {
        var headLen = isSelected ? 14 : 10;
        var headRad = ac.heading * DEG_TO_RAD;
        var hx = pos.x + headLen * Math.sin(headRad);
        var hy = pos.y - headLen * Math.cos(headRad);
        ctx.strokeStyle = colorWithAlpha(color, nightMode ? 0.3 : 0.5);
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(pos.x, pos.y);
        ctx.lineTo(hx, hy);
        ctx.stroke();
      }

      // Main dot (diamond for UAT, circle for ADS-B)
      ctx.fillStyle = color;
      if (isUat) {
        ctx.beginPath();
        ctx.moveTo(pos.x, pos.y - dotRadius);
        ctx.lineTo(pos.x + dotRadius, pos.y);
        ctx.lineTo(pos.x, pos.y + dotRadius);
        ctx.lineTo(pos.x - dotRadius, pos.y);
        ctx.closePath();
        ctx.fill();
      } else {
        ctx.beginPath();
        ctx.arc(pos.x, pos.y, dotRadius, 0, TWO_PI);
        ctx.fill();
      }

      // Selected ring
      if (isSelected) {
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.2;
        if (isUat) {
          var sr = dotRadius + 4;
          ctx.beginPath();
          ctx.moveTo(pos.x, pos.y - sr);
          ctx.lineTo(pos.x + sr, pos.y);
          ctx.lineTo(pos.x, pos.y + sr);
          ctx.lineTo(pos.x - sr, pos.y);
          ctx.closePath();
          ctx.stroke();
        } else {
          ctx.beginPath();
          ctx.arc(pos.x, pos.y, dotRadius + 4, 0, TWO_PI);
          ctx.stroke();
        }
      }

      // Rarity badge (gold star marker)
      if (isRare) {
        var badgeX = pos.x + dotRadius + 3;
        var badgeY = pos.y - dotRadius - 3;
        var badgeColor = ac.rarity >= 9 ? '#ff6b2b' : '#f5c842';

        ctx.fillStyle = badgeColor;
        ctx.font = '600 0.5rem sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('\u2605', badgeX, badgeY);
      }

      // Callsign label
      if (ac.callsign) {
        var labelY = pos.y + dotRadius + 10;
        ctx.fillStyle = colorWithAlpha(color, nightMode ? 0.5 : 0.7);
        ctx.font = '500 0.5rem monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillText(ac.callsign, pos.x, labelY);
      }
    }
  }

  function drawTrail(icao, color) {
    var trail = trails[icao];
    if (!trail || trail.length < 2) return;

    for (var i = 0; i < trail.length; i++) {
      var t = trail[i];
      var pos = polarToCanvas(t.bearing, t.distance);
      if (!pos) continue;

      var alpha = ((i + 1) / trail.length) * (nightMode ? 0.15 : 0.25);
      ctx.fillStyle = colorWithAlpha(color, alpha);
      ctx.beginPath();
      ctx.arc(pos.x, pos.y, TRAIL_DOT_RADIUS, 0, TWO_PI);
      ctx.fill();
    }
  }

  // --- Coordinate conversion ---
  function polarToCanvas(bearingDeg, distanceNm) {
    // Bearing: 0 = north, 90 = east (clockwise)
    // Canvas: angle from top, clockwise
    var fraction = distanceNm / maxRangeNm;
    if (fraction > 1) return null;

    var r = fraction * radarRadius;
    var angle = bearingDeg * DEG_TO_RAD;

    return {
      x: centerX + r * Math.sin(angle),
      y: centerY - r * Math.cos(angle)
    };
  }

  // --- Altitude color ---
  function getAltitudeColor(altitude) {
    if (altitude == null) return nightMode ? '#555570' : '#888888';
    if (altitude > 30000) return ALT_COLORS.cyan.hex;
    if (altitude > 15000) return ALT_COLORS.purple.hex;
    if (altitude > 5000)  return ALT_COLORS.green.hex;
    return ALT_COLORS.amber.hex;
  }

  function getAltitudeClass(altitude) {
    if (altitude == null) return '';
    if (altitude > 30000) return 'alt-cyan';
    if (altitude > 15000) return 'alt-purple';
    if (altitude > 5000)  return 'alt-green';
    return 'alt-amber';
  }

  // --- Color utilities ---
  function colorWithAlpha(hex, alpha) {
    var r = parseInt(hex.slice(1, 3), 16);
    var g = parseInt(hex.slice(3, 5), 16);
    var b = parseInt(hex.slice(5, 7), 16);
    return 'rgba(' + r + ',' + g + ',' + b + ',' + alpha + ')';
  }

  function getCSSVar(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  }

  // --- Hover cursor ---
  function handleMouseMove(e) {
    var rect = canvas.getBoundingClientRect();
    var mx = e.clientX - rect.left;
    var my = e.clientY - rect.top;

    var overAircraft = false;
    for (var i = 0; i < aircraftData.length; i++) {
      var ac = aircraftData[i];
      if (ac.distance == null || ac.bearing == null) continue;
      var pos = polarToCanvas(ac.bearing, ac.distance);
      if (!pos) continue;
      var dx = mx - pos.x;
      var dy = my - pos.y;
      if (Math.sqrt(dx * dx + dy * dy) < CLICK_TOLERANCE) {
        overAircraft = true;
        break;
      }
    }
    canvas.style.cursor = overAircraft ? 'pointer' : '';
  }

  // --- Click handling ---
  function handleClick(e) {
    var rect = canvas.getBoundingClientRect();
    var mx = e.clientX - rect.left;
    var my = e.clientY - rect.top;

    var closest = null;
    var closestDist = CLICK_TOLERANCE;

    for (var i = 0; i < aircraftData.length; i++) {
      var ac = aircraftData[i];
      if (ac.distance == null || ac.bearing == null) continue;

      var pos = polarToCanvas(ac.bearing, ac.distance);
      if (!pos) continue;

      var dx = mx - pos.x;
      var dy = my - pos.y;
      var d = Math.sqrt(dx * dx + dy * dy);

      if (d < closestDist) {
        closestDist = d;
        closest = ac;
      }
    }

    if (closest) {
      selectAircraft(closest.icao);
    } else {
      selectAircraft(null);
    }
  }

  function handleTouch(e) {
    if (e.touches.length === 1) {
      e.preventDefault();
      var touch = e.touches[0];
      var rect = canvas.getBoundingClientRect();
      var mx = touch.clientX - rect.left;
      var my = touch.clientY - rect.top;

      // Reuse click logic
      handleClick({ clientX: touch.clientX, clientY: touch.clientY });
    }
  }

  function selectAircraft(icao) {
    selectedIcao = icao;
    if (onSelectCallback) {
      onSelectCallback(icao);
    }
  }

  // --- Data update handler ---
  function handleAircraftUpdate(data) {
    var newAircraft = data.aircraft || [];

    // Update station position
    if (data.station) {
      if (data.station.lat != null) stationLat = data.station.lat;
      if (data.station.lon != null) stationLon = data.station.lon;
      if (data.station.name) {
        var nameEl = document.getElementById('station-name');
        if (nameEl) nameEl.textContent = data.station.name;
      }
    }

    // Update trails before replacing aircraft data
    updateTrails(newAircraft);

    aircraftData = newAircraft;

    // Update aircraft count
    var countEl = document.getElementById('aircraft-count');
    if (countEl) {
      var count = aircraftData.length;
      countEl.textContent = count + ' aircraft';
    }
  }

  function handleConfig(config) {
    if (config.max_range_nm != null) {
      maxRangeNm = config.max_range_nm;
    }
    if (config.night_mode != null) {
      nightMode = config.night_mode;
      document.body.classList.toggle('night-mode', nightMode);
    }
  }

  function updateTrails(newAircraft) {
    var seen = {};

    for (var i = 0; i < newAircraft.length; i++) {
      var ac = newAircraft[i];
      if (!ac.icao || ac.bearing == null || ac.distance == null) continue;

      seen[ac.icao] = true;

      if (!trails[ac.icao]) {
        trails[ac.icao] = [];
      }

      var trail = trails[ac.icao];
      var last = trail.length > 0 ? trail[trail.length - 1] : null;

      // Only add if position changed
      if (!last || last.bearing !== ac.bearing || last.distance !== ac.distance) {
        trail.push({ bearing: ac.bearing, distance: ac.distance });
        if (trail.length > TRAIL_LENGTH) {
          trail.shift();
        }
      }
    }

    // Remove trails for aircraft no longer visible
    for (var icao in trails) {
      if (!seen[icao]) {
        delete trails[icao];
      }
    }
  }

  // --- Public API ---
  function onSelect(callback) {
    onSelectCallback = callback;
  }

  function setSelected(icao) {
    selectedIcao = icao;
  }

  function getAircraft() {
    return aircraftData;
  }

  function destroy() {
    if (animFrameId) {
      cancelAnimationFrame(animFrameId);
    }
    window.removeEventListener('resize', resize);
    if (canvas) {
      canvas.removeEventListener('click', handleClick);
      canvas.removeEventListener('touchstart', handleTouch);
    }
  }

  return {
    init: init,
    destroy: destroy,
    onSelect: onSelect,
    setSelected: setSelected,
    getAircraft: getAircraft,
    getAltitudeColor: getAltitudeColor,
    getAltitudeClass: getAltitudeClass,
    polarToCanvas: polarToCanvas
  };
})();

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', function () {
  SkyTrackerRadar.init();
});
