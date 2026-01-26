// API Client for DTVN REST API
export class API {
    constructor(baseURL) {
        this.baseURL = baseURL;
        this.apiBase = `${baseURL}/api/v1`;
    }

    async request(endpoint, options = {}) {
        const url = endpoint.startsWith('http') ? endpoint : `${this.apiBase}${endpoint}`;

        const defaultOptions = {
            headers: {
                'Content-Type': 'application/json',
            },
        };

        const config = { ...defaultOptions, ...options };

        try {
            const response = await fetch(url, config);
            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error || `HTTP ${response.status}: ${response.statusText}`);
            }

            // Backend wraps responses in {success, message, data, error} format
            // Return the data field if it exists, otherwise return the whole response
            if (data && typeof data === 'object' && 'success' in data) {
                if (!data.success) {
                    throw new Error(data.error || 'Request failed');
                }
                return data.data !== undefined ? data.data : data;
            }

            return data;
        } catch (error) {
            console.error(`API request failed: ${endpoint}`, error);
            throw error;
        }
    }

    // Ticket operations
    async validateTicket(ticketID, data) {
        return this.request('/tickets/validate', {
            method: 'POST',
            body: JSON.stringify({ ticket_id: ticketID, data }),
        });
    }

    async consumeTicket(ticketID) {
        return this.request('/tickets/consume', {
            method: 'POST',
            body: JSON.stringify({ ticket_id: ticketID }),
        });
    }

    async disputeTicket(ticketID) {
        return this.request('/tickets/dispute', {
            method: 'POST',
            body: JSON.stringify({ ticket_id: ticketID }),
        });
    }

    async getTicket(ticketID) {
        return this.request(`/tickets/${ticketID}`);
    }

    async getAllTickets() {
        return this.request('/tickets');
    }

    // Node status and stats
    async getStatus() {
        return this.request('/status');
    }

    async getStats() {
        return this.request('/stats');
    }

    async getHealth() {
        return this.request(`${this.baseURL}/health`);
    }

    // Peer information
    async getPeers() {
        return this.request('/peers');
    }

    async getConfig() {
        return this.request('/config');
    }

    // Node management (supervisor APIs)
    async startNode(config) {
        return this.request('/nodes/start', {
            method: 'POST',
            body: JSON.stringify(config),
        });
    }

    async stopNode(nodeID) {
        return this.request(`/nodes/stop/${nodeID}`, {
            method: 'POST',
        });
    }

    async getNodeList() {
        return this.request('/nodes/list');
    }

    async getNodeLogs(nodeID, lines = 100) {
        return this.request(`/nodes/${nodeID}/logs?lines=${lines}`);
    }

    // Simulator control
    async startSimulation(config) {
        return this.request('/simulator/start', {
            method: 'POST',
            body: JSON.stringify(config),
        });
    }

    async stopSimulation(simID) {
        return this.request(`/simulator/stop/${simID}`, {
            method: 'POST',
        });
    }

    async getSimulationStatus(simID) {
        return this.request(`/simulator/status/${simID}`);
    }

    async getSimulationList() {
        return this.request('/simulator/list');
    }
}
