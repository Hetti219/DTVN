// Main application entry point
import { API } from './api.js';
import { WebSocketClient } from './websocket.js';
import { Dashboard } from './dashboard.js';

class App {
    constructor() {
        this.api = null;
        this.ws = null;
        this.currentSection = 'dashboard';
        this.modules = {};
    }

    async init() {
        console.log('Initializing DTVN Web Interface...');

        // Detect API base URL (handle both validator and supervisor modes)
        const baseURL = window.location.origin;
        this.api = new API(baseURL);

        // Initialize WebSocket
        this.ws = new WebSocketClient(baseURL);
        this.ws.on('connected', () => this.handleWSConnected());
        this.ws.on('disconnected', () => this.handleWSDisconnected());
        this.ws.on('message', (event) => this.handleWSMessage(event));
        this.ws.connect();

        // Initialize modules
        this.modules.dashboard = new Dashboard(this.api, this.ws);

        // Setup navigation
        this.setupNavigation();

        // Load initial section
        await this.loadSection('dashboard');

        // Setup periodic stats updates (fallback if WebSocket fails)
        setInterval(() => this.updateStats(), 5000);

        console.log('DTVN Web Interface initialized');
    }

    setupNavigation() {
        const navItems = document.querySelectorAll('.nav-item');
        navItems.forEach(item => {
            item.addEventListener('click', () => {
                const section = item.dataset.section;
                this.loadSection(section);
            });
        });
    }

    async loadSection(section) {
        console.log(`Loading section: ${section}`);

        // Update active nav item
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.toggle('active', item.dataset.section === section);
        });

        // Get content area
        const contentArea = document.getElementById('content-area');
        contentArea.innerHTML = '<div class="loading"><div class="spinner"></div><p>Loading...</p></div>';

        this.currentSection = section;

        // Load appropriate module
        try {
            switch (section) {
                case 'dashboard':
                    if (!this.modules.dashboard) {
                        this.modules.dashboard = new Dashboard(this.api, this.ws);
                    }
                    await this.modules.dashboard.render(contentArea);
                    break;

                case 'nodes':
                    if (!this.modules.nodes) {
                        const { NodeManager } = await import('./nodes.js');
                        this.modules.nodes = new NodeManager(this.api, this.ws);
                        // Make nodeManager globally accessible for onclick handlers
                        window.nodeManager = this.modules.nodes;
                    }
                    await this.modules.nodes.render(contentArea);
                    break;

                case 'network':
                    if (!this.modules.network) {
                        const { NetworkViz } = await import('./network-viz.js');
                        this.modules.network = new NetworkViz(this.api, this.ws);
                    }
                    await this.modules.network.render(contentArea);
                    break;

                case 'tickets':
                    if (!this.modules.tickets) {
                        const { TicketManager } = await import('./tickets.js');
                        this.modules.tickets = new TicketManager(this.api, this.ws);
                    }
                    await this.modules.tickets.render(contentArea);
                    break;

                case 'simulator':
                    if (!this.modules.simulator) {
                        const { Simulator } = await import('./simulator.js');
                        this.modules.simulator = new Simulator(this.api, this.ws);
                    }
                    await this.modules.simulator.render(contentArea);
                    break;

                case 'metrics':
                    if (!this.modules.metrics) {
                        const { Metrics } = await import('./metrics.js');
                        this.modules.metrics = new Metrics(this.api, this.ws);
                    }
                    await this.modules.metrics.render(contentArea);
                    break;

                default:
                    contentArea.innerHTML = `
                        <div class="empty-state">
                            <div class="empty-state-text">Section not found</div>
                            <div class="empty-state-subtext">The requested section is not available</div>
                        </div>
                    `;
            }
        } catch (error) {
            console.error(`Failed to load section ${section}:`, error);
            contentArea.innerHTML = `
                <div class="alert alert-error">
                    <strong>Error loading section:</strong> ${error.message}
                </div>
            `;
        }
    }

    async updateStats() {
        if (!this.api) return;

        try {
            const stats = await this.api.getStats();

            // Update header
            document.getElementById('node-id').textContent = stats.node_id || 'Unknown';

            // Notify current module
            if (this.modules[this.currentSection]?.updateStats) {
                this.modules[this.currentSection].updateStats(stats);
            }
        } catch (error) {
            console.error('Failed to fetch stats:', error);
        }
    }

    handleWSConnected() {
        console.log('WebSocket connected');
        document.getElementById('connection-status').classList.remove('offline');
        document.getElementById('connection-status').classList.add('online');
        document.getElementById('connection-text').textContent = 'Connected';
        this.showToast('Connected to server', 'success');
    }

    handleWSDisconnected() {
        console.log('WebSocket disconnected');
        document.getElementById('connection-status').classList.remove('online');
        document.getElementById('connection-status').classList.add('offline');
        document.getElementById('connection-text').textContent = 'Disconnected';
        this.showToast('Disconnected from server', 'warning');
    }

    handleWSMessage(event) {
        console.log('WebSocket message:', event);

        // Route to current module if it has a handler
        if (this.modules[this.currentSection]?.handleWSEvent) {
            this.modules[this.currentSection].handleWSEvent(event);
        }

        // Handle global events
        switch (event.type) {
            case 'ticket_validated':
            case 'ticket_consumed':
            case 'ticket_disputed':
                this.showToast(`Ticket ${event.ticket_id}: ${event.type.replace('ticket_', '')}`, 'success');
                break;

            case 'node_started':
                this.showToast(`Node ${event.node_id} started`, 'success');
                break;

            case 'node_stopped':
                this.showToast(`Node ${event.node_id} stopped`, 'warning');
                break;

            case 'node_failed':
                this.showToast(`Node ${event.node_id} failed: ${event.error}`, 'error');
                break;

            case 'simulation_completed':
                this.showToast('Simulation completed', 'success');
                break;
        }
    }

    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            <span class="toast-icon">${this.getToastIcon(type)}</span>
            <span class="toast-message">${message}</span>
        `;

        container.appendChild(toast);

        // Auto-remove after 5 seconds
        setTimeout(() => {
            toast.style.animation = 'slideOut 0.3s ease-out';
            setTimeout(() => toast.remove(), 300);
        }, 5000);
    }

    getToastIcon(type) {
        switch (type) {
            case 'success': return '✓';
            case 'error': return '✗';
            case 'warning': return '⚠';
            default: return 'ℹ';
        }
    }
}

// Add slideOut animation for toasts
const style = document.createElement('style');
style.textContent = `
    @keyframes slideOut {
        to {
            transform: translateX(400px);
            opacity: 0;
        }
    }
`;
document.head.appendChild(style);

// Initialize app when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        const app = new App();
        app.init();
        window.app = app; // Make available for debugging
    });
} else {
    const app = new App();
    app.init();
    window.app = app;
}
