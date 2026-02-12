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
                <div class="page-header-actions">
                    <button class="btn btn-secondary" id="seed-tickets-btn">Seed 500 Tickets</button>
                    <span id="seed-result"></span>
                </div>
            </div>

            <!-- Ticket List -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">All Tickets</h3>
                    <div class="filter-bar">
                        <input type="text" id="ticket-search" class="form-input" placeholder="Search tickets...">
                        <select id="ticket-filter" class="form-select">
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
            resultSpan.innerHTML = `<span class="inline-feedback success">Seeded ${totalSeeded} tickets</span>`;
            await this.loadTickets();
        } catch (error) {
            resultSpan.innerHTML = `<span class="inline-feedback error">Error: ${error.message}</span>`;
        } finally {
            btn.disabled = false;
            btn.textContent = 'Seed 500 Tickets';
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
                                <div class="ticket-actions">
                                    ${ticket.State === 'ISSUED' ? '<button class="btn btn-sm btn-primary" onclick="window.app.modules.tickets.quickValidate(\'' + ticket.ID + '\')">Validate</button>' : ''}
                                    ${ticket.State === 'VALIDATED' ? '<button class="btn btn-sm btn-primary" onclick="window.app.modules.tickets.consumeTicket(\'' + ticket.ID + '\')">Consume</button>' : ''}
                                    ${ticket.State !== 'DISPUTED' && ticket.State !== 'ISSUED' ? '<button class="btn btn-sm btn-danger" onclick="window.app.modules.tickets.disputeTicket(\'' + ticket.ID + '\')">Dispute</button>' : ''}
                                </div>
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
        if (event.type.startsWith('ticket_') || event.type === 'tickets_seeded') {
            this.loadTickets();
        }
    }
}
