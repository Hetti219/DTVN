// Ticket Management Interface
export class TicketManager {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.tickets = [];
        this.filterState = 'all';
        this.searchQuery = '';
        this.wsListenersRegistered = false;
    }

    async render(container) {
        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Ticket Management</h2>
                <p class="page-description">Validate, consume, and manage tickets</p>
                <div style="display: flex; gap: 0.5rem; margin-top: 0.5rem;">
                    <button class="btn btn-secondary" id="seed-tickets-btn">Seed 500 Tickets</button>
                    <span id="seed-result" style="align-self: center;"></span>
                </div>
            </div>

            <!-- Validate Ticket Form -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Validate Ticket</h3>
                </div>
                <div class="card-body">
                    <form id="validate-ticket-form">
                        <div class="form-group">
                            <label class="form-label" for="ticket-id">Ticket ID</label>
                            <input type="text" id="ticket-id" class="form-input" placeholder="TICKET-001" required>
                            <span class="form-help">Enter an issued ticket ID (e.g. TICKET-001 through TICKET-500)</span>
                        </div>

                        <div class="form-group">
                            <label class="form-label" for="ticket-data">Data (JSON)</label>
                            <textarea id="ticket-data" class="form-textarea" placeholder='{"event": "concert", "seat": "A1"}'></textarea>
                            <span class="form-help">Optional ticket metadata in JSON format</span>
                        </div>

                        <div id="validation-result"></div>

                        <button type="submit" class="btn btn-primary">Validate Ticket</button>
                    </form>
                </div>
            </div>

            <!-- Ticket List -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">All Tickets</h3>
                    <div style="display: flex; gap: 1rem; align-items: center;">
                        <input type="text" id="ticket-search" class="form-input" placeholder="Search tickets..." style="width: 200px;">
                        <select id="ticket-filter" class="form-select" style="width: 150px;">
                            <option value="all">All States</option>
                            <option value="ISSUED">Issued</option>
                            <option value="VALIDATED">Validated</option>
                            <option value="CONSUMED">Consumed</option>
                            <option value="DISPUTED">Disputed</option>
                        </select>
                    </div>
                </div>
                <div class="card-body">
                    <div id="ticket-list" class="table-container">
                        <p class="text-muted text-center">Loading tickets...</p>
                    </div>
                </div>
            </div>
        `;

        // Setup seed button handler
        document.getElementById('seed-tickets-btn').addEventListener('click', () => {
            this.handleSeedTickets();
        });

        // Setup form handler
        document.getElementById('validate-ticket-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.handleValidateTicket();
        });

        // Setup filter and search
        document.getElementById('ticket-filter').addEventListener('change', (e) => {
            this.filterState = e.target.value;
            this.renderTicketList();
        });

        document.getElementById('ticket-search').addEventListener('input', (e) => {
            this.searchQuery = e.target.value;
            this.renderTicketList();
        });

        // Load tickets
        await this.loadTickets();

        // Setup WebSocket listeners (only once to prevent accumulation on re-render)
        if (!this.wsListenersRegistered) {
            this.ws.on('ticket_validated', () => this.loadTickets());
            this.ws.on('ticket_consumed', () => this.loadTickets());
            this.ws.on('ticket_disputed', () => this.loadTickets());
            this.ws.on('tickets_seeded', () => this.loadTickets());
            this.wsListenersRegistered = true;
        }
    }

    async handleSeedTickets() {
        const btn = document.getElementById('seed-tickets-btn');
        const resultSpan = document.getElementById('seed-result');

        btn.disabled = true;
        btn.textContent = 'Seeding...';
        resultSpan.innerHTML = '';

        try {
            const result = await this.api.seedTickets();
            const totalSeeded = result?.total_seeded ?? result?.seeded ?? 0;
            resultSpan.innerHTML = `<span class="alert alert-success" style="padding: 0.25rem 0.5rem;">Seeded ${totalSeeded} tickets</span>`;
            await this.loadTickets();
        } catch (error) {
            resultSpan.innerHTML = `<span class="alert alert-error" style="padding: 0.25rem 0.5rem;">Error: ${error.message}</span>`;
        } finally {
            btn.disabled = false;
            btn.textContent = 'Seed 500 Tickets';
        }
    }

    async handleValidateTicket() {
        const ticketID = document.getElementById('ticket-id').value;
        const dataStr = document.getElementById('ticket-data').value;
        const resultDiv = document.getElementById('validation-result');

        // Parse data
        let data = null;
        if (dataStr.trim()) {
            try {
                data = JSON.parse(dataStr);
            } catch (error) {
                resultDiv.innerHTML = '<div class="alert alert-error">Invalid JSON data</div>';
                return;
            }
        }

        try {
            resultDiv.innerHTML = '<div class="alert alert-info">Validating ticket...</div>';

            const dataBase64 = data ? btoa(JSON.stringify(data)) : null;
            await this.api.validateTicket(ticketID, dataBase64);

            // If we get here, validation succeeded (no exception thrown)
            resultDiv.innerHTML = '<div class="alert alert-success">Ticket validated successfully!</div>';
            document.getElementById('validate-ticket-form').reset();
            await this.loadTickets();
        } catch (error) {
            resultDiv.innerHTML = `<div class="alert alert-error">Error: ${error.message}</div>`;
        }
    }

    async loadTickets() {
        try {
            const tickets = await this.api.getAllTickets();
            // API client now unwraps the response automatically
            this.tickets = tickets || [];
            this.renderTicketList();
        } catch (error) {
            console.error('Failed to load tickets:', error);
            document.getElementById('ticket-list').innerHTML = `
                <div class="alert alert-error">Failed to load tickets: ${error.message}</div>
            `;
        }
    }

    renderTicketList() {
        const listContainer = document.getElementById('ticket-list');

        if (!this.tickets || this.tickets.length === 0) {
            listContainer.innerHTML = '<p class="text-muted text-center">No tickets found. Click "Seed 500 Tickets" to load the purchased ticket database.</p>';
            return;
        }

        // Filter tickets
        let filtered = this.tickets;

        if (this.filterState !== 'all') {
            filtered = filtered.filter(t => t.State === this.filterState);
        }

        if (this.searchQuery) {
            const query = this.searchQuery.toLowerCase();
            filtered = filtered.filter(t =>
                (t.ID || '').toLowerCase().includes(query) ||
                (t.ValidatorID || '').toLowerCase().includes(query)
            );
        }

        if (filtered.length === 0) {
            listContainer.innerHTML = '<p class="text-muted text-center">No tickets match your filter</p>';
            return;
        }

        // Sort by timestamp (most recent first)
        filtered.sort((a, b) => (b.Timestamp || 0) - (a.Timestamp || 0));

        // Render table
        listContainer.innerHTML = `
            <table>
                <thead>
                    <tr>
                        <th>Ticket ID</th>
                        <th>State</th>
                        <th>Validator</th>
                        <th>Timestamp</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    ${filtered.map(ticket => `
                        <tr>
                            <td class="font-mono">${ticket.ID || '-'}</td>
                            <td><span class="badge badge-${(ticket.State || 'issued').toLowerCase()}">${ticket.State || 'ISSUED'}</span></td>
                            <td class="font-mono">${ticket.ValidatorID || '-'}</td>
                            <td>${this.formatTimestamp(ticket.Timestamp)}</td>
                            <td>
                                ${ticket.State === 'ISSUED' ? '<button class="btn btn-sm btn-primary" onclick="window.app.modules.tickets.quickValidate(\'' + ticket.ID + '\')">Validate</button>' : ''}
                                ${ticket.State === 'VALIDATED' ? '<button class="btn btn-sm btn-primary" onclick="window.app.modules.tickets.consumeTicket(\'' + ticket.ID + '\')">Consume</button>' : ''}
                                ${ticket.State !== 'DISPUTED' && ticket.State !== 'ISSUED' ? '<button class="btn btn-sm btn-error" onclick="window.app.modules.tickets.disputeTicket(\'' + ticket.ID + '\')">Dispute</button>' : ''}
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    async quickValidate(ticketID) {
        try {
            await this.api.validateTicket(ticketID, null);
            await this.loadTickets();
        } catch (error) {
            alert(`Failed to validate ticket: ${error.message}`);
        }
    }

    async consumeTicket(ticketID) {
        if (!confirm(`Consume ticket ${ticketID}?`)) return;

        try {
            await this.api.consumeTicket(ticketID);
            await this.loadTickets();
        } catch (error) {
            alert(`Failed to consume ticket: ${error.message}`);
        }
    }

    async disputeTicket(ticketID) {
        if (!confirm(`Dispute ticket ${ticketID}?`)) return;

        try {
            await this.api.disputeTicket(ticketID);
            await this.loadTickets();
        } catch (error) {
            alert(`Failed to dispute ticket: ${error.message}`);
        }
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return '-';
        const date = new Date(timestamp * 1000);
        return date.toLocaleString();
    }

    handleWSEvent(event) {
        if (event.type.startsWith('ticket_')) {
            this.loadTickets();
        }
    }
}
