// Node Lifecycle Management
export class NodeManager {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.nodes = [];
        this.container = null;
        this.isSupervisorMode = false;
        this.selectedNodeId = null;
    }

    async render(container) {
        this.container = container;

        // Check if we're in supervisor mode
        await this.checkSupervisorMode();

        const supervisorControlsDisabled = !this.isSupervisorMode ? 'disabled' : '';
        const supervisorControlsTitle = !this.isSupervisorMode ? 'title="Run supervisor binary for node control"' : '';

        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Node Management</h2>
                <p class="page-description">Start, stop, and monitor validator nodes</p>
            </div>

            <div id="supervisor-mode-alert" class="alert alert-info" style="display: none;">
                <strong>Note:</strong> Running in validator mode - limited node management.
                Use the supervisor binary for full control.
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Cluster Control</h3>
                    <div class="btn-group">
                        <button class="btn btn-success" id="start-cluster-btn" ${supervisorControlsDisabled} ${supervisorControlsTitle}>
                            Start Cluster
                        </button>
                        <button class="btn btn-danger" id="stop-cluster-btn" ${supervisorControlsDisabled} ${supervisorControlsTitle}>
                            Stop All
                        </button>
                    </div>
                </div>
                <div class="card-body">
                    <div class="form-row">
                        <div class="form-group">
                            <label for="cluster-size">Cluster Size</label>
                            <input type="number" id="cluster-size" class="form-control"
                                   value="4" min="1" max="30" style="width: 100px;" ${supervisorControlsDisabled}>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Running Nodes</h3>
                    <button class="btn btn-primary" id="add-node-btn" ${supervisorControlsDisabled} ${supervisorControlsTitle}>
                        + Add Node
                    </button>
                </div>
                <div class="card-body">
                    <div id="node-list" class="node-grid">
                        <div class="loading">Loading nodes...</div>
                    </div>
                </div>
            </div>

            <div class="card" id="node-logs-card" style="display: none;">
                <div class="card-header">
                    <h3 class="card-title">Node Logs: <span id="logs-node-id"></span></h3>
                    <button class="btn btn-secondary btn-sm" id="close-logs-btn">Close</button>
                </div>
                <div class="card-body">
                    <div id="node-logs-output" class="console-output"></div>
                </div>
            </div>

            <!-- Start Node Modal -->
            <div id="start-node-modal" class="modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3>Start New Node</h3>
                        <button class="modal-close" id="close-modal-btn">&times;</button>
                    </div>
                    <div class="modal-body">
                        <div class="form-group">
                            <label for="new-node-id">Node ID</label>
                            <input type="text" id="new-node-id" class="form-control"
                                   placeholder="e.g., node5">
                        </div>
                        <div class="form-group">
                            <label>
                                <input type="checkbox" id="new-node-primary"> Primary Node
                            </label>
                        </div>
                        <div class="form-group">
                            <label>
                                <input type="checkbox" id="new-node-bootstrap"> Bootstrap Node
                            </label>
                        </div>
                        <div class="form-group">
                            <label for="new-node-bootstrap-addr">Bootstrap Address (optional)</label>
                            <input type="text" id="new-node-bootstrap-addr" class="form-control"
                                   placeholder="/ip4/127.0.0.1/tcp/4001/p2p/...">
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="cancel-start-btn">Cancel</button>
                        <button class="btn btn-primary" id="confirm-start-btn">Start Node</button>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
        this.setupWSHandlers();
        this.updateSupervisorModeAlert();
        await this.loadNodes();
    }

    setupEventListeners() {
        // Cluster controls
        document.getElementById('start-cluster-btn').addEventListener('click', () => this.startCluster());
        document.getElementById('stop-cluster-btn').addEventListener('click', () => this.stopAllNodes());

        // Add node button
        document.getElementById('add-node-btn').addEventListener('click', () => this.showStartNodeModal());

        // Modal controls
        document.getElementById('close-modal-btn').addEventListener('click', () => this.hideStartNodeModal());
        document.getElementById('cancel-start-btn').addEventListener('click', () => this.hideStartNodeModal());
        document.getElementById('confirm-start-btn').addEventListener('click', () => this.startNewNode());

        // Logs close
        document.getElementById('close-logs-btn').addEventListener('click', () => this.hideLogs());

        // Close modal on outside click
        document.getElementById('start-node-modal').addEventListener('click', (e) => {
            if (e.target.id === 'start-node-modal') {
                this.hideStartNodeModal();
            }
        });
    }

    setupWSHandlers() {
        this.ws.on('node_status', (data) => {
            this.updateNodeStatus(data.node_id, data.status, data.node);
        });

        this.ws.on('node_output', (data) => {
            if (this.selectedNodeId === data.node_id) {
                this.appendLog(data.line);
            }
        });

        this.ws.on('cluster_status', (data) => {
            this.handleClusterStatus(data);
        });
    }

    handleClusterStatus(data) {
        const startBtn = document.getElementById('start-cluster-btn');
        if (startBtn) {
            if (data.status === 'starting') {
                startBtn.disabled = true;
                startBtn.textContent = `Starting... (${data.nodes_started}/${data.nodes_total})`;
            } else {
                startBtn.disabled = false;
                startBtn.textContent = 'Start Cluster';
                if (data.status === 'running') {
                    window.app.showToast(`Cluster ready: ${data.nodes_total} nodes started`, 'success');
                }
            }
        }
    }

    async checkSupervisorMode() {
        try {
            // Try to access supervisor-specific endpoint
            const response = await this.api.request('/nodes');
            if (response && Array.isArray(response)) {
                this.isSupervisorMode = true;
            }
        } catch (err) {
            // Not in supervisor mode
            this.isSupervisorMode = false;
        }
    }

    updateSupervisorModeAlert() {
        const alert = document.getElementById('supervisor-mode-alert');
        if (alert) {
            alert.style.display = this.isSupervisorMode ? 'none' : 'block';
        }
    }

    async loadNodes() {
        const nodeList = document.getElementById('node-list');
        if (!nodeList) return;

        try {
            const nodes = await this.api.request('/nodes');
            this.nodes = nodes || [];
            this.renderNodeList();
        } catch (err) {
            // Fallback: Show current node info from stats
            try {
                const stats = await this.api.getStats();
                this.nodes = [{
                    id: stats.node_id || 'current',
                    status: 'running',
                    is_primary: stats.is_primary,
                    api_port: window.location.port || 8080,
                    port: 4001
                }];
                this.renderNodeList();
            } catch (e) {
                nodeList.innerHTML = `
                    <div class="alert alert-warning">
                        Unable to load node information.
                        Make sure you're running in supervisor mode for full node management.
                    </div>
                `;
            }
        }
    }

    renderNodeList() {
        const nodeList = document.getElementById('node-list');
        if (!nodeList) return;

        if (this.nodes.length === 0) {
            nodeList.innerHTML = `
                <div class="empty-state">
                    <p>No nodes running. ${this.isSupervisorMode ? 'Start a cluster or add individual nodes.' : 'Run the supervisor binary for node management.'}</p>
                </div>
            `;
            return;
        }

        nodeList.innerHTML = this.nodes.map(node => `
            <div class="node-card ${node.status}" data-node-id="${node.id}">
                <div class="node-header">
                    <span class="node-id">${node.id}</span>
                    <span class="node-status status-${node.status}">${node.status}</span>
                </div>
                <div class="node-details">
                    <div class="node-detail">
                        <span class="detail-label">Role:</span>
                        <span class="detail-value">${node.is_primary ? 'Primary' : 'Replica'}</span>
                    </div>
                    <div class="node-detail">
                        <span class="detail-label">P2P Port:</span>
                        <span class="detail-value">${node.port || 'N/A'}</span>
                    </div>
                    <div class="node-detail">
                        <span class="detail-label">API Port:</span>
                        <span class="detail-value">${node.api_port || 'N/A'}</span>
                    </div>
                    ${node.error ? `
                        <div class="node-detail error">
                            <span class="detail-label">Error:</span>
                            <span class="detail-value">${node.error}</span>
                        </div>
                    ` : ''}
                </div>
                ${this.isSupervisorMode ? `
                    <div class="node-actions">
                        <button class="btn btn-sm btn-secondary" onclick="window.nodeManager.viewLogs('${node.id}')">
                            Logs
                        </button>
                        ${node.status === 'running' ? `
                            <button class="btn btn-sm btn-warning" onclick="window.nodeManager.restartNode('${node.id}')">
                                Restart
                            </button>
                            <button class="btn btn-sm btn-danger" onclick="window.nodeManager.stopNode('${node.id}')">
                                Stop
                            </button>
                        ` : node.status === 'stopped' || node.status === 'error' ? `
                            <button class="btn btn-sm btn-success" onclick="window.nodeManager.startExistingNode('${node.id}')">
                                Start
                            </button>
                        ` : ''}
                    </div>
                ` : `
                    <div class="node-actions">
                        <p class="text-muted" style="font-size: 0.85em; margin: 0;">View only - use supervisor for control</p>
                    </div>
                `}
            </div>
        `).join('');
    }

    updateNodeStatus(nodeId, status, nodeInfo) {
        // Update or add to nodes array
        let node = this.nodes.find(n => n.id === nodeId);
        if (node) {
            node.status = status;
        } else if (nodeInfo) {
            // New node we don't know about yet - add it from the WebSocket payload
            this.nodes.push({
                id: nodeInfo.id || nodeId,
                port: nodeInfo.port,
                api_port: nodeInfo.api_port,
                status: status,
                is_primary: nodeInfo.is_primary,
                is_bootstrap: nodeInfo.is_bootstrap,
                error: nodeInfo.error
            });
        } else {
            // Minimal fallback - add with just id and status
            this.nodes.push({ id: nodeId, status: status });
        }

        // Re-render to update cards and action buttons
        this.renderNodeList();
    }

    showStartNodeModal() {
        const modal = document.getElementById('start-node-modal');
        if (modal) {
            modal.style.display = 'flex';
            // Generate suggested node ID
            const nextId = `node${this.nodes.length}`;
            document.getElementById('new-node-id').value = nextId;
        }
    }

    hideStartNodeModal() {
        const modal = document.getElementById('start-node-modal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    async startNewNode() {
        const nodeId = document.getElementById('new-node-id').value.trim();
        const isPrimary = document.getElementById('new-node-primary').checked;
        const isBootstrap = document.getElementById('new-node-bootstrap').checked;
        const bootstrapAddr = document.getElementById('new-node-bootstrap-addr').value.trim();

        if (!nodeId) {
            window.app.showToast('Please enter a node ID', 'error');
            return;
        }

        try {
            await this.api.request('/nodes', {
                method: 'POST',
                body: JSON.stringify({
                    node_id: nodeId,
                    is_primary: isPrimary,
                    is_bootstrap: isBootstrap,
                    bootstrap_addr: bootstrapAddr || undefined
                })
            });

            window.app.showToast(`Node ${nodeId} starting...`, 'success');
            this.hideStartNodeModal();

            // Reload nodes after a short delay
            setTimeout(() => this.loadNodes(), 1000);
        } catch (err) {
            window.app.showToast(`Failed to start node: ${err.message}`, 'error');
        }
    }

    async startExistingNode(nodeId) {
        const node = this.nodes.find(n => n.id === nodeId);
        if (!node) return;

        try {
            await this.api.request('/nodes', {
                method: 'POST',
                body: JSON.stringify({
                    node_id: nodeId,
                    is_primary: node.is_primary,
                    is_bootstrap: node.is_bootstrap
                })
            });

            window.app.showToast(`Node ${nodeId} starting...`, 'success');
            setTimeout(() => this.loadNodes(), 1000);
        } catch (err) {
            window.app.showToast(`Failed to start node: ${err.message}`, 'error');
        }
    }

    async stopNode(nodeId) {
        if (!confirm(`Are you sure you want to stop node ${nodeId}?`)) {
            return;
        }

        try {
            await this.api.request(`/nodes/${nodeId}`, {
                method: 'DELETE'
            });

            window.app.showToast(`Node ${nodeId} stopping...`, 'success');
            setTimeout(() => this.loadNodes(), 1000);
        } catch (err) {
            window.app.showToast(`Failed to stop node: ${err.message}`, 'error');
        }
    }

    async restartNode(nodeId) {
        try {
            await this.api.request(`/nodes/${nodeId}/restart`, {
                method: 'POST'
            });

            window.app.showToast(`Node ${nodeId} restarting...`, 'success');
            setTimeout(() => this.loadNodes(), 2000);
        } catch (err) {
            window.app.showToast(`Failed to restart node: ${err.message}`, 'error');
        }
    }

    async startCluster() {
        const sizeInput = document.getElementById('cluster-size');
        const nodeCount = parseInt(sizeInput.value, 10) || 4;

        if (nodeCount < 1 || nodeCount > 30) {
            window.app.showToast('Cluster size must be between 1 and 30', 'error');
            return;
        }

        const startBtn = document.getElementById('start-cluster-btn');
        if (startBtn) {
            startBtn.disabled = true;
            startBtn.textContent = 'Starting...';
        }

        // Clear current nodes display for fresh start
        this.nodes = [];
        this.renderNodeList();

        try {
            // This returns immediately - nodes start in the background.
            // Progress updates arrive via WebSocket cluster_status and node_status events.
            await this.api.request('/cluster/start', {
                method: 'POST',
                body: JSON.stringify({ node_count: nodeCount })
            });

            window.app.showToast(`Starting cluster with ${nodeCount} nodes...`, 'success');
        } catch (err) {
            window.app.showToast(`Failed to start cluster: ${err.message}`, 'error');
            if (startBtn) {
                startBtn.disabled = false;
                startBtn.textContent = 'Start Cluster';
            }
        }
    }

    async stopAllNodes() {
        if (!confirm('Are you sure you want to stop all nodes?')) {
            return;
        }

        try {
            await this.api.request('/cluster/stop', {
                method: 'POST'
            });

            window.app.showToast('Stopping all nodes...', 'success');
            setTimeout(() => this.loadNodes(), 2000);
        } catch (err) {
            window.app.showToast(`Failed to stop cluster: ${err.message}`, 'error');
        }
    }

    async viewLogs(nodeId) {
        this.selectedNodeId = nodeId;

        const logsCard = document.getElementById('node-logs-card');
        const logsNodeId = document.getElementById('logs-node-id');
        const logsOutput = document.getElementById('node-logs-output');

        if (logsCard && logsNodeId && logsOutput) {
            logsCard.style.display = 'block';
            logsNodeId.textContent = nodeId;
            logsOutput.innerHTML = '<div class="loading">Loading logs...</div>';

            try {
                const response = await this.api.request(`/nodes/${nodeId}/logs`);
                const logs = response.logs || [];
                logsOutput.innerHTML = logs.length > 0
                    ? logs.map(line => `<div class="log-line">${this.escapeHtml(line)}</div>`).join('')
                    : '<div class="text-muted">No logs available</div>';

                // Scroll to bottom
                logsOutput.scrollTop = logsOutput.scrollHeight;
            } catch (err) {
                logsOutput.innerHTML = `<div class="alert alert-warning">Unable to load logs: ${err.message}</div>`;
            }
        }
    }

    appendLog(line) {
        const logsOutput = document.getElementById('node-logs-output');
        if (logsOutput && this.selectedNodeId) {
            const lineDiv = document.createElement('div');
            lineDiv.className = 'log-line';
            lineDiv.textContent = line;
            logsOutput.appendChild(lineDiv);

            // Auto-scroll if near bottom
            const isNearBottom = logsOutput.scrollHeight - logsOutput.scrollTop - logsOutput.clientHeight < 100;
            if (isNearBottom) {
                logsOutput.scrollTop = logsOutput.scrollHeight;
            }
        }
    }

    hideLogs() {
        this.selectedNodeId = null;
        const logsCard = document.getElementById('node-logs-card');
        if (logsCard) {
            logsCard.style.display = 'none';
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    handleWSEvent(event) {
        if (event.type === 'node_status') {
            this.updateNodeStatus(event.node_id, event.status, event.node);
        } else if (event.type === 'node_output' && this.selectedNodeId === event.node_id) {
            this.appendLog(event.line);
        } else if (event.type === 'cluster_status') {
            this.handleClusterStatus(event);
        }
    }
}

// Export for global access
window.nodeManager = null;
