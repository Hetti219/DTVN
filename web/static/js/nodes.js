// Node Lifecycle Management
export class NodeManager {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.nodes = [];
    }

    async render(container) {
        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Node Management</h2>
                <p class="page-description">Start, stop, and monitor validator nodes</p>
            </div>

            <div class="alert alert-info">
                <strong>Note:</strong> Node management will be available in supervisor mode.
                Currently running in validator mode - use CLI to manage nodes.
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Running Nodes</h3>
                    <button class="btn btn-primary" onclick="this.showStartNodeModal()" disabled>
                        + Start Node
                    </button>
                </div>
                <div class="card-body">
                    <div id="node-list">
                        <p class="text-muted text-center">Node management available in supervisor mode</p>
                    </div>
                </div>
            </div>
        `;

        // TODO: Full implementation in Stage 5 (supervisor)
    }

    handleWSEvent(event) {
        if (event.type === 'node_started' || event.type === 'node_stopped') {
            this.loadNodes();
        }
    }

    async loadNodes() {
        // TODO: Implement in Stage 5
        console.log('Loading nodes...');
    }
}
