// Simulator Controls
export class Simulator {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.status = 'idle';
        this.progress = null;
        this.results = null;
        this.isSupervisorMode = false;
    }

    async render(container) {
        this.container = container;

        // Check if we're in supervisor mode
        await this.checkSupervisorMode();

        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Network Simulator</h2>
                <p class="page-description">Run Byzantine fault tolerance simulations</p>
            </div>

            <div id="supervisor-mode-alert" class="alert alert-info" style="display: ${this.isSupervisorMode ? 'none' : 'block'};">
                <strong>Note:</strong> Running in validator mode - simulation features limited.
                Use supervisor mode for full simulation control with live output.
                <br><br>
                CLI command: <code class="code-block">./bin/simulator -nodes 7 -byzantine 2 -tickets 100</code>
            </div>

            <!-- Simulator Configuration -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Simulation Configuration</h3>
                    <span id="sim-status" class="badge badge-secondary">${this.status}</span>
                </div>
                <div class="card-body">
                    <form id="simulator-form">
                        <div class="form-grid">
                            <div class="form-group">
                                <label class="form-label" for="sim-nodes">Total Nodes</label>
                                <input type="number" id="sim-nodes" class="form-control" value="7" min="4" max="30">
                                <span class="form-help">4-30 nodes supported</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-byzantine">Byzantine Nodes</label>
                                <input type="number" id="sim-byzantine" class="form-control" value="2" min="0" max="10">
                                <span class="form-help" id="byzantine-limit">Max: 2 (n/3)</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-tickets">Tickets</label>
                                <input type="number" id="sim-tickets" class="form-control" value="100" min="1" max="1000">
                                <span class="form-help">1-1000 tickets</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-duration">Duration (seconds)</label>
                                <input type="number" id="sim-duration" class="form-control" value="60" min="10" max="600">
                                <span class="form-help">10-600 seconds</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-latency">Network Latency (ms)</label>
                                <input type="number" id="sim-latency" class="form-control" value="50" min="0" max="1000">
                                <span class="form-help">Simulated delay per hop</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-packet-loss">Packet Loss (%)</label>
                                <input type="number" id="sim-packet-loss" class="form-control" value="1" min="0" max="50" step="0.1">
                                <span class="form-help">0-50% loss rate</span>
                            </div>
                        </div>

                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" id="sim-partition">
                                <span>Enable Network Partitions</span>
                            </label>
                            <span class="form-help">Randomly partition network during simulation</span>
                        </div>

                        <div class="form-actions">
                            <button type="submit" class="btn btn-primary" id="start-sim-btn">
                                Start Simulation
                            </button>
                            <button type="button" class="btn btn-danger" id="stop-sim-btn" style="display: none;">
                                Stop Simulation
                            </button>
                        </div>
                    </form>
                </div>
            </div>

            <!-- Progress Card -->
            <div class="card" id="progress-card" style="display: none;">
                <div class="card-header">
                    <h3 class="card-title">Simulation Progress</h3>
                </div>
                <div class="card-body">
                    <div class="progress-container">
                        <div class="progress-bar">
                            <div class="progress-fill" id="progress-fill" style="width: 0%"></div>
                        </div>
                        <div class="progress-text" id="progress-text">0%</div>
                    </div>
                    <div class="progress-stats" id="progress-stats">
                        <div class="stat">
                            <span class="stat-label">Tickets:</span>
                            <span class="stat-value" id="stat-tickets">0/0</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Consensus Rounds:</span>
                            <span class="stat-value" id="stat-rounds">0</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Success:</span>
                            <span class="stat-value" id="stat-success">0</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Failed:</span>
                            <span class="stat-value" id="stat-failed">0</span>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Results Card -->
            <div class="card" id="results-card" style="display: none;">
                <div class="card-header">
                    <h3 class="card-title">Simulation Results</h3>
                </div>
                <div class="card-body">
                    <div class="results-grid" id="results-grid">
                        <!-- Results will be populated here -->
                    </div>
                </div>
            </div>

            <!-- Simulation Output -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Simulation Output</h3>
                    <button class="btn btn-sm btn-secondary" id="clear-output-btn">Clear</button>
                </div>
                <div class="card-body">
                    <div id="sim-output" class="console-output">
                        <div class="console-line text-muted">Simulation output will appear here...</div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();

        // Load current state if available
        await this.loadCurrentState();
    }

    setupEventListeners() {
        // Form submission
        document.getElementById('simulator-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.startSimulation();
        });

        // Stop button
        document.getElementById('stop-sim-btn').addEventListener('click', () => this.stopSimulation());

        // Clear output
        document.getElementById('clear-output-btn').addEventListener('click', () => this.clearOutput());

        // Byzantine validation
        document.getElementById('sim-nodes').addEventListener('input', () => this.validateByzantine());
        document.getElementById('sim-byzantine').addEventListener('input', () => this.validateByzantine());
    }

    async checkSupervisorMode() {
        try {
            const response = await this.api.request('/simulator');
            if (response && response.status !== undefined) {
                this.isSupervisorMode = true;
                this.status = response.status;
            }
        } catch (err) {
            this.isSupervisorMode = false;
        }
    }

    async loadCurrentState() {
        if (!this.isSupervisorMode) return;

        try {
            const state = await this.api.request('/simulator');
            if (state) {
                this.status = state.status || 'idle';
                this.updateStatus(this.status);

                if (state.progress) {
                    this.updateProgress(state.progress);
                }

                if (state.results) {
                    this.showResults(state.results);
                }

                if (state.output && state.output.length > 0) {
                    const output = document.getElementById('sim-output');
                    output.innerHTML = state.output.map(line =>
                        `<div class="console-line">${this.escapeHtml(line)}</div>`
                    ).join('');
                    output.scrollTop = output.scrollHeight;
                }
            }
        } catch (err) {
            console.error('Failed to load simulator state:', err);
        }
    }

    validateByzantine() {
        const totalNodes = parseInt(document.getElementById('sim-nodes').value) || 7;
        const byzantineInput = document.getElementById('sim-byzantine');
        const byzantineNodes = parseInt(byzantineInput.value) || 0;

        const maxByzantine = Math.floor((totalNodes - 1) / 3);
        document.getElementById('byzantine-limit').textContent = `Max: ${maxByzantine} (n/3)`;

        if (byzantineNodes > maxByzantine) {
            byzantineInput.value = maxByzantine;
        }
    }

    async startSimulation() {
        if (!this.isSupervisorMode) {
            window.app.showToast('Simulation requires supervisor mode', 'error');
            return;
        }

        const config = {
            nodes: parseInt(document.getElementById('sim-nodes').value) || 7,
            byzantine_nodes: parseInt(document.getElementById('sim-byzantine').value) || 2,
            tickets: parseInt(document.getElementById('sim-tickets').value) || 100,
            duration_seconds: parseInt(document.getElementById('sim-duration').value) || 60,
            latency_ms: parseInt(document.getElementById('sim-latency').value) || 50,
            packet_loss_rate: (parseFloat(document.getElementById('sim-packet-loss').value) || 1) / 100,
            enable_partitions: document.getElementById('sim-partition').checked
        };

        // Validate
        const maxByzantine = Math.floor((config.nodes - 1) / 3);
        if (config.byzantine_nodes > maxByzantine) {
            window.app.showToast(`Byzantine nodes cannot exceed ${maxByzantine}`, 'error');
            return;
        }

        try {
            await this.api.request('/simulator/start', {
                method: 'POST',
                body: JSON.stringify(config)
            });

            this.clearOutput();
            this.appendOutput('Starting simulation...');
            this.appendOutput(`Configuration: ${config.nodes} nodes, ${config.byzantine_nodes} Byzantine, ${config.tickets} tickets`);

            window.app.showToast('Simulation started', 'success');
        } catch (err) {
            window.app.showToast(`Failed to start simulation: ${err.message}`, 'error');
        }
    }

    async stopSimulation() {
        if (!this.isSupervisorMode) return;

        try {
            await this.api.request('/simulator/stop', {
                method: 'POST'
            });

            window.app.showToast('Stopping simulation...', 'info');
        } catch (err) {
            window.app.showToast(`Failed to stop simulation: ${err.message}`, 'error');
        }
    }

    updateStatus(status) {
        this.status = status;

        const statusBadge = document.getElementById('sim-status');
        const startBtn = document.getElementById('start-sim-btn');
        const stopBtn = document.getElementById('stop-sim-btn');
        const progressCard = document.getElementById('progress-card');
        const form = document.getElementById('simulator-form');

        if (statusBadge) {
            statusBadge.textContent = status;
            statusBadge.className = `badge badge-${this.getStatusBadgeClass(status)}`;
        }

        const isRunning = status === 'running';

        if (startBtn) {
            startBtn.style.display = isRunning ? 'none' : 'inline-block';
            startBtn.disabled = !this.isSupervisorMode;
        }

        if (stopBtn) {
            stopBtn.style.display = isRunning ? 'inline-block' : 'none';
        }

        if (progressCard) {
            progressCard.style.display = isRunning ? 'block' : 'none';
        }

        // Disable form inputs while running
        if (form) {
            const inputs = form.querySelectorAll('input');
            inputs.forEach(input => {
                input.disabled = isRunning;
            });
        }
    }

    getStatusBadgeClass(status) {
        switch (status) {
            case 'running': return 'primary';
            case 'complete': return 'success';
            case 'error': return 'danger';
            default: return 'secondary';
        }
    }

    updateProgress(progress) {
        if (!progress) return;

        this.progress = progress;

        const progressFill = document.getElementById('progress-fill');
        const progressText = document.getElementById('progress-text');
        const statTickets = document.getElementById('stat-tickets');
        const statRounds = document.getElementById('stat-rounds');
        const statSuccess = document.getElementById('stat-success');
        const statFailed = document.getElementById('stat-failed');

        const percent = progress.progress_percent || 0;

        if (progressFill) {
            progressFill.style.width = `${percent}%`;
        }

        if (progressText) {
            progressText.textContent = `${percent.toFixed(1)}%`;
        }

        if (statTickets) {
            statTickets.textContent = `${progress.current_ticket || 0}/${progress.total_tickets || 0}`;
        }

        if (statRounds) {
            statRounds.textContent = progress.consensus_rounds || 0;
        }

        if (statSuccess) {
            statSuccess.textContent = progress.successful_rounds || 0;
        }

        if (statFailed) {
            statFailed.textContent = progress.failed_rounds || 0;
        }

        // Show progress card
        const progressCard = document.getElementById('progress-card');
        if (progressCard) {
            progressCard.style.display = 'block';
        }
    }

    showResults(results) {
        if (!results) return;

        this.results = results;

        const resultsCard = document.getElementById('results-card');
        const resultsGrid = document.getElementById('results-grid');

        if (resultsCard && resultsGrid) {
            resultsCard.style.display = 'block';

            const successRate = results.success_rate || 0;
            const successClass = successRate >= 90 ? 'success' : successRate >= 70 ? 'warning' : 'danger';

            resultsGrid.innerHTML = `
                <div class="result-item">
                    <span class="result-label">Total Tickets</span>
                    <span class="result-value">${results.total_tickets || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Validated Tickets</span>
                    <span class="result-value">${results.validated_tickets || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Consensus Rounds</span>
                    <span class="result-value">${results.consensus_rounds || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Successful Rounds</span>
                    <span class="result-value text-success">${results.successful_rounds || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Failed Rounds</span>
                    <span class="result-value text-danger">${results.failed_rounds || 0}</span>
                </div>
                <div class="result-item highlight">
                    <span class="result-label">Success Rate</span>
                    <span class="result-value text-${successClass}">${successRate.toFixed(2)}%</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Messages Sent</span>
                    <span class="result-value">${results.messages_sent || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Messages Received</span>
                    <span class="result-value">${results.messages_received || 0}</span>
                </div>
                <div class="result-item">
                    <span class="result-label">Partition Events</span>
                    <span class="result-value">${results.partition_events || 0}</span>
                </div>
            `;
        }
    }

    appendOutput(line) {
        const output = document.getElementById('sim-output');
        if (!output) return;

        // Remove placeholder message
        const placeholder = output.querySelector('.text-muted');
        if (placeholder) {
            placeholder.remove();
        }

        const lineDiv = document.createElement('div');
        lineDiv.className = 'console-line';
        lineDiv.textContent = line;
        output.appendChild(lineDiv);

        // Auto-scroll if near bottom
        const isNearBottom = output.scrollHeight - output.scrollTop - output.clientHeight < 100;
        if (isNearBottom) {
            output.scrollTop = output.scrollHeight;
        }

        // Limit output lines
        while (output.children.length > 500) {
            output.removeChild(output.firstChild);
        }
    }

    clearOutput() {
        const output = document.getElementById('sim-output');
        if (output) {
            output.innerHTML = '<div class="console-line text-muted">Simulation output will appear here...</div>';
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    handleWSEvent(event) {
        if (event.type === 'simulator_status') {
            this.updateStatus(event.status);
        } else if (event.type === 'simulator_progress') {
            this.updateProgress(event.progress);
        } else if (event.type === 'simulator_output') {
            this.appendOutput(event.line);
        } else if (event.type === 'simulator_results') {
            this.showResults(event.results);
        }
    }
}
