// Metrics and Charts - Real-time performance monitoring
export class Metrics {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.charts = {};
        this.metricsHistory = {
            timestamps: [],
            sequence: [],
            peerCount: []
        };
        this.MAX_HISTORY = 60;
        this.wsListenersRegistered = false;
    }

    async render(container) {
        this.destroyCharts();

        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Metrics</h2>
                <p class="page-description">Real-time performance metrics and network monitoring</p>
            </div>

            <!-- Summary Stats -->
            <div class="results-grid" style="margin-bottom: var(--spacing-lg);">
                <div class="result-item">
                    <span class="result-label">Consensus Rounds</span>
                    <span class="result-value" id="metric-sequence">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Connected Peers</span>
                    <span class="result-value" id="metric-peers">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Total Tickets</span>
                    <span class="result-value" id="metric-total-tickets">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Cluster Nodes</span>
                    <span class="result-value" id="metric-cluster-nodes">-</span>
                </div>
            </div>

            <!-- Secondary Stats -->
            <div class="results-grid" style="margin-bottom: var(--spacing-lg);">
                <div class="result-item">
                    <span class="result-label">PBFT View</span>
                    <span class="result-value" id="metric-view">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Gossip Cache</span>
                    <span class="result-value" id="metric-cache">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Storage Writes</span>
                    <span class="result-value" id="metric-writes">-</span>
                </div>
                <div class="result-item">
                    <span class="result-label">DB Pages</span>
                    <span class="result-value" id="metric-pages">-</span>
                </div>
            </div>

            <!-- Charts -->
            <div class="dashboard-grid">
                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Consensus Progress</h3>
                    </div>
                    <div class="card-body" style="height: 250px;">
                        <canvas id="consensus-chart"></canvas>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Peer Count</h3>
                    </div>
                    <div class="card-body" style="height: 250px;">
                        <canvas id="peer-chart"></canvas>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Ticket Distribution</h3>
                    </div>
                    <div class="card-body" style="height: 250px;">
                        <canvas id="ticket-chart"></canvas>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Node Health</h3>
                    </div>
                    <div class="card-body" style="height: 250px;">
                        <canvas id="node-health-chart"></canvas>
                    </div>
                </div>
            </div>
        `;

        this.initCharts();
        await this.loadMetrics();

        if (!this.wsListenersRegistered) {
            this.ws.on('ticket_validated', () => this.fetchTickets());
            this.ws.on('ticket_consumed', () => this.fetchTickets());
            this.ws.on('ticket_disputed', () => this.fetchTickets());
            this.ws.on('tickets_seeded', () => this.fetchTickets());
            this.ws.on('node_status', () => this.fetchClusterStatus());
            this.ws.on('cluster_status', () => this.fetchClusterStatus());
            this.wsListenersRegistered = true;
        }
    }

    initCharts() {
        const chartFont = { family: "'Inter', -apple-system, sans-serif" };
        const gridColor = 'rgba(47, 51, 54, 0.5)';

        this.charts.consensus = new Chart(document.getElementById('consensus-chart'), {
            type: 'line',
            data: {
                labels: [],
                datasets: [{
                    label: 'Sequence #',
                    data: [],
                    borderColor: '#1D9BF0',
                    backgroundColor: 'rgba(29, 155, 240, 0.08)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0,
                    borderWidth: 2
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                animation: { duration: 300 },
                interaction: { intersect: false, mode: 'index' },
                scales: {
                    x: {
                        display: true,
                        grid: { display: false },
                        ticks: { maxTicksLimit: 8, font: chartFont, color: '#71767B' }
                    },
                    y: {
                        beginAtZero: true,
                        grid: { color: gridColor },
                        ticks: { font: chartFont, color: '#71767B', precision: 0 },
                        title: { display: true, text: 'Sequence #', font: chartFont, color: '#71767B' }
                    }
                },
                plugins: { legend: { display: false } }
            }
        });

        this.charts.peers = new Chart(document.getElementById('peer-chart'), {
            type: 'line',
            data: {
                labels: [],
                datasets: [{
                    label: 'Peers',
                    data: [],
                    borderColor: '#00BA7C',
                    backgroundColor: 'rgba(0, 186, 124, 0.08)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0,
                    borderWidth: 2
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                animation: { duration: 300 },
                interaction: { intersect: false, mode: 'index' },
                scales: {
                    x: {
                        display: true,
                        grid: { display: false },
                        ticks: { maxTicksLimit: 8, font: chartFont, color: '#71767B' }
                    },
                    y: {
                        beginAtZero: true,
                        grid: { color: gridColor },
                        ticks: { font: chartFont, color: '#71767B', stepSize: 1 },
                        title: { display: true, text: 'Peers', font: chartFont, color: '#71767B' }
                    }
                },
                plugins: { legend: { display: false } }
            }
        });

        this.charts.tickets = new Chart(document.getElementById('ticket-chart'), {
            type: 'doughnut',
            data: {
                labels: ['Issued', 'Validated', 'Consumed', 'Disputed'],
                datasets: [{
                    data: [0, 0, 0, 0],
                    backgroundColor: ['#71767B', '#00BA7C', '#1D9BF0', '#F4212E'],
                    borderWidth: 0,
                    hoverOffset: 4
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                animation: { duration: 300 },
                cutout: '60%',
                plugins: {
                    legend: {
                        position: 'bottom',
                        labels: { font: chartFont, color: '#71767B', padding: 16, usePointStyle: true }
                    }
                }
            }
        });

        this.charts.nodeHealth = new Chart(document.getElementById('node-health-chart'), {
            type: 'bar',
            data: {
                labels: ['Running', 'Starting', 'Stopped', 'Error'],
                datasets: [{
                    label: 'Nodes',
                    data: [0, 0, 0, 0],
                    backgroundColor: ['#00BA7C', '#FFD400', '#71767B', '#F4212E'],
                    borderRadius: 4,
                    borderSkipped: false
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                animation: { duration: 300 },
                scales: {
                    x: {
                        grid: { display: false },
                        ticks: { font: chartFont, color: '#71767B' }
                    },
                    y: {
                        beginAtZero: true,
                        grid: { color: gridColor },
                        ticks: { font: chartFont, color: '#71767B', stepSize: 1, precision: 0 }
                    }
                },
                plugins: { legend: { display: false } }
            }
        });
    }

    destroyCharts() {
        Object.values(this.charts).forEach(chart => {
            if (chart) chart.destroy();
        });
        this.charts = {};
    }

    async loadMetrics() {
        const [statsResult, ticketsResult, statusResult] = await Promise.allSettled([
            this.api.getStats(),
            this.api.getAllTickets(),
            this.api.getStatus()
        ]);

        if (statsResult.status === 'fulfilled' && statsResult.value) {
            this.pushStatsToHistory(statsResult.value);
            this.updateSummaryFromStats(statsResult.value);
            this.updateLineCharts();
        }

        if (ticketsResult.status === 'fulfilled') {
            this.updateTicketChart(ticketsResult.value);
        }

        if (statusResult.status === 'fulfilled' && statusResult.value) {
            this.updateNodeHealthChart(statusResult.value);
        }
    }

    updateStats(stats) {
        if (!stats) return;

        this.pushStatsToHistory(stats);
        this.updateSummaryFromStats(stats);
        this.updateLineCharts();

        this.fetchTickets();
        this.fetchClusterStatus();
    }

    pushStatsToHistory(stats) {
        const now = new Date();
        const timeLabel = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

        this.metricsHistory.timestamps.push(timeLabel);
        this.metricsHistory.sequence.push(stats.sequence || 0);
        this.metricsHistory.peerCount.push(stats.peer_count || 0);

        if (this.metricsHistory.timestamps.length > this.MAX_HISTORY) {
            this.metricsHistory.timestamps.shift();
            this.metricsHistory.sequence.shift();
            this.metricsHistory.peerCount.shift();
        }
    }

    updateSummaryFromStats(stats) {
        this.setSummaryValue('metric-sequence', stats.sequence || 0);
        this.setSummaryValue('metric-peers', stats.peer_count || 0);
        this.setSummaryValue('metric-view', stats.current_view || 0);
        this.setSummaryValue('metric-cache', stats.cache_size || 0);

        if (stats.storage_stats) {
            this.setSummaryValue('metric-writes', stats.storage_stats.write || 0);
            this.setSummaryValue('metric-pages', stats.storage_stats.page_count || 0);
        }
    }

    updateLineCharts() {
        if (this.charts.consensus) {
            this.charts.consensus.data.labels = [...this.metricsHistory.timestamps];
            this.charts.consensus.data.datasets[0].data = [...this.metricsHistory.sequence];
            this.charts.consensus.update('none');
        }
        if (this.charts.peers) {
            this.charts.peers.data.labels = [...this.metricsHistory.timestamps];
            this.charts.peers.data.datasets[0].data = [...this.metricsHistory.peerCount];
            this.charts.peers.update('none');
        }
    }

    async fetchTickets() {
        try {
            const tickets = await this.api.getAllTickets();
            this.updateTicketChart(tickets);
        } catch (err) {
            // Silently fail — tickets endpoint may not be available when no nodes are running
        }
    }

    updateTicketChart(tickets) {
        if (!Array.isArray(tickets)) {
            this.setSummaryValue('metric-total-tickets', 0);
            return;
        }

        const counts = { ISSUED: 0, VALIDATED: 0, CONSUMED: 0, DISPUTED: 0 };
        tickets.forEach(t => {
            if (t.State in counts) counts[t.State]++;
        });

        this.setSummaryValue('metric-total-tickets', tickets.length);

        if (this.charts.tickets) {
            this.charts.tickets.data.datasets[0].data = [
                counts.ISSUED, counts.VALIDATED, counts.CONSUMED, counts.DISPUTED
            ];
            this.charts.tickets.update('none');
        }
    }

    async fetchClusterStatus() {
        try {
            const status = await this.api.getStatus();
            this.updateNodeHealthChart(status);
        } catch (err) {
            // Silently fail
        }
    }

    updateNodeHealthChart(status) {
        if (!status) return;
        const nodes = status.nodes || [];

        const running = nodes.filter(n => n.status === 'running').length;
        const starting = nodes.filter(n => n.status === 'starting' || n.status === 'stopping').length;
        const stopped = nodes.filter(n => n.status === 'stopped').length;
        const errored = nodes.filter(n => n.status === 'error').length;

        this.setSummaryValue('metric-cluster-nodes', nodes.length > 0 ? `${running}/${nodes.length}` : '-');

        if (this.charts.nodeHealth) {
            this.charts.nodeHealth.data.datasets[0].data = [running, starting, stopped, errored];
            this.charts.nodeHealth.update('none');
        }
    }

    setSummaryValue(id, value) {
        const el = document.getElementById(id);
        if (el) el.textContent = value;
    }

    handleWSEvent(event) {
        switch (event.type) {
            case 'stats_update':
                if (event.stats) this.updateStats(event.stats);
                break;
            case 'ticket_validated':
            case 'ticket_consumed':
            case 'ticket_disputed':
            case 'tickets_seeded':
                this.fetchTickets();
                break;
            case 'node_status':
            case 'cluster_status':
                this.fetchClusterStatus();
                break;
        }
    }
}
