// Simulator Controls
export class Simulator {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.activeSimulations = [];
    }

    async render(container) {
        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Network Simulator</h2>
                <p class="page-description">Run Byzantine fault tolerance simulations</p>
            </div>

            <div class="alert alert-info">
                <strong>Note:</strong> Full simulator integration will be available in supervisor mode.
                For now, use CLI: <code class="code-block">./bin/simulator -nodes 7 -byzantine 2 -tickets 100</code>
            </div>

            <!-- Simulator Configuration -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Simulation Configuration</h3>
                </div>
                <div class="card-body">
                    <form id="simulator-form">
                        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 1rem;">
                            <div class="form-group">
                                <label class="form-label" for="sim-nodes">Total Nodes</label>
                                <input type="number" id="sim-nodes" class="form-input" value="7" min="4" max="30">
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-byzantine">Byzantine Nodes</label>
                                <input type="number" id="sim-byzantine" class="form-input" value="2" min="0" max="10">
                                <span class="form-help" id="byzantine-limit">Max: 2 (n/3)</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-tickets">Tickets</label>
                                <input type="number" id="sim-tickets" class="form-input" value="100" min="1" max="1000">
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-duration">Duration (seconds)</label>
                                <input type="number" id="sim-duration" class="form-input" value="60" min="10" max="600">
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-latency">Network Latency (ms)</label>
                                <input type="number" id="sim-latency" class="form-input" value="50" min="0" max="1000">
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="sim-packet-loss">Packet Loss (%)</label>
                                <input type="number" id="sim-packet-loss" class="form-input" value="1" min="0" max="50" step="0.1">
                            </div>
                        </div>

                        <div class="form-group">
                            <label style="display: flex; align-items: center; gap: 0.5rem;">
                                <input type="checkbox" id="sim-partition">
                                <span>Enable Network Partitions</span>
                            </label>
                        </div>

                        <button type="submit" class="btn btn-primary" disabled>Start Simulation (Available in Supervisor Mode)</button>
                    </form>
                </div>
            </div>

            <!-- Simulation Output -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Simulation Output</h3>
                </div>
                <div class="card-body">
                    <div id="sim-output" class="console">
                        <div class="console-line">Simulation output will appear here when running...</div>
                    </div>
                </div>
            </div>
        `;

        // Setup Byzantine validation
        document.getElementById('sim-nodes').addEventListener('input', () => this.validateByzantine());
        document.getElementById('sim-byzantine').addEventListener('input', () => this.validateByzantine());

        // TODO: Full implementation in Stage 5
    }

    validateByzantine() {
        const totalNodes = parseInt(document.getElementById('sim-nodes').value);
        const byzantineNodes = parseInt(document.getElementById('sim-byzantine').value);

        const maxByzantine = Math.floor((totalNodes - 1) / 3);
        document.getElementById('byzantine-limit').textContent = `Max: ${maxByzantine} (n/3)`;

        if (byzantineNodes > maxByzantine) {
            document.getElementById('sim-byzantine').value = maxByzantine;
        }
    }

    handleWSEvent(event) {
        if (event.type.startsWith('simulation_')) {
            // TODO: Handle simulation events in Stage 5
            console.log('Simulation event:', event);
        }
    }
}
