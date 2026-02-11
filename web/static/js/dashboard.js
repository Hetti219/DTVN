// Dashboard module - Overview of node status and recent activity
export class Dashboard {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.stats = null;
        this.recentTickets = [];
        this.wsListenersRegistered = false;
    }

    async render(container) {
        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Dashboard</h2>
                <p class="page-description">Overview of node status and network activity</p>
            </div>

            <div class="dashboard-grid">
                <!-- Node Status Card -->
                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Node Status</h3>
                    </div>
                    <div class="card-body">
                        <div class="stat-row">
                            <span>Node ID</span>
                            <span id="dash-node-id" class="font-mono">-</span>
                        </div>
                        <div class="stat-row">
                            <span>Role</span>
                            <span id="dash-role" class="badge">-</span>
                        </div>
                        <div class="stat-row">
                            <span>Peers Connected</span>
                            <span id="dash-peer-count">-</span>
                        </div>
                        <div class="stat-row">
                            <span>PBFT View</span>
                            <span id="dash-view" class="font-mono">-</span>
                        </div>
                        <div class="stat-row">
                            <span>Sequence Number</span>
                            <span id="dash-sequence" class="font-mono">-</span>
                        </div>
                        <div class="stat-row">
                            <span>Cache Size</span>
                            <span id="dash-cache-size">-</span>
                        </div>
                    </div>
                </div>

                <!-- Network Health Card -->
                <div class="card">
                    <div class="card-header">
                        <h3 class="card-title">Network Health</h3>
                    </div>
                    <div class="card-body">
                        <div class="stat">
                            <span class="stat-label">Connection Status</span>
                            <span id="dash-connection" class="badge badge-validated">Healthy</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Total Tickets</span>
                            <span id="dash-total-tickets" class="stat-value">0</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Issued (Awaiting Scan)</span>
                            <span id="dash-issued-tickets" class="stat-value">0</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Validated</span>
                            <span id="dash-validated-tickets" class="stat-value">0</span>
                        </div>
                    </div>
                </div>

                <!-- Recent Activity Card -->
                <div class="card" style="grid-column: 1 / -1;">
                    <div class="card-header">
                        <h3 class="card-title">Recent Activity</h3>
                        <button class="btn btn-sm btn-secondary" onclick="window.app.loadSection('tickets')">
                            View All Tickets
                        </button>
                    </div>
                    <div class="card-body">
                        <div id="recent-activity" class="table-container">
                            <p class="text-muted text-center">No recent activity</p>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Quick Actions -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Quick Actions</h3>
                </div>
                <div class="card-body">
                    <div class="quick-actions">
                        <button class="btn btn-primary" onclick="window.app.loadSection('tickets')">
                            Validate Ticket
                        </button>
                        <button class="btn btn-primary" onclick="window.app.loadSection('simulator')">
                            Run Simulation
                        </button>
                        <button class="btn btn-secondary" onclick="window.app.loadSection('network')">
                            View Network
                        </button>
                        <button class="btn btn-secondary" onclick="window.app.loadSection('metrics')">
                            View Metrics
                        </button>
                    </div>
                </div>
            </div>
        `;

        // Load initial data
        await this.loadData();

        // Setup WebSocket listeners (only once to prevent accumulation on re-render)
        if (!this.wsListenersRegistered) {
            this.ws.on('ticket_validated', () => this.loadData());
            this.ws.on('ticket_consumed', () => this.loadData());
            this.ws.on('ticket_disputed', () => this.loadData());
            this.ws.on('tickets_seeded', () => this.loadData());
            this.ws.on('stats_update', (data) => this.updateStats(data.stats));
            this.wsListenersRegistered = true;
        }
    }

    async loadData() {
        try {
            // Fetch stats (api.js already unwraps {success, data} envelope)
            const stats = await this.api.getStats();
            if (stats) {
                this.updateStats(stats);
            }

            // Fetch tickets (api.js already unwraps {success, data} envelope)
            const tickets = await this.api.getAllTickets();
            if (tickets) {
                this.updateRecentActivity(Array.isArray(tickets) ? tickets : []);
            }
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
        }
    }

    updateStats(stats) {
        if (!stats) return;

        this.stats = stats;

        // Update node info
        document.getElementById('dash-node-id').textContent = stats.node_id || '-';

        // Update role badge
        const roleElement = document.getElementById('dash-role');
        const isPrimary = stats.is_primary;
        roleElement.textContent = isPrimary ? 'Primary' : 'Replica';
        roleElement.className = isPrimary ? 'badge badge-primary' : 'badge badge-replica';

        // Update peer count
        document.getElementById('dash-peer-count').textContent = stats.peer_count || 0;

        // Update PBFT state
        document.getElementById('dash-view').textContent = stats.current_view || 0;
        document.getElementById('dash-sequence').textContent = stats.sequence || 0;

        // Update cache size
        document.getElementById('dash-cache-size').textContent = stats.cache_size || 0;
    }

    updateRecentActivity(tickets) {
        if (!Array.isArray(tickets)) return;

        // Sort by timestamp (most recent first) and take last 10
        this.recentTickets = tickets
            .sort((a, b) => (b.Timestamp || 0) - (a.Timestamp || 0))
            .slice(0, 10);

        const activityContainer = document.getElementById('recent-activity');

        if (this.recentTickets.length === 0) {
            activityContainer.innerHTML = '<p class="text-muted text-center">No recent activity</p>';
            return;
        }

        // Update ticket counts
        document.getElementById('dash-total-tickets').textContent = tickets.length;
        document.getElementById('dash-issued-tickets').textContent = tickets.filter(t => t.State === 'ISSUED').length;
        document.getElementById('dash-validated-tickets').textContent = tickets.filter(t => t.State === 'VALIDATED').length;

        // Build table
        activityContainer.innerHTML = `
            <table>
                <thead>
                    <tr>
                        <th>Ticket ID</th>
                        <th>State</th>
                        <th>Validator</th>
                        <th>Timestamp</th>
                    </tr>
                </thead>
                <tbody>
                    ${this.recentTickets.map(ticket => `
                        <tr onclick="window.app.loadSection('tickets')">
                            <td class="font-mono">${ticket.ID || '-'}</td>
                            <td><span class="badge badge-${(ticket.State || 'issued').toLowerCase()}">${ticket.State || 'ISSUED'}</span></td>
                            <td class="font-mono">${ticket.ValidatorID || '-'}</td>
                            <td>${this.formatTimestamp(ticket.Timestamp)}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return '-';

        // Convert Unix timestamp to readable format
        const date = new Date(timestamp * 1000);
        return date.toLocaleString();
    }

    handleWSEvent(event) {
        // Handle WebSocket events specific to dashboard
        if (event.type === 'stats_update' && event.stats) {
            this.updateStats(event.stats);
        }
    }
}
