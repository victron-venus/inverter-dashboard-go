<script>
const { createApp, ref, reactive, computed, onMounted, onUnmounted, watch, nextTick } = Vue;

createApp({
  setup() {
    const MAX_HOME_SLOTS = 5;
    const MAX_HEADER_SLOTS = 7;
    const state = reactive({
      booleans: {},
      features: {},
      ui_config: {},
      daily_stats: {},
      mppt_individual: [],
      tasmota_individual: [],
      mppt_chargers: [],
      batteries: [],
      solar_sources: [],
      loads: {},
      water_level: 0,
      water_valve: false,
      pump_switch: false,
      dry_run: false,
      dishwasher_running: false,
      dishwasher_duration: 0,
      dishwasher_time: 0,
      dishwasher_active: false,
      car_soc: 0,
      ev_charging_kw: 0,
      ev_power: 0,
      washer_time: 0,
      washer_power: false,
      dryer_time: 0,
      dryer_power: false,
    });
    const wsConnected = ref(false);
    const mqttConnected = ref(false);
    const chartEl = ref(null);
    const updating = ref(false);
    const isDark = ref(localStorage.getItem('theme') === 'dark');
    document.body.classList.toggle('light', !isDark.value);

    function toggleTheme() {
      isDark.value = !isDark.value;
      document.body.classList.toggle('light', !isDark.value);
      localStorage.setItem('theme', isDark.value ? 'dark' : 'light');
    }

    let ws = null;
    let chart = null;
    let reconnectTimer = null;
    let lastMessageTime = Date.now();
    let heartbeatTimer = null;
    let historyData = {timestamps: [], grid: [], solar: [], battery: [], setpoint: []};

    function connect() {
      if (ws && ws.readyState === WebSocket.OPEN) return;
      if (reconnectTimer) clearTimeout(reconnectTimer);

      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      try {
        ws = new WebSocket(proto + '//' + location.host + '/ws');
      } catch (e) {
        wsConnected.value = false;
        reconnectTimer = setTimeout(connect, 2000);
        return;
      }

      ws.onopen = () => {
        wsConnected.value = true;
        lastMessageTime = Date.now();
        startHeartbeat();
      };

      ws.onclose = () => {
        wsConnected.value = false;
        stopHeartbeat();
        reconnectTimer = setTimeout(connect, 2000);
      };

      ws.onerror = () => {
        wsConnected.value = false;
        ws.close();
      };

      ws.onmessage = (e) => {
        lastMessageTime = Date.now();
        const data = JSON.parse(e.data);
        // Merge state to avoid full replacement and prevent flashing
        const stateData = data.state || data;
        Object.assign(state, stateData);
        if (data.ui_config !== undefined) {
          state.ui_config = data.ui_config;
          mqttConnected.value = true;
        }

          const now = Date.now() / 1000;
          historyData.timestamps.push(now);
          historyData.grid.push(stateData.gt || 0);
          historyData.solar.push(stateData.solar_total || 0);
          console.log('DEBUG: Pushing battery value:', stateData.battery_power);
      historyData.battery.push(stateData.battery_power || 0);
          console.log('DEBUG: Pushing setpoint value:', stateData.setpoint);
      historyData.setpoint.push(stateData.setpoint || 0);

          if (historyData.timestamps.length > 1800) {
            historyData.timestamps.shift();
            historyData.grid.shift();
            historyData.solar.shift();
            historyData.battery.shift();
            historyData.setpoint.shift();
          }
          updateChart();
      };
    }

    function startHeartbeat() {
      stopHeartbeat();
      heartbeatTimer = setInterval(() => {
        if (Date.now() - lastMessageTime > 15000) {
          console.log('No data received, reconnecting...');
          if (ws) ws.close();
        }
      }, 5000);
    }

    function stopHeartbeat() {
      if (heartbeatTimer) {
        clearInterval(heartbeatTimer);
        heartbeatTimer = null;
      }
    }

    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        if (!ws || ws.readyState !== WebSocket.OPEN) connect();
      }
    });

    window.addEventListener('online', () => {
      if (ws) ws.close();
      setTimeout(connect, 500);
    });

    function send(action, payload = {}) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({action, ...payload}));
      }
    }

    function formatPower(w) {
      const v = Math.abs(Math.floor(w || 0));
      const sign = w < 0 ? '-' : '';
      return v >= 1000 ? sign + (v/1000).toFixed(1) + 'kW' : sign + v + 'W';
    }

    function formatDuration(s) {
      if (!s || s <= 0) return '0s';
      const h = Math.floor(s / 3600);
      const m = Math.floor((s % 3600) / 60);
      if (h > 0) return h + 'h ' + m + 'm';
      return m + 'm';
    }

    function formatSemverLabel(ver) {
      if (!ver || ver === '?' || ver === '') return '?';
      const s = String(ver).trim();
      if (/^v[0-9]/i.test(s)) return s;
      if (/^[0-9]/.test(s)) return 'v' + s;
      return s;
    }

    function getButtonState(btn) {
      const stateKey = btn.state_key || 'home_' + btn.id;
      return state[stateKey] ? 'on' : 'off';
    }

    function getHomeButton(index) {
      return home_buttons.value[index] || null;
    }
function getHomeButtonState(index) {
  const btn = home_buttons.value[index];
  if (!btn || !btn.state_key) return 'off';
  // Explicitly access state[btn.state_key] to ensure Vue 3 reactivity tracking
  return state[btn.state_key] ? 'on' : 'off';
}


    const essClass = computed(() => {
      const m = state.ess_mode;
      if (!m || !m.mode_name) return 'off';
      return (m.mode_name === 'Off' || m.mode_name === 'Charger only') ? 'off' : 'on';
    });

    const essText = computed(() => {
      const m = state.ess_mode;
      if (!m) return 'ESS';
      if (m.is_external) return 'External';
      return m.mode_name || 'ESS';
    });

    const mpptTotal = computed(() => (state.mppt_individual || []).reduce((a, b) => a + b, 0));
    const tasmotaTotal = computed(() => (state.tasmota_individual || []).reduce((a, b) => a + b, 0));

    const evCharging = computed(() => {
      const kw = parseFloat(state.ev_charging_kw) || 0;
      return kw > 0 ? kw.toFixed(1) + 'kW' : '0';
    });
    const evPower = computed(() => formatPower(state.ev_power || 0));

    const sortedLoads = computed(() => {
      const loads = state.loads || {};
      const uiConfig = state.ui_config || {};
      const loadsConfig = uiConfig.loads || {};
      const hiddenLoads = loadsConfig.hidden || ['solar_shed'];
      const minWatts = loadsConfig.min_watts || 10;
      return Object.entries(loads)
        .filter(([name, v]) => v > minWatts && !hiddenLoads.includes(name))
        .sort((a, b) => b[1] - a[1]);
    });

    const home_buttons = computed(() => {
      const uiConfig = state.ui_config || {};
      return (uiConfig.home_buttons || []).slice(0, MAX_HOME_SLOTS);
    });

    const headerToggles = computed(() => {
      const uiConfig = state.ui_config || {};
      return (uiConfig.boolean_buttons || []).slice(0, MAX_HEADER_SLOTS);
    });

    function getHeaderToggle(index) {
      return headerToggles.value[index] || null;
    }

    const batteries = computed(() => {
      return (state.batteries || []).map(b => ({
        name: b.name || 'Battery',
        voltage: b.voltage || 0,
        current: b.current,
        power: b.power,
        soc: b.soc || 0,
        state: b.state || 'Unknown',
        time_to_go: b.time_to_go || ''
      }));
    });

    const solarSources = computed(() => {
      const sources = [];
      (state.mppt_chargers || []).forEach(m => {
        sources.push({name: m.name || 'MPPT', pv_voltage: m.pv_voltage || 0, current: m.current || 0, power: m.power || 0});
      });
      (state.tasmota_individual || []).forEach((power, i) => {
        sources.push({name: 'PV Inverter ' + (i + 1), power: power || 0});
      });
      return sources;
    });

    function initChart() {
      if (!chartEl.value) return;
      chart = new uPlot({
        width: chartEl.value.clientWidth, height: 200,
        series: [
          {label: 'Time'},
          {stroke: '#4a90d9', fill: 'rgba(74,144,217,0.05)', label: 'Grid'},
          {stroke: '#f5a623', fill: 'rgba(245,166,35,0.05)', label: 'Solar'},
          {stroke: '#7ed321', fill: 'rgba(126,211,33,0.05)', label: 'Battery'},
          {stroke: '#00d4aa', dash: [5,5], label: 'Setpoint'}
        ],
        axes: [{show: false}, {grid: {stroke: '#e0e0e0'}, ticks: {stroke: '#ccc'}}],
        legend: {show: true, live: true},
        cursor: {show: true, points:{show: false}, drag:{setScale: false, x: false, y: false}}
      }, [[], [], [], [], []], chartEl.value);
    }

    function updateChart() {
      if (!chart) return;
      chart.setData([historyData.timestamps, historyData.grid, historyData.solar, historyData.battery, historyData.setpoint]);
    }

    const dailyStatsHtml = computed(() => {
      const ds = state.daily_stats || {};
      const prod = (ds.produced_today || 0).toFixed(2);
      const dollars = (ds.produced_dollars || 0).toFixed(2);
      const grid = (ds.grid_kwh || 0).toFixed(2);
      const gridCost = (parseFloat(grid) * 0.31).toFixed(2);
      const batIn = (ds.battery_in || 0).toFixed(2);
      const batOut = (ds.battery_out || 0).toFixed(2);
      const batInY = (ds.battery_in_yesterday || 0).toFixed(1);
      const batOutY = (ds.battery_out_yesterday || 0).toFixed(1);
      const batDelta = (parseFloat(batIn) - parseFloat(batOut)).toFixed(2);
      const batDeltaY = (parseFloat(batInY) - parseFloat(batOutY)).toFixed(1);
      const tasmotaDaily = ds.tasmota_daily || [];
      const mpptDaily = ds.mppt_daily || [];
      const pvTotalDaily = ds.pv_total_daily || 0;
      let solarParts = [];
      tasmotaDaily.forEach(v => { if (v > 0) solarParts.push(v.toFixed(2)); });
      solarParts.push(pvTotalDaily.toFixed(2) + '(' + mpptDaily.map(v => v.toFixed(2)).join('+') + ')');
      let result = '<span class="highlight">☀️ ' + prod + 'kWh</span> <span class="detail">' + solarParts.join('+') + '</span> ';
      result += '<span class="money">($' + dollars + ')</span> | Grid: ' + grid + 'kWh <span class="money">($' + gridCost + ')</span> | ';
      result += '🔋 I: ' + batIn + 'kWh <span class="dim">(' + batInY + ')</span>, O: ' + batOut + 'kWh <span class="dim">(' + batOutY + ')</span>; Δ: ' + batDelta + 'kWh <span class="dim">(' + batDeltaY + ')</span>';
      return result;
    });

    const hasUpdate = computed(() => {
      const latest = state.latest_version;
      const current = state.dashboard_version;
      return latest && current && latest !== current;
    });

    const updateBtnText = computed(() => {
      return hasUpdate.value ? 'Update' : 'Check';
    });

    const updateTitle = computed(() => {
      return hasUpdate.value ? 'Update to v' + state.latest_version : 'Check for updates';
    });

    async function checkOrUpdate() {
      if (hasUpdate.value) {
        if (confirm('Update to v' + state.latest_version + '?')) {
          updating.value = true;
          try {
            const res = await fetch('/api/update', {method: 'POST'});
            const data = await res.json();
            if (data.error) {
              alert('Update failed: ' + data.error);
              updating.value = false;
            } else {
              alert('Updated to v' + data.version + ', restarting...');
              setTimeout(() => location.reload(), 3000);
            }
          } catch (e) {
            alert('Update failed: ' + e.message);
            updating.value = false;
          }
        }
      } else {
        try {
          const res = await fetch('/api/check-update', {method: 'POST'});
          const data = await res.json();
          if (data.latest && data.latest !== data.current) {
            state.latest_version = data.latest;
          } else {
            alert('You are running the latest version (v' + data.current + ')');
          }
        } catch (e) {
          alert('Failed to check for updates');
        }
      }
    }

    onMounted(() => {
      initChart();
      connect();
      window.addEventListener('resize', () => {
        if (chart && chartEl.value) chart.setSize({width: chartEl.value.clientWidth, height: 200});
      });
    });

    onUnmounted(() => {
      if (ws) ws.close();
      stopHeartbeat();
      if (reconnectTimer) clearTimeout(reconnectTimer);
    });

    return {
      state, wsConnected, mqttConnected, chartEl, isDark, updating,
      essClass, essText, mpptTotal, tasmotaTotal, evCharging, evPower, sortedLoads, dailyStatsHtml,
      batteries, solarSources, home_buttons, headerToggles,
      hasUpdate, updateBtnText, updateTitle, formatPower, formatDuration, formatSemverLabel, toggleTheme, checkOrUpdate, send, getButtonState, getHomeButton, getHomeButtonState, getHeaderToggle, MAX_HOME_SLOTS, MAX_HEADER_SLOTS
    };
  }
}).mount('#app');
</script>